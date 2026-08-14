package scheduler

import (
	"github.com/deliciousbuding/metapi-go/config"
)

// ProxyFileRetentionScheduler periodically prunes expired proxy files.
// Implemented by the shared RetentionScheduler.
type ProxyFileRetentionScheduler = RetentionScheduler

// NewProxyFileRetentionScheduler creates a new proxy file retention scheduler.
func NewProxyFileRetentionScheduler(cfg *config.Config) *RetentionScheduler {
	return NewRetentionScheduler(cfg, RetentionSchedulerOptions{
		Name:               "proxy-file-retention",
		Table:              "proxy_files",
		ExtraWhere:         " AND deleted_at IS NULL",
		DefaultIntervalMin: 60,
		RetentionDaysFn:    func(c *config.Config) int { return c.ProxyFileRetentionDays },
		IntervalMinFn:      func(c *config.Config) int { return c.ProxyFileRetentionPruneIntervalMinutes },
		DisabledFn: func(c *config.Config) (bool, string) {
			if c.ProxyFileRetentionDays <= 0 {
				return true, "retention_days=0"
			}
			return false, ""
		},
	})
}
