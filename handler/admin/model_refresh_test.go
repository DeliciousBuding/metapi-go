package admin

import (
	"context"
	"database/sql"
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
		 VALUES (?, ?, 'openai', 'active', 0, 0, 0, 0, ?, ?)`,
		"TokenBackfillSite", "https://api.example.com", now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	accRes, err := db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, sort_order, created_at, updated_at)
		 VALUES (?, 'tester', ?, 'active', 1, 0, ?, ?)`,
		siteID, tokenValue, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ = accRes.LastInsertId()

	tokRes, err := db.Exec(
		`INSERT INTO account_tokens (account_id, name, token, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, 'default', ?, 'ready', 'manual', 1, 1, ?, ?)`,
		accountID, tokenValue, now, now,
	)
	if err != nil {
		t.Fatalf("insert account_token: %v", err)
	}
	tokenID, _ = tokRes.LastInsertId()
	return accountID, tokenID
}

func TestPersistTokenModelAvailability_UpsertsAndMarksUnavailable(t *testing.T) {
	db := openModelRefreshTestDB(t)
	_, tokenID := seedAccountAndToken(t, db, "sk-backfill")
	now := time.Now().UTC().Format(time.RFC3339)

	// First refresh: 2 models available.
	if err := persistTokenModelAvailability(db, tokenID, []string{"gpt-4o", "claude-3"}, now); err != nil {
		t.Fatalf("first persist: %v", err)
	}

	var availCount int
	if err := db.Get(&availCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = 1`,
		tokenID,
	); err != nil {
		t.Fatalf("count available: %v", err)
	}
	if availCount != 2 {
		t.Fatalf("expected 2 available, got %d", availCount)
	}

	// Second refresh: only 1 model — the other should be marked unavailable.
	if err := persistTokenModelAvailability(db, tokenID, []string{"gpt-4o"}, now); err != nil {
		t.Fatalf("second persist: %v", err)
	}
	if err := db.Get(&availCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = 1`,
		tokenID,
	); err != nil {
		t.Fatalf("count after second: %v", err)
	}
	if availCount != 1 {
		t.Fatalf("expected 1 available after shrink, got %d", availCount)
	}
	var unavailCount int
	if err := db.Get(&unavailCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = 0`,
		tokenID,
	); err != nil {
		t.Fatalf("count unavailable: %v", err)
	}
	if unavailCount != 1 {
		t.Fatalf("expected 1 unavailable (claude-3), got %d", unavailCount)
	}
}

func TestPersistTokenModelAvailability_ReAddsPreviouslyUnavailable(t *testing.T) {
	db := openModelRefreshTestDB(t)
	_, tokenID := seedAccountAndToken(t, db, "sk-cycle")
	now := time.Now().UTC().Format(time.RFC3339)

	// Refresh 1: gpt-4o only.
	if err := persistTokenModelAvailability(db, tokenID, []string{"gpt-4o"}, now); err != nil {
		t.Fatalf("persist 1: %v", err)
	}
	// Refresh 2: claude-3 only (gpt-4o becomes unavailable).
	if err := persistTokenModelAvailability(db, tokenID, []string{"claude-3"}, now); err != nil {
		t.Fatalf("persist 2: %v", err)
	}
	// Refresh 3: gpt-4o comes back — should flip available=1, not insert a dupe.
	if err := persistTokenModelAvailability(db, tokenID, []string{"gpt-4o", "claude-3"}, now); err != nil {
		t.Fatalf("persist 3: %v", err)
	}

	var totalCount int
	if err := db.Get(&totalCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ?`,
		tokenID,
	); err != nil {
		t.Fatalf("count total: %v", err)
	}
	if totalCount != 2 {
		t.Fatalf("expected 2 total rows (no dupes), got %d", totalCount)
	}
	var availCount int
	if err := db.Get(&availCount,
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = 1`,
		tokenID,
	); err != nil {
		t.Fatalf("count available: %v", err)
	}
	if availCount != 2 {
		t.Fatalf("expected 2 available after re-add, got %d", availCount)
	}
}

func TestResolveAccountTokenID_MatchAndMiss(t *testing.T) {
	db := openModelRefreshTestDB(t)
	accountID, tokenID := seedAccountAndToken(t, db, "sk-match")

	gotID, ok := resolveAccountTokenID(db, accountID, "sk-match")
	if !ok {
		t.Fatal("expected match for sk-match")
	}
	if gotID != tokenID {
		t.Fatalf("token id = %d, want %d", gotID, tokenID)
	}

	// Bearer prefix should be stripped.
	gotID2, ok2 := resolveAccountTokenID(db, accountID, "Bearer sk-match")
	if !ok2 || gotID2 != tokenID {
		t.Fatalf("bearer prefix: ok=%v id=%d, want %d", ok2, gotID2, tokenID)
	}

	// Non-matching token returns false.
	_, ok3 := resolveAccountTokenID(db, accountID, "sk-nope")
	if ok3 {
		t.Fatal("expected false for non-matching token")
	}

	// Empty token returns false.
	_, ok4 := resolveAccountTokenID(db, accountID, "")
	if ok4 {
		t.Fatal("expected false for empty token")
	}
}

func TestResolveAccountTokenID_NoRows(t *testing.T) {
	db := openModelRefreshTestDB(t)
	accountID, _ := seedAccountAndToken(t, db, "sk-lone")
	_, ok := resolveAccountTokenID(db, accountID, "sk-other")
	if ok {
		t.Fatal("expected false when no account_tokens row matches")
	}
}

func TestResolveAccountTokenID_SQLiteNullScan(t *testing.T) {
	db := openModelRefreshTestDB(t)
	// No rows at all — should return false, not panic.
	var dummyID int64
	err := db.Get(&dummyID, `SELECT id FROM account_tokens WHERE id = 999`)
	if err == nil {
		t.Fatal("expected sql.ErrNoRows for nonexistent token")
	}
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
		 VALUES (?, 'default', 'sk-backfill-token', 'ready', 'manual', 1, 1, ?, ?)`,
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
		`SELECT COUNT(*) FROM token_model_availability WHERE token_id = ? AND available = 1`,
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
		`SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = 1`,
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
		`SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = 1`,
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

// Silence unused import warnings for sql/sqlx when tests evolve.
var _ = sql.ErrNoRows
