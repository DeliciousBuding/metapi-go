package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Startup-jitter tests.
//
// Observed bug: Registry.StartAll starts all 15 schedulers at once and every
// interval runner fired its immediate first run immediately, so all passes
// hit the DB within a ~2ms window and same-interval passes stayed aligned on
// the same millisecond forever. The fix delays each immediate first run by
// rand(0, interval/10) (capped), de-synchronizing startup and steady state.
// =============================================================================

// TestDefaultStartupJitterWithinBounds verifies the jitter stays in
// [0, min(interval/10, maxStartupJitter)) and varies between draws.
func TestDefaultStartupJitterWithinBounds(t *testing.T) {
	for _, interval := range []time.Duration{5 * time.Second, 60 * time.Second, 30 * time.Minute, 24 * time.Hour} {
		spread := interval / 10
		if spread > maxStartupJitter {
			spread = maxStartupJitter
		}
		seen := map[time.Duration]struct{}{}
		for i := 0; i < 100; i++ {
			d := defaultStartupJitter(interval)
			if d < 0 || d >= spread {
				t.Fatalf("interval %v: jitter %v outside [0, %v)", interval, d, spread)
			}
			seen[d] = struct{}{}
		}
		if len(seen) < 2 {
			t.Fatalf("interval %v: 100 jitter draws produced no variation", interval)
		}
	}
}

// TestDefaultStartupJitterDegenerate verifies tiny/zero intervals yield a
// zero delay without panicking.
func TestDefaultStartupJitterDegenerate(t *testing.T) {
	for _, interval := range []time.Duration{0, time.Nanosecond, 9 * time.Nanosecond} {
		if d := defaultStartupJitter(interval); d != 0 {
			t.Fatalf("interval %v: expected zero jitter, got %v", interval, d)
		}
	}
}

// TestIntervalRunnerImmediateRunHonorsInjectedJitter pins the jitter via the
// injectable hook (tests never depend on the random draw) and asserts the
// immediate run only starts once the delay has elapsed.
func TestIntervalRunnerImmediateRunHonorsInjectedJitter(t *testing.T) {
	const delay = 300 * time.Millisecond
	r := &intervalRunner{jitter: func(time.Duration) time.Duration { return delay }}

	ranCh := make(chan time.Time, 1)
	t0 := time.Now()
	if err := r.start(context.Background(), time.Hour, true, func() {
		ranCh <- time.Now()
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	t.Cleanup(func() { _ = r.stop() })

	select {
	case at := <-ranCh:
		if offset := at.Sub(t0); offset < delay {
			t.Fatalf("immediate run started %v after start; jitter delay %v not applied", offset, delay)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("immediate run never started")
	}
}

// TestIntervalRunnerStopDuringJitterDropsRun verifies a stop that arrives
// while the immediate run is still waiting out its jitter delay drops the
// run instead of starting work during shutdown. stop also must not hang on
// the never-started run.
func TestIntervalRunnerStopDuringJitterDropsRun(t *testing.T) {
	var ran atomic.Bool
	r := &intervalRunner{jitter: func(time.Duration) time.Duration { return time.Hour }}
	if err := r.start(context.Background(), 2*time.Hour, true, func() { ran.Store(true) }); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if err := r.stop(); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if ran.Load() {
		t.Fatal("immediate run started even though stop arrived during the jitter delay")
	}
}

// TestIntervalRunnerImmediateRunsSpreadOut starts several runners with the
// real (random) jitter and asserts their first-run times are spread out
// instead of firing in the same instant.
func TestIntervalRunnerImmediateRunsSpreadOut(t *testing.T) {
	const n = 6
	// interval 2s => default jitter spread [0, 200ms). The ticker interval is
	// irrelevant to the test: the CAS guard records only the first run.
	const interval = 2 * time.Second

	var mu sync.Mutex
	offsets := make([]time.Duration, 0, n)
	var wg sync.WaitGroup
	t0 := time.Now()

	runners := make([]*intervalRunner, n)
	for i := 0; i < n; i++ {
		r := &intervalRunner{}
		runners[i] = r
		var first atomic.Bool
		wg.Add(1)
		if err := r.start(context.Background(), interval, true, func() {
			if !first.CompareAndSwap(false, true) {
				return
			}
			off := time.Since(t0)
			mu.Lock()
			offsets = append(offsets, off)
			mu.Unlock()
			wg.Done()
		}); err != nil {
			t.Fatalf("start runner %d failed: %v", i, err)
		}
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(15 * time.Second):
		t.Fatal("not all immediate runs started within 15s")
	}
	for _, r := range runners {
		_ = r.stop()
	}

	minOff, maxOff := offsets[0], offsets[0]
	for _, o := range offsets[1:] {
		if o < minOff {
			minOff = o
		}
		if o > maxOff {
			maxOff = o
		}
	}
	if spread := maxOff - minOff; spread < 30*time.Millisecond {
		t.Fatalf("immediate first runs were not spread out: offsets %v (spread %v)", offsets, spread)
	}
}
