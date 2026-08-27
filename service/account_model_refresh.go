package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// Account model refresh (#1005 Wave 15): the single-account refresh core
// moved here from handler/admin/model_refresh.go so the admin handler, the
// periodic model-sync scheduler and future callers share one owner and one
// data flow. The handler keeps the accountModelRefresher seam and shapes the
// unchanged camelCase JSON payload from AccountModelRefreshResult.

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

	models, getErr := adapter.GetModels(callCtx, site.URL, token, platformUserID, proxyCfg)
	if getErr != nil {
		code, msg := classifyModelRefreshError(getErr, callCtx)
		return AccountModelRefreshResult{
			ErrorCode:    code,
			ErrorMessage: msg,
			Message:      msg,
		}
	}

	clean := cleanModelNames(models)
	if len(clean) == 0 {
		return AccountModelRefreshResult{
			ErrorCode:    "empty_models",
			ErrorMessage: "no models available",
			Message:      "no models available",
			Models:       []string{},
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := persistAccountModelAvailability(db, accountID, clean, now); err != nil {
		return AccountModelRefreshResult{
			ErrorCode:    "persist_failed",
			ErrorMessage: err.Error(),
			Message:      "failed to write models: " + err.Error(),
			Models:       clean,
		}
	}

	// Backfill token_model_availability for the resolved token so the
	// marketplace stats and site model list (which read this table) reflect
	// the freshly refreshed model set (#675). Best-effort: if the token does
	// not match an account_tokens row, skip silently — the account-level
	// model_availability rows remain the source of truth.
	tokenBackfilled := false
	if tokenID, ok := resolveAccountTokenID(db, accountID, token); ok {
		if err := persistTokenModelAvailability(db, tokenID, clean, now); err != nil {
			slog.Warn("model-refresh: token_model_availability backfill failed",
				"account_id", accountID, "token_id", tokenID, "error", err)
		} else {
			tokenBackfilled = true
		}
	}

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

	// K1a: best-effort sync generation of model name redirects
	// (canonical -> actual). Never blocks the refresh outcome.
	redirectsCreated, redirectErr := GenerateModelRedirects(context.Background(), db, accountID, clean)
	if redirectErr != nil {
		slog.Warn("model-refresh: redirect generation failed", "account_id", accountID, "error", redirectErr)
	} else {
		// K1b: keep the in-process hot-path registry in sync after generation.
		ReloadRedirectRegistry(context.Background(), db)
		result.RedirectsCreated = redirectsCreated
	}

	return result
}

// ModelSyncSummary reports one periodic model-sync pass.
type ModelSyncSummary struct {
	Total   int
	Success int
	Failed  int

	Rebuild    RouteRebuildStats
	RebuildErr error
	RebuildRan bool
}

// SyncAllAccountModels refreshes upstream model lists for all candidate
// accounts sequentially (polite to upstreams, no concurrency races). Each
// account refresh skips its own route rebuild; after the pass, when at least
// one account succeeded, exactly one route rebuild + one routing-cache
// invalidation runs for the whole batch. Per-account failures are logged and
// counted without aborting the pass.
func SyncAllAccountModels(ctx context.Context, db *sqlx.DB) ModelSyncSummary {
	var summary ModelSyncSummary
	if db == nil {
		slog.Error("model-sync: database not available")
		return summary
	}

	candidates := listModelSyncCandidates(db)
	summary.Total = len(candidates)

	for i, accountID := range candidates {
		if err := ctx.Err(); err != nil {
			slog.Warn("model-sync: pass aborted before finishing all candidates",
				"processed", i, "remaining", len(candidates)-i, "error", err)
			break
		}
		res := RefreshAccountModels(ctx, db, accountID, false, true)
		if res.Success {
			summary.Success++
			slog.Info("model-sync: account refreshed", "account_id", accountID, "models", len(res.Models))
			continue
		}
		summary.Failed++
		slog.Warn("model-sync: account refresh failed",
			"account_id", accountID, "error_code", res.ErrorCode, "error", res.ErrorMessage)
	}

	if summary.Success > 0 {
		rebuildStats, rebuildErr := modelRefreshRebuildRoutes(ctx, db)
		summary.Rebuild = rebuildStats
		summary.RebuildErr = rebuildErr
		summary.RebuildRan = true
		modelRefreshInvalidateCache()
	}

	slog.Info("model-sync complete", "total", summary.Total, "success", summary.Success, "failed", summary.Failed)
	return summary
}

// listModelSyncCandidates returns the account IDs eligible for periodic model
// sync: status "active" or empty (disabled and other non-empty statuses are
// skipped), a resolvable credential (api_token or access_token), and a known
// site adapter. Accounts failing any check are not candidates at all — they
// never count against the pass's failed total.
func listModelSyncCandidates(db *sqlx.DB) []int64 {
	var ids []int64
	err := db.Select(&ids, db.Rebind(
		"SELECT id FROM accounts WHERE status = ? OR status = ? OR status IS NULL ORDER BY id",
	), "active", "")
	if err != nil {
		slog.Error("model-sync: failed to query candidate accounts", "error", err)
		return nil
	}

	candidates := make([]int64, 0, len(ids))
	for _, id := range ids {
		row, err := GetAccountWithSiteByID(db, id)
		if err != nil || row == nil {
			continue
		}
		if platform.GetAdapter(row.Site.Platform) == nil {
			continue
		}
		if resolveAccountModelToken(&row.Account) == "" {
			continue
		}
		candidates = append(candidates, id)
	}
	return candidates
}

// resolveAccountModelToken picks the credential used for model-list fetches:
// api_token wins over access_token.
func resolveAccountModelToken(account *store.Account) string {
	if account == nil {
		return ""
	}
	if account.APIToken != nil && strings.TrimSpace(*account.APIToken) != "" {
		return strings.TrimSpace(*account.APIToken)
	}
	return strings.TrimSpace(account.AccessToken)
}

// resolveModelRefreshPlatformUserIDPtr mirrors the pre-Wave-15 handler
// behavior exactly: only ResolvePlatformUserID (extraConfig platformUserId),
// not the richer ResolvePlatformUserIDPtr username guessing.
func resolveModelRefreshPlatformUserIDPtr(extraConfig *string, username *string) *int {
	id := ResolvePlatformUserID(extraConfig, username)
	if id <= 0 {
		return nil
	}
	v := int(id)
	return &v
}

// cleanModelNames trims whitespace and drops empty/case-insensitive
// duplicates, preserving first-seen casing.
func cleanModelNames(models []string) []string {
	seen := map[string]struct{}{}
	clean := make([]string, 0, len(models))
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		key := strings.ToLower(m)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		clean = append(clean, m)
	}
	return clean
}

func classifyModelRefreshError(err error, ctx context.Context) (code, message string) {
	if err == nil {
		return "unknown", "model fetch failed"
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	timedOut := (ctx != nil && ctx.Err() == context.DeadlineExceeded) ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded")
	if timedOut {
		return "timeout", "model fetch failed (request timed out)"
	}
	if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401") || strings.Contains(lower, "invalid api key") || strings.Contains(lower, "authentication") {
		return "unauthorized", "model fetch failed: API key is invalid"
	}
	if msg == "" {
		msg = "model fetch failed"
	}
	return "upstream_error", msg
}

func persistAccountModelAvailability(db *sqlx.DB, accountID int64, models []string, now string) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Preserve manual rows; mark non-manual previous rows unavailable then upsert.
	// `NOT is_manual` is dialect-safe: PostgreSQL BOOLEAN and SQLite INTEGER 0/1
	// both evaluate NULL->NULL (excluded) and false/0->true in WHERE.
	if _, err := tx.Exec(tx.Rebind(`
		UPDATE model_availability
		SET available = ?, checked_at = ?
		WHERE account_id = ? AND NOT is_manual
	`), false, now, accountID); err != nil {
		return err
	}

	for _, model := range models {
		// Single dialect-aware upsert replaces the SELECT-then-UPDATE-or-INSERT
		// pattern (3 round-trips/model -> 1). ON CONFLICT (account_id, model_name)
		// targets the model_availability_account_model_unique constraint. The
		// is_manual column is deliberately NOT updated on conflict so the
		// auto-refresh path preserves operator-set manual rows (mirroring the
		// old UPDATE which only touched available/latency_ms/checked_at).
		if _, err := tx.Exec(tx.Rebind(`
			INSERT INTO model_availability (account_id, model_name, available, is_manual, latency_ms, checked_at)
			VALUES (?, ?, ?, ?, NULL, ?)
			ON CONFLICT (account_id, model_name) DO UPDATE SET
				available = EXCLUDED.available,
				latency_ms = NULL,
				checked_at = EXCLUDED.checked_at
		`), accountID, model, true, false, now); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// resolveAccountTokenID finds the account_tokens row id matching the resolved
// refresh token. Tokens are stored normalized (TrimSpace, no Bearer prefix);
// the resolved token is also normalized, so an exact match is sufficient.
// Returns (id, true) on match, (0, false) when no row matches — in which case
// the caller skips the token_model_availability backfill (#675).
func resolveAccountTokenID(db *sqlx.DB, accountID int64, token string) (int64, bool) {
	normalized := strings.TrimSpace(token)
	normalized = strings.TrimPrefix(normalized, "Bearer ")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return 0, false
	}
	var tokenID int64
	err := db.Get(&tokenID, db.Rebind(
		"SELECT id FROM account_tokens WHERE account_id = ? AND token = ? ORDER BY id DESC LIMIT 1",
	), accountID, normalized)
	if err != nil {
		return 0, false
	}
	return tokenID, true
}

// persistTokenModelAvailability mirrors persistAccountModelAvailability for
// the token_model_availability table: mark all existing rows for the token
// unavailable, then upsert the current model set as available. Uses its own
// transaction so a failure here never rolls back the account-level write.
func persistTokenModelAvailability(db *sqlx.DB, tokenID int64, models []string, now string) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Mark all existing rows for this token unavailable before upserting the
	// current set — mirrors the account-level "mark non-manual unavailable"
	// pattern. token_model_availability has no is_manual column, so we clear
	// everything and re-enable the freshly observed models.
	if _, err := tx.Exec(tx.Rebind(`
		UPDATE token_model_availability
		SET available = ?, checked_at = ?
		WHERE token_id = ?
	`), false, now, tokenID); err != nil {
		return err
	}

	for _, model := range models {
		var existingID int64
		err := tx.Get(&existingID, tx.Rebind(`
			SELECT id FROM token_model_availability WHERE token_id = ? AND model_name = ?
		`), tokenID, model)
		if err == nil {
			if _, err := tx.Exec(tx.Rebind(`
				UPDATE token_model_availability
				SET available = ?, latency_ms = NULL, checked_at = ?
				WHERE id = ?
			`), true, now, existingID); err != nil {
				return err
			}
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(tx.Rebind(`
			INSERT INTO token_model_availability (token_id, model_name, available, latency_ms, checked_at)
			VALUES (?, ?, ?, NULL, ?)
		`), tokenID, model, true, now); err != nil {
			// Unique constraint violation: fall back to UPDATE.
			if _, err2 := tx.Exec(tx.Rebind(`
				UPDATE token_model_availability
				SET available = ?, latency_ms = NULL, checked_at = ?
				WHERE token_id = ? AND model_name = ?
			`), true, now, tokenID, model); err2 != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
