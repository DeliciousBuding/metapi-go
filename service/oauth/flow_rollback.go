package oauth

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// accountSnapshot captures the state of an account before mutation for rollback.
type accountSnapshot struct {
	account *store.Account
	// modelAvailability is captured as a raw row list for rollback.
	modelAvailability []map[string]interface{}
}

// captureAccountSnapshot captures the current state of an account for rollback.
func captureAccountSnapshot(db *store.DB, account *store.Account) *accountSnapshot {
	snap := &accountSnapshot{
		account: account,
	}
	// Capture model availability rows.
	rows, err := db.Queryx("SELECT * FROM model_availability WHERE account_id = ?", account.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			row := make(map[string]interface{})
			if err := rows.MapScan(row); err == nil {
				snap.modelAvailability = append(snap.modelAvailability, row)
			}
		}
	}
	return snap
}

// revertPersistedOauthAccount rolls back a persisted OAuth account to its previous state.
// If created=true, deletes the account. If created=false, restores previous fields only
// when the stored row still matches the version written by this attempt (expectedAccessToken
// and expectedExtraConfig), preventing a stale failed rebind from clobbering a newer commit.
func revertPersistedOauthAccount(db *store.DB, accountID int64, created bool, snapshot *accountSnapshot, expectedAccessToken string, expectedExtraConfig *string) error {
	if created {
		// Delete the newly created account.
		_, err := db.Exec("DELETE FROM accounts WHERE id = ?", accountID)
		return err
	}

	if snapshot == nil || snapshot.account == nil {
		return fmt.Errorf("no snapshot available for rollback")
	}

	prev := snapshot.account
	now := time.Now().Format(time.RFC3339)

	// Restore previous fields in a transaction, fenced by our written version.
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var res sql.Result
	if expectedExtraConfig == nil {
		res, err = tx.Exec(tx.Rebind(
			`UPDATE accounts SET site_id = ?, username = ?, access_token = ?, api_token = ?,
			 checkin_enabled = ?, status = ?, oauth_provider = ?, oauth_account_key = ?,
			 oauth_project_id = ?, extra_config = ?, updated_at = ?
			 WHERE id = ? AND access_token = ? AND extra_config IS NULL`),
			prev.SiteID, prev.Username, prev.AccessToken, prev.APIToken,
			prev.CheckinEnabled, prev.Status, prev.OAuthProvider, prev.OAuthAccountKey,
			prev.OAuthProjectID, prev.ExtraConfig, now, accountID, expectedAccessToken,
		)
	} else {
		res, err = tx.Exec(tx.Rebind(
			`UPDATE accounts SET site_id = ?, username = ?, access_token = ?, api_token = ?,
			 checkin_enabled = ?, status = ?, oauth_provider = ?, oauth_account_key = ?,
			 oauth_project_id = ?, extra_config = ?, updated_at = ?
			 WHERE id = ? AND access_token = ? AND extra_config = ?`),
			prev.SiteID, prev.Username, prev.AccessToken, prev.APIToken,
			prev.CheckinEnabled, prev.Status, prev.OAuthProvider, prev.OAuthAccountKey,
			prev.OAuthProjectID, prev.ExtraConfig, now, accountID, expectedAccessToken, *expectedExtraConfig,
		)
	}
	if err != nil {
		return fmt.Errorf("rollback account update failed: %w", err)
	}
	if affected, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("rollback account rows affected failed: %w", err)
	} else if affected == 0 {
		// A newer rebind/refresh superseded this attempt; leave the newer state intact.
		slog.Warn("skipping oauth account rollback because a newer write superseded this attempt", "accountID", accountID)
		return nil
	}

	// Restore model availability: delete current, re-insert previous.
	_, err = tx.Exec(tx.Rebind("DELETE FROM model_availability WHERE account_id = ?"), accountID)
	if err != nil {
		return fmt.Errorf("rollback model_availability delete failed: %w", err)
	}

	for _, row := range snapshot.modelAvailability {
		// Build a safe insert from the snapshot.
		cols := make([]string, 0, len(row))
		vals := make([]interface{}, 0, len(row))
		placeholders := make([]string, 0, len(row))
		for k, v := range row {
			cols = append(cols, k)
			vals = append(vals, v)
			placeholders = append(placeholders, "?")
		}
		if len(cols) > 0 {
			query := fmt.Sprintf("INSERT INTO model_availability (%s) VALUES (%s)",
				strings.Join(cols, ", "), strings.Join(placeholders, ", "))
			_, err := tx.Exec(tx.Rebind(query), vals...)
			if err != nil {
				slog.Warn("rollback model_availability re-insert failed", "error", err)
			}
		}
	}

	return tx.Commit()
}
