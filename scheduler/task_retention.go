package scheduler

import (
	"github.com/deliciousbuding/metapi-go/config"
)

// adminBackgroundTaskRetentionDays is the fixed retention window for
// admin_background_tasks. Only terminal rows (succeeded/failed) older than
// this window are pruned; pending/running rows are never deleted so in-flight
// tasks remain visible to operators and cross-process task listings.
const adminBackgroundTaskRetentionDays = 30

// adminBackgroundTaskPruneIntervalMin is the prune cadence. A daily run keeps
// the table bounded while never threatening a 30-day retention window: a row
// is eligible for at most ~31 days (retention + up to one prune interval).
const adminBackgroundTaskPruneIntervalMin = 24 * 60

// AdminBackgroundTaskRetentionScheduler periodically prunes terminal
// admin_background_tasks rows older than a 30-day window. Without this job the
// table grows unboundedly (no DELETE path existed), forcing full-scan + sort
// on every GET /api/tasks list. Implemented by the shared RetentionScheduler.
type AdminBackgroundTaskRetentionScheduler = RetentionScheduler

// NewAdminBackgroundTaskRetentionScheduler creates the admin background task
// retention scheduler. It deletes only rows whose status is 'succeeded' or
// 'failed' and whose created_at is older than the 30-day cutoff, so pending
// and running tasks are always retained.
func NewAdminBackgroundTaskRetentionScheduler(cfg *config.Config) *RetentionScheduler {
	return NewRetentionScheduler(cfg, RetentionSchedulerOptions{
		Name:               "admin-background-task-retention",
		Table:              "admin_background_tasks",
		ExtraWhere:         " AND status IN ('succeeded', 'failed')",
		UseUTC:             true,
		DefaultIntervalMin: adminBackgroundTaskPruneIntervalMin,
		RetentionDaysFn:    func(*config.Config) int { return adminBackgroundTaskRetentionDays },
		IntervalMinFn:      func(*config.Config) int { return adminBackgroundTaskPruneIntervalMin },
		DisabledFn:         func(*config.Config) (bool, string) { return false, "" },
	})
}
