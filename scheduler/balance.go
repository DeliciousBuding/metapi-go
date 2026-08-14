package scheduler

import (
	"context"
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
// settings or falls back to config default.
func (s *BalanceScheduler) Start(ctx context.Context) error {
	activeCron := resolveCronSetting("balance_refresh_cron", s.cfg.BalanceRefreshCron)
	s.cfg.BalanceRefreshCron = activeCron

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
		return formatErr("invalid cron expression: %s", cronExpr)
	}
	s.cfg.BalanceRefreshCron = cronExpr
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

func (s *BalanceScheduler) runJob() {
	slog.Info("balance: refreshing all balances")
	dbw := store.GetDB()
	if dbw == nil {
		slog.Error("balance: database not available")
		return
	}
	runWithSchedulerLease(context.Background(), dbw, s.Name(), func() {
		s.runJobLocked(dbw)
	})
}

func (s *BalanceScheduler) runJobLocked(dbw *store.DB) {
	// Refresh all balances.
	results := balance.RefreshAllBalances(s.cfg, dbw.DB)
	slog.Info("balance: refresh complete", "accounts", len(results))
}
