package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

const logCleanupDefaultCron = "0 6 * * *"

// LogCleanupScheduler cleans up proxy log records and event records.
type LogCleanupScheduler struct {
	cfg        *config.Config
	cronRunner *cronRunner
}

func NewLogCleanupScheduler(cfg *config.Config) *LogCleanupScheduler {
	return &LogCleanupScheduler{cfg: cfg}
}

func (s *LogCleanupScheduler) Name() string { return "log-cleanup" }

func (s *LogCleanupScheduler) Start(ctx context.Context) error {
	rt := config.Runtime()
	fallback := rt.LogCleanupCron
	if fallback == "" {
		fallback = logCleanupDefaultCron
	}
	activeCron := resolveCronSetting("log_cleanup_cron", fallback)
	usageEnabled := resolveBooleanSetting("log_cleanup_usage_logs_enabled", rt.LogCleanupUsageLogsEnabled)
	programEnabled := resolveBooleanSetting("log_cleanup_program_logs_enabled", rt.LogCleanupProgramLogsEnabled)
	retentionDays := config.ClampInt(
		resolvePositiveIntegerSetting("log_cleanup_retention_days", rt.LogCleanupRetentionDays),
		1, 3650,
	)
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.LogCleanupCron = activeCron
		r.LogCleanupUsageLogsEnabled = usageEnabled
		r.LogCleanupProgramLogsEnabled = programEnabled
		r.LogCleanupRetentionDays = retentionDays
	})

	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(activeCron, s.runJob)
	if err != nil {
		slog.Error("log-cleanup: failed to add cron job", "error", err)
		return err
	}
	s.cronRunner.start()

	slog.Info("log-cleanup scheduler started",
		"cron", activeCron,
		"configured", s.cfg.LogCleanupConfigured,
		"usage_enabled", usageEnabled,
		"program_enabled", programEnabled,
		"retention_days", retentionDays,
	)
	return nil
}

func (s *LogCleanupScheduler) Stop() error {
	if s.cronRunner != nil {
		s.cronRunner.stop()
		s.cronRunner = nil
	}
	return nil
}

func (s *LogCleanupScheduler) UpdateSettings(cronExpr string, usageEnabled, programEnabled bool, retentionDays int) error {
	if !ValidateCronExpr(cronExpr) {
		return formatErr("invalid cron expression: %s", cronExpr)
	}

	clamped := config.ClampInt(retentionDays, 1, 3650)
	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.LogCleanupCron = cronExpr
		r.LogCleanupUsageLogsEnabled = usageEnabled
		r.LogCleanupProgramLogsEnabled = programEnabled
		r.LogCleanupRetentionDays = clamped
	})

	if s.cronRunner != nil {
		s.cronRunner.stop()
	}
	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(cronExpr, s.runJob)
	if err != nil {
		return err
	}
	s.cronRunner.start()
	return nil
}

func (s *LogCleanupScheduler) runJob() {
	if !s.cfg.LogCleanupConfigured {
		slog.Info("log-cleanup: skipped, legacy fallback mode active")
		return
	}
	// One snapshot for the whole job so a concurrent settings change cannot
	// split the decision from the retention window it runs with.
	rt := config.Runtime()
	if !rt.LogCleanupUsageLogsEnabled && !rt.LogCleanupProgramLogsEnabled {
		slog.Info("log-cleanup: skipped, no log target enabled")
		return
	}

	slog.Info("log-cleanup: running cleanup")
	dbw := store.GetDB()
	if dbw == nil {
		slog.Error("log-cleanup: database not available")
		return
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), logCleanupJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runJobLocked(dbw, rt)
	})
}

func (s *LogCleanupScheduler) runJobLocked(dbw *store.DB, rt *config.RuntimeSettings) {
	now := time.Now()
	cutoff := now.Add(-time.Duration(rt.LogCleanupRetentionDays) * 24 * time.Hour)
	cutoffStr := formatTimeToSQL(cutoff)

	var usageDeleted, programDeleted int64

	if rt.LogCleanupUsageLogsEnabled {
		result, err := dbw.Exec("DELETE FROM proxy_logs WHERE created_at < ?", cutoffStr)
		if err != nil {
			slog.Error("log-cleanup: failed to cleanup proxy_logs", "error", err)
		} else {
			usageDeleted, _ = result.RowsAffected()
		}
	}

	if rt.LogCleanupProgramLogsEnabled {
		result, err := dbw.Exec("DELETE FROM events WHERE created_at < ?", cutoffStr)
		if err != nil {
			slog.Error("log-cleanup: failed to cleanup events", "error", err)
		} else {
			programDeleted, _ = result.RowsAffected()
		}
	}

	slog.Info("log-cleanup: complete",
		"usage_deleted", usageDeleted,
		"program_deleted", programDeleted,
		"cutoff", cutoffStr,
	)
}

// formatTimeToSQL formats a retention cutoff for lexicographic compare against
// TEXT created_at columns written as UTC RFC3339 (e.g. proxy_logs/events/files).
// Space-separated "2006-01-02 15:04:05" is wrong: 'T' > ' ' so same-day old rows
// never satisfy created_at < cutoff.
func formatTimeToSQL(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
