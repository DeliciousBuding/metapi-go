package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/schedulespec"
	"github.com/deliciousbuding/metapi-go/service/checkin"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

const checkinPollMs = int64(60_000) // 60 seconds between interval polls

// CheckinScheduler implements dual-mode (cron + interval) checkin scheduling.
// Mirrors TS checkinScheduler.ts.
type CheckinScheduler struct {
	db  *store.DB
	cfg *config.Config

	mu               sync.Mutex
	mode             string
	cronRunner       *cronRunner
	intervalTimer    *time.Ticker
	intervalStop     chan struct{}
	attemptByAccount map[int64]int64 // accountId -> last attempt timestamp (ms)

	// ctx is the lifecycle context captured from Start. Job timeouts derive
	// from it instead of context.Background() so that Stop (which cancels it)
	// also cancels in-flight checkin passes on shutdown. Defaults to
	// context.Background() so behavior before/without Start matches the old
	// code (just not cancellable by Stop). UpdateCheckinSchedule does NOT
	// reset it — the lifecycle ctx is tied to Start/Stop, not config reload.
	ctx    context.Context
	cancel context.CancelFunc

	// checkinAll is the checkin execution function. Defaults to
	// checkin.CheckinAllContext, so the round carries the job ctx (derived from
	// the lifecycle ctx) and its same-site pacing wait ends when Stop() is
	// called instead of sleeping out the remaining accounts; overridable in
	// tests to inject a mock that records calls without touching real
	// upstreams.
	checkinAll func(ctx context.Context, cfg *config.Config, db *sqlx.DB, accountIDs []int64, scheduleMode string) []checkin.CheckinAllResult
}

// NewCheckinScheduler creates a new checkin scheduler.
func NewCheckinScheduler(db *store.DB, cfg *config.Config) *CheckinScheduler {
	return &CheckinScheduler{
		db:               db,
		cfg:              cfg,
		mode:             config.Runtime().CheckinScheduleMode,
		attemptByAccount: make(map[int64]int64),
		checkinAll:       checkin.CheckinAllContext,
		ctx:              context.Background(),
	}
}

// Name returns "checkin".
func (s *CheckinScheduler) Name() string { return "checkin" }

// Start starts the checkin scheduler. Loads settings from DB, applies fallbacks,
// and runs the selected mode.
func (s *CheckinScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Capture the runner lifecycle context so per-job timeouts derive from it
	// (cancelled on Stop) instead of context.Background() (never cancelled).
	// Set before startLocked so goroutines launched by maybeCatchUpCheckin
	// (runCronJob / runStaleAccountCatchUp) observe the captured ctx.
	s.ctx, s.cancel = context.WithCancel(ctx)

	rt := config.Runtime()
	activeCron := resolveCronSetting(s.db, "checkin_cron", rt.CheckinCron)
	activeMode := resolveCheckinScheduleMode(s.db, rt)
	activeIntervalHours := config.ClampInt(
		resolvePositiveIntegerSetting(s.db, "checkin_interval_hours", rt.CheckinIntervalHours),
		1, 24,
	)

	// E1: window bounds (HH:mm) hydrate from settings with env defaults.
	windowStart := resolveStringSetting(s.db, "checkin_window_start", rt.CheckinWindowStart)
	windowEnd := resolveStringSetting(s.db, "checkin_window_end", rt.CheckinWindowEnd)

	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.CheckinCron = activeCron
		r.CheckinScheduleMode = activeMode
		r.CheckinIntervalHours = activeIntervalHours
		r.CheckinWindowStart = windowStart
		r.CheckinWindowEnd = windowEnd
	})
	s.mode = activeMode

	// #1027: global check-in kill switch (env CHECKIN_ENABLED or the
	// checkin_enabled runtime setting). Default true keeps historical
	// behavior; when disabled no schedule is armed at all.
	enabled := resolveBooleanSetting(s.db, "checkin_enabled", !rt.CheckinDisabled)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinDisabled = !enabled })
	if !enabled {
		slog.Info("checkin scheduler disabled (checkinEnabled=false)")
		return nil
	}

	s.startLocked()

	slog.Info("checkin scheduler started",
		"mode", activeMode,
		"cron", activeCron,
		"interval_hours", activeIntervalHours,
	)
	return nil
}

// Stop stops the checkin scheduler.
func (s *CheckinScheduler) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
	// Cancel the lifecycle context so any in-flight checkin pass whose job
	// timeout derives from it aborts promptly. Lease Release falls back to
	// context.Background() when the job ctx is already done, so this does
	// not strand an advisory lock.
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}

func (s *CheckinScheduler) startLocked() {
	s.stopLocked()

	if s.mode == "interval" {
		s.intervalTimer = time.NewTicker(time.Duration(checkinPollMs) * time.Millisecond)
		s.intervalStop = make(chan struct{})
		go func() {
			for {
				select {
				case <-s.intervalTimer.C:
					// Boundary panic recovery: this goroutine is not
					// covered by cronRunner's wrapper; an unrecovered
					// panic here would crash the process.
					safeJob("checkin-interval", s.runIntervalPass)
				case <-s.intervalStop:
					return
				}
			}
		}()
		return
	}

	// E1: window mode — pick a random HH:mm inside the
	// configured window and schedule as a daily cron. The roll is re-done per
	// start/setting change, giving load spreading + anti-fingerprint without
	// a separate re-roll job. Bounds are HH:mm in 24h format.
	wrt := config.Runtime()
	if s.mode == "window" {
		expr, err := RandomCronInWindow(wrt.CheckinWindowStart, wrt.CheckinWindowEnd)
		if err != nil {
			slog.Error("checkin: invalid window bounds, falling back to cron", "error", err, "start", wrt.CheckinWindowStart, "end", wrt.CheckinWindowEnd)
			s.mode = "cron"
		} else {
			config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinCron = expr })
			s.cronRunner = newCronRunner()
			_, err := s.cronRunner.addJob(expr, s.runCronJob)
			if err != nil {
				slog.Error("checkin: failed to add window cron job", "error", err)
				return
			}
			s.cronRunner.start()
			slog.Info("checkin: window mode armed", "cron", expr, "windowStart", wrt.CheckinWindowStart, "windowEnd", wrt.CheckinWindowEnd)
			s.maybeCatchUpCheckin()
			return
		}
	}

	// Cron mode
	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(config.Runtime().CheckinCron, s.runCronJob)
	if err != nil {
		slog.Error("checkin: failed to add cron job", "error", err)
		return
	}
	s.cronRunner.start()

	// E1b: if the instance started after today's scheduled time and nothing
	// has run today, compensate immediately — a restart must not lose the day.
	s.maybeCatchUpCheckin()
}

func (s *CheckinScheduler) stopLocked() {
	if s.cronRunner != nil {
		s.cronRunner.stop()
		s.cronRunner = nil
	}
	if s.intervalTimer != nil {
		s.intervalTimer.Stop()
	}
	// Only close intervalStop if it hasn't been closed yet.
	// The background goroutine reads intervalStop without holding the lock,
	// so we must NOT nil it out — closing is the signal, and Go's closed-channel
	// read returns immediately without a data race.
	if s.intervalStop != nil {
		select {
		case <-s.intervalStop:
			// already closed, skip
		default:
			close(s.intervalStop)
		}
	}
}

// UpdateCheckinSchedule updates the checkin configuration at runtime.
// Mirrors TS updateCheckinSchedule(). windowStart/windowEnd are HH:mm bounds
// for E1 window mode (ignored in cron/interval modes).
func (s *CheckinScheduler) UpdateCheckinSchedule(mode, cronExpr string, intervalHours int, windowStart, windowEnd string) error {
	mode = stringsTrimLower(mode)
	if mode != "cron" && mode != "interval" && mode != "window" {
		return fmt.Errorf("invalid checkin schedule mode: %s", mode)
	}
	if mode == "cron" && !ValidateCronExpr(cronExpr) {
		return fmt.Errorf("invalid cron expression: %s", cronExpr)
	}
	if mode == "interval" && (intervalHours < 1 || intervalHours > 24) {
		return fmt.Errorf("invalid interval hours: %d (must be 1-24)", intervalHours)
	}
	if mode == "window" {
		if _, _, err := schedulespec.WindowBounds(windowStart, windowEnd); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.mode = mode
	clampedHours := config.ClampInt(intervalHours, 1, 24)
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		if mode == "cron" {
			r.CheckinCron = cronExpr
		}
		if mode == "window" {
			r.CheckinWindowStart = windowStart
			r.CheckinWindowEnd = windowEnd
		}
		r.CheckinScheduleMode = mode
		r.CheckinIntervalHours = clampedHours
	})
	// #1027: a schedule update while check-in is globally disabled must
	// persist the new schedule without silently re-arming the runner.
	if config.Runtime().CheckinDisabled {
		return nil
	}
	s.startLocked()
	return nil
}

// SetEnabled hot-toggles the global check-in switch (#1027). Disabling stops
// the running cron/interval schedule; enabling restarts with the current mode
// and schedule (same behavior as a fresh Start, including catch-up). Callers
// serialize this against other scheduler updates via app.updateMu.
func (s *CheckinScheduler) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinDisabled = !enabled })
	if !enabled {
		s.stopLocked()
		slog.Info("checkin scheduler disabled (checkinEnabled=false)")
		return
	}
	s.startLocked()
	slog.Info("checkin scheduler enabled", "mode", s.mode)
}
