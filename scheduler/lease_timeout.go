package scheduler

import "time"

// Per-job lease hold deadlines. Each scheduler pass that acquires the
// cluster-wide advisory lease is bounded by one of these timeouts so a
// pathological pass over N accounts cannot block every other instance
// indefinitely. The deadline bounds the lease context (acquire + release);
// service-layer I/O inside the job uses its own HTTP timeouts. When a job
// runs past its deadline, runWithSchedulerLease logs and the lease is
// released with context.Background() so the advisory-lock unlock never
// fails due to a deadline-exceeded context.
const (
	// Heavy fan-out passes over many accounts/sites.
	balanceJobTimeout      = 15 * time.Minute
	checkinJobTimeout      = 15 * time.Minute
	dailySummaryJobTimeout = 15 * time.Minute
	retentionJobTimeout    = 15 * time.Minute
	logCleanupJobTimeout   = 15 * time.Minute
	backupWebdavJobTimeout = 15 * time.Minute
	// model-sync walks every candidate account sequentially with a 30s
	// upstream budget each (#1005), so it gets the largest lease budget.
	modelSyncJobTimeout = 30 * time.Minute

	// Lightweight probe/refresh sweeps.
	channelRecoveryJobTimeout  = 5 * time.Minute
	modelProbeJobTimeout       = 5 * time.Minute
	oauthRefreshJobTimeout     = 5 * time.Minute
	siteAnnouncementJobTimeout = 5 * time.Minute
	sub2apiRefreshJobTimeout   = 5 * time.Minute
)
