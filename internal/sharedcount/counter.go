package sharedcount

import (
	"context"
	"time"
)

// WindowCounter increments a named key inside a sliding/fixed window and returns
// the post-increment count. Used for multi-instance RPM/TPM admission.
type WindowCounter interface {
	// Incr increments key by 1 within window and returns the new count.
	// Implementations may approximate sliding windows with fixed TTL buckets.
	Incr(ctx context.Context, key string, window time.Duration) (count int64, err error)
	// Decr decrements key by 1 (compensating rollback for a prior Incr,).
	// Does not extend/refresh TTL. Count is floored at 0 for memory; Redis DECR
	// may go negative, and a rollback that lands after the window expired drops
	// the re-created non-positive key instead of leaving it behind without a TTL.
	Decr(ctx context.Context, key string, window time.Duration) (count int64, err error)
	// IncrBy increments key by delta within window and returns the new total.
	// Used for TPM token reservations. delta==0 returns the current total.
	// Negative delta is a compensating rollback and does not refresh TTL; like
	// Decr it removes the key when the rollback takes the total to <= 0.
	IncrBy(ctx context.Context, key string, delta int64, window time.Duration) (count int64, err error)
	// Get returns the current count without incrementing.
	Get(ctx context.Context, key string) (count int64, err error)
}
