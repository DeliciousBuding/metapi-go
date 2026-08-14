package routing

import (
	"sort"
	"time"
)

// GetRoundRobinCandidates sorts candidates by lastSelectedAt || lastUsedAt ascending.
func GetRoundRobinCandidates(candidates []RouteChannelCandidate) []RouteChannelCandidate {
	sorted := make([]RouteChannelCandidate, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := sorted[i]
		right := sorted[j]
		lo := left.Channel.LastSelectedAt
		if lo == nil {
			lo = left.Channel.LastUsedAt
		}
		ro := right.Channel.LastSelectedAt
		if ro == nil {
			ro = right.Channel.LastUsedAt
		}
		cmp := CompareNullableTimeAsc(lo, ro)
		if cmp != 0 {
			return cmp < 0
		}
		cmp = CompareNullableTimeAsc(left.Channel.LastUsedAt, right.Channel.LastUsedAt)
		if cmp != 0 {
			return cmp < 0
		}
		return left.Channel.ID < right.Channel.ID
	})
	return sorted
}

// SelectRoundRobinCandidate picks the first candidate in round-robin order.
func SelectRoundRobinCandidate(candidates []RouteChannelCandidate) *RouteChannelCandidate {
	ordered := GetRoundRobinCandidates(candidates)
	if len(ordered) == 0 {
		return nil
	}
	return &ordered[0]
}

// ApplyRoundRobinCooldown applies tiered cooldown to a failure-aware struct.
// Callers must pass the raw consecutiveFailCount (no pre-increment); this helper alone does +1.
// If the post-increment count reaches RoundRobinFailureThreshold, increments cooldownLevel and resets count.
// Returns the updated values and cooldownUntil ISO string.
func ApplyRoundRobinCooldown(
	consecutiveFailCount int64,
	cooldownLevel int64,
	nowMs int64,
	configuredMaxSec int,
) (nextConsecutiveFailCount int64, nextCooldownLevel int64, cooldownUntilISO *string) {
	nextConsecutiveFailCount = consecutiveFailCount + 1
	nextCooldownLevel = cooldownLevel

	if nextConsecutiveFailCount >= RoundRobinFailureThreshold {
		nextCooldownLevel = min(cooldownLevel+1, int64(len(RoundRobinCooldownLevelsSec)-1))
		cooldownSec := ResolveRoundRobinCooldownSec(int(nextCooldownLevel))
		if cooldownSec > 0 {
			untilMs := nowMs + ClampFailureCooldownMs(cooldownSec*1000, configuredMaxSec)
			iso := formatUnixMillisISO(untilMs)
			cooldownUntilISO = &iso
		}
		nextConsecutiveFailCount = 0
	}
	return
}

// ApplyFibonacciCooldown applies Fibonacci backoff cooldown.
func ApplyFibonacciCooldown(failCount int64, nowMs int64, configuredMaxSec int) (cooldownUntilISO *string) {
	fc := failCount
	effectiveMs := ResolveEffectiveFailureCooldownMs(&fc, configuredMaxSec)
	untilMs := nowMs + effectiveMs
	iso := formatUnixMillisISO(untilMs)
	return &iso
}

// formatUnixMillisISO formats Unix milliseconds as ISO 8601 with always-present
// milliseconds ("2006-01-02T15:04:05.000Z") so millis-precision values cannot
// lose to second-precision RFC3339 strings in lexical order.
func formatUnixMillisISO(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
}
