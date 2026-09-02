package sharedcount

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// countDial wraps a dialer and counts how many connections were opened, which is
// how these tests observe pooling (one dial for many commands) and reconnects
// (exactly one extra dial after the server drops a connection).
type countDial struct {
	calls atomic.Int64
	next  func(network, address string, timeout time.Duration) (net.Conn, error)
}

func (d *countDial) dial(network, address string, timeout time.Duration) (net.Conn, error) {
	d.calls.Add(1)
	return d.next(network, address, timeout)
}

func (d *countDial) install(c *RedisCounter) {
	d.next = net.DialTimeout
	c.dial = d.dial
}

// silentServer accepts connections and never replies, so only ctx (or the
// connection deadline) can end an operation.
type silentServer struct {
	ln    net.Listener
	mu    sync.Mutex
	conns []net.Conn
	done  chan struct{}
}

func newSilentServer(t *testing.T) *silentServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("silent server listen: %v", err)
	}
	s := &silentServer{ln: ln, done: make(chan struct{})}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, c)
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *silentServer) addr() string { return s.ln.Addr().String() }

func (s *silentServer) close() {
	_ = s.ln.Close()
	s.mu.Lock()
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// TestRedisCounter_PoolsConnectionAndHandshake is the core of the pooling change:
// twenty commands must cost one connection, one AUTH and one SELECT — not twenty.
func TestRedisCounter_PoolsConnectionAndHandshake(t *testing.T) {
	t.Parallel()
	f := newFakeRedisWithAuth(t, "bench-pw")
	defer f.close()
	c, err := NewRedisCounter("redis://:bench-pw@" + f.addr() + "/3")
	if err != nil {
		t.Fatalf("NewRedisCounter: %v", err)
	}
	c.timeout = 5 * time.Second
	var d countDial
	d.install(c)

	ctx := context.Background()
	for i := int64(1); i <= 20; i++ {
		n, err := c.Incr(ctx, "pooled", time.Minute)
		if err != nil {
			t.Fatalf("Incr %d: %v", i, err)
		}
		if n != i {
			t.Fatalf("Incr %d = %d, want %d", i, n, i)
		}
	}
	if got := d.calls.Load(); got != 1 {
		t.Fatalf("dials = %d for 20 commands, want 1 (connection must be reused)", got)
	}
	if got := f.auths.Load(); got != 1 {
		t.Fatalf("AUTH sent %d times, want once per connection", got)
	}
	if got := f.selects.Load(); got != 1 {
		t.Fatalf("SELECT sent %d times, want once per connection", got)
	}
}

// TestRedisCounter_RedialsAfterServerDropsConnection covers automatic reconnect:
// a connection the server closed while it sat idle must be replaced transparently,
// repeatedly, without surfacing an error to the caller.
func TestRedisCounter_RedialsAfterServerDropsConnection(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	var d countDial
	d.install(c)

	ctx := context.Background()
	want := int64(0)
	for round := 1; round <= 3; round++ {
		want++
		n, err := c.Incr(ctx, "reconnect", time.Minute)
		if err != nil || n != want {
			t.Fatalf("round %d before drop: Incr = %d, %v, want %d", round, n, err, want)
		}
		// Server drops the idle pooled connection.
		f.closeConns()
		want++
		n, err = c.Incr(ctx, "reconnect", time.Minute)
		if err != nil {
			t.Fatalf("round %d after drop: Incr error = %v (reconnect failed)", round, err)
		}
		if n != want {
			t.Fatalf("round %d after drop: Incr = %d, want %d", round, n, want)
		}
	}
	if got, wantDials := d.calls.Load(), int64(4); got != wantDials {
		t.Fatalf("dials = %d, want %d (1 initial + 3 reconnects)", got, wantDials)
	}
}

// TestRedisCounter_RecoversFromWholePoolDrop covers a server restart or failover:
// every parked connection dies at once, and the very next call must still succeed
// because the retry drains the dead free list instead of surfacing a fail-open
// error to the admission path.
func TestRedisCounter_RecoversFromWholePoolDrop(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	f.setDelay(5 * time.Millisecond)
	defer f.close()
	c := newCounterAt(t, f.addr())

	// Fill the pool: concurrent commands force several connections to be dialed
	// and then parked idle.
	var wg sync.WaitGroup
	ctx := context.Background()
	for i := 0; i < maxRedisConns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Incr(ctx, "before-restart", time.Minute); err != nil {
				t.Errorf("Incr before drop: %v", err)
			}
		}()
	}
	wg.Wait()
	parked := f.peakConns()
	if parked < 2 {
		t.Fatalf("pool only reached %d connections, test cannot exercise a mass drop", parked)
	}

	// Every idle connection dies at once.
	f.closeConns()
	if _, err := c.Incr(ctx, "after-restart", time.Minute); err != nil {
		t.Fatalf("first call after the whole pool was dropped: %v", err)
	}
	// And the counter keeps working normally afterwards.
	for i := 0; i < 5; i++ {
		if _, err := c.Incr(ctx, "after-restart", time.Minute); err != nil {
			t.Fatalf("Incr %d after recovery: %v", i, err)
		}
	}
	if n, err := c.Get(ctx, "after-restart"); err != nil || n != 6 {
		t.Fatalf("count after recovery = %d, %v, want 6", n, err)
	}
}

// TestRedisCounter_PoolBoundsAndUsesSeveralConnections pins both halves of the
// pool contract: concurrency is bounded by maxRedisConns (no FD blow-up) and the
// pool really is parallel (not one mutex-serialized connection).
func TestRedisCounter_PoolBoundsAndUsesSeveralConnections(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	f.setDelay(2 * time.Millisecond)
	defer f.close()
	c := newCounterAt(t, f.addr())

	const workers = 32
	const perWorker = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	var failures atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ctx := context.Background()
			for j := 0; j < perWorker; j++ {
				if _, err := c.Incr(ctx, "bounded", time.Minute); err != nil {
					failures.Add(1)
				}
			}
		}(i)
	}
	begin := time.Now()
	close(start)
	wg.Wait()
	elapsed := time.Since(begin)

	if n := failures.Load(); n != 0 {
		t.Fatalf("%d/%d pooled operations failed", n, workers*perWorker)
	}
	peak := f.peakConns()
	if peak > maxRedisConns {
		t.Fatalf("peak connections = %d, want <= maxRedisConns (%d)", peak, maxRedisConns)
	}
	if peak < 2 {
		t.Fatalf("peak connections = %d, want the pool to run in parallel", peak)
	}
	// Wall-clock throughput is deliberately not asserted here: on a shared 2-core
	// box the effective parallelism is whatever the scheduler can run, so the
	// number measures CPU availability rather than the pool. The structural
	// assertions above (bounded peak, no failures, exact total) are the contract;
	// throughput is measured in BenchmarkRedisCounterIncr.
	t.Logf("%d commands over %d peak connections in %v", workers*perWorker, peak, elapsed)
	total, err := c.Get(context.Background(), "bounded")
	if err != nil || total != workers*perWorker {
		t.Fatalf("total = %d, %v, want %d", total, err, workers*perWorker)
	}
}

// TestRedisCounter_ReplyErrorIsNotRetried keeps reconnect-retry from turning a
// rejected AUTH (or any Redis error reply) into repeated connection storms.
func TestRedisCounter_ReplyErrorIsNotRetried(t *testing.T) {
	t.Parallel()
	f := newFakeRedisWithAuth(t, "right-secret")
	defer f.close()
	c := newCounterWithAuth(t, f.addr(), "wrong-secret")
	var d countDial
	d.install(c)

	if _, err := c.Incr(context.Background(), "k", time.Minute); err == nil {
		t.Fatal("expected AUTH error, got nil")
	} else if !isReplyError(err) {
		t.Fatalf("error = %v (%T), want a redis reply error", err, err)
	}
	if got := d.calls.Load(); got != 1 {
		t.Fatalf("dials = %d, want 1 (an error reply must not trigger a reconnect retry)", got)
	}
}

// TestRedisCounter_SlotIsReleasedAfterDialFailure proves the pool cannot leak
// slots: after repeated dial failures every slot is still available, otherwise
// callers would block forever once maxRedisConns failures had piled up.
func TestRedisCounter_SlotIsReleasedAfterDialFailure(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	var attempts atomic.Int64
	c.dial = func(network, address string, timeout time.Duration) (net.Conn, error) {
		if attempts.Add(1) <= 3 {
			return nil, errors.New("dial boom")
		}
		return net.DialTimeout(network, address, timeout)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := c.Incr(ctx, "leaky", time.Minute); err == nil {
			t.Fatalf("Incr %d: expected dial error, got nil", i)
		}
	}
	// Now that dialing works, a full pool's worth of concurrent callers must all
	// get through: a leaked slot would deadlock here.
	const workers = maxRedisConns * 4
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Incr(ctx, "leaky", time.Minute)
			if err != nil {
				errs <- err
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("pool deadlocked after dial failures — slots were not released")
	}
	close(errs)
	for err := range errs {
		t.Fatalf("Incr after recovery: %v", err)
	}
	if n, err := c.Get(ctx, "leaky"); err != nil || n != workers {
		t.Fatalf("count after recovery = %d, %v, want %d", n, err, workers)
	}
}

// TestRedisCounter_ContextCancelInterruptsBlockingIO is the ctx half of the
// contract: a cancelled context must end an in-flight command against a server
// that never replies, and must report context.Canceled.
func TestRedisCounter_ContextCancelInterruptsBlockingIO(t *testing.T) {
	t.Parallel()
	s := newSilentServer(t)
	defer s.close()
	c := newCounterAt(t, s.addr())
	c.timeout = 30 * time.Second // long on purpose: only ctx may end this

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := c.Incr(ctx, "cancel", time.Minute)
		errc <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Incr did not return after the context was cancelled")
	}
	// The aborted connection must have been dropped, not lost: the pool still
	// hands out slots, so a bounded call returns instead of blocking forever.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	begin := time.Now()
	// The silent server never replies, so the ctx-derived deadline ends the call;
	// what matters here is that it ends at all (no slot was lost) and stays bounded.
	if _, err := c.Get(ctx2, "cancel"); err == nil {
		t.Fatal("post-cancel Get against a silent server should fail")
	}
	if elapsed := time.Since(begin); elapsed > 5*time.Second {
		t.Fatalf("post-cancel Get took %v — a pool slot was lost", elapsed)
	}
}

// TestRedisCounter_ContextDeadlineIsHonored checks deadline propagation: the
// operation must end at the ctx deadline, well before the counter's own timeout.
func TestRedisCounter_ContextDeadlineIsHonored(t *testing.T) {
	t.Parallel()
	s := newSilentServer(t)
	defer s.close()
	c := newCounterAt(t, s.addr())
	c.timeout = 30 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	begin := time.Now()
	_, err := c.Incr(ctx, "deadline", time.Minute)
	elapsed := time.Since(begin)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("operation took %v, want it bounded by the 100ms ctx deadline", elapsed)
	}
}

// TestRedisCounter_CancelledContextShortCircuits: an already-cancelled context
// must not even open a connection.
func TestRedisCounter_CancelledContextShortCircuits(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	var d countDial
	d.install(c)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Incr(ctx, "short", time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("Incr with cancelled ctx = %v, want context.Canceled", err)
	}
	if _, err := c.Decr(ctx, "short", time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("Decr with cancelled ctx = %v, want context.Canceled", err)
	}
	if _, err := c.IncrBy(ctx, "short", 5, time.Minute); !errors.Is(err, context.Canceled) {
		t.Fatalf("IncrBy with cancelled ctx = %v, want context.Canceled", err)
	}
	if _, err := c.Get(ctx, "short"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get with cancelled ctx = %v, want context.Canceled", err)
	}
	if got := d.calls.Load(); got != 0 {
		t.Fatalf("dials = %d with a cancelled context, want 0", got)
	}
}

// TestRedisCounter_PoolIsConcurrencySafe hammers one counter from many goroutines
// with a mix of operations; run under -race this is the pool's safety net.
func TestRedisCounter_PoolIsConcurrencySafe(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	ctx := context.Background()
	const workers = 24
	const perWorker = 25
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for j := 0; j < perWorker; j++ {
				if _, err := c.Incr(ctx, "mixed", time.Minute); err != nil {
					t.Errorf("Incr: %v", err)
					return
				}
				if j%5 == 0 {
					if _, err := c.IncrBy(ctx, "mixed-tpm", 10, time.Minute); err != nil {
						t.Errorf("IncrBy: %v", err)
						return
					}
					if _, err := c.Decr(ctx, "mixed", time.Minute); err != nil {
						t.Errorf("Decr: %v", err)
						return
					}
				}
				if _, err := c.Get(ctx, "mixed"); err != nil {
					t.Errorf("Get: %v", err)
					return
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	// Every Incr is paired with a Decr on every 5th iteration.
	decrs := int64(workers * perWorker / 5)
	incs := int64(workers * perWorker)
	if n, err := c.Get(ctx, "mixed"); err != nil || n != incs-decrs {
		t.Fatalf("mixed total = %d, %v, want %d", n, err, incs-decrs)
	}
}
