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
}

// NewRetentionScheduler builds a generic retention scheduler.
func NewRetentionScheduler(cfg *config.Config, opts RetentionSchedulerOptions) *RetentionScheduler {
	return &RetentionScheduler{
		cfg:    cfg,
		opts:   opts,
		runner: &intervalRunner{},
	}
}

func (s *RetentionScheduler) Name() string { return s.opts.Name }

func (s *RetentionScheduler) Start(ctx context.Context) error {
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
	jobCtx, cancel := context.WithTimeout(context.Background(), retentionJobTimeout)
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
