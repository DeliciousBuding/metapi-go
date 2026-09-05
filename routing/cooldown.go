package routing

import (
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// ---- Cooldown constants ----

const (
	FailureBackoffBaseSec      = 15
	ShortWindowLimitCooldownMs = 5 * 60 * 1000
	MaxFailureBackoffSec       = 30 * 24 * 60 * 60 // 30 days
	RoundRobinFailureThreshold = 3
)

// RoundRobinCooldownLevelsSec defines tiered cooldown for round-robin: [0s, 10min, 1h, 24h].
var RoundRobinCooldownLevelsSec = []int64{0, 10 * 60, 60 * 60, 24 * 60 * 60}

// FibonacciNumber returns the nth Fibonacci number (1-indexed: fib(1)=1, fib(2)=1, fib(3)=2,...).
func FibonacciNumber(index int64) int64 {
	if index <= 2 {
		return 1
	}
	var prev, current int64 = 1, 1
	for i := int64(3); i <= index; i++ {
		next := prev + current
		prev = current
		current = next
	}
	return current
}

// ResolveFailureBackoffSec computes Fibonacci backoff: 15 * fib(failCount).
// failCount=1 → 15s, failCount=2 → 15s, failCount=3 → 30s, etc.
func ResolveFailureBackoffSec(failCount *int64) int64 {
	normalized := int64(1)
	if failCount != nil && *failCount > 1 {
		normalized = *failCount
	}
	fib := FibonacciNumber(normalized)
	return min(FailureBackoffBaseSec*fib, MaxFailureBackoffSec)
}

// ResolveConfiguredFailureCooldownMaxMs returns the configured max cooldown in ms.
func ResolveConfiguredFailureCooldownMaxMs(configuredMaxSec int) int64 {
	if configuredMaxSec <= 0 {
		configuredMaxSec = config.TokenRouterFailureCooldownMaxSecCeiling
	}
	normalized := int64(configuredMaxSec) * 1000
	if normalized < 1000 {
		normalized = 1000
	}
	return normalized
}

// ClampFailureCooldownMs clamps a cooldown duration to the configured maximum.
func ClampFailureCooldownMs(cooldownMs int64, configuredMaxSec int) int64 {
	if cooldownMs < 0 {
		cooldownMs = 0
	}
	maxMs := ResolveConfiguredFailureCooldownMaxMs(configuredMaxSec)
	if cooldownMs > maxMs {
		cooldownMs = maxMs
	}
	return cooldownMs
}

// ResolveEffectiveFailureCooldownMs computes the full effective cooldown with clamping.
func ResolveEffectiveFailureCooldownMs(failCount *int64, configuredMaxSec int) int64 {
	backoffSec := ResolveFailureBackoffSec(failCount)
	return ClampFailureCooldownMs(backoffSec*1000, configuredMaxSec)
}

// ResolveRoundRobinCooldownSec returns the cooldown duration for a given level.
func ResolveRoundRobinCooldownSec(level int) int64 {
	normalized := level
	if normalized < 0 {
		normalized = 0
	}
	if normalized >= len(RoundRobinCooldownLevelsSec) {
		normalized = len(RoundRobinCooldownLevelsSec) - 1
	}
	return RoundRobinCooldownLevelsSec[normalized]
}

// IsChannelRecentlyFailed checks if a channel failed recently within the backoff window.
func IsChannelRecentlyFailed(failCount *int64, lastFailAt *string, nowMs int64, configuredMaxSec int) bool {
	if failCount == nil || *failCount <= 0 {
		return false
	}
	if lastFailAt == nil || *lastFailAt == "" {
		return false
	}

	failTs, err := time.Parse(time.RFC3339, *lastFailAt)
	if err != nil {
		// Try parsing as ISO 8601 with space separator
		failTs, err = time.Parse("2006-01-02 15:04:05", *lastFailAt)
		if err != nil {
			return false
		}
	}

	avoidSec := ResolveFailureBackoffSec(failCount)
	avoidMs := ClampFailureCooldownMs(avoidSec*1000, configuredMaxSec)
	if avoidMs <= 0 {
		return false
	}

	return nowMs-failTs.UnixMilli() < avoidMs
}

// FilterRecentlyFailedCandidates filters a list of candidates to those not recently failed.
// If all are recently failed, returns all candidates.
func FilterRecentlyFailedCandidates[T any](
	candidates []T,
	getFailInfo func(T) (failCount *int64, lastFailAt *string),
	nowMs int64,
	configuredMaxSec int,
) []T {
	if len(candidates) <= 1 {
		return candidates
	}
	healthy := make([]T, 0, len(candidates))
	for _, candidate := range candidates {
		fc, lfa := getFailInfo(candidate)
		if !IsChannelRecentlyFailed(fc, lfa, nowMs, configuredMaxSec) {
			healthy = append(healthy, candidate)
		}
	}
	if len(healthy) > 0 {
		return healthy
	}
	return candidates
}

// ParseISOTimeMs parses an ISO timestamp to Unix milliseconds.
func ParseISOTimeMs(value *string) *int64 {
	if value == nil || *value == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", *value)
		if err != nil {
			return nil
		}
	}
	ms := t.UnixMilli()
	return &ms
}

// CompareNullableTimeAsc compares two nullable ISO timestamps ascending.
func CompareNullableTimeAsc(left, right *string) int {
	lm := ParseISOTimeMs(left)
	rm := ParseISOTimeMs(right)
	if lm == nil && rm == nil {
		return 0
	}
	if lm == nil {
		return -1
	}
	if rm == nil {
		return 1
	}
	if *lm < *rm {
		return -1
	}
	if *lm > *rm {
		return 1
	}
	return 0
}

// CompareNullableTimeDesc compares two nullable ISO timestamps descending.
func CompareNullableTimeDesc(left, right *string) int {
	return CompareNullableTimeAsc(right, left)
}

// IsCooldownActive reports whether cooldownUntil is still in the future relative to nowISO.
// Both timestamps are parsed to milliseconds before comparison so millis-bearing writer
// formats (formatUnixMillisISO) cannot lose to second-precision RFC3339 nowISO via lexical order.
// Example bug: "…T15:04:05.500Z" > "…T15:04:05Z" is false as strings ('.' < 'Z') but true by time.
func IsCooldownActive(cooldownUntil *string, nowISO string) bool {
	if cooldownUntil == nil || *cooldownUntil == "" {
		return false
	}
	untilMs := ParseISOTimeMs(cooldownUntil)
	if untilMs == nil {
		return false
	}
	now := nowISO
	nowMs := ParseISOTimeMs(&now)
	if nowMs == nil {
		return false
	}
	return *untilMs > *nowMs
}

// ClampNumber clamps a float64 to [lo, hi].
func ClampNumber(value, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, value))
}

// IsContributionCloseToBest checks if a value is within ratio of best.
func IsContributionCloseToBest(value, bestValue, ratio float64) bool {
	if bestValue <= 0 {
		return true
	}
	return value >= (bestValue * ratio)
}

// isFiniteFloat checks if a float64 is finite.
func isFiniteFloat(v float64) bool {
	return !math.IsInf(v, 0) && !math.IsNaN(v)
}
