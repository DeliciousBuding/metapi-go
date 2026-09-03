package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// ModelSyncScheduler periodically refreshes upstream model lists for all
// candidate accounts (#1005). Structure mirrors BalanceScheduler: DB setting
// model_sync_cron overrides the env default; UpdateCron hot-reloads the
// running runner; the job runs under the cluster-wide scheduler lease.
type ModelSyncScheduler struct {
	cronRunner *cronRunner
}

// NewModelSyncScheduler creates a new model sync scheduler. The cron state
// lives in the runtime-settings snapshot (hot-updated by settings apply).
func NewModelSyncScheduler() *ModelSyncScheduler {
	return &ModelSyncScheduler{}
}

// Name returns "model-sync".
func (s *ModelSyncScheduler) Name() string { return "model-sync" }

// Start begins periodic model sync. Loads the cron expression from DB
// settings or falls back to config default.
func (s *ModelSyncScheduler) Start(ctx context.Context) error {
	activeCron := resolveCronSetting("model_sync_cron", config.Runtime().ModelSyncCron)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelSyncCron = activeCron })

	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(activeCron, s.runJob)
	if err != nil {
		slog.Error("model-sync: failed to add cron job", "error", err, "cron", activeCron)
		return err
	}
	s.cronRunner.start()

	slog.Info("model-sync scheduler started", "cron", activeCron)
	return nil
}

// Stop halts the model sync scheduler.
func (s *ModelSyncScheduler) Stop() error {
	if s.cronRunner != nil {
		s.cronRunner.stop()
		s.cronRunner = nil
	}
	return nil
}

// UpdateCron updates the cron expression at runtime.
func (s *ModelSyncScheduler) UpdateCron(cronExpr string) error {
	if !ValidateCronExpr(cronExpr) {
		return fmt.Errorf("invalid cron expression: %s", cronExpr)
	}
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelSyncCron = cronExpr })
	if s.cronRunner != nil {
		s.cronRunner.stop()
	}
	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(cronExpr, s.runJob)
	if err != nil {
		return err
	}
	s.cronRunner.start()
	slog.Info("model-sync scheduler updated", "cron", cronExpr)
	return nil
}

func (s *ModelSyncScheduler) runJob() {
	slog.Info("model-sync: refreshing upstream model lists")
	dbw := store.GetDB()
	if dbw == nil {
		slog.Error("model-sync: database not available")
		return
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), modelSyncJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runJobLocked(jobCtx, dbw)
	})
}

func (s *ModelSyncScheduler) runJobLocked(ctx context.Context, dbw *store.DB) {
	// Sequential per-account refresh with one rebuild + one routing-cache
	// invalidation for the whole pass; summary logging happens inside.
	service.SyncAllAccountModels(ctx, dbw.DB)
}
