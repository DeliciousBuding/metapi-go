// Package channellimit owns process-local direct-channel admission and load.
// Routing reads snapshots; the proxy acquires/releases permits and reports
// measured usage. Neither consumer owns a second counter.
package channellimit

import (
	"fmt"
	"math"
	"time"
)

// Config uses zero for an unlimited dimension. Queue settings only apply when
// MaxConcurrent is positive. QueueSize zero means an unbounded FIFO queue.
type Config struct {
	RPM            int64 `json:"rpm,omitempty"`
	TPM            int64 `json:"tpm,omitempty"`
	MaxConcurrent  int64 `json:"maxConcurrent,omitempty"`
	QueueSize      int64 `json:"queueSize,omitempty"`
	QueueTimeoutMs int64 `json:"queueTimeoutMs,omitempty"`
}

func (c Config) Enabled() bool {
	return c.RPM > 0 || c.TPM > 0 || c.MaxConcurrent > 0
}

func (c Config) Validate() error {
	if c.RPM < 0 || c.TPM < 0 || c.MaxConcurrent < 0 || c.QueueSize < 0 || c.QueueTimeoutMs < 0 {
		return fmt.Errorf("channel rate limits must be non-negative")
	}
	if c.QueueTimeoutMs > math.MaxInt64/int64(time.Millisecond) {
		return fmt.Errorf("channel queue timeout exceeds the supported duration")
	}
	return nil
}
