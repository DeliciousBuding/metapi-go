package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// Graceful-stop drain tests.
//
// Observed bug: at shutdown the DB was closed ~5.8s before in-flight
// scheduled tasks finished. cronRunner.stop() returns robfig's drain context
// (done when all running jobs have completed) but every caller discarded it,
// and intervalRunner fired each tick fire-and-forget with no WaitGroup, so
// Stop returned while work was still running.
// =============================================================================

// TestCronRunnerStopDrainsInFlightJob triggers a cron job that sleeps 2s and
// asserts the job has completed by the time stop() returns. Fails before the
// fix because the robfig drain context is discarded.
func TestCronRunnerStopDrainsInFlightJob(t *testing.T) {
	cr := newCronRunner()
	var started, done atomic.Bool
	// Fires every second so the test does not wait for a wall-clock minute.
	_, err := cr.addJob("* * * * * *", func() {
		// Only the first trigger runs; later ticks return immediately so the
		// drain window is deterministic.
		if started.Swap(true) {
			return
		}
		time.Sleep(2 * time.Second)
		done.Store(true)
	})
	if err != nil {
		t.Fatalf("addJob failed: %v", err)
	}
	cr.start()
	t.Cleanup(func() { cr.stop() })

	deadline := time.Now().Add(10 * time.Second)
	for !started.Load() {
		if time.Now().After(deadline) {
			t.Fatal("cron job did not start within 10s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cr.stop()

	if !done.Load() {
		t.Fatal("stop() returned while the 2s job was still in flight: the robfig drain context is being discarded")
	}
}

// TestIntervalRunnerStopDrainsInFlightRun starts an interval runner whose
// immediate first run sleeps 2s and asserts the run has completed by the
// time stop() returns. Fails before the fix because ticks are
// fire-and-forget with no WaitGroup.
func TestIntervalRunnerStopDrainsInFlightRun(t *testing.T) {
	r := &intervalRunner{}
	var started, done atomic.Bool
	// Interval far longer than the test window: only the immediate run matters.
	if err := r.start(context.Background(), time.Hour, true, func() {
		if started.Swap(true) {
			return
		}
		time.Sleep(2 * time.Second)
		done.Store(true)
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for !started.Load() {
		if time.Now().After(deadline) {
			t.Fatal("immediate run did not start within 10s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := r.stop(); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	if !done.Load() {
		t.Fatal("stop() returned while the 2s run was still in flight: interval runs are fire-and-forget")
	}
}

// blockingStopScheduler blocks inside Stop for a fixed duration.
type blockingStopScheduler struct {
	name  string
	block time.Duration
}

func (b *blockingStopScheduler) Name() string                    { return b.name }
func (b *blockingStopScheduler) Start(ctx context.Context) error { return nil }
func (b *blockingStopScheduler) Stop() error {
	time.Sleep(b.block)
	return nil
}

// TestRegistryStopAllBoundedByBudget verifies StopAll cannot be held hostage
// by a stuck scheduler Stop: it must return once the budget expires.
func TestRegistryStopAllBoundedByBudget(t *testing.T) {
	r := NewRegistry()
	r.Register(&blockingStopScheduler{name: "stuck", block: 30 * time.Second})

	start := time.Now()
	r.StopAllWithTimeout(500 * time.Millisecond)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("StopAllWithTimeout(500ms) took %v; stop budget not enforced", elapsed)
	}
}

// TestRegistryStopAllStopsConcurrently verifies stops run in parallel: two
// schedulers that each block 2s must finish in roughly 2s, not 4s.
func TestRegistryStopAllStopsConcurrently(t *testing.T) {
	r := NewRegistry()
	r.Register(&blockingStopScheduler{name: "slow-1", block: 2 * time.Second})
	r.Register(&blockingStopScheduler{name: "slow-2", block: 2 * time.Second})

	start := time.Now()
	r.StopAllWithTimeout(10 * time.Second)
	elapsed := time.Since(start)

	if elapsed > 3500*time.Millisecond {
		t.Fatalf("two 2s stops took %v; stops appear to run sequentially", elapsed)
	}
}
