package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/checkin"
	"github.com/deliciousbuding/metapi-go/store"
)

const checkinPollMs = int64(60_000) // 60 seconds between interval polls

// intervalCandidate holds the fields needed for interval-based checkin filtering.
type intervalCandidate struct {
	id            int64
	lastCheckinAt *string
}

// CheckinScheduler implements dual-mode (cron + interval) checkin scheduling.
// Mirrors TS checkinScheduler.ts.
type CheckinScheduler struct {
	cfg *config.Config

	mu               sync.Mutex
	mode             string
	cronRunner       *cronRunner
	intervalTimer    *time.Ticker
	intervalStop     chan struct{}
	attemptByAccount map[int64]int64 // accountId -> last attempt timestamp (ms)
}

// NewCheckinScheduler creates a new checkin scheduler.
func NewCheckinScheduler(cfg *config.Config) *CheckinScheduler {
	return &CheckinScheduler{
		cfg:              cfg,
		mode:             cfg.CheckinScheduleMode,
		attemptByAccount: make(map[int64]int64),
	}
}

// Name returns "checkin".
func (s *CheckinScheduler) Name() string { return "checkin" }

// Start starts the checkin scheduler. Loads settings from DB, applies fallbacks,
// and runs the selected mode.
func (s *CheckinScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	activeCron := resolveCronSetting("checkin_cron", s.cfg.CheckinCron)
	activeMode := resolveCheckinScheduleMode(s.cfg)
	activeIntervalHours := config.ClampInt(
		resolvePositiveIntegerSetting("checkin_interval_hours", s.cfg.CheckinIntervalHours),
		1, 24,
	)

	// E1: window bounds (HH:mm) hydrate from settings with env defaults.
	s.cfg.CheckinWindowStart = resolveStringSetting("checkin_window_start", s.cfg.CheckinWindowStart)
	s.cfg.CheckinWindowEnd = resolveStringSetting("checkin_window_end", s.cfg.CheckinWindowEnd)

	s.cfg.CheckinCron = activeCron
	s.cfg.CheckinScheduleMode = activeMode
	s.cfg.CheckinIntervalHours = activeIntervalHours
	s.mode = activeMode

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
					s.runIntervalPass()
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
	if s.mode == "window" {
		expr, err := RandomCronInWindow(s.cfg.CheckinWindowStart, s.cfg.CheckinWindowEnd)
		if err != nil {
			slog.Error("checkin: invalid window bounds, falling back to cron", "error", err, "start", s.cfg.CheckinWindowStart, "end", s.cfg.CheckinWindowEnd)
			s.mode = "cron"
		} else {
			s.cfg.CheckinCron = expr
			s.cronRunner = newCronRunner()
			_, err := s.cronRunner.addJob(expr, s.runCronJob)
			if err != nil {
				slog.Error("checkin: failed to add window cron job", "error", err)
				return
			}
			s.cronRunner.start()
			slog.Info("checkin: window mode armed", "cron", expr, "windowStart", s.cfg.CheckinWindowStart, "windowEnd", s.cfg.CheckinWindowEnd)
			s.maybeCatchUpCheckin()
			return
		}
	}

	// Cron mode
	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(s.cfg.CheckinCron, s.runCronJob)
	if err != nil {
		slog.Error("checkin: failed to add cron job", "error", err)
		return
	}
	s.cronRunner.start()

	// E1b: if the instance started after today's scheduled time and nothing
	// has run today, compensate immediately — a restart must not lose the day.
	s.maybeCatchUpCheckin()
}

// maybeCatchUpCheckin queries whether today's scheduled check-in already
// passed without a run and, if so, triggers an immediate run (guarded by the
// scheduler lease like every other run). Idempotent by construction.
func (s *CheckinScheduler) maybeCatchUpCheckin() {
	dbw := store.GetDB()
	if dbw == nil {
		return
	}
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	var ranToday int
	if err := dbw.QueryRow(`SELECT COUNT(*) FROM checkin_logs WHERE created_at >= ?`, startOfDay.Format(time.RFC3339)).Scan(&ranToday); err != nil {
		slog.Warn("checkin catch-up: ran-today query failed", "error", err)
		return
	}
	var enabled int
	if err := dbw.QueryRow(`SELECT COUNT(*) FROM accounts WHERE checkin_enabled = TRUE AND status = 'active'`).Scan(&enabled); err != nil {
		slog.Warn("checkin catch-up: enabled-count query failed", "error", err)
		return
	}

	if !shouldCatchUpCheckin(now, s.cfg.CheckinCron, ranToday > 0, enabled) {
		return
	}
	slog.Info("checkin: missed today's scheduled time, compensating with immediate run", "spec", s.cfg.CheckinCron)
	go s.runCronJob()
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
		return formatErr("invalid checkin schedule mode: %s", mode)
	}
	if mode == "cron" && !ValidateCronExpr(cronExpr) {
		return formatErr("invalid cron expression: %s", cronExpr)
	}
	if mode == "interval" && (intervalHours < 1 || intervalHours > 24) {
		return formatErr("invalid interval hours: %d (must be 1-24)", intervalHours)
	}
	if mode == "window" {
		if _, err := RandomCronInWindow(windowStart, windowEnd); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.mode = mode
	if mode == "cron" {
		s.cfg.CheckinCron = cronExpr
	}
	if mode == "window" {
		s.cfg.CheckinWindowStart = windowStart
		s.cfg.CheckinWindowEnd = windowEnd
	}
	s.cfg.CheckinScheduleMode = mode
	s.cfg.CheckinIntervalHours = config.ClampInt(intervalHours, 1, 24)
	s.startLocked()
	return nil
}

// ResetAttempts clears the interval attempt map (for tests).
func (s *CheckinScheduler) ResetAttempts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attemptByAccount = make(map[int64]int64)
}

// ---- Internal ----

func (s *CheckinScheduler) runCronJob() {
	slog.Info("checkin: cron job starting")
	dbw := store.GetDB()
	if dbw == nil {
		slog.Error("checkin: database not available")
		return
	}
	runWithSchedulerLease(context.Background(), dbw, s.Name(), func() {
		results := checkin.CheckinAll(s.cfg, dbw.DB, nil, "cron")
		ok, bad := countResults(results)
		slog.Info("checkin: cron job done", "success", ok, "failed", bad)
	})
}

func (s *CheckinScheduler) runIntervalPass() {
	dbw := store.GetDB()
	if dbw == nil {
		return
	}
	runWithSchedulerLease(context.Background(), dbw, s.Name(), func() {
		s.runIntervalPassLocked(dbw)
	})
}

func (s *CheckinScheduler) runIntervalPassLocked(dbw *store.DB) {
	now := time.Now()

	// Query all account+site pairs
	rows, err := dbw.Query(`
		SELECT a.id, a.last_checkin_at
		FROM accounts a
		INNER JOIN sites s ON a.site_id = s.id
		WHERE a.checkin_enabled = TRUE
		  AND a.status = 'active'
		  AND s.status <> 'disabled'
	`)
	if err != nil {
		slog.Error("checkin interval: query failed", "error", err)
		return
	}
	defer rows.Close()

	var candidates []intervalCandidate
	for rows.Next() {
		var c intervalCandidate
		if err := rows.Scan(&c.id, &c.lastCheckinAt); err != nil {
			continue
		}
		candidates = append(candidates, c)
	}

	dueIDs := s.filterDue(candidates, now)
	if len(dueIDs) == 0 {
		return
	}

	results := checkin.CheckinAll(s.cfg, dbw.DB, dueIDs, "interval")

	nowMs := now.UnixMilli()
	s.mu.Lock()
	for _, r := range results {
		s.attemptByAccount[r.AccountID] = nowMs
	}
	s.mu.Unlock()

	ok, bad := countResults(results)
	slog.Info("checkin: interval pass done",
		"due", len(dueIDs), "success", ok, "failed", bad)
}

// filterDue mirrors TS selectDueIntervalCheckinAccountIds().
func (s *CheckinScheduler) filterDue(rows []intervalCandidate, now time.Time) []int64 {
	nowMs := now.UnixMilli()
	intervalHours := config.ClampInt(s.cfg.CheckinIntervalHours, 1, 24)
	intervalMs := int64(intervalHours) * 3600 * 1000

	s.mu.Lock()
	defer s.mu.Unlock()

	var due []int64
	for _, row := range rows {
		hasCheckin := false
		var checkinMs int64
		if row.lastCheckinAt != nil && *row.lastCheckinAt != "" {
			if t, err := time.Parse(time.RFC3339, *row.lastCheckinAt); err == nil {
				checkinMs = t.UnixMilli()
				hasCheckin = true
			}
		}
		attemptMs, hasAttempt := s.attemptByAccount[row.id]

		if hasCheckin {
			if nowMs-checkinMs < intervalMs {
				continue // already checked in within window
			}
			if hasAttempt && attemptMs >= checkinMs && nowMs-attemptMs < intervalMs {
				continue // mid-flight checkin
			}
		} else {
			if hasAttempt && nowMs-attemptMs < intervalMs {
				continue // recently attempted, no known checkin
			}
		}
		due = append(due, row.id)
	}
	return due
}

func countResults(results []checkin.CheckinAllResult) (success, failed int) {
	for _, r := range results {
		if r.Result.Success {
			success++
		} else {
			failed++
		}
	}
	return
}
