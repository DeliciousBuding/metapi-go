package admin

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
)

// accountModelRefresher is the package-level model refresh entrypoint.
// Tests may inject a fake implementation; production uses refreshAccountModels.
var accountModelRefresher = refreshAccountModels

// invalidateRoutingCache remains the side-effect seam for the manual-models
// path (accounts_models.go). The refresh path moved to the service layer in
// Wave 15 (#1005) carries its own rebuild/invalidate seams there — see
// service.SetModelRefreshSideEffectsForTest.
var invalidateRoutingCache = routing.InvalidateCache

// refreshAccountModels performs a real platform.GetModels refresh by
// delegating to the service-layer owner and shapes the operator-facing check
// payload. The camelCase payload shape is API-compatible with the pre-Wave-15
// implementation (single-account admin refresh, expired-recovery and
// marketplace stats callers all consume it unchanged).
func refreshAccountModels(ctx context.Context, db *sqlx.DB, accountID int64, allowInactive bool) map[string]any {
	result := service.RefreshAccountModels(ctx, db, accountID, allowInactive, false)
	return accountModelRefreshPayload(accountID, result)
}

// accountModelRefreshPayload maps a service.AccountModelRefreshResult onto the
// exact operator-facing JSON shape the handler returned before Wave 15.
func accountModelRefreshPayload(accountID int64, result service.AccountModelRefreshResult) map[string]any {
	if result.Success {
		rebuildPayload := map[string]any{
			"routesConsidered": result.Rebuild.RoutesConsidered,
			"patternRoutes":    result.Rebuild.PatternRoutes,
			"groupRoutes":      result.Rebuild.GroupRoutes,
			"channelsInserted": result.Rebuild.ChannelsInserted,
			"channelsRemoved":  result.Rebuild.ChannelsRemoved,
			"channelsKept":     result.Rebuild.ChannelsKept,
		}
		if result.RebuildErr != nil {
			rebuildPayload["success"] = false
			rebuildPayload["error"] = result.RebuildErr.Error()
		} else {
			rebuildPayload["success"] = true
		}
		return map[string]any{
			"success": true,
			"refresh": map[string]any{
				"id":         accountID,
				"status":     "success",
				"modelCount": len(result.Models),
				"models":     result.Models,
				"checkedAt":  result.CheckedAt,
			},
			"rebuild": rebuildPayload,
			"redirects": map[string]any{
				"generated": result.RedirectsCreated,
			},
			"tokenBackfilled": result.TokenBackfilled,
		}
	}

	refresh := map[string]any{
		"id":           accountID,
		"status":       "failed",
		"errorCode":    result.ErrorCode,
		"errorMessage": result.ErrorMessage,
	}
	switch result.ErrorCode {
	case "empty_models":
		refresh["modelCount"] = 0
		refresh["models"] = []string{}
	case "persist_failed":
		refresh["modelCount"] = len(result.Models)
		refresh["models"] = result.Models
	}

	payload := map[string]any{
		"success": false,
		"refresh": refresh,
		"rebuild": map[string]any{},
	}
	if result.TopError != "" {
		payload["error"] = result.TopError
	}
	if result.Message != "" {
		payload["message"] = result.Message
	}
	return payload
}

func modelRefreshSucceeded(result map[string]any) bool {
	if result == nil {
		return false
	}
	ok, _ := result["success"].(bool)
	return ok
}

func modelRefreshErrorMessage(result map[string]any) string {
	if result == nil {
		return "model refresh failed"
	}
	if msg, ok := result["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if errMsg, ok := result["error"].(string); ok && strings.TrimSpace(errMsg) != "" {
		return strings.TrimSpace(errMsg)
	}
	if refresh, ok := result["refresh"].(map[string]any); ok {
		if msg, ok := refresh["errorMessage"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}
	return "model refresh failed"
}
