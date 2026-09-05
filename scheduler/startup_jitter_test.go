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

// TestIntervalRunnerStartupJitterFallsBackToDefault owns the production
// fallback: a runner with no injected jitter must reach defaultStartupJitter.
// Every other test in this file injects `jitter`, so without this one the
// `r.jitter == nil` branch -- the branch every real scheduler takes -- would
// have no owner, and a regression to `return 0` there would leave the whole
// suite green while every immediate first run fired in the same instant
// (the exact bug this feature was written for).
func TestIntervalRunnerStartupJitterFallsBackToDefault(t *testing.T) {
	const interval = 2 * time.Second // spread [0, 200ms)
	r := &intervalRunner{}           // no injected jitter: the production shape
	if r.jitter != nil {
		t.Fatal("precondition: this runner must have no injected jitter")
	}
	seen := map[time.Duration]struct{}{}
	for i := 0; i < 200; i++ {
		d := r.startupJitter(interval)
		if d < 0 || d >= interval/10 {
			t.Fatalf("fallback jitter %v outside [0, %v)", d, interval/10)
		}
		seen[d] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("200 fallback draws produced no variation: %v", seen)
	}
}

// TestIntervalRunnerImmediateRunsSpreadOut starts several runners together and
// asserts each one drew its OWN startup jitter, and that those draws are spread
// across the window rather than colliding.
//
// What it measures, and why it changed (this gate reddened master once):
// the previous version recorded time.Since(t0) inside each immediate run and
// required max-min >= 30ms. That observes assigned delay + goroutine scheduling
// latency, and only the delay is ours. On a loaded runner the six callbacks
// bunched into a 27.8ms band and the required check test-pg went red on a
// commit whose own PR run had been green (run 33987131273, 2026-09-05). It was
// a dice roll on its own terms as well: six uniform draws in [0, 200ms) land
// within 30ms of each other about once in 2500 runs even with fair scheduling.
// Both halves are fixed here -- the assertion reads the delay the runner was
// actually given (through the same injectable seam its siblings use, wrapping
// the REAL defaultStartupJitter), and 16 draws make the 30ms threshold a ~7e-12
// event instead of a ~4e-4 one. The behavioural half (an immediate run really
// happens) is still asserted via the WaitGroup, but carries no timing claim;
// "the delay is honored" is owned deterministically by
// TestIntervalRunnerImmediateRunHonorsInjectedJitter.
func TestIntervalRunnerImmediateRunsSpreadOut(t *testing.T) {
	const n = 16
	// interval 2s => default jitter spread [0, 200ms). The ticker interval is
	// irrelevant to the test: the CAS guard admits only the first run.
	const interval = 2 * time.Second

	var mu sync.Mutex
	delays := make([]time.Duration, 0, n)
	var wg sync.WaitGroup

	runners := make([]*intervalRunner, n)
	for i := 0; i < n; i++ {
		// Wrap the real jitter so the spread claim reads the assigned delay
		// instead of the instant the OS happened to schedule the callback.
		r := &intervalRunner{jitter: func(iv time.Duration) time.Duration {
			d := defaultStartupJitter(iv)
			mu.Lock()
			delays = append(delays, d)
			mu.Unlock()
			return d
		}}
		runners[i] = r
		var first atomic.Bool
		wg.Add(1)
		if err := r.start(context.Background(), interval, true, func() {
			if !first.CompareAndSwap(false, true) {
				return
			}
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

	mu.Lock()
	got := append([]time.Duration(nil), delays...)
	mu.Unlock()

	// One draw per runner: a single shared draw is exactly the regression this
	// gate exists for, and it is invisible to both sibling tests (the jitter
	// function would still vary, and each runner would still honor "a" delay).
	if len(got) != n {
		t.Fatalf("startup jitter was drawn %d times for %d runners: %v", len(got), n, got)
	}
	minD, maxD := got[0], got[0]
	for _, d := range got {
		if d < 0 || d >= interval/10 {
			t.Fatalf("assigned jitter %v outside [0, %v): %v", d, interval/10, got)
		}
		if d < minD {
			minD = d
		}
		if d > maxD {
			maxD = d
		}
	}
	if spread := maxD - minD; spread < 30*time.Millisecond {
		t.Fatalf("assigned startup jitter was not spread out: %v (spread %v)", got, spread)
	}
}
