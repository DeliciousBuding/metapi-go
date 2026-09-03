package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/balance"
	"github.com/deliciousbuding/metapi-go/store"
)

// BalanceScheduler periodically refreshes account balances.
type BalanceScheduler struct {
	cfg        *config.Config
	cronRunner *cronRunner
}

// NewBalanceScheduler creates a new balance refresh scheduler.
func NewBalanceScheduler(cfg *config.Config) *BalanceScheduler {
	return &BalanceScheduler{cfg: cfg}
}

// Name returns "balance-refresh".
func (s *BalanceScheduler) Name() string { return "balance-refresh" }

// Start begins periodic balance refresh. Loads the cron expression from DB
// settings or falls back to config default. Honors the global kill switch
// (env BALANCE_REFRESH_ENABLED / runtime setting balanceRefreshEnabled,
// issue #1027): when disabled, no cron job is scheduled at all.
func (s *BalanceScheduler) Start(ctx context.Context) error {
	rt := config.Runtime()
	enabled := resolveBooleanSetting("balance_refresh_enabled", !rt.BalanceRefreshDisabled)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshDisabled = !enabled })
	if !enabled {
		slog.Info("balance scheduler disabled (balanceRefreshEnabled=false)")
		return nil
	}

	activeCron := resolveCronSetting("balance_refresh_cron", rt.BalanceRefreshCron)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshCron = activeCron })

	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(activeCron, s.runJob)
	if err != nil {
		slog.Error("balance: failed to add cron job", "error", err, "cron", activeCron)
		return err
	}
	s.cronRunner.start()

	slog.Info("balance scheduler started", "cron", activeCron)
	return nil
}

// Stop halts the balance refresh scheduler.
func (s *BalanceScheduler) Stop() error {
	if s.cronRunner != nil {
		s.cronRunner.stop()
		s.cronRunner = nil
	}
	return nil
}

// UpdateCron updates the cron expression at runtime.
func (s *BalanceScheduler) UpdateCron(cronExpr string) error {
	if !ValidateCronExpr(cronExpr) {
		return fmt.Errorf("invalid cron expression: %s", cronExpr)
	}
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshCron = cronExpr })
	if s.cronRunner != nil {
		s.cronRunner.stop()
	}
	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(cronExpr, s.runJob)
	if err != nil {
		return err
	}
	s.cronRunner.start()
	slog.Info("balance scheduler updated", "cron", cronExpr)
	return nil
}

// SetEnabled hot-toggles the global balance-refresh switch (#1027).
// Disabling stops the running cron job entirely; enabling (re)resolves the
// cron from DB settings (same contract as Start) and schedules it. Callers
// serialize this against other scheduler updates via app.updateMu.
func (s *BalanceScheduler) SetEnabled(enabled bool) error {
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshDisabled = !enabled })
	if !enabled {
		if s.cronRunner != nil {
			s.cronRunner.stop()
			s.cronRunner = nil
		}
		slog.Info("balance scheduler disabled (balanceRefreshEnabled=false)")
		return nil
	}
	activeCron := resolveCronSetting("balance_refresh_cron", config.Runtime().BalanceRefreshCron)
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshCron = activeCron })
	if s.cronRunner != nil {
		s.cronRunner.stop()
	}
	s.cronRunner = newCronRunner()
	if _, err := s.cronRunner.addJob(activeCron, s.runJob); err != nil {
		return err
	}
	s.cronRunner.start()
	slog.Info("balance scheduler enabled", "cron", activeCron)
	return nil
}

func (s *BalanceScheduler) runJob() {
	slog.Info("balance: refreshing all balances")
	dbw := store.GetDB()
	if dbw == nil {
		slog.Error("balance: database not available")
		return
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), balanceJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runJobLocked(dbw)
	})
}

func (s *BalanceScheduler) runJobLocked(dbw *store.DB) {
	// Refresh all balances.
	results := balance.RefreshAllBalances(s.cfg, dbw.DB)
	slog.Info("balance: refresh complete", "accounts", len(results))
}
