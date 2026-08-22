package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

func (h *accountTokensHandler) syncAccount(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "accountId")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")
		return
	}

	row, err := service.GetAccountWithSiteByID(h.db, accountID)
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, syncErr := executeAccountTokenSync(ctx, h.db, h.cfg, row)
	if syncErr != nil {
		writeError(w, http.StatusBadGateway, syncErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ---- Sync All ----

func (h *accountTokensHandler) syncAll(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountTokenSyncAllPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		// Empty body is allowed for background mode.
		body = payloads.AccountTokenSyncAllPayload{}
	}

	wait := body.Wait != nil && *body.Wait
	db := h.db
	cfg := h.cfg

	if wait {
		summary, results := executeSyncAllAccountTokens(context.Background(), db, cfg)
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"summary": summary,
			"results": results,
		})
		return
	}

	task, reused := StartBackgroundTask(BackgroundTaskStartOptions{
		Type:      "sync-all-account-tokens",
		Title:     "Sync all account tokens",
		DedupeKey: "sync-all-account-tokens",
	}, func() (any, error) {
		summary, results := executeSyncAllAccountTokens(context.Background(), db, cfg)
		return map[string]any{
			"summary": summary,
			"results": results,
		}, nil
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"queued":  true,
		"reused":  reused,
		"jobId":   task.ID,
		"taskId":  task.ID,
		"status":  string(task.Status),
		"message": "all-account token sync started; check the program logs later",
	})
}

// ---- Get Groups ----

func (h *accountTokensHandler) getGroups(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "accountId")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")
		return
	}

	row, err := service.GetAccountWithSiteByID(h.db, accountID)
	if err != nil {
		writeError(w, http.StatusNotFound, "account not found")
		return
	}

	if service.IsAPIKeyConnection(&row.Account) {
		writeError(w, http.StatusBadRequest, "API key connections do not support fetching account token groups")
		return
	}

	accessToken := strings.TrimSpace(row.Account.AccessToken)
	adapter := platform.GetAdapter(row.Site.Platform)
	proxyCfg := service.BuildPlatformProxyConfig(h.cfg, &row.Account, &row.Site)
	platformUserID := service.ResolvePlatformUserIDPtr(&row.Account)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	groups, err := service.GetTokenGroups(
		ctx,
		h.db,
		accountID,
		adapter,
		row.Site.URL,
		accessToken,
		platformUserID,
		proxyCfg,
	)
	if err != nil {
		slog.Error("Failed to load token groups", "err", err, "account_id", accountID)
		writeError(w, http.StatusInternalServerError, "Failed to load token groups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"groups":  groups,
	})
}

// ---- Get Account Default Token ----

func (h *accountTokensHandler) getAccountDefault(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "accountId")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")
		return
	}

	var account store.Account
	if err := h.db.Get(&account, h.db.Rebind("SELECT * FROM accounts WHERE id = ?"), accountID); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": nil})
		return
	}

	if service.IsAPIKeyConnection(&account) {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": nil})
		return
	}

	token, err := service.GetDefaultTokenForAccount(h.db, accountID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": nil})
		return
	}

	var site store.Site
	if err := h.db.Get(&site, h.db.Rebind("SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?"), account.SiteID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the site")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   tokenToMapMasked(*token, site.Platform),
	})
}

// ---- Upstream helpers ----

func parseFlexibleEpochSeconds(v any) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case float32:
		return int64(val), true
	case int:
		return int64(val), true
	case int64:
		return val, true
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.Unix(), true
		}
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.Unix(), true
		}
		return 0, false
	default:
		if n, ok := parseFlexibleFloat(v); ok {
			return int64(n), true
		}
		return 0, false
	}
}

func executeAccountTokenSync(ctx context.Context, db *sqlx.DB, cfg *config.Config, row *service.AccountWithSite) (map[string]any, error) {
	if row == nil {
		return nil, fmt.Errorf("account not found")
	}
	accountID := row.Account.ID

	base := map[string]any{
		"success":   true,
		"accountId": accountID,
		"synced":    false,
		"created":   0,
		"updated":   0,
		"total":     0,
	}

	if strings.EqualFold(strings.TrimSpace(row.Site.Status), "disabled") {
		base["status"] = "skipped"
		base["reason"] = "site_disabled"
		base["message"] = "site is disabled; skipping token sync"
		return base, nil
	}
	if service.IsAPIKeyConnection(&row.Account) {
		base["status"] = "skipped"
		base["reason"] = "apikey_connection"
		base["message"] = "API key connections do not support syncing account tokens"
		return base, nil
	}

	accessToken := strings.TrimSpace(row.Account.AccessToken)
	if accessToken == "" {
		if row.Account.APIToken != nil && strings.TrimSpace(*row.Account.APIToken) != "" {
			if _, err := service.EnsureDefaultTokenForAccount(
				db,
				accountID,
				strings.TrimSpace(*row.Account.APIToken),
				"default",
				"legacy",
				"default",
				true,
			); err != nil {
				return nil, err
			}
			base["status"] = "skipped"
			base["reason"] = "no_access_token"
			base["message"] = "account has no access token; kept the local default token"
			return base, nil
		}
		base["status"] = "skipped"
		base["reason"] = "no_access_token"
		base["message"] = "account has no access token; cannot sync site tokens"
		return base, nil
	}

	adapter := platform.GetAdapter(row.Site.Platform)
	if adapter == nil {
		base["status"] = "skipped"
		base["reason"] = "unsupported_platform"
		base["message"] = "unsupported platform; cannot sync site tokens: " + row.Site.Platform
		return base, nil
	}

	callCtx := ctx
	if callCtx == nil {
		callCtx = context.Background()
	}
	if _, hasDeadline := callCtx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(callCtx, 15*time.Second)
		defer cancel()
	}

	proxyCfg := service.BuildPlatformProxyConfig(cfg, &row.Account, &row.Site)
	platformUserID := service.ResolvePlatformUserIDPtr(&row.Account)

	upstreamTokens, err := service.FetchUpstreamAPITokens(
		callCtx,
		adapter,
		row.Site.URL,
		accessToken,
		platformUserID,
		proxyCfg,
	)
	if err != nil {
		_ = service.CreateEvent(db, "token_sync", "account token sync failed", err.Error(), "error", accountID, "account")
		return nil, fmt.Errorf("failed to sync upstream tokens: %w", err)
	}
	if len(upstreamTokens) == 0 {
		base["status"] = "skipped"
		base["reason"] = "no_upstream_tokens"
		base["message"] = "upstream returned no api tokens"
		return base, nil
	}

	syncResult, syncErr := service.SyncTokensFromUpstream(db, accountID, upstreamTokens)
	if syncErr != nil {
		_ = service.CreateEvent(db, "token_sync", "account token sync failed", syncErr.Error(), "error", accountID, "account")
		return nil, syncErr
	}

	base["status"] = "synced"
	base["synced"] = true
	base["created"] = syncResult.Created
	base["updated"] = syncResult.Updated
	base["total"] = syncResult.Total
	base["maskedPending"] = syncResult.MaskedPending
	if syncResult.DefaultTokenID != nil {
		base["defaultTokenId"] = *syncResult.DefaultTokenID
	}
	base["message"] = fmt.Sprintf("sync completed: %d created, %d updated, %d total", syncResult.Created, syncResult.Updated, syncResult.Total)

	_ = service.CreateEvent(
		db,
		"token_sync",
		"Account token sync completed",
		base["message"].(string),
		"info",
		accountID,
		"account",
	)
	return base, nil
}

func executeSyncAllAccountTokens(ctx context.Context, db *sqlx.DB, cfg *config.Config) (map[string]int, []map[string]any) {
	type accountIDRow struct {
		ID int64 `db:"id"`
	}
	var ids []accountIDRow
	if err := db.Select(&ids, `SELECT id FROM accounts WHERE status = 'active' ORDER BY id`); err != nil {
		slog.Error("sync-all account tokens: list accounts failed", "err", err)
		return map[string]int{
			"total": 0, "synced": 0, "skipped": 0, "failed": 0, "created": 0, "updated": 0,
		}, []map[string]any{}
	}

	summary := map[string]int{
		"total": len(ids), "synced": 0, "skipped": 0, "failed": 0, "created": 0, "updated": 0,
	}
	results := make([]map[string]any, 0, len(ids))

	const batchSize = 3
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		type batchItem struct {
			result map[string]any
		}
		out := make([]batchItem, len(batch))
		var wg sync.WaitGroup
		for idx, item := range batch {
			wg.Add(1)
			go func(idx int, accountID int64) {
				defer wg.Done()
				row, err := service.GetAccountWithSiteByID(db, accountID)
				if err != nil {
					out[idx] = batchItem{result: map[string]any{
						"success":   false,
						"accountId": accountID,
						"status":    "failed",
						"reason":    "account_not_found",
						"message":   "account not found",
						"synced":    false,
						"created":   0,
						"updated":   0,
						"total":     0,
					}}
					return
				}
				result, syncErr := executeAccountTokenSync(ctx, db, cfg, row)
				if syncErr != nil {
					out[idx] = batchItem{result: map[string]any{
						"success":   false,
						"accountId": accountID,
						"status":    "failed",
						"reason":    "sync_failed",
						"message":   syncErr.Error(),
						"synced":    false,
						"created":   0,
						"updated":   0,
						"total":     0,
					}}
					return
				}
				out[idx] = batchItem{result: result}
			}(idx, item.ID)
		}
		wg.Wait()

		for _, item := range out {
			result := item.result
			results = append(results, result)
			status, _ := result["status"].(string)
			switch status {
			case "synced":
				summary["synced"]++
			case "skipped":
				summary["skipped"]++
			default:
				summary["failed"]++
			}
			if created, ok := asInt(result["created"]); ok {
				summary["created"] += created
			}
			if updated, ok := asInt(result["updated"]); ok {
				summary["updated"] += updated
			}
		}
	}

	return summary, results
}

// ---- Helper functions ----

// TokenValueStatusReady and MaskedPending
const (
	TokenValueStatusReady         = "ready"
	TokenValueStatusMaskedPending = "masked_pending"
)
