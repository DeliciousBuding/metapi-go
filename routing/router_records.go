package routing

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// Detect a regular channel rebind between dispatch and health recording.
func validateRecordingAccount(ch store.RouteChannel, accountID int64, actual *int64) error {
	if (ch.OAuthRouteUnitID == nil || *ch.OAuthRouteUnitID <= 0) && actual != nil && *actual > 0 && *actual != accountID {
		return fmt.Errorf("channel %d no longer belongs to account %d", ch.ID, *actual)
	}
	return nil
}

// recordingMember resolves the credential that actually handled this attempt.
func (tr *TokenRouter) recordingMember(ctx context.Context, unitID, accountID int64, actualAccountID *int64) (*struct {
	Member  store.OAuthRouteUnitMember
	Account store.Account
	Unit    store.OAuthRouteUnit
}, error) {
	if actualAccountID != nil && *actualAccountID > 0 {
		accountID = *actualAccountID
	}
	row, err := tr.db.LoadRouteUnitMemberWithAccount(ctx, unitID, accountID)
	if err != nil {
		return nil, fmt.Errorf("load route unit %d member account %d: %w", unitID, accountID, err)
	}
	if row == nil {
		return nil, fmt.Errorf("route unit %d has no member for account %d", unitID, accountID)
	}
	return row, nil
}

// RecordSuccess records a successful channel usage.
func (tr *TokenRouter) RecordSuccess(ctx context.Context, channelID int64, latencyMs float64, cost float64, modelName *string, actualAccountID *int64) error {
	if channelID < 0 {
		return tr.recordDirectSuccess(ctx, -channelID, latencyMs, cost)
	}
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
	nextSuccessCount := max(0, ch.SuccessCountOrZero()) + 1
	nextTotalLatencyMs := max(0, ch.TotalLatencyMsOrZero()) + int64(latencyMs)
	nextTotalCost := math.Max(0, ch.TotalCostOrZero()) + cost
	// Decay (not zero) accumulated failCount on success: a recovered channel
	// keeps a small memory of past failures, but its next failure is no longer
	// over-penalized by Fibonacci backoff based on the historical peak.
	nextFailCount := max(0, ch.FailCountOrZero()) / 2

	if ch.OAuthRouteUnitID != nil && *ch.OAuthRouteUnitID > 0 {
		memberRow, err := tr.recordingMember(ctx, *ch.OAuthRouteUnitID, account.ID, actualAccountID)
		if err != nil {
			return err
		}
		memberSuccessCount := max(0, memberRow.Member.SuccessCountOrZero()) + 1
		memberTotalLatencyMs := max(0, memberRow.Member.TotalLatencyMsOrZero()) + int64(latencyMs)
		memberTotalCost := math.Max(0, memberRow.Member.TotalCostOrZero()) + cost
		if err := tr.db.UpdateRouteUnitMemberSuccessFields(ctx, memberRow.Member.ID, map[string]interface{}{
			"successCount":         memberSuccessCount,
			"failCount":            max(0, memberRow.Member.FailCountOrZero()) / 2,
			"totalLatencyMs":       memberTotalLatencyMs,
			"totalCost":            memberTotalCost,
			"lastUsedAt":           nowISO,
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
		tr.cache.InvalidateRouteScopedCache(ch.RouteID)
	} else {
		RecordSiteRuntimeSuccess(account.SiteID, latencyMs, modelName)
	}

	if err := tr.db.UpdateChannelSuccessFields(ctx, channelID, map[string]interface{}{
		"successCount":         nextSuccessCount,
		"failCount":            nextFailCount,
		"totalLatencyMs":       nextTotalLatencyMs,
		"totalCost":            nextTotalCost,
		"lastUsedAt":           nowISO,
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
		ch.SuccessCount = &nextSuccessCount
		ch.FailCount = &nextFailCount
		ch.TotalLatencyMs = &nextTotalLatencyMs
		ch.TotalCost = &nextTotalCost
		ch.LastUsedAt = &nowISO
		ch.CooldownUntil = nil
		ch.LastFailAt = nil
		ch.ConsecutiveFailCount = 0
		ch.CooldownLevel = 0
		ch.CooldownReasonCode = nil
		ch.CooldownReason = nil
		ch.CooldownReasonAt = nil
	})

	return nil
}

// InvalidateRouteScopedCache clears cache for a specific route.
func (tr *TokenRouter) InvalidateRouteScopedCache(routeID int64) {
	tr.cache.InvalidateRouteScopedCache(routeID)
}
