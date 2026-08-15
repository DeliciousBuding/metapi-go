package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestRetentionScheduler_StopCancelsInFlightJobContext verifies the core of
// the shutdown-cancel fix: a per-job timeout derived from the scheduler's
// lifecycle ctx (exactly what runCleanup does) is cancelled when Stop is
// called. Before the fix, jobs derived from context.Background() which is
// never cancelled, so in-flight cleanups survived shutdown.
func TestRetentionScheduler_StopCancelsInFlightJobContext(t *testing.T) {
	cfg := testConfig()
	s := NewRetentionScheduler(cfg, RetentionSchedulerOptions{
		Name:               "test-retention-cancel",
		Table:              "proxy_logs",
		DefaultIntervalMin: 30,
		RetentionDaysFn:    func(*config.Config) int { return 30 },
		IntervalMinFn:      func(*config.Config) int { return 30 },
		// Disabled so Start arms the lifecycle ctx but does not start the
		// tick runner — keeps the test free of DB/lease machinery.
		DisabledFn: func(*config.Config) (bool, string) { return true, "test" },
	})

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	if err := s.Start(rootCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.ctx == nil {
		t.Fatal("lifecycle ctx not captured after Start")
	}

	// Mirror runCleanup's derivation: context.WithTimeout(s.ctx, jobTimeout).
	jobCtx, jobCancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer jobCancel()
	if jobCtx.Err() != nil {
		t.Fatal("job ctx should be alive before Stop")
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if s.ctx.Err() == nil {
		t.Fatal("lifecycle ctx must be cancelled by Stop")
	}
	if jobCtx.Err() == nil {
		t.Fatal("in-flight job ctx (derived from lifecycle ctx) must be cancelled by Stop")
	}
}

// TestOAuthRefreshScheduler_StopCancelsInFlightJobContext mirrors the
// retention assertion for the oauth-refresh scheduler. runPass derives its
// job timeout from s.ctx; Stop must cancel that chain.
func TestOAuthRefreshScheduler_StopCancelsInFlightJobContext(t *testing.T) {
	cfg := testConfig()
	s := NewOAuthRefreshScheduler(cfg)

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	if err := s.Start(rootCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.ctx == nil {
		t.Fatal("lifecycle ctx not captured after Start")
	}
	// Give the immediate runPass goroutine (no-ops on nil DB) a moment to
	// settle so it does not race the Stop.
	time.Sleep(50 * time.Millisecond)

	jobCtx, jobCancel := context.WithTimeout(s.ctx, oauthRefreshJobTimeout)
	defer jobCancel()
	if jobCtx.Err() != nil {
		t.Fatal("job ctx should be alive before Stop")
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if s.ctx.Err() == nil {
		t.Fatal("lifecycle ctx must be cancelled by Stop")
	}
	if jobCtx.Err() == nil {
		t.Fatal("in-flight oauth-refresh job ctx must be cancelled by Stop")
	}
}

// TestCheckinScheduler_StopCancelsInFlightJobContext mirrors the assertion
// for the checkin scheduler, whose runCronJob / runIntervalPass /
// runStaleAccountCatchUp all derive from s.ctx. Uses cron mode with no DB so
// Start arms the lifecycle ctx + cron runner without firing a real pass.
func TestCheckinScheduler_StopCancelsInFlightJobContext(t *testing.T) {
	cfg := testConfig() // cron mode, valid CheckinCron
	s := NewCheckinScheduler(cfg)

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	if err := s.Start(rootCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.ctx == nil {
		t.Fatal("lifecycle ctx not captured after Start")
	}

	jobCtx, jobCancel := context.WithTimeout(s.ctx, checkinJobTimeout)
	defer jobCancel()
	if jobCtx.Err() != nil {
		t.Fatal("job ctx should be alive before Stop")
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if s.ctx.Err() == nil {
		t.Fatal("lifecycle ctx must be cancelled by Stop")
	}
	if jobCtx.Err() == nil {
		t.Fatal("in-flight checkin job ctx must be cancelled by Stop")
	}
}

// TestSchedulerJobContextFallsBackBeforeStart verifies the safe default: a
// scheduler that has not been Started still has s.ctx == context.Background()
// (from the constructor), so deriving a job ctx does not panic and is simply
// not cancellable by Stop — matching the pre-fix behavior for any code path
// that runs before Start completes.
func TestSchedulerJobContextFallsBackBeforeStart(t *testing.T) {
	cfg := testConfig()
	s := NewRetentionScheduler(cfg, RetentionSchedulerOptions{
		Name:               "test-retention-fallback",
		Table:              "proxy_logs",
		DefaultIntervalMin: 30,
		RetentionDaysFn:    func(*config.Config) int { return 30 },
		IntervalMinFn:      func(*config.Config) int { return 30 },
		DisabledFn:         func(*config.Config) (bool, string) { return true, "test" },
	})
	if s.ctx == nil {
		t.Fatal("constructor must default s.ctx to context.Background()")
	}
	if s.ctx != context.Background() {
		t.Fatal("constructor default ctx must be context.Background()")
	}
	// Deriving a job timeout from the default ctx must not panic.
	jobCtx, cancel := context.WithTimeout(s.ctx, time.Second)
	defer cancel()
	if jobCtx.Err() != nil {
		t.Fatal("job ctx derived from default ctx should be alive")
	}
}
