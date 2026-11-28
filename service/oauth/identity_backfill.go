package oauth

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// oauthIdentityBackfillMarkerKey is the settings KV flag that marks the
// one-time OAuth identity column backfill as complete. Once set, a healthy
// instance never re-scans the accounts table for backfill candidates.
const oauthIdentityBackfillMarkerKey = "oauth.identity_backfill_complete"

// RunOauthIdentityBackfillOnce is a one-time startup migration that backfills
// the oauth_provider, oauth_account_key, and oauth_project_id columns from
// extraConfig.oauth for accounts that still carry the data only in extra_config.
//
// It is bounded (LIMIT-batched) and gated behind a settings marker so a healthy
// instance never re-scans accounts after the first successful run. A partial
// run (batch error) leaves the marker unset and retries on the next boot.
//
// This was previously invoked from ListOauthConnections on every page load,
// which forced a full-table scan + N+1 UPDATEs per request on the admin list
// hot path. Moving it to startup keeps the list endpoint O(limit).
func RunOauthIdentityBackfillOnce(db *store.DB) {
	settings := store.NewSettingsStore(db)
	if done, _ := settings.Get(oauthIdentityBackfillMarkerKey); strings.TrimSpace(done) == "1" {
		return
	}

	for {
		backfilled, err := backfillOauthIdentityBatch(db, 100)
		if err != nil {
			slog.Warn("oauth identity backfill batch failed; will retry on next boot", "error", err)
			return
		}
		if backfilled == 0 {
			break
		}
	}

	if err := settings.Set(oauthIdentityBackfillMarkerKey, "1"); err != nil {
		slog.Warn("oauth identity backfill marker could not be set; will re-run on next boot", "error", err)
		return
	}
	slog.Info("oauth identity backfill complete; settings marker set")
}

// backfillOauthIdentityBatch scans up to limit accounts whose oauth identity
// columns are still NULL/empty while extra_config carries oauth data, and
// updates them in place. Returns the number of rows updated. Row-level scan
// and update failures are logged and skipped so a single bad row cannot
// abort the whole migration; a query/scan failure (the whole batch) is
// returned so the caller retries on the next boot.
func backfillOauthIdentityBatch(db *store.DB, limit int) (int, error) {
	rows, err := db.Queryx(
		`SELECT id, oauth_provider, oauth_account_key, oauth_project_id, extra_config
		 FROM accounts
		 WHERE extra_config IS NOT NULL AND extra_config != ''
		   AND (oauth_provider IS NULL OR oauth_provider = ''
		        OR oauth_account_key IS NULL OR oauth_account_key = ''
		        OR oauth_project_id IS NULL OR oauth_project_id = '')
		 LIMIT ?`, limit)
	if err != nil {
		return 0, fmt.Errorf("scan oauth identity backfill candidates: %w", err)
	}
	defer rows.Close()

	type backfillRow struct {
		ID              int64   `db:"id"`
		OAuthProvider   *string `db:"oauth_provider"`
		OAuthAccountKey *string `db:"oauth_account_key"`
		OAuthProjectID  *string `db:"oauth_project_id"`
		ExtraConfig     *string `db:"extra_config"`
	}

	updated := 0
	for rows.Next() {
		var row backfillRow
		if err := rows.StructScan(&row); err != nil {
			slog.Warn("oauth identity backfill: row scan failed; skipping", "accountID", row.ID, "error", err)
			continue
		}

		oauth := GetOauthInfoFromExtraConfig(row.ExtraConfig)
		if oauth == nil {
			continue
		}

		provider := ""
		accountKey := ""
		projectID := ""
		needsUpdate := false
		if (row.OAuthProvider == nil || *row.OAuthProvider == "") && oauth.Provider != "" {
			provider = oauth.Provider
			needsUpdate = true
		}
		if (row.OAuthAccountKey == nil || *row.OAuthAccountKey == "") && oauth.AccountKey != "" {
			accountKey = oauth.AccountKey
			needsUpdate = true
		}
		if (row.OAuthProjectID == nil || *row.OAuthProjectID == "") && oauth.ProjectID != "" {
			projectID = oauth.ProjectID
			needsUpdate = true
		}
		if !needsUpdate {
			continue
		}

		now := time.Now().Format(time.RFC3339)
		if _, err := db.Exec(
			`UPDATE accounts SET oauth_provider = COALESCE(NULLIF(?, ''), oauth_provider),
			 oauth_account_key = COALESCE(NULLIF(?, ''), oauth_account_key),
			 oauth_project_id = COALESCE(NULLIF(?, ''), oauth_project_id),
			 updated_at = ?
			 WHERE id = ?`,
			provider, accountKey, projectID, now, row.ID); err != nil {
			slog.Warn("oauth identity backfill: update failed for account", "accountID", row.ID, "error", err)
			continue
		}
		updated++
	}
	return updated, nil
}
