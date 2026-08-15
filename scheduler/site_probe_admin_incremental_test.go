package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// timedProbe is a ChannelHealthProbe that sleeps a per-channel duration before
// returning. It is used to prove probeSiteTargets streams results in
// completion order rather than buffering everything until the end.
type timedProbe struct {
	mu     sync.Mutex
	calls  []int64
	delays map[int64]time.Duration
}

func (p *timedProbe) ProbeChannel(_ context.Context, target ProbeTarget) (ProbeOutcome, error) {
	p.mu.Lock()
	p.calls = append(p.calls, target.ChannelID)
	delay := time.Duration(0)
	if d, ok := p.delays[target.ChannelID]; ok {
		delay = d
	}
	p.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return ProbeOutcome{Status: "success", LatencyMs: float64(delay.Milliseconds())}, nil
}

// TestProbeSiteTargets_StreamsResultsIncrementally verifies that results are
// delivered to onResult as each probe completes, not buffered and replayed
// after every probe finishes. Three targets with staggered delays (10ms /
// 80ms / 150ms) are probed concurrently; the first result must arrive before
// the slowest probe's delay elapses.
func TestProbeSiteTargets_StreamsResultsIncrementally(t *testing.T) {
	s := NewModelProbeScheduler(testConfig())
	probe := &timedProbe{delays: map[int64]time.Duration{
		101: 10 * time.Millisecond,
		102: 80 * time.Millisecond,
		103: 150 * time.Millisecond,
	}}
	s.SetProbeExecutor(probe)
	s.SetHealthRecorder(&fakeRecorder{})

	// Drive the bounded-concurrency runner directly with synthetic targets so
	// this test does not need a seeded store.GetDB().
	targets := []ProbeTarget{
		{ChannelID: 101, AccountID: 1, ModelName: "fast"},
		{ChannelID: 102, AccountID: 1, ModelName: "mid"},
		{ChannelID: 103, AccountID: 1, ModelName: "slow"},
	}

	type timedResult struct {
		ChannelID int64
		At        time.Duration
	}
	var mu sync.Mutex
	var got []timedResult
	start := time.Now()
	s.probeSiteTargets(context.Background(), targets, 5000, func(r ProbeSiteResult) {
		mu.Lock()
		got = append(got, timedResult{ChannelID: r.ChannelID, At: time.Since(start)})
		mu.Unlock()
	})

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("expected 3 streamed results, got %d (%+v)", len(got), got)
	}
	// Results must arrive in completion order: fast(10ms) → mid(80ms) → slow(150ms).
	if got[0].ChannelID != 101 {
		t.Fatalf("first result should be the fast channel 101, got %d (order=%+v)", got[0].ChannelID, got)
	}
	if got[1].ChannelID != 102 {
		t.Fatalf("second result should be the mid channel 102, got %d (order=%+v)", got[1].ChannelID, got)
	}
	if got[2].ChannelID != 103 {
		t.Fatalf("third result should be the slow channel 103, got %d (order=%+v)", got[2].ChannelID, got)
	}
	// The critical incremental assertion: the first result arrived BEFORE the
	// slowest probe could have finished. The old sequential/replay behavior
	// would not write any result until ~150ms (all done). Allow generous
	// slack to avoid CI flake.
	if got[0].At >= 100*time.Millisecond {
		t.Fatalf("first result arrived at %v — looks buffered until probes finished (want < 100ms)", got[0].At)
	}
	if got[2].At < 100*time.Millisecond {
		t.Fatalf("last result arrived at %v — too fast, slowest probe is 150ms", got[2].At)
	}
}

// releaseProbe blocks every ProbeChannel call on a shared release channel so
// tests can control exactly when in-flight probes finish. It records how many
// calls ever started.
type releaseProbe struct {
	mu      sync.Mutex
	calls   []int64
	release chan struct{}
}

func (p *releaseProbe) ProbeChannel(_ context.Context, target ProbeTarget) (ProbeOutcome, error) {
	p.mu.Lock()
	p.calls = append(p.calls, target.ChannelID)
	p.mu.Unlock()
	<-p.release
	return ProbeOutcome{Status: "success"}, nil
}

func (p *releaseProbe) startedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// TestProbeSiteTargets_CancelStopsScheduling verifies that cancelling the
// context stops launching new probes: with 12 targets and concurrency 8, the
// first 8 start (filling every slot), then a cancel must prevent targets 9-12
// from ever starting. In-flight probes finish after the test releases them,
// but their results are NOT streamed to the cancelled caller.
func TestProbeSiteTargets_CancelStopsScheduling(t *testing.T) {
	s := NewModelProbeScheduler(testConfig())
	probe := &releaseProbe{release: make(chan struct{})}
	s.SetProbeExecutor(probe)
	s.SetHealthRecorder(&fakeRecorder{})

	// 12 targets > adminProbeConcurrency (8) so the 9th can never acquire a
	// slot until an in-flight probe finishes.
	targets := make([]ProbeTarget, 12)
	for i := range targets {
		targets[i] = ProbeTarget{ChannelID: int64(200 + i), AccountID: 1, ModelName: "m"}
	}

	ctx, cancel := context.WithCancel(context.Background())
	var streamed atomic.Int32
	done := make(chan struct{})
	go func() {
		s.probeSiteTargets(ctx, targets, 5000, func(ProbeSiteResult) {
			streamed.Add(1)
		})
		close(done)
	}()

	// Wait until the 8-slot semaphore is saturated (8 probes started).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if probe.startedCount() == adminProbeConcurrency {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := probe.startedCount(); got != adminProbeConcurrency {
		t.Fatalf("expected %d probes started before cancel, got %d", adminProbeConcurrency, got)
	}

	// Cancel: targets 9-12 must never start.
	cancel()

	// Give the scheduler a moment to observe cancellation and exit the
	// scheduling loop. No new ProbeChannel calls should appear.
	time.Sleep(40 * time.Millisecond)
	if got := probe.startedCount(); got != adminProbeConcurrency {
		t.Fatalf("after cancel, %d probes started (want %d — no new probes should start)", got, adminProbeConcurrency)
	}

	// Release the 8 in-flight probes so probeSiteTargets' wg.Wait() returns.
	close(probe.release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probeSiteTargets did not return within 2s after in-flight probes were released")
	}

	// No results should have been streamed: ctx was cancelled before any
	// in-flight probe finished, so onResult is skipped for all of them.
	if n := streamed.Load(); n != 0 {
		t.Fatalf("expected 0 streamed results after cancel, got %d", n)
	}
	if got := probe.startedCount(); got != adminProbeConcurrency {
		t.Fatalf("final started count = %d, want %d (cancellation must not launch the remaining targets)", got, adminProbeConcurrency)
	}
}
