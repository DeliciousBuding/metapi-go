package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// Panic-recovery boundary tests.
//
// Observed defect: cronRunner.addJob wraps every cron-triggered run in a
// recover, but intervalRunner (9 schedulers) launched each run in a bare
// goroutine with no recover, and several schedulers spawn additional bare
// goroutines (checkin interval loop + catch-ups, model-probe TriggerNow,
// per-target probe workers, admin-snapshot warm workers, sub2api-refresh
// per-account workers). An unrecovered panic in any of them crashes the
// entire server process, not just the job. usage_aggregation carries its own
// in-pass recover with a comment acknowledging exactly this hazard
// ("Re-panicking would crash the entire server") — proof the boundary gap
// was known but only patched for one job.

// TestIntervalRunnerPanickingJobSurvivesAndKeepsTicking starts an interval
// runner whose immediate first run panics and asserts the runner survives
// and keeps producing runs. Before the fix the panic is unrecovered in the
// run goroutine, so the entire test binary (like the server process in
// production) crashes — that crash is the failing evidence.
func TestIntervalRunnerPanickingJobSurvivesAndKeepsTicking(t *testing.T) {
	r := &intervalRunner{jitter: func(time.Duration) time.Duration { return 0 }}
	var runs atomic.Int64
	if err := r.start(context.Background(), 10*time.Millisecond, true, func() {
		if runs.Add(1) == 1 {
			panic("boom: first run panics")
		}
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	t.Cleanup(func() { _ = r.stop() })

	deadline := time.Now().Add(10 * time.Second)
	for runs.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("runner stopped ticking after a panicking run: only %d runs observed", runs.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestIntervalRunnerTickRunPanickingJobSurvives covers the tick-fired path
// (not just the immediate run): a panicking tick run must not kill the
// runner or stop future ticks.
func TestIntervalRunnerTickRunPanickingJobSurvives(t *testing.T) {
	r := &intervalRunner{jitter: func(time.Duration) time.Duration { return 0 }}
	var runs atomic.Int64
	// No immediate run: the first execution is tick-fired after ~15ms.
	if err := r.start(context.Background(), 15*time.Millisecond, false, func() {
		if runs.Add(1) == 1 {
			panic("boom: tick run panics")
		}
	}); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	t.Cleanup(func() { _ = r.stop() })

	deadline := time.Now().Add(10 * time.Second)
	for runs.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("runner stopped ticking after a panicking tick run: only %d runs observed", runs.Load())
		}
		time.Sleep(5 * time.Millisecond)
	}
}
