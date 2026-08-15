package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// RetentionSchedulerOptions configures a single retention job. The three
// production instances (proxy logs / proxy files / proxy video tasks) were
// ~95% identical and are collapsed onto one implementation.
type RetentionSchedulerOptions struct {
	// Name is the scheduler/lease/log-prefix name.
	Name string
	// Table is pruned via DELETE FROM <table> WHERE created_at < ?
	Table string
	// ExtraWhere is appended after "created_at < ?" (e.g. " AND deleted_at IS NULL").
	ExtraWhere string
	// UseUTC computes the cutoff with time.Now().UTC() instead of local time.
	UseUTC bool
	// DefaultIntervalMin applies when IntervalMinFn yields a non-positive value.
	DefaultIntervalMin int
	// RetentionDaysFn / IntervalMinFn read the per-job config values.
	RetentionDaysFn func(cfg *config.Config) int
	IntervalMinFn   func(cfg *config.Config) int
	// DisabledFn reports whether Start should no-op, with a log reason.
	DisabledFn func(cfg *config.Config) (disabled bool, reason string)
	// StartLogSuffix is appended to the "started" log message ("" for none).
	StartLogSuffix string
}

// RetentionScheduler prunes rows older than a per-config retention window on
// a fixed interval, under a scheduler lease (multi-instance safe).
type RetentionScheduler struct {
	cfg    *config.Config
	opts   RetentionSchedulerOptions
	runner *intervalRunner
	// ctx is the lifecycle context captured from Start. Job timeouts derive
	// from it instead of context.Background() so that Stop (which cancels it)
	// also cancels in-flight cleanups on shutdown. Defaults to
	// context.Background() so a job running before/without Start behaves like
	// the old code (just not cancellable by Stop).
	ctx    context.Context
	cancel context.CancelFunc
}

// NewRetentionScheduler builds a generic retention scheduler.
func NewRetentionScheduler(cfg *config.Config, opts RetentionSchedulerOptions) *RetentionScheduler {
	return &RetentionScheduler{
		cfg:    cfg,
		opts:   opts,
		runner: &intervalRunner{},
		ctx:    context.Background(),
	}
}

func (s *RetentionScheduler) Name() string { return s.opts.Name }

func (s *RetentionScheduler) Start(ctx context.Context) error {
	// Capture the runner lifecycle context so per-job timeouts derive from it
	// (cancelled on Stop) instead of context.Background() (never cancelled).
	s.ctx, s.cancel = context.WithCancel(ctx)

	if disabled, reason := s.opts.DisabledFn(s.cfg); disabled {
		slog.Info(s.opts.Name + ": disabled (" + reason + ")")
		return nil
	}

	intervalMin := config.MaxInt(s.opts.IntervalMinFn(s.cfg), 1)
	if intervalMin == 0 {
		intervalMin = s.opts.DefaultIntervalMin
	}
	interval := time.Duration(intervalMin) * time.Minute

	slog.Info(s.opts.Name+" scheduler started"+s.opts.StartLogSuffix,
		"interval_min", intervalMin,
		"retention_days", s.opts.RetentionDaysFn(s.cfg),
	)
	return s.runner.start(ctx, interval, true, s.runCleanup)
}

func (s *RetentionScheduler) Stop() error {
	// Cancel the lifecycle context first so any in-flight cleanup whose job
	// timeout derives from it aborts promptly. The runner stop then halts
	// future ticks. Lease Release falls back to context.Background() when the
	// job ctx is already done, so this does not strand an advisory lock.
	if s.cancel != nil {
		s.cancel()
	}
	return s.runner.stop()
}

func (s *RetentionScheduler) runCleanup() {
	dbw := store.GetDB()
	if dbw == nil {
		return
	}

	retentionDays := s.opts.RetentionDaysFn(s.cfg)
	if retentionDays <= 0 {
		return
	}
	// Derive the job timeout from the lifecycle ctx (cancellable on Stop),
	// not context.Background(). runWithSchedulerLease still releases the
	// lease via context.Background() when this ctx is already done.
	jobCtx, cancel := context.WithTimeout(s.ctx, retentionJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runCleanupLocked(dbw, retentionDays)
	})
}

func (s *RetentionScheduler) runCleanupLocked(dbw *store.DB, retentionDays int) {
	now := time.Now()
	if s.opts.UseUTC {
		now = now.UTC()
	}
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	cutoffStr := formatTimeToSQL(cutoff)

	query := "DELETE FROM " + s.opts.Table + " WHERE created_at < ?" + s.opts.ExtraWhere
	result, err := dbw.Exec(query, cutoffStr)
	if err != nil {
		slog.Warn(s.opts.Name+": cleanup failed", "error", err)
		return
	}
	deleted, _ := result.RowsAffected()
	if deleted > 0 {
		slog.Info(s.opts.Name+": cleanup complete",
			"deleted", deleted,
			"cutoff", cutoffStr,
			"retention_days", retentionDays,
		)
	}
}
