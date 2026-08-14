package admin

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
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// accountModelRefresher is the package-level model refresh entrypoint.
// Tests may inject a fake implementation; production uses refreshAccountModels.
var accountModelRefresher = refreshAccountModels

// refreshAccountModels performs a real platform.GetModels refresh and persists
// results into model_availability. Returns the operator-facing check payload.

// allowInactive=true permits non-active accounts (e.g. expired) to refresh so
// recovery workflows can validate new credentials before reactivation.
// Disabled accounts are always rejected.
func refreshAccountModels(ctx context.Context, db *sqlx.DB, accountID int64, allowInactive bool) map[string]any {
	if db == nil {
		return map[string]any{
			"success": false,
			"error":   "database not configured",
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    "db_unavailable",
				"errorMessage": "database not configured",
			},
			"rebuild": map[string]any{},
		}
	}

	row, err := service.GetAccountWithSiteByID(db, accountID)
	if err != nil || row == nil {
		return map[string]any{
			"success": false,
			"error":   "account not found",
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    "not_found",
				"errorMessage": "账号不存在",
			},
			"rebuild": map[string]any{},
		}
	}

	account := row.Account
	site := row.Site
	status := strings.TrimSpace(account.Status)
	if strings.EqualFold(status, "disabled") {
		return map[string]any{
			"success": false,
			"message": "账号已禁用，无法刷新模型",
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    "disabled",
				"errorMessage": "账号已禁用",
			},
			"rebuild": map[string]any{},
		}
	}
	if !allowInactive && !strings.EqualFold(status, "active") && status != "" {
		return map[string]any{
			"success": false,
			"message": "账号未激活，无法刷新模型",
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    "inactive",
				"errorMessage": "账号未激活",
			},
			"rebuild": map[string]any{},
		}
	}

	adapter := platform.GetAdapter(site.Platform)
	if adapter == nil {
		return map[string]any{
			"success": false,
			"message": "unsupported platform: " + site.Platform,
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    "unsupported_platform",
				"errorMessage": "不支持的平台: " + site.Platform,
			},
			"rebuild": map[string]any{},
		}
	}

	token := resolveAccountModelToken(&account)
	if token == "" {
		return map[string]any{
			"success": false,
			"message": "账号缺少可用凭证",
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    "missing_credential",
				"errorMessage": "账号缺少 access_token / api_token",
			},
			"rebuild": map[string]any{},
		}
	}

	platformUserID := resolvePlatformUserIDPtr(account.ExtraConfig, account.Username)
	proxyCfg := service.BuildPlatformProxyConfig(nil, &account, &site)

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	models, getErr := adapter.GetModels(callCtx, site.URL, token, platformUserID, proxyCfg)
	if getErr != nil {
		code, msg := classifyModelRefreshError(getErr, callCtx)
		return map[string]any{
			"success": false,
			"message": msg,
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    code,
				"errorMessage": msg,
			},
			"rebuild": map[string]any{},
		}
	}

	// Deduplicate / trim.
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
	if len(clean) == 0 {
		return map[string]any{
			"success": false,
			"message": "未获取到可用模型",
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    "empty_models",
				"errorMessage": "未获取到可用模型",
				"modelCount":   0,
				"models":       []string{},
			},
			"rebuild": map[string]any{},
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if err := persistAccountModelAvailability(db, accountID, clean, now); err != nil {
		return map[string]any{
			"success": false,
			"message": "模型写入失败: " + err.Error(),
			"refresh": map[string]any{
				"id":           accountID,
				"status":       "failed",
				"errorCode":    "persist_failed",
				"errorMessage": err.Error(),
				"modelCount":   len(clean),
				"models":       clean,
			},
			"rebuild": map[string]any{},
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

	// Best-effort route rebuild from updated availability.
	rebuildStats, rebuildErr := service.RebuildTokenRoutesFromAvailability(context.Background(), db)
	rebuildPayload := map[string]any{
		"routesConsidered": rebuildStats.RoutesConsidered,
		"patternRoutes":    rebuildStats.PatternRoutes,
		"groupRoutes":      rebuildStats.GroupRoutes,
		"channelsInserted": rebuildStats.ChannelsInserted,
		"channelsRemoved":  rebuildStats.ChannelsRemoved,
		"channelsKept":     rebuildStats.ChannelsKept,
	}
	if rebuildErr != nil {
		rebuildPayload["success"] = false
		rebuildPayload["error"] = rebuildErr.Error()
	} else {
		rebuildPayload["success"] = true
	}
	routing.InvalidateCache()

	// K1a: best-effort sync generation of model name
	// redirects (canonical → actual). Never blocks the refresh outcome.
	redirectsCreated, redirectErr := service.GenerateModelRedirects(context.Background(), db, accountID, clean)
	if redirectErr != nil {
		slog.Warn("model-refresh: redirect generation failed", "account_id", accountID, "error", redirectErr)
	} else {
		// K1b: keep the in-process hot-path registry in sync after generation.
		service.ReloadRedirectRegistry(context.Background(), db)
	}

	return map[string]any{
		"success": true,
		"refresh": map[string]any{
			"id":         accountID,
			"status":     "success",
			"modelCount": len(clean),
			"models":     clean,
			"checkedAt":  now,
		},
		"rebuild": rebuildPayload,
		"redirects": map[string]any{
			"generated": redirectsCreated,
		},
		"tokenBackfilled": tokenBackfilled,
	}
}

func resolveAccountModelToken(account *store.Account) string {
	if account == nil {
		return ""
	}
	if account.APIToken != nil && strings.TrimSpace(*account.APIToken) != "" {
		return strings.TrimSpace(*account.APIToken)
	}
	return strings.TrimSpace(account.AccessToken)
}

func resolvePlatformUserIDPtr(extraConfig *string, username *string) *int {
	id := service.ResolvePlatformUserID(extraConfig, username)
	if id <= 0 {
		return nil
	}
	v := int(id)
	return &v
}

func classifyModelRefreshError(err error, ctx context.Context) (code, message string) {
	if err == nil {
		return "unknown", "模型获取失败"
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	timedOut := (ctx != nil && ctx.Err() == context.DeadlineExceeded) ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded")
	if timedOut {
		return "timeout", "模型获取失败（请求超时）"
	}
	if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401") || strings.Contains(lower, "invalid api key") || strings.Contains(lower, "authentication") {
		return "unauthorized", "模型获取失败，API Key 已无效"
	}
	if msg == "" {
		msg = "模型获取失败"
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
	// both evaluate NULL→NULL (excluded) and false/0→true in WHERE.
	if _, err := tx.Exec(tx.Rebind(`
		UPDATE model_availability
		SET available = ?, checked_at = ?
		WHERE account_id = ? AND NOT is_manual
	`), false, now, accountID); err != nil {
		return err
	}

	for _, model := range models {
		var existingID int64
		err := tx.Get(&existingID, tx.Rebind(`
			SELECT id FROM model_availability WHERE account_id = ? AND model_name = ?
		`), accountID, model)
		if err == nil {
			if _, err := tx.Exec(tx.Rebind(`
				UPDATE model_availability
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
			INSERT INTO model_availability (account_id, model_name, available, is_manual, latency_ms, checked_at)
			VALUES (?, ?, ?, ?, NULL, ?)
		`), accountID, model, true, false, now); err != nil {
			if _, err2 := tx.Exec(tx.Rebind(`
				UPDATE model_availability
				SET available = ?, latency_ms = NULL, checked_at = ?
				WHERE account_id = ? AND model_name = ?
			`), true, now, accountID, model); err2 != nil {
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

func modelRefreshSucceeded(result map[string]any) bool {
	if result == nil {
		return false
	}
	ok, _ := result["success"].(bool)
	return ok
}

func modelRefreshErrorMessage(result map[string]any) string {
	if result == nil {
		return "模型刷新失败"
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
	return "模型刷新失败"
}
