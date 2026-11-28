package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// RecordProbeSuccess records a successful background health probe.
// It clears cooldown for credential-scoped channels, feeds runtime health
// success, and stamps last probe status. It never marks credentials expired.
func (tr *TokenRouter) RecordProbeSuccess(ctx context.Context, channelID int64, latencyMs float64, modelName *string, actualAccountID *int64) error {
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return err
	}

	row, err := tr.db.LoadChannelWithAccount(ctx, channelID)
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
	nowISO := time.Now().UTC().Format(time.RFC3339)
	channelIDCopy := channelID

	if ch.OAuthRouteUnitID != nil && *ch.OAuthRouteUnitID > 0 {
		memberRow, err := tr.recordingMember(ctx, *ch.OAuthRouteUnitID, account.ID, actualAccountID)
		if err != nil {
			return err
		}
		if err := tr.db.UpdateRouteUnitMemberCooldownFields(ctx, memberRow.Member.ID, map[string]interface{}{
			"cooldownUntil":        nil,
			"lastFailAt":           nil,
			"consecutiveFailCount": int64(0),
			"cooldownLevel":        int64(0),
			"cooldownReasonCode":   nil,
			"cooldownReason":       nil,
			"cooldownReasonAt":     nil,
			"updatedAt":            nowISO,
		}); err != nil {
			return err
		}
		RecordSiteRuntimeSuccess(memberRow.Account.SiteID, latencyMs, modelName)
		RecordSiteProbeOutcome(memberRow.Account.SiteID, "success", latencyMs, modelName, &channelIDCopy, nil)

		if err := tr.db.UpdateChannelCooldownFields(ctx, []int64{channelID}, map[string]interface{}{
			"cooldownUntil":        nil,
			"lastFailAt":           nil,
			"consecutiveFailCount": int64(0),
			"cooldownLevel":        int64(0),
			"cooldownReasonCode":   nil,
			"cooldownReason":       nil,
			"cooldownReasonAt":     nil,
		}); err != nil {
			return err
		}
		tr.cache.PatchCachedChannel(channelID, func(ch *store.RouteChannel) {
			ch.CooldownUntil = nil
			ch.LastFailAt = nil
			ch.ConsecutiveFailCount = 0
			ch.CooldownLevel = 0
			ch.CooldownReasonCode = nil
			ch.CooldownReason = nil
			ch.CooldownReasonAt = nil
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
		if err := tr.db.UpdateChannelCooldownFields(ctx, affectedChannelIDs, map[string]interface{}{
			"cooldownUntil":        nil,
			"lastFailAt":           nil,
			"consecutiveFailCount": int64(0),
			"cooldownLevel":        int64(0),
			"cooldownReasonCode":   nil,
			"cooldownReason":       nil,
			"cooldownReasonAt":     nil,
		}); err != nil {
			return err
		}
		for _, id := range affectedChannelIDs {
			tr.cache.PatchCachedChannel(id, func(ch *store.RouteChannel) {
				ch.CooldownUntil = nil
				ch.LastFailAt = nil
				ch.ConsecutiveFailCount = 0
				ch.CooldownLevel = 0
				ch.CooldownReasonCode = nil
				ch.CooldownReason = nil
				ch.CooldownReasonAt = nil
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
	channelIDCopy := channelID
	reasonCode, reasonSummary := ClassifyCooldownReason(failureCtx, CooldownTriggerProbe)

	// Normalize model onto failure context for runtime health model-level state.
	if failureCtx.ModelName == nil || *failureCtx.ModelName == "" {
		if ch.SourceModel != nil && *ch.SourceModel != "" {
			failureCtx.ModelName = ch.SourceModel
		}
	}

	// OAuth unit: update member cooldown only; outer channel fields stay clean.
	if ch.OAuthRouteUnitID != nil && *ch.OAuthRouteUnitID > 0 {
		memberRow, err := tr.recordingMember(ctx, *ch.OAuthRouteUnitID, account.ID, actualAccountID)
		if err != nil {
			return err
		}
		// Probe path intentionally ignores usage-limit credential cascade and
		// never zeros failCount for short-window limits: probes are synthetic.
		failCount := max(0, memberRow.Member.FailCountOrZero()) + 1
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
		RecordSiteProbeOutcome(memberRow.Account.SiteID, "failure", 0, failureCtx.ModelName, &channelIDCopy, failureCtx.ErrorText)
		tr.cache.InvalidateRouteScopedCache(route.ID)
		return nil
	}

	// Regular channel: probe failure cools only the probed channel (no credential cascade).
	failCount := max(0, ch.FailCountOrZero()) + 1
	routeStrategy := NormalizeRouteRoutingStrategy(route.RoutingStrategy)
	affectedChannelIDs := []int64{channelID}

	var cooldownUntil *string
	// Pass raw consecutiveFailCount; ApplyRoundRobinCooldown alone does +1.
	consecutiveFailCount := max(0, ch.ConsecutiveFailCount)
	cooldownLevel := max(0, ch.CooldownLevel)

	consecutiveFailCount, cooldownLevel, cooldownUntil = resolveCooldownUpdate(
		routeStrategy == StrategyRoundRobin,
		consecutiveFailCount, cooldownLevel, failCount, nowMs, tr.configuredMaxSec)

	if err := tr.db.UpdateChannelCooldownFields(ctx, affectedChannelIDs, map[string]interface{}{
		"failCount":            failCount,
		"lastFailAt":           nowISO,
		"consecutiveFailCount": consecutiveFailCount,
		"cooldownLevel":        cooldownLevel,
		"cooldownUntil":        cooldownUntil,
		"cooldownReasonCode":   reasonCode,
		"cooldownReason":       CooldownReasonSummaryArg(reasonSummary),
		"cooldownReasonAt":     nowISO,
	}); err != nil {
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
		})
	}

	RecordSiteRuntimeFailure(account.SiteID, failureCtx)
	RecordSiteProbeOutcome(account.SiteID, "failure", 0, failureCtx.ModelName, &channelIDCopy, failureCtx.ErrorText)
	return nil
}
