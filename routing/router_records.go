package routing

import (
	"context"
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Success / failure recording ----

// RecordSuccess records a successful channel usage.
func (tr *TokenRouter) RecordSuccess(ctx context.Context, channelID int64, latencyMs float64, cost float64, modelName *string, actualAccountID *int64) error {
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return err
	}

	row, err := tr.db.LoadChannelWithAccount(ctx, channelID)
	if err != nil || row == nil {
		return err
	}

	ch := row.Channel
	account := row.Account
	nowISO := time.Now().UTC().Format(time.RFC3339)
	nextSuccessCount := max(0, ch.SuccessCount) + 1
	nextTotalLatencyMs := max(0, ch.TotalLatencyMs) + int64(latencyMs)
	nextTotalCost := math.Max(0, ch.TotalCost) + cost

	if ch.OAuthRouteUnitID != nil && *ch.OAuthRouteUnitID > 0 {
		targetAccountID := account.ID
		if actualAccountID != nil && *actualAccountID > 0 {
			targetAccountID = *actualAccountID
		}

		memberRow, err := tr.db.LoadRouteUnitMemberWithAccount(ctx, *ch.OAuthRouteUnitID, targetAccountID)
		if err == nil && memberRow != nil {
			memberSuccessCount := max(0, memberRow.Member.SuccessCount) + 1
			memberTotalLatencyMs := max(0, memberRow.Member.TotalLatencyMs) + int64(latencyMs)
			memberTotalCost := math.Max(0, memberRow.Member.TotalCost) + cost
			_ = tr.db.UpdateRouteUnitMemberSuccessFields(ctx, memberRow.Member.ID, map[string]interface{}{
				"successCount":   memberSuccessCount,
				"totalLatencyMs": memberTotalLatencyMs,
				"totalCost":      memberTotalCost,
				"lastUsedAt":     nowISO,
				"cooldownUntil":  nil,
				"lastFailAt":     nil,
				"consecutiveFailCount": int64(0),
				"cooldownLevel":  int64(0),
				"updatedAt":      nowISO,
			})
			RecordSiteRuntimeSuccess(memberRow.Account.SiteID, latencyMs, modelName)
		} else {
			RecordSiteRuntimeSuccess(account.SiteID, latencyMs, modelName)
		}
		tr.cache.InvalidateRouteScopedCache(ch.RouteID)
	} else {
		RecordSiteRuntimeSuccess(account.SiteID, latencyMs, modelName)
	}

	_ = tr.db.UpdateChannelSuccessFields(ctx, channelID, map[string]interface{}{
		"successCount":   nextSuccessCount,
		"totalLatencyMs": nextTotalLatencyMs,
		"totalCost":      nextTotalCost,
		"lastUsedAt":     nowISO,
		"cooldownUntil":  nil,
		"lastFailAt":     nil,
		"consecutiveFailCount": int64(0),
		"cooldownLevel":  int64(0),
	})

	tr.cache.PatchCachedChannel(channelID, func(ch *store.RouteChannel) {
		ch.SuccessCount = nextSuccessCount
		ch.TotalLatencyMs = nextTotalLatencyMs
		ch.TotalCost = nextTotalCost
		ch.LastUsedAt = &nowISO
		ch.CooldownUntil = nil
		ch.LastFailAt = nil
		ch.ConsecutiveFailCount = 0
		ch.CooldownLevel = 0
	})

	return nil
}

// RecordProbeSuccess records a successful background health probe.
// It clears cooldown for credential-scoped channels, feeds runtime health
// success, and stamps last probe status. It never marks credentials expired.
func (tr *TokenRouter) RecordProbeSuccess(ctx context.Context, channelID int64, latencyMs float64, modelName *string, actualAccountID *int64) error {
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return err
	}

	row, err := tr.db.LoadChannelWithAccount(ctx, channelID)
	if err != nil || row == nil {
		return err
	}

	ch := row.Channel
	account := row.Account
	nowISO := time.Now().UTC().Format(time.RFC3339)
	channelIDCopy := channelID

	if ch.OAuthRouteUnitID != nil && *ch.OAuthRouteUnitID > 0 {
		targetAccountID := account.ID
		if actualAccountID != nil && *actualAccountID > 0 {
			targetAccountID = *actualAccountID
		}

		memberRow, err := tr.db.LoadRouteUnitMemberWithAccount(ctx, *ch.OAuthRouteUnitID, targetAccountID)
		if err == nil && memberRow != nil {
			_ = tr.db.UpdateRouteUnitMemberCooldownFields(ctx, memberRow.Member.ID, map[string]interface{}{
				"cooldownUntil":        nil,
				"lastFailAt":           nil,
				"consecutiveFailCount": int64(0),
				"cooldownLevel":        int64(0),
				"updatedAt":            nowISO,
			})
			RecordSiteRuntimeSuccess(memberRow.Account.SiteID, latencyMs, modelName)
			RecordSiteProbeOutcome(memberRow.Account.SiteID, "success", latencyMs, modelName, &channelIDCopy, nil)
		} else {
			RecordSiteRuntimeSuccess(account.SiteID, latencyMs, modelName)
			RecordSiteProbeOutcome(account.SiteID, "success", latencyMs, modelName, &channelIDCopy, nil)
		}

		_ = tr.db.UpdateChannelCooldownFields(ctx, []int64{channelID}, map[string]interface{}{
			"cooldownUntil":        nil,
			"lastFailAt":           nil,
			"consecutiveFailCount": int64(0),
			"cooldownLevel":        int64(0),
		})
		tr.cache.PatchCachedChannel(channelID, func(ch *store.RouteChannel) {
			ch.CooldownUntil = nil
			ch.LastFailAt = nil
			ch.ConsecutiveFailCount = 0
			ch.CooldownLevel = 0
		})
		tr.cache.InvalidateRouteScopedCache(ch.RouteID)
		return nil
	}

	affectedChannelIDs, err := tr.db.LoadCredentialScopedChannelIDs(ctx, ch, account.ID)
	if err != nil {
		return err
	}

	needsReset := ch.CooldownUntil != nil || ch.LastFailAt != nil || ch.ConsecutiveFailCount > 0 || ch.CooldownLevel > 0
	if needsReset {
		_ = tr.db.UpdateChannelCooldownFields(ctx, affectedChannelIDs, map[string]interface{}{
			"cooldownUntil":        nil,
			"lastFailAt":           nil,
			"consecutiveFailCount": int64(0),
			"cooldownLevel":        int64(0),
		})
		for _, id := range affectedChannelIDs {
			tr.cache.PatchCachedChannel(id, func(ch *store.RouteChannel) {
				ch.CooldownUntil = nil
				ch.LastFailAt = nil
				ch.ConsecutiveFailCount = 0
				ch.CooldownLevel = 0
			})
		}
	}

	RecordSiteRuntimeSuccess(account.SiteID, latencyMs, modelName)
	RecordSiteProbeOutcome(account.SiteID, "success", latencyMs, modelName, &channelIDCopy, nil)
	return nil
}

// RecordProbeFailure records a failed background health probe.

// Unlike RecordFailure (user traffic path), probe failures:
// - update site/model runtime health + breaker streak (so routing can avoid dead channels)
// - apply a short channel cooldown only for the probed channel (no credential cascade)
// - never mark accounts/keys expired
// - never treat auth-looking probe errors as credential expiry
func (tr *TokenRouter) RecordProbeFailure(ctx context.Context, channelID int64, failureCtx SiteRuntimeFailureContext, actualAccountID *int64) error {
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return err
	}

	row, err := tr.db.LoadChannelWithAccountAndRoute(ctx, channelID)
	if err != nil || row == nil {
		return err
	}

	ch := row.Channel
	account := row.Account
	route := row.Route
	nowMs := time.Now().UnixMilli()
	nowISO := time.Now().UTC().Format(time.RFC3339)
	channelIDCopy := channelID

	// Normalize model onto failure context for runtime health model-level state.
	if failureCtx.ModelName == nil || *failureCtx.ModelName == "" {
		if ch.SourceModel != nil && *ch.SourceModel != "" {
			failureCtx.ModelName = ch.SourceModel
		}
	}

	// OAuth unit: update member cooldown only; outer channel fields stay clean.
	if ch.OAuthRouteUnitID != nil && *ch.OAuthRouteUnitID > 0 {
		targetAccountID := account.ID
		if actualAccountID != nil && *actualAccountID > 0 {
			targetAccountID = *actualAccountID
		}

		memberRow, err := tr.db.LoadRouteUnitMemberWithAccount(ctx, *ch.OAuthRouteUnitID, targetAccountID)
		if err == nil && memberRow != nil {
			// Probe path intentionally ignores usage-limit credential cascade and
			// never zeros failCount for short-window limits: probes are synthetic.
			failCount := max(0, memberRow.Member.FailCount) + 1
			routeUnitStrategy := memberRow.Unit.Strategy
			if routeUnitStrategy == "" {
				routeUnitStrategy = "round_robin"
			}

			var cooldownUntil *string
			// Pass raw consecutiveFailCount; ApplyRoundRobinCooldown alone does +1.
			consecutiveFailCount := max(0, memberRow.Member.ConsecutiveFailCount)
			cooldownLevel := max(0, memberRow.Member.CooldownLevel)

			consecutiveFailCount, cooldownLevel, cooldownUntil = resolveCooldownUpdate(
				routeUnitStrategy == "round_robin",
				consecutiveFailCount, cooldownLevel, failCount, nowMs, tr.configuredMaxSec)

			_ = tr.db.UpdateRouteUnitMemberCooldownFields(ctx, memberRow.Member.ID, map[string]interface{}{
				"failCount":            failCount,
				"lastFailAt":           nowISO,
				"consecutiveFailCount": consecutiveFailCount,
				"cooldownLevel":        cooldownLevel,
				"cooldownUntil":        cooldownUntil,
				"updatedAt":            nowISO,
			})
			RecordSiteRuntimeFailure(memberRow.Account.SiteID, failureCtx)
			RecordSiteProbeOutcome(memberRow.Account.SiteID, "failure", 0, failureCtx.ModelName, &channelIDCopy, failureCtx.ErrorText)
			tr.cache.InvalidateRouteScopedCache(route.ID)
		}
		return nil
	}

	// Regular channel: probe failure cools only the probed channel (no credential cascade).
	failCount := max(0, ch.FailCount) + 1
	routeStrategy := NormalizeRouteRoutingStrategy(route.RoutingStrategy)
	affectedChannelIDs := []int64{channelID}

	var cooldownUntil *string
	// Pass raw consecutiveFailCount; ApplyRoundRobinCooldown alone does +1.
	consecutiveFailCount := max(0, ch.ConsecutiveFailCount)
	cooldownLevel := max(0, ch.CooldownLevel)

	consecutiveFailCount, cooldownLevel, cooldownUntil = resolveCooldownUpdate(
		routeStrategy == StrategyRoundRobin,
		consecutiveFailCount, cooldownLevel, failCount, nowMs, tr.configuredMaxSec)

	_ = tr.db.UpdateChannelCooldownFields(ctx, affectedChannelIDs, map[string]interface{}{
		"failCount":            failCount,
		"lastFailAt":           nowISO,
		"consecutiveFailCount": consecutiveFailCount,
		"cooldownLevel":        cooldownLevel,
		"cooldownUntil":        cooldownUntil,
	})

	for _, id := range affectedChannelIDs {
		tr.cache.PatchCachedChannel(id, func(ch *store.RouteChannel) {
			ch.FailCount = failCount
			ch.LastFailAt = &nowISO
			ch.ConsecutiveFailCount = consecutiveFailCount
			ch.CooldownLevel = cooldownLevel
			ch.CooldownUntil = cooldownUntil
		})
	}

	RecordSiteRuntimeFailure(account.SiteID, failureCtx)
	RecordSiteProbeOutcome(account.SiteID, "failure", 0, failureCtx.ModelName, &channelIDCopy, failureCtx.ErrorText)
	return nil
}

// RecordFailure records a channel failure and sets cooldown.
func (tr *TokenRouter) RecordFailure(ctx context.Context, channelID int64, failureCtx SiteRuntimeFailureContext, actualAccountID *int64) error {
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return err
	}

	row, err := tr.db.LoadChannelWithAccountAndRoute(ctx, channelID)
	if err != nil || row == nil {
		return err
	}

	ch := row.Channel
	account := row.Account
	route := row.Route
	nowMs := time.Now().UnixMilli()
	nowISO := time.Now().UTC().Format(time.RFC3339)

	// Handle OAuth route unit member
	if ch.OAuthRouteUnitID != nil && *ch.OAuthRouteUnitID > 0 {
		targetAccountID := account.ID
		if actualAccountID != nil && *actualAccountID > 0 {
			targetAccountID = *actualAccountID
		}

		memberRow, err := tr.db.LoadRouteUnitMemberWithAccount(ctx, *ch.OAuthRouteUnitID, targetAccountID)
		if err == nil && memberRow != nil {
			shortWindowCooldown := resolveShortWindowLimitCooldownTS(memberRow.Account, failureCtx, nowMs)
			failCount := max(0, memberRow.Member.FailCount)
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

			_ = tr.db.UpdateRouteUnitMemberCooldownFields(ctx, memberRow.Member.ID, map[string]interface{}{
				"failCount":          failCount,
				"lastFailAt":         nowISO,
				"consecutiveFailCount": consecutiveFailCount,
				"cooldownLevel":      cooldownLevel,
				"cooldownUntil":      cooldownUntil,
				"updatedAt":          nowISO,
			})
			RecordSiteRuntimeFailure(memberRow.Account.SiteID, failureCtx)
			tr.cache.InvalidateRouteScopedCache(route.ID)
		}
		return nil
	}

	// Regular channel
	shortWindowCooldown := resolveShortWindowLimitCooldownTS(account, failureCtx, nowMs)
	failCount := max(0, ch.FailCount)
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

	_ = tr.db.UpdateChannelCooldownFields(ctx, affectedChannelIDs, map[string]interface{}{
		"failCount":          failCount,
		"lastFailAt":         nowISO,
		"consecutiveFailCount": consecutiveFailCount,
		"cooldownLevel":      cooldownLevel,
		"cooldownUntil":      cooldownUntil,
	})

	for _, id := range affectedChannelIDs {
		tr.cache.PatchCachedChannel(id, func(ch *store.RouteChannel) {
			ch.FailCount = failCount
			ch.LastFailAt = &nowISO
			ch.ConsecutiveFailCount = consecutiveFailCount
			ch.CooldownLevel = cooldownLevel
			ch.CooldownUntil = cooldownUntil
		})
	}

	RecordSiteRuntimeFailure(account.SiteID, failureCtx)
	return nil
}

// ClearChannelFailureState clears failure/cooldown for channels.
func (tr *TokenRouter) ClearChannelFailureState(ctx context.Context, channelIDs []int64) (int64, error) {
	if len(channelIDs) == 0 {
		return 0, nil
	}

	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return 0, err
	}

	// Clear runtime health states for affected channels
	runtimeHealthRows, _ := tr.db.LoadRuntimeHealthChannelRows(ctx, channelIDs)
	if len(runtimeHealthRows) > 0 {
		healthRows := make([]ChannelRuntimeHealthRow, len(runtimeHealthRows))
		for i, r := range runtimeHealthRows {
			healthRows[i] = ChannelRuntimeHealthRow{
				SiteID:            r.SiteID,
				SourceModel:       r.SourceModel,
				RouteModelPattern: r.RouteModelPattern,
			}
		}
		if ClearRuntimeHealthStatesForChannels(healthRows) {
			persistSiteRuntimeHealthState()
		}
	}

	if err := tr.db.ClearChannelFailureStates(ctx, channelIDs); err != nil {
		return 0, err
	}

	tr.cache.InvalidateAll()
	return int64(len(channelIDs)), nil
}

// InvalidateRouteScopedCache clears cache for a specific route.
func (tr *TokenRouter) InvalidateRouteScopedCache(routeID int64) {
	tr.cache.InvalidateRouteScopedCache(routeID)
}

// InvalidateAllCaches clears all caches.
func (tr *TokenRouter) InvalidateAllCaches() {
	tr.cache.InvalidateAll()
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

	// Check OAuth quota lastLimitResetAt
	if account.OAuthProvider != nil && *account.OAuthProvider == "codex" {
		// Could read from extraConfig, simplified for now
	}

	iso := formatUnixMillisISO(untilMs)
	return &iso
}
