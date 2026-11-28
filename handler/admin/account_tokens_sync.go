package admin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func executeAccountTokenSync(ctx context.Context, db *store.DB, cfg *config.Config, row *service.AccountWithSite) (map[string]any, error) {
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
				db.DB,
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

	upstreamListing, err := service.FetchUpstreamAPITokenListing(
		callCtx,
		adapter,
		row.Site.URL,
		accessToken,
		platformUserID,
		proxyCfg,
	)
	if err != nil {
		_ = service.CreateEvent(db.DB, "token_sync", "account token sync failed", err.Error(), "error", accountID, "account")
		return nil, fmt.Errorf("failed to sync upstream tokens: %w", err)
	}
	if len(upstreamListing.Tokens) == 0 {
		base["status"] = "skipped"
		base["reason"] = "no_upstream_tokens"
		base["message"] = "upstream returned no api tokens"
		return base, nil
	}

	syncResult, syncErr := service.SyncTokensFromUpstream(db.DB, accountID, upstreamListing)
	if syncErr != nil {
		_ = service.CreateEvent(db.DB, "token_sync", "account token sync failed", syncErr.Error(), "error", accountID, "account")
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

	eventTitle := "Account token sync completed"
	eventLevel := "info"
	if switched := syncResult.DefaultSwitch; switched != nil {
		base["defaultSwitched"] = true
		base["defaultSwitchedFrom"] = map[string]any{"id": switched.FromTokenID, "name": switched.FromTokenName}
		base["defaultSwitchedTo"] = map[string]any{"id": switched.ToTokenID, "name": switched.ToTokenName}
		base["message"] = base["message"].(string) + fmt.Sprintf(
			"; default relay token switched: %q is no longer listed upstream, now using %q",
			switched.FromTokenName, switched.ToTokenName,
		)
		// The account's relay credential moved without the operator clicking
		// anything. That is the fact they need after an upstream key rotation, so
		// it gets its own event at warning instead of folding into a routine
		// success line.
		eventTitle = "Account token sync switched the default relay token"
		eventLevel = "warning"
	} else if reason := syncResult.DefaultSwitchSkipped; reason != "" {
		base["defaultSwitchSkipped"] = reason
	}

	_ = service.CreateEvent(
		db.DB,
		"token_sync",
		eventTitle,
		base["message"].(string),
		eventLevel,
		accountID,
		"account",
	)
	return base, nil
}

func executeSyncAllAccountTokens(ctx context.Context, db *store.DB, cfg *config.Config) (map[string]int, []map[string]any) {
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
				row, err := service.GetAccountWithSiteByID(db.DB, accountID)
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

// Post-create token sync statuses. These are reported truthfully in account
// create/login responses: never a fixed count, never a fake success.
const (
	postCreateSyncStatusSynced  = "synced"
	postCreateSyncStatusEmpty   = "empty"
	postCreateSyncStatusFailed  = "failed"
	postCreateSyncStatusSkipped = "skipped"
)

// postCreateTokenSyncReport is the truth-only outcome of the automatic token
// sync that runs after a session-credential account is created or rebound.
type postCreateTokenSyncReport struct {
	Status     string
	TokenCount int
	Message    string
}

// syncTokensAfterAccountCreate runs the existing account token sync path for a
// freshly created or rebound session-credential account (#1002). It never
// fails the surrounding create/login request: a sync failure is reported as a
// partial-initialization warning while the already-persisted account is kept.
func syncTokensAfterAccountCreate(ctx context.Context, db *store.DB, cfg *config.Config, accountID int64) postCreateTokenSyncReport {
	row, err := service.GetAccountWithSiteByID(db.DB, accountID)
	if err != nil {
		return postCreateTokenSyncReport{
			Status:     postCreateSyncStatusSkipped,
			TokenCount: countPersistedAccountTokens(db, accountID),
			Message:    "account row not found after create; token sync skipped",
		}
	}

	// Product decision: only session-credential accounts sync
	// automatically; API-key connections are explicitly skipped.
	if service.IsAPIKeyConnection(&row.Account) {
		return postCreateTokenSyncReport{
			Status:     postCreateSyncStatusSkipped,
			TokenCount: countPersistedAccountTokens(db, accountID),
			Message:    "API key connection; token sync skipped",
		}
	}

	result, syncErr := executeAccountTokenSync(ctx, db, cfg, row)
	tokenCount := countPersistedAccountTokens(db, accountID)
	if syncErr != nil {
		// Partial initialization: keep the verified, persisted account and
		// report the failure honestly. Never roll back the account here.
		slog.Warn("Post-create token sync failed; keeping persisted account",
			"err", syncErr, "account_id", accountID)
		return postCreateTokenSyncReport{
			Status:     postCreateSyncStatusFailed,
			TokenCount: tokenCount,
			Message:    "partial initialization: account created, but token sync failed: " + syncErr.Error(),
		}
	}

	status, _ := result["status"].(string)
	message, _ := result["message"].(string)
	if status == "synced" {
		return postCreateTokenSyncReport{Status: postCreateSyncStatusSynced, TokenCount: tokenCount, Message: message}
	}
	if status == "skipped" {
		if reason, _ := result["reason"].(string); reason == "no_upstream_tokens" {
			return postCreateTokenSyncReport{Status: postCreateSyncStatusEmpty, TokenCount: tokenCount, Message: message}
		}
	}
	return postCreateTokenSyncReport{Status: postCreateSyncStatusSkipped, TokenCount: tokenCount, Message: message}
}

// countPersistedAccountTokens returns the real persisted account_tokens row
// count for an account. Any read error reports 0: never a fake count.
func countPersistedAccountTokens(db *store.DB, accountID int64) int {
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM account_tokens WHERE account_id = ?", accountID); err != nil {
		return 0
	}
	return count
}
