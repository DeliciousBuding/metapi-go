package scheduler

import (
	"github.com/deliciousbuding/metapi-go/config"
)

// ProxyLogRetentionScheduler is a legacy fallback for cleaning up proxy logs
// when the main log_cleanup system is not configured (logCleanupConfigured=false).
// Implemented by the shared RetentionScheduler.
type ProxyLogRetentionScheduler = RetentionScheduler

// NewProxyLogRetentionScheduler creates a new proxy log retention scheduler.
func NewProxyLogRetentionScheduler(cfg *config.Config) *RetentionScheduler {
	return NewRetentionScheduler(cfg, RetentionSchedulerOptions{
		Name:              "proxy-log-retention",
		Table:             "proxy_logs",
		DefaultIntervalMin: 30,
		RetentionDaysFn:   func(c *config.Config) int { return c.ProxyLogRetentionDays },
		IntervalMinFn:     func(c *config.Config) int { return c.ProxyLogRetentionPruneIntervalMinutes },
		DisabledFn: func(c *config.Config) (bool, string) {
			if c.LogCleanupConfigured {
				return true, "log_cleanup configured"
			}
			if c.ProxyLogRetentionDays <= 0 {
				return true, "retention_days=0"
			}
			return false, ""
		},
		StartLogSuffix: " (legacy fallback)",
	})
}
