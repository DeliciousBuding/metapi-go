package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/deliciousbuding/metapi-go/store"
)

// openModelRefreshTestDB opens an in-memory SQLite DB with the full schema
// migrated, ready for model_availability / token_model_availability tests.
func openModelRefreshTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := store.AutoMigrate(db); err != nil {
		db.Close()
		t.Fatalf("automigrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db.DB
}

// seedAccountAndToken inserts a site, account, and an account_tokens row,
// returning the accountID and tokenID for backfill tests.
func seedAccountAndToken(t *testing.T, db *sqlx.DB, tokenValue string) (accountID, tokenID int64) {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, use_system_proxy, sort_order, global_weight, post_refresh_probe_enabled, created_at, updated_at)
		 VALUES (?, ?, 'openai', 'active', FALSE, 0, 0, FALSE, ?, ?)`,
		"TokenBackfillSite", "https://api.example.com", now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	accRes, err := db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, sort_order, created_at, updated_at)
		 VALUES (?, 'tester', ?, 'active', TRUE, 0, ?, ?)`,
		siteID, tokenValue, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ = accRes.LastInsertId()

	tokRes, err := db.Exec(
		`INSERT INTO account_tokens (account_id, name, token, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, 'default', ?, 'ready', 'manual', TRUE, TRUE, ?, ?)`,
		accountID, tokenValue, now, now,
	)
	if err != nil {
		t.Fatalf("insert account_token: %v", err)
	}
	tokenID, _ = tokRes.LastInsertId()
	return accountID, tokenID
}

// TestRefreshAccountModels_TokenBackfillEndToEnd verifies the full
// refreshAccountModels path writes token_model_availability when the resolved
// token matches an account_tokens row.
func TestRefreshAccountModels_TokenBackfillEndToEnd(t *testing.T) {
	dbConn, r, _ := setupAccountsTest(t)

	// httptest OpenAI server that returns models for sk-backfill-token.
	server := newOpenAIModelsServer(t, "sk-backfill-token", []string{"gpt-4o", "gpt-4o-mini"})

	siteResp := doPostJSON(t, r, "/api/sites", map[string]any{
		"name":     "TokenBackfillE2E",
		"url":      server.URL,
		"platform": "openai",
	})
	if siteResp.Code != http.StatusOK {
		t.Fatalf("create site: %d %s", siteResp.Code, siteResp.Body.String())
	}
	var site map[string]any
	_ = json.Unmarshal(siteResp.Body.Bytes(), &site)
	siteID := int64(site["id"].(float64))

	createResp := doPostJSON(t, r, "/api/accounts", map[string]any{
		"siteId":         siteID,
		"accessToken":    "sk-backfill-token",
		"credentialMode": "apikey",
		"skipModelFetch": true,
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create account: %d %s", createResp.Code, createResp.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(createResp.Body.Bytes(), &created)
	accountID := int64(created["id"].(float64))

	// Seed an account_tokens row matching the resolved token so backfill can fire.
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := dbConn.Exec(
		`INSERT INTO account_tokens (account_id, name, token, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, 'default', 'sk-backfill-token', 'ready', 'manual', TRUE, TRUE, ?, ?)`,
		accountID, now, now,
	)
	if err != nil {
		t.Fatalf("seed account_token: %v", err)
	}
	tokenID, _ := res.LastInsertId()

	// Run the real refresh.
	result := refreshAccountModels(context.Background(), dbConn.DB, accountID, false)
	if !modelRefreshSucceeded(result) {
		t.Fatalf("refresh failed: %s", modelRefreshErrorMessage(result))
	}
	if backfilled, _ := result["tokenBackfilled"].(bool); !backfilled {
		t.Fatal("expected tokenBackfilled=true, got false")
	}

	// Verify token_model_availability rows were written.
	var availCount int
	if err := dbConn.Get(&availCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = TRUE`,
		tokenID,
	); err != nil {
		t.Fatalf("count token models: %v", err)
	}
	if availCount != 2 {
		t.Fatalf("expected 2 available token_model_availability rows, got %d", availCount)
	}

	// Also verify account-level model_availability was written.
	var accountModelCount int
	if err := dbConn.Get(&accountModelCount,
		`SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = TRUE`,
		accountID,
	); err != nil {
		t.Fatalf("count account models: %v", err)
	}
	if accountModelCount != 2 {
		t.Fatalf("expected 2 available model_availability rows, got %d", accountModelCount)
	}
}

// TestRefreshAccountModels_TokenBackfillSkipsWhenNoTokenMatch verifies the
// backfill is silently skipped (tokenBackfilled=false) when no account_tokens
// row matches the resolved token — account-level availability still works.
func TestRefreshAccountModels_TokenBackfillSkipsWhenNoTokenMatch(t *testing.T) {
	dbConn, r, _ := setupAccountsTest(t)
	server := newOpenAIModelsServer(t, "sk-no-token-row", []string{"gpt-4o"})

	siteResp := doPostJSON(t, r, "/api/sites", map[string]any{
		"name":     "TokenBackfillNoMatch",
		"url":      server.URL,
		"platform": "openai",
	})
	if siteResp.Code != http.StatusOK {
		t.Fatalf("create site: %d %s", siteResp.Code, siteResp.Body.String())
	}
	var site map[string]any
	_ = json.Unmarshal(siteResp.Body.Bytes(), &site)
	siteID := int64(site["id"].(float64))

	createResp := doPostJSON(t, r, "/api/accounts", map[string]any{
		"siteId":         siteID,
		"accessToken":    "sk-no-token-row",
		"credentialMode": "apikey",
		"skipModelFetch": true,
	})
	if createResp.Code != http.StatusOK {
		t.Fatalf("create: %d %s", createResp.Code, createResp.Body.String())
	}
	var created map[string]any
	_ = json.Unmarshal(createResp.Body.Bytes(), &created)
	accountID := int64(created["id"].(float64))

	// No account_tokens row seeded — backfill should skip.
	result := refreshAccountModels(context.Background(), dbConn.DB, accountID, false)
	if !modelRefreshSucceeded(result) {
		t.Fatalf("refresh failed: %s", modelRefreshErrorMessage(result))
	}
	if backfilled, _ := result["tokenBackfilled"].(bool); backfilled {
		t.Fatal("expected tokenBackfilled=false when no token match, got true")
	}

	// Account-level should still be written.
	var accountModelCount int
	if err := dbConn.Get(&accountModelCount,
		`SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = TRUE`,
		accountID,
	); err != nil {
		t.Fatalf("count account models: %v", err)
	}
	if accountModelCount != 1 {
		t.Fatalf("expected 1 available model_availability row, got %d", accountModelCount)
	}

	// token_model_availability should be empty.
	var tokenModelCount int
	if err := dbConn.Get(&tokenModelCount,
		`SELECT COUNT(*) FROM token_model_availability`,
	); err != nil {
		t.Fatalf("count token models: %v", err)
	}
	if tokenModelCount != 0 {
		t.Fatalf("expected 0 token_model_availability rows, got %d", tokenModelCount)
	}
}

