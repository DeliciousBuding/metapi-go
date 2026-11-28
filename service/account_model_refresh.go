package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
)

// modelRefreshFetchTimeout bounds a single upstream GetModels call.
const modelRefreshFetchTimeout = 30 * time.Second

// Side-effect seams for the model refresh path (moved from handler/admin).
// Production delegates to the real service/routing owners; focused tests swap
// in counting fakes to assert the "exactly one route rebuild + one
// routing-cache invalidation" contract.
var modelRefreshRebuildRoutes = RebuildTokenRoutesFromAvailability
var modelRefreshInvalidateCache = routing.InvalidateCache

// SetModelRefreshSideEffectsForTest swaps the rebuild/invalidate seams used by
// the refresh path and returns a restore function. Test-only.
func SetModelRefreshSideEffectsForTest(
	rebuild func(ctx context.Context, db *sqlx.DB) (RouteRebuildStats, error),
	invalidate func(),
) func() {
	prevRebuild := modelRefreshRebuildRoutes
	prevInvalidate := modelRefreshInvalidateCache
	modelRefreshRebuildRoutes = rebuild
	modelRefreshInvalidateCache = invalidate
	return func() {
		modelRefreshRebuildRoutes = prevRebuild
		modelRefreshInvalidateCache = prevInvalidate
	}
}

// AccountModelRefreshResult carries the outcome of one account model refresh.
// The handler layer maps it onto the operator-facing JSON payload; the
// scheduler consumes the counters directly.
type AccountModelRefreshResult struct {
	Success      bool
	ErrorCode    string // "" on success; db_unavailable / not_found / disabled / inactive / unsupported_platform / missing_credential / timeout / unauthorized / upstream_error / empty_models / persist_failed
	ErrorMessage string // refresh.errorMessage payload field
	Message      string // optional top-level "message" payload field
	TopError     string // top-level "error" payload field (db_unavailable / not_found paths)

	Models    []string // cleaned upstream model list (also on empty/persist failures)
	CheckedAt string   // RFC3339 availability write timestamp (success only)

	TokenBackfilled  bool
	RedirectsCreated int

	Rebuild    RouteRebuildStats
	RebuildErr error
	RebuildRan bool // false when the caller deferred the rebuild (batch mode)
}

// RefreshAccountModels performs a real platform.GetModels refresh and persists
// results into model_availability.
//
// allowInactive=true permits non-active accounts (e.g. expired) to refresh so
// recovery workflows can validate new credentials before reactivation.
// Disabled accounts are always rejected.
//
// skipRebuild=true defers the route rebuild + routing-cache invalidation to
// the caller (periodic batch mode rebuilds exactly once after the whole pass);
// the single-account admin path passes false and rebuilds inline.
func RefreshAccountModels(ctx context.Context, db *sqlx.DB, accountID int64, allowInactive, skipRebuild bool) AccountModelRefreshResult {
	if db == nil {
		return AccountModelRefreshResult{
			ErrorCode:    "db_unavailable",
			ErrorMessage: "database not configured",
			TopError:     "database not configured",
		}
	}

	row, err := GetAccountWithSiteByID(db, accountID)
	if err != nil || row == nil {
		return AccountModelRefreshResult{
			ErrorCode:    "not_found",
			ErrorMessage: "account not found",
			TopError:     "account not found",
		}
	}

	account := row.Account
	site := row.Site
	status := strings.TrimSpace(account.Status)
	if strings.EqualFold(status, "disabled") {
		return AccountModelRefreshResult{
			ErrorCode:    "disabled",
			ErrorMessage: "account disabled",
			Message:      "account disabled; cannot refresh models",
		}
	}
	if !allowInactive && !strings.EqualFold(status, "active") && status != "" {
		return AccountModelRefreshResult{
			ErrorCode:    "inactive",
			ErrorMessage: "account not active",
			Message:      "account not active; cannot refresh models",
		}
	}

	adapter := platform.GetAdapter(site.Platform)
	if adapter == nil {
		return AccountModelRefreshResult{
			ErrorCode:    "unsupported_platform",
			ErrorMessage: "unsupported platform: " + site.Platform,
			Message:      "unsupported platform: " + site.Platform,
		}
	}

	token := resolveAccountModelToken(&account)
	if token == "" {
		return AccountModelRefreshResult{
			ErrorCode:    "missing_credential",
			ErrorMessage: "account is missing access_token / api_token",
			Message:      "account has no usable credentials",
		}
	}

	platformUserID := resolveModelRefreshPlatformUserIDPtr(account.ExtraConfig, account.Username)
	proxyCfg := BuildPlatformProxyConfig(nil, &account, &site)

	callCtx, cancel := context.WithTimeout(ctx, modelRefreshFetchTimeout)
	defer cancel()

	modelToken := token
	models, getErr := adapter.GetModels(callCtx, site.URL, token, platformUserID, proxyCfg)
	if getErr != nil {
		if fallbackToken, ok := newAPIAccessTokenFallback(site.Platform, &account, token, getErr); ok {
			modelToken = fallbackToken
			models, getErr = adapter.GetModels(callCtx, site.URL, fallbackToken, platformUserID, proxyCfg)
		}
	}
	if getErr != nil {
		code, msg := classifyModelRefreshError(getErr, callCtx)
		return AccountModelRefreshResult{
			ErrorCode:    code,
			ErrorMessage: msg,
			Message:      msg,
		}
	}

	clean := cleanModelNames(models)
	// err == nil also makes an empty list an authoritative observation. Clear
	// obsolete automatic availability before reporting empty_models; failed
	// requests returned above and must retain their last observed snapshot.

	now := time.Now().UTC().Format(time.RFC3339)
	if err := persistAccountModelAvailability(db, accountID, clean, now); err != nil {
		return AccountModelRefreshResult{
			ErrorCode:    "persist_failed",
			ErrorMessage: err.Error(),
			Message:      "failed to write models: " + err.Error(),
			Models:       clean,
		}
	}

	// Only the probed relay credential can establish token-scoped availability.
	tokenBackfilled := modelToken == token && backfillAccountTokenModels(db, accountID, token, clean, now)

	result := AccountModelRefreshResult{
		Success:         true,
		Models:          clean,
		CheckedAt:       now,
		TokenBackfilled: tokenBackfilled,
	}

	// Best-effort route rebuild from updated availability (exactly once per
	// refresh action; batch callers defer it via skipRebuild).
	if !skipRebuild {
		rebuildStats, rebuildErr := modelRefreshRebuildRoutes(context.Background(), db)
		result.Rebuild = rebuildStats
		result.RebuildErr = rebuildErr
		result.RebuildRan = true
		modelRefreshInvalidateCache()
	}

	if len(clean) == 0 {
		result.Success = false
		result.ErrorCode = "empty_models"
		result.ErrorMessage = "no models available"
		result.Message = "upstream returned no models; automatic availability refreshed"
		result.Models = []string{}
		return result
	}

	// K1a: best-effort sync generation of model name redirects
	// (canonical -> actual). Never blocks the refresh outcome.
	redirectsCreated, redirectErr := GenerateModelRedirects(context.Background(), db, accountID, clean)
	if redirectErr != nil {
		slog.Warn("model-refresh: redirect generation failed", "account_id", accountID, "error", redirectErr)
	} else {
		// Keep the in-process hot-path registry in sync after generation.
		ReloadRedirectRegistry(context.Background(), db)
		result.RedirectsCreated = redirectsCreated
	}

	return result
}
