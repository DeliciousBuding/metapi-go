package proxyhandler

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// upstreamRetryBudget bounds consecutive response-header waits of a
// conversation request. A response (including 401) completes that wait; it
// does not consume another fallback's header window. The deadline is never
// attached to a body. Media polling/downloads retain their own budgets.
type upstreamRetryBudget struct {
	mu       sync.Mutex
	deadline time.Time
}

func (c *Ctx) startRetryBudget(timeout time.Duration) {
	if c == nil || timeout <= 0 || c.retryBudget == nil || proxy.IsDirectMediaPath(c.DownstreamPath) {
		return
	}
	c.retryBudget.mu.Lock()
	defer c.retryBudget.mu.Unlock()
	if c.retryBudget.deadline.IsZero() {
		c.retryBudget.deadline = time.Now().Add(timeout)
	}
}

func (c *Ctx) armRetryBudget() {
	if c == nil || c.retryBudget == nil {
		return
	}
	c.retryBudget.mu.Lock()
	defer c.retryBudget.mu.Unlock()
	c.retryBudget.deadline = time.Time{}
}

func (c *Ctx) remaining(configured int64) int64 {
	if c == nil || configured <= 0 || c.retryBudget == nil {
		return configured
	}
	c.retryBudget.mu.Lock()
	defer c.retryBudget.mu.Unlock()
	if c.retryBudget.deadline.IsZero() {
		return configured
	}
	remaining := time.Until(c.retryBudget.deadline)
	if remaining <= 0 {
		return 1
	}
	ms := int64(math.Ceil(float64(remaining) / float64(time.Millisecond)))
	if ms < configured {
		return ms
	}
	return configured
}

// retryBudgetExpired stops selection before another stalled attempt. A real
// pending failure is still returned, rather than replaced with a generic 503.
func (c *Ctx) retryBudgetExpired() bool {
	if c == nil || c.retryBudget == nil {
		return false
	}
	c.retryBudget.mu.Lock()
	defer c.retryBudget.mu.Unlock()
	return !c.retryBudget.deadline.IsZero() && !time.Now().Before(c.retryBudget.deadline)
}

// expiredBeforeHeaders distinguishes the configured first-byte window from a
// caller cancellation. Buffered clients can report the same deadline through
// client.Timeout instead of the stream header timer's sentinel error.
func (c *Ctx) expiredBeforeHeaders(err error, configured int64) bool {
	if configured <= 0 || c == nil || c.retryBudget == nil || !errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	c.retryBudget.mu.Lock()
	defer c.retryBudget.mu.Unlock()
	if c.retryBudget.deadline.IsZero() || time.Until(c.retryBudget.deadline) > 0 {
		return false
	}
	return time.Since(c.retryBudget.deadline) < time.Second
}
