package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// slowWindow is a WindowCounter that simulates a shared (Redis) round trip with a
// fixed latency. Totals live in atomics and no lock is held across the delay, so
// whatever serialization a test measures comes from the limiter, not the fake.
//
// hook (optional) runs inside the simulated round trip, i.e. while Allow is in its
// shared-counter section, which is what makes "no limiter lock is held here"
// directly observable. ops (optional) records the call sequence per key for
// ordering assertions.
type slowWindow struct {
	delay time.Duration
	err   error
	hook  func(key string)
	rpm   atomic.Int64
	tpm   atomic.Int64

	recordOps bool
	mu        sync.Mutex
	ops       []string
}

func (c *slowWindow) isTPM(key string) bool { return strings.HasPrefix(key, "metapi:tpm:") }

func (c *slowWindow) wait() {
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
}

func (c *slowWindow) log(op string) {
	if !c.recordOps {
		return
	}
	c.mu.Lock()
	c.ops = append(c.ops, op)
	c.mu.Unlock()
}

func (c *slowWindow) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = ctx
	_ = window
	c.wait()
	if c.hook != nil {
		c.hook(key)
	}
	if c.err != nil {
		return 0, c.err
	}
	if c.isTPM(key) {
		n := c.tpm.Add(1)
		c.log("INCR")
		return n, nil
	}
	n := c.rpm.Add(1)
	c.log("INCR")
	return n, nil
}

func (c *slowWindow) Decr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = ctx
	_ = window
	c.wait()
	if c.err != nil {
		return 0, c.err
	}
	if c.isTPM(key) {
		n := c.tpm.Add(-1)
		c.log("DECR")
		return n, nil
	}
	n := c.rpm.Add(-1)
	c.log("DECR")
	return n, nil
}

func (c *slowWindow) IncrBy(ctx context.Context, key string, delta int64, window time.Duration) (int64, error) {
	_ = ctx
	_ = window
	c.wait()
	if c.err != nil {
		return 0, c.err
	}
	if c.isTPM(key) {
		n := c.tpm.Add(delta)
		c.log("INCRBY")
		return n, nil
	}
	n := c.rpm.Add(delta)
	c.log("INCRBY")
	return n, nil
}

func (c *slowWindow) Get(ctx context.Context, key string) (int64, error) {
	_ = ctx
	if c.isTPM(key) {
		return c.tpm.Load(), c.err
	}
	return c.rpm.Load(), c.err
}

// withinTimeout fails the test unless fn returns in time. Used to turn "the
// limiter is blocked" into a test failure instead of a hung suite.
func withinTimeout(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s did not finish within %v — admission is blocked", what, d)
	}
}

// TestKeyAdmission_SharedCounterRunsOutsideShardLock is the direct proof for the
// refactor: from inside a shared-counter round trip (where the old code sat inside
// the global mutex) we run a full Allow for a *different key in the same shard
// stripe* plus a Snapshot for both keys. Any limiter lock held across the Redis
// I/O would deadlock here.
func TestKeyAdmission_SharedCounterRunsOutsideShardLock(t *testing.T) {
	t.Parallel()
	l := NewKeyAdmissionLimiter()
	const keyA = int64(7)
	keyB := keyA + admissionShards // different window, same shard stripe
	limit := int64(1000)

	var hookRan atomic.Int64
	// The hook runs on the goroutine that is inside Allow, which may still be
	// parked there when this test times out, so failures are recorded instead of
	// reported through t from a foreign goroutine.
	var hookErr atomic.Value // string
	fail := func(format string, args ...any) { hookErr.Store(fmt.Sprintf(format, args...)) }
	c := &slowWindow{delay: 5 * time.Millisecond}
	c.hook = func(key string) {
		if key != rpmSharedKey(keyA) {
			return // the nested Allow below shares this counter
		}
		hookRan.Add(1)
		// Nested admission for another key in the SAME shard stripe.
		if d := l.Allow(context.Background(), keyB, &limit, nil, 0); !d.Allowed {
			fail("nested Allow denied: %#v", d)
		}
		// Snapshot takes the shard lock but must never take the per-key shared
		// mutex, so it works for the key whose round trip is in flight too.
		if used, _ := l.Snapshot(keyB); used != 1 {
			fail("nested Snapshot(keyB) = %d, want 1", used)
		}
		if used, _ := l.Snapshot(keyA); used != 0 {
			fail("Snapshot(keyA) during its own round trip = %d, want 0", used)
		}
	}
	l.SetSharedRPMCounter(c)

	var d AdmissionDecision
	withinTimeout(t, 10*time.Second, "Allow with a slow shared counter", func() {
		d = l.Allow(context.Background(), keyA, &limit, nil, 0)
	})
	if hookRan.Load() == 0 {
		t.Fatal("shared counter hook never ran — test did not exercise the shared path")
	}
	if msg, ok := hookErr.Load().(string); ok {
		t.Fatal(msg)
	}
	if !d.Allowed {
		t.Fatalf("outer Allow denied: %#v", d)
	}
}

// TestKeyAdmission_DistinctKeysNotSerializedBySharedCounter quantifies the same
// property: with a 25ms shared round trip, 16 distinct keys must finish well under
// the 400ms a single global lock would need. The bound is half of that serial cost,
// which the parallel version clears by ~6x even on a loaded box (sleeping
// goroutines need no CPU, so the assertion is not scheduler-sensitive).
func TestKeyAdmission_DistinctKeysNotSerializedBySharedCounter(t *testing.T) {
	t.Parallel()
	const keys = 16
	const delay = 25 * time.Millisecond
	l := NewKeyAdmissionLimiter()
	l.SetSharedRPMCounter(&slowWindow{delay: delay})
	limit := int64(1_000_000)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < keys; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if d := l.Allow(context.Background(), int64(i+1), &limit, nil, 0); !d.Allowed {
				t.Errorf("key %d denied: %#v", i+1, d)
			}
		}(i)
	}
	begin := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(begin)
	serial := time.Duration(keys) * delay
	if elapsed >= serial/2 {
		t.Fatalf("%d distinct keys took %v; a serialized limiter needs %v", keys, elapsed, serial)
	}
}

// TestKeyAdmission_DenyCompensationStaysOrderedPerKey pins the ordering guarantee
// that replaces the global lock: for one key the compensating DECR of a denied
// request can never be reordered against another request's INCR, so the shared
// window does not drift and exactly the limit is admitted.
func TestKeyAdmission_DenyCompensationStaysOrderedPerKey(t *testing.T) {
	t.Parallel()
	l := NewKeyAdmissionLimiter()
	c := &slowWindow{delay: time.Millisecond, recordOps: true}
	l.SetSharedRPMCounter(c)
	limit := int64(3)
	const requests = 40

	decisions := make([]AdmissionDecision, requests)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			decisions[i] = l.Allow(context.Background(), 101, &limit, nil, 0)
		}(i)
	}
	close(start)
	wg.Wait()

	allowed := 0
	for _, d := range decisions {
		if d.Allowed {
			allowed++
			continue
		}
		if d.Reason != "over_rpm" || d.RetryAfter != time.Second {
			t.Fatalf("denied decision = %#v, want over_rpm + 1s", d)
		}
	}
	if allowed != int(limit) {
		t.Fatalf("allowed = %d, want exactly %d", allowed, limit)
	}
	if got := c.rpm.Load(); got != limit {
		t.Fatalf("shared counter = %d after %d requests, want %d (compensation drifted)", got, requests, limit)
	}
	// Every DECR must directly follow the INCR it compensates: no other request
	// may interleave between them.
	c.mu.Lock()
	ops := append([]string(nil), c.ops...)
	c.mu.Unlock()
	if len(ops) != requests+(requests-allowed) {
		t.Fatalf("op log length = %d, want %d (%#v)", len(ops), requests+(requests-allowed), ops)
	}
	for i, op := range ops {
		switch op {
		case "INCR":
		case "DECR":
			if i == 0 || ops[i-1] != "INCR" {
				t.Fatalf("DECR at %d is not adjacent to its INCR: %v", i, ops)
			}
		default:
			t.Fatalf("unexpected op %q in log", op)
		}
	}
	// Denied requests must not occupy the process-local window either.
	if used, _ := l.Snapshot(101); used != int64(limit) {
		t.Fatalf("local window = %d, want %d", used, limit)
	}
}

// TestKeyAdmission_SharedFailOpenKeepsLocalLimitExact guards the fail-open path
// under concurrency: when Redis errors, the local window still enforces the limit
// exactly (no overshoot from moving the I/O out of the lock).
func TestKeyAdmission_SharedFailOpenKeepsLocalLimitExact(t *testing.T) {
	t.Parallel()
	l := NewKeyAdmissionLimiter()
	l.SetSharedRPMCounter(&slowWindow{delay: time.Millisecond, err: errors.New("redis down")})
	limit := int64(50)
	const requests = 100

	var allowed atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if d := l.Allow(context.Background(), 202, &limit, nil, 0); d.Allowed {
				allowed.Add(1)
			} else if d.Reason != "over_rpm" {
				t.Errorf("fail-open deny reason = %q, want over_rpm", d.Reason)
			}
		}()
	}
	close(start)
	wg.Wait()
	if allowed.Load() != limit {
		t.Fatalf("fail-open allowed = %d, want exactly %d", allowed.Load(), limit)
	}
	if used, _ := l.Snapshot(202); used != limit {
		t.Fatalf("local window = %d, want %d", used, limit)
	}
}

// TestKeyAdmission_ShardMapping documents the striping: keys one modulus apart
// share a stripe, neighbours do not.
func TestKeyAdmission_ShardMapping(t *testing.T) {
	t.Parallel()
	l := NewKeyAdmissionLimiter()
	if admissionShards < 2 {
		t.Fatalf("admissionShards = %d, want striping", admissionShards)
	}
	if l.shard(1) != l.shard(1+admissionShards) {
		t.Fatal("keys one modulus apart must share a stripe")
	}
	if l.shard(1) == l.shard(2) {
		t.Fatal("neighbouring keys must not share a stripe")
	}
}
