package scheduler

import (
	"github.com/deliciousbuding/metapi-go/config"
)

// ProxyVideoTaskRetentionScheduler periodically prunes aged proxy_video_tasks rows.
// Disabled when ProxyVideoTaskRetentionDays <= 0 (default).
// Implemented by the shared RetentionScheduler.
type ProxyVideoTaskRetentionScheduler = RetentionScheduler

// NewProxyVideoTaskRetentionScheduler creates a video task retention scheduler.
func NewProxyVideoTaskRetentionScheduler(cfg *config.Config) *RetentionScheduler {
	return NewRetentionScheduler(cfg, RetentionSchedulerOptions{
		Name:               "proxy-video-task-retention",
		Table:              "proxy_video_tasks",
		UseUTC:             true,
		DefaultIntervalMin: 60,
		RetentionDaysFn:    func(c *config.Config) int { return c.ProxyVideoTaskRetentionDays },
		IntervalMinFn:      func(c *config.Config) int { return c.ProxyVideoTaskRetentionPruneIntervalMinutes },
		DisabledFn: func(c *config.Config) (bool, string) {
			if c.ProxyVideoTaskRetentionDays <= 0 {
				return true, "retention_days=0"
			}
			return false, ""
		},
	})
}
