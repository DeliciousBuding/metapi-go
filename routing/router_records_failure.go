package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// RecordFailure records a channel failure and sets cooldown.
func (tr *TokenRouter) RecordFailure(ctx context.Context, channelID int64, failureCtx SiteRuntimeFailureContext, actualAccountID *int64) error {
	if channelID < 0 {
		return tr.recordDirectFailure(ctx, -channelID, failureCtx)
	}
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return err
	}

	row, err := tr.db.LoadChannelWithAccountAndRoute(ctx, channelID)
	if err != nil {
		return err
	}
	if row == nil {
		return fmt.Errorf("routing channel %d not found", channelID)
	}
	if err := validateRecordingAccount(row.Channel, row.Account.ID, actualAccountID); err != nil {
		return err
	}

	ch := row.Channel
	account := row.Account
	route := row.Route
	nowMs := time.Now().UnixMilli()
	nowISO := time.Now().UTC().Format(time.RFC3339)
	reasonCode, reasonSummary := ClassifyCooldownReason(failureCtx, CooldownTriggerTraffic)

	// Handle OAuth route unit member
	if ch.OAuthRouteUnitID != nil && *ch.OAuthRouteUnitID > 0 {
		memberRow, err := tr.recordingMember(ctx, *ch.OAuthRouteUnitID, account.ID, actualAccountID)
		if err != nil {
			return err
		}
		shortWindowCooldown := resolveShortWindowLimitCooldownTS(memberRow.Account, failureCtx, nowMs)
		failCount := max(0, memberRow.Member.FailCountOrZero())
		if shortWindowCooldown == nil {
			failCount++
		} else {
			failCount = 0
		}

		routeUnitStrategy := memberRow.Unit.Strategy
		if routeUnitStrategy == "" {
			routeUnitStrategy = "round_robin"
		}

		var cooldownUntil *string
		// Pass raw consecutiveFailCount; ApplyRoundRobinCooldown alone does +1.
		consecutiveFailCount := max(0, memberRow.Member.ConsecutiveFailCount)
		cooldownLevel := max(0, memberRow.Member.CooldownLevel)

		if shortWindowCooldown != nil {
			cooldownUntil = shortWindowCooldown
			consecutiveFailCount = 0
			cooldownLevel = 0
		} else {
			consecutiveFailCount, cooldownLevel, cooldownUntil = resolveCooldownUpdate(
				routeUnitStrategy == "round_robin",
				consecutiveFailCount, cooldownLevel, failCount, nowMs, tr.configuredMaxSec)
		}

		if err := tr.db.UpdateRouteUnitMemberCooldownFields(ctx, memberRow.Member.ID, map[string]interface{}{
			"failCount":            failCount,
			"lastFailAt":           nowISO,
			"consecutiveFailCount": consecutiveFailCount,
			"cooldownLevel":        cooldownLevel,
			"cooldownUntil":        cooldownUntil,
			"cooldownReasonCode":   reasonCode,
			"cooldownReason":       CooldownReasonSummaryArg(reasonSummary),
			"cooldownReasonAt":     nowISO,
			"updatedAt":            nowISO,
		}); err != nil {
			return err
		}
		RecordSiteRuntimeFailure(memberRow.Account.SiteID, failureCtx)
		tr.cache.InvalidateRouteScopedCache(route.ID)
		return nil
	}

	// Regular channel
	shortWindowCooldown := resolveShortWindowLimitCooldownTS(account, failureCtx, nowMs)
	failCount := max(0, ch.FailCountOrZero())
	if shortWindowCooldown == nil {
		failCount++
	} else {
		failCount = 0
	}

	routeStrategy := NormalizeRouteRoutingStrategy(route.RoutingStrategy)
	var affectedChannelIDs []int64
	if shortWindowCooldown != nil {
		// known limitation: usage-limit short-window cools credential-scoped siblings
		// only (shared-key truth) — intentional multi-channel impact, not cascade bug.
		ids, err := tr.db.LoadCredentialScopedChannelIDs(ctx, ch, account.ID)
		if err != nil {
			return err
		}
		affectedChannelIDs = ids
	} else {
		// Non-usage-limit: channel-scoped only (no credential expand).
		affectedChannelIDs = []int64{channelID}
	}

	var cooldownUntil *string
	// Pass raw consecutiveFailCount; ApplyRoundRobinCooldown alone does +1.
	consecutiveFailCount := max(0, ch.ConsecutiveFailCount)
	cooldownLevel := max(0, ch.CooldownLevel)

	if shortWindowCooldown != nil {
		cooldownUntil = shortWindowCooldown
		consecutiveFailCount = 0
		cooldownLevel = 0
	} else {
		consecutiveFailCount, cooldownLevel, cooldownUntil = resolveCooldownUpdate(
			routeStrategy == StrategyRoundRobin,
			consecutiveFailCount, cooldownLevel, failCount, nowMs, tr.configuredMaxSec)
	}

	// Operator-tunable auto-disable: when the failing status falls in
	// the configured disable ranges (default: none — historical behavior),
	// the channel is disabled outright on top of the cooldown escalation.
	// manual_override shields it from route rebuilds; the operator
	// re-enables through the same surface as manual channel edits. Regular
	// channels only — OAuth route unit members keep their member-level
	// cooldown path.
	disableHit := failureCtx.Status != nil &&
		StatusInRanges(*failureCtx.Status, ActiveDisableStatusRanges())

	updates := map[string]interface{}{
		"failCount":            failCount,
		"lastFailAt":           nowISO,
		"consecutiveFailCount": consecutiveFailCount,
		"cooldownLevel":        cooldownLevel,
		"cooldownUntil":        cooldownUntil,
		"cooldownReasonCode":   reasonCode,
		"cooldownReason":       CooldownReasonSummaryArg(reasonSummary),
		"cooldownReasonAt":     nowISO,
	}
	if disableHit {
		updates["enabled"] = false
		updates["manualOverride"] = true
	}
	if err := tr.db.UpdateChannelCooldownFields(ctx, affectedChannelIDs, updates); err != nil {
		return err
	}

	for _, id := range affectedChannelIDs {
		tr.cache.PatchCachedChannel(id, func(ch *store.RouteChannel) {
			ch.FailCount = &failCount
			ch.LastFailAt = &nowISO
			ch.ConsecutiveFailCount = consecutiveFailCount
			ch.CooldownLevel = cooldownLevel
			ch.CooldownUntil = cooldownUntil
			ch.CooldownReasonCode = &reasonCode
			ch.CooldownReason = reasonSummaryPtr(reasonSummary)
			ch.CooldownReasonAt = &nowISO
			if disableHit {
				ch.Enabled = false
				ch.ManualOverride = true
			}
		})
	}

	RecordSiteRuntimeFailure(account.SiteID, failureCtx)
	return nil
}

// resolveCooldownUpdate computes the next consecutiveFailCount/cooldownLevel/
// cooldownUntil for a failure record. isRoundRobin selects tiered round-robin
// cooldown; otherwise Fibonacci backoff. Shared by RecordFailure and
// RecordProbeFailure (channel and OAuth-unit paths).
func resolveCooldownUpdate(
	isRoundRobin bool,
	consecutiveFailCount int64,
	cooldownLevel int64,
	failCount int64,
	nowMs int64,
	configuredMaxSec int,
) (nextConsecutiveFailCount int64, nextCooldownLevel int64, cooldownUntil *string) {
	if isRoundRobin {
		// Callers pass raw consecutiveFailCount; ApplyRoundRobinCooldown alone does +1.
		nextCF, nextCL, cu := ApplyRoundRobinCooldown(consecutiveFailCount, cooldownLevel, nowMs, configuredMaxSec)
		return nextCF, nextCL, cu
	}
	cu := ApplyFibonacciCooldown(failCount, nowMs, configuredMaxSec)
	return 0, 0, cu
}

// ResolveShortWindowLimitCooldown resolves the short-window limit cooldown for a failure.
func resolveShortWindowLimitCooldownTS(account store.Account, ctx SiteRuntimeFailureContext, nowMs int64) *string {
	status := 0
	if ctx.Status != nil {
		status = *ctx.Status
	}
	errorText := ""
	if ctx.ErrorText != nil {
		errorText = *ctx.ErrorText
	}
	if !IsUsageLimitRateLimitFailure(SiteRuntimeFailureContext{Status: &status, ErrorText: &errorText}) {
		return nil
	}

	// Check for quota reset hint in error text
	// Simple check — real implementation would parse structured hints
	// For now, default to 5 minute cooldown
	untilMs := nowMs + ShortWindowLimitCooldownMs

	iso := formatUnixMillisISO(untilMs)
	return &iso
}
