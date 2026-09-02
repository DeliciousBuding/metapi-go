package oauth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MaxRefreshRetries caps how many times a transiently failing token refresh
// is retried before giving up. Aligned with codex2api's MaxRetries constant.
const MaxRefreshRetries = 3

// retryBackoffFn is the backoff source used by RefreshWithRetry. It is a
// package-level var (not a const) so tests can swap in a no-op sleeper to
// keep the suite fast without short-circuiting real retry behavior.
var retryBackoffFn = RetryBackoff

// nonRetryableNeedles lists OAuth RFC 6749 error codes that signal a
// permanent failure for the current credential — retrying cannot fix them.
// Mirrors codex2api's isNonRetryable set so that a revoked refresh token or
// a denied client does not burn the full retry budget.
var nonRetryableNeedles = []string{
	"invalid_grant",
	"invalid_client",
	"unauthorized_client",
	"access_denied",
	"refresh_token_reused",
}

// RetryBackoff returns the wait before the (attempt+1)-th retry. The first
// retry (attempt=0) waits 1s, then 2s, then 4s — exponential with a 2x step.
// attempt < 0 returns 0; attempt >= MaxRefreshRetries returns 0 so callers
// can use it to detect end-of-budget without a separate bounds check.
func RetryBackoff(attempt int) time.Duration {
	if attempt < 0 || attempt >= MaxRefreshRetries {
		return 0
	}
	return time.Duration(1<<uint(attempt)) * time.Second
}

// IsNonRetryable reports whether err carries one of the OAuth error codes
// that cannot be fixed by retrying. The check is case-insensitive and matches
// substrings because provider implementations surface these codes via
// fmt.Errorf with mixed surrounding context (HTTP body, status, etc.).
func IsNonRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range nonRetryableNeedles {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// RefreshWithRetry invokes fn up to MaxRefreshRetries+1 times, applying
// RetryBackoff between attempts. A nil error short-circuits immediately. An
// IsNonRetryable error short-circuits immediately (no point burning budget
// on a revoked grant). ctx cancellation during the backoff window aborts the
// retry loop with ctx.Err().
//
// fn is invoked at least once even when ctx is already cancelled, matching
// the semantics of the existing singleflight refresh path which prefers one
// real attempt over a zero-attempt failure.
func RefreshWithRetry[T any](ctx context.Context, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error
	for attempt := 0; attempt <= MaxRefreshRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, retryBackoffFn(attempt-1)); err != nil {
				return zero, err
			}
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}
		if IsNonRetryable(err) {
			return zero, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("refresh failed after %d attempts", MaxRefreshRetries+1)
	}
	return zero, lastErr
}

// sleepCtx waits for d and returns nil, or returns ctx.Err() as soon as ctx ends
// (an already-finished ctx returns immediately instead of racing the timer).
// Every wait between two upstream calls in this package goes through it, so a
// cancelled exchange or refresh stops at the wait instead of paying the full
// interval plus the next round trip. The duration semantics are unchanged: only
// the wait became interruptible.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
