package app

import (
	"context"
	"log/slog"
	"sync"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service/oauth"
)

var (
	servicesMu sync.RWMutex
	// updateMu serializes scheduler config updates against each other and
	// against StopBackgroundServices so a hot reload cannot race a shutdown.
	updateMu            sync.Mutex
	registry            *scheduler.Registry
	checkinScheduler    *scheduler.CheckinScheduler
	balanceScheduler    *scheduler.BalanceScheduler
	logCleanupScheduler *scheduler.LogCleanupScheduler
	webdavScheduler     *scheduler.BackupWebdavScheduler
)

// StartBackgroundServices creates and starts all 16 background schedulers.
func StartBackgroundServices() {
	slog.Info("starting background schedulers")

	cfg := config.Get()
	newRegistry, checkin, balance, logCleanup, webdav := buildSchedulers(cfg)

	// Start all
	newRegistry.StartAll(context.Background())

	// Publish scheduler pointers only after StartAll so a config update can
	// never observe a half-started scheduler.
	servicesMu.Lock()
	registry = newRegistry
	checkinScheduler = checkin
	balanceScheduler = balance
	logCleanupScheduler = logCleanup
	webdavScheduler = webdav
	servicesMu.Unlock()

	slog.Info("all background schedulers registered",
		"count", len(newRegistry.List()),
	)
}

// buildSchedulers constructs and registers every background scheduler without
// starting them, so registration coverage can be asserted independently of the
// runtime side effects of StartAll.
func buildSchedulers(cfg *config.Config) (
	*scheduler.Registry,
	*scheduler.CheckinScheduler,
	*scheduler.BalanceScheduler,
	*scheduler.LogCleanupScheduler,
	*scheduler.BackupWebdavScheduler,
) {
	newRegistry := scheduler.NewRegistry()

	// ---- Usage Aggregation (needed by admin-snapshot) ----
	usageAgg := scheduler.NewUsageAggregationScheduler(cfg)
	newRegistry.Register(usageAgg)

	// ---- Scheduler 1: Checkin ----
	checkin := scheduler.NewCheckinScheduler(cfg)
	newRegistry.Register(checkin)

	// ---- Scheduler 2: Balance Refresh ----
	balance := scheduler.NewBalanceScheduler(cfg)
	newRegistry.Register(balance)

	// ---- Scheduler 3: Daily Summary ----
	newRegistry.Register(scheduler.NewDailySummaryScheduler(cfg))

	// ---- Scheduler 4: Log Cleanup ----
	logCleanup := scheduler.NewLogCleanupScheduler(cfg)
	newRegistry.Register(logCleanup)

	// ---- Scheduler 5: Backup WebDAV ----
	webdav := scheduler.NewBackupWebdavScheduler(cfg)
	newRegistry.Register(webdav)

	// ---- Scheduler 6: Site Announcements ----
	newRegistry.Register(scheduler.NewSiteAnnouncementScheduler(cfg))

	// ---- Scheduler 7: Model Probe ----
	// Inject live ChannelHealthProbe + TokenRouter health recorder when the
	// proxy upstream router is already configured.
	modelProbe := scheduler.NewModelProbeScheduler(cfg)
	WireModelProbeScheduler(modelProbe)
	newRegistry.Register(modelProbe)

	// ---- Scheduler 8: Channel Recovery ----
	newRegistry.Register(scheduler.NewChannelRecoveryScheduler(cfg))

	// ---- Scheduler 9: Sub2API Refresh ----
	newRegistry.Register(scheduler.NewSub2APIRefreshScheduler(cfg))

	// ---- Scheduler 10: Update Center ----
	// Gated behind METAPI_ENABLE_UPDATE_CENTER (default false). The scheduler
	// is a log-only no-op today (no remote registry / version discovery), so it
	// is registered only when an operator explicitly opts in.
	if cfg.UpdateCenterEnabled {
		newRegistry.Register(scheduler.NewUpdateCenterScheduler(cfg))
	} else {
		slog.Info("update center scheduler disabled (METAPI_ENABLE_UPDATE_CENTER not set)")
	}

	// ---- Scheduler 12: Admin Snapshot ----
	newRegistry.Register(scheduler.NewAdminSnapshotScheduler(cfg, usageAgg))

	// ---- Scheduler 13: Proxy File Retention ----
	newRegistry.Register(scheduler.NewProxyFileRetentionScheduler(cfg))

	// ---- Scheduler 13b: Proxy Video Task Retention ----
	newRegistry.Register(scheduler.NewProxyVideoTaskRetentionScheduler(cfg))

	// ---- Scheduler 14: Proxy Log Retention (legacy fallback) ----
	newRegistry.Register(scheduler.NewProxyLogRetentionScheduler(cfg))

	// ---- Scheduler 15: OAuth Token Refresh ----
	newRegistry.Register(scheduler.NewOAuthRefreshScheduler(cfg))

	return newRegistry, checkin, balance, logCleanup, webdav
}

// StopBackgroundServices stops all background schedulers.
func StopBackgroundServices() {
	slog.Info("stopping background schedulers")
	updateMu.Lock()
	defer updateMu.Unlock()
	servicesMu.Lock()
	activeRegistry := registry
	registry = nil
	checkinScheduler = nil
	balanceScheduler = nil
	logCleanupScheduler = nil
	webdavScheduler = nil
	servicesMu.Unlock()
	if activeRegistry != nil {
		activeRegistry.StopAll()
	}
	oauth.StopLoopbackCallbackServers()
}

// UpdateCheckinSchedule applies a persisted checkin schedule to the running
// scheduler. It is a no-op before background services have started.
// windowStart/windowEnd are HH:mm bounds for E1 window mode (ignored otherwise).
func UpdateCheckinSchedule(mode, cronExpr string, intervalHours int, windowStart, windowEnd string) error {
	updateMu.Lock()
	defer updateMu.Unlock()
	servicesMu.RLock()
	activeScheduler := checkinScheduler
	servicesMu.RUnlock()
	if activeScheduler == nil {
		return nil
	}
	return activeScheduler.UpdateCheckinSchedule(mode, cronExpr, intervalHours, windowStart, windowEnd)
}

// UpdateBalanceCron hot-reloads the running balance-refresh scheduler with a
// newly persisted cron. It is a no-op before background services have started.
func UpdateBalanceCron(cronExpr string) error {
	updateMu.Lock()
	defer updateMu.Unlock()
	servicesMu.RLock()
	activeScheduler := balanceScheduler
	servicesMu.RUnlock()
	if activeScheduler == nil {
		return nil
	}
	return activeScheduler.UpdateCron(cronExpr)
}

// UpdateLogCleanupSettings hot-reloads the running log-cleanup scheduler.
// It is a no-op before background services have started.
func UpdateLogCleanupSettings(cronExpr string, usageEnabled, programEnabled bool, retentionDays int) error {
	updateMu.Lock()
	defer updateMu.Unlock()
	servicesMu.RLock()
	activeScheduler := logCleanupScheduler
	servicesMu.RUnlock()
	if activeScheduler == nil {
		return nil
	}
	return activeScheduler.UpdateSettings(cronExpr, usageEnabled, programEnabled, retentionDays)
}

// ReloadWebdavBackup hot-reloads the running WebDAV backup scheduler after a
// config save. It is a no-op before background services have started.
func ReloadWebdavBackup() error {
	updateMu.Lock()
	defer updateMu.Unlock()
	servicesMu.RLock()
	activeScheduler := webdavScheduler
	servicesMu.RUnlock()
	if activeScheduler == nil {
		return nil
	}
	return activeScheduler.Reload()
}
