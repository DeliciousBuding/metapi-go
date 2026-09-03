package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// #1176 — the account edit form sends credentialMode on every save and the
// update handler dropped the field: switching an existing API-key account to
// session mode reported success, but reopening the dialog still showed API key
// because extraConfig.credentialMode was never written. These tests pin the
// persisted mode plus the fail-loud behaviour when the stored credential cannot
// back the requested mode.

// setupCredentialModeAccountFixture seeds one site + one account whose stored
// credential shape is explicit, so a mode switch has something to switch from.
func setupCredentialModeAccountFixture(t *testing.T, db *store.DB, r chi.Router, extraConfig, accessToken string, apiToken *string) (int64, int64) {
	t.Helper()
	siteResp := doPostJSON(t, r, "/api/sites", map[string]any{
		"name":     "Credential Mode Site",
		"url":      "https://credmode.example.com",
		"platform": "new-api",
	})
	if siteResp.Code != http.StatusOK {
		t.Fatalf("create site: %d %s", siteResp.Code, siteResp.Body.String())
	}
	var site map[string]any
	if err := json.Unmarshal(siteResp.Body.Bytes(), &site); err != nil {
		t.Fatalf("decode site: %v", err)
	}
	siteID := int64(site["id"].(float64))

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, extra_config, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', FALSE, ?, ?, ?)",
		siteID, "cred-user", accessToken, apiToken, extraConfig, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("account LastInsertId: %v", err)
	}
	return siteID, accountID
}

func storedCredentialMode(t *testing.T, db *store.DB, accountID int64) string {
	t.Helper()
	var extra *string
	if err := db.QueryRow("SELECT extra_config FROM accounts WHERE id = ?", accountID).Scan(&extra); err != nil {
		t.Fatalf("read extra_config: %v", err)
	}
	if extra == nil {
		return ""
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(*extra), &cfg); err != nil {
		t.Fatalf("decode extra_config %q: %v", *extra, err)
	}
	mode, _ := cfg["credentialMode"].(string)
	return mode
}

func TestAccounts_Update_PersistsRequestedCredentialMode(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	_, accountID := setupCredentialModeAccountFixture(t, db, r,
		`{"credentialMode":"apikey"}`, "", strPtrCredMode("sk-ant-key-1"))

	resp := doPutJSON(t, r, "/api/accounts/"+itoa(accountID), map[string]any{
		"credentialMode": "session",
		"accessToken":    "session-jwt-1",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("switch apikey -> session: %d %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body["credentialMode"]; got != "session" {
		t.Fatalf("response credentialMode = %#v, want session", got)
	}
	if got := storedCredentialMode(t, db, accountID); got != "session" {
		t.Fatalf("stored credentialMode = %q, want session", got)
	}

	// And back again: the api token supplied in the same save backs the mode.
	resp = doPutJSON(t, r, "/api/accounts/"+itoa(accountID), map[string]any{
		"credentialMode": "apikey",
		"apiToken":       "sk-ant-key-2",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("switch session -> apikey: %d %s", resp.Code, resp.Body.String())
	}
	if got := storedCredentialMode(t, db, accountID); got != "apikey" {
		t.Fatalf("stored credentialMode = %q, want apikey", got)
	}
}

func TestAccounts_Update_CredentialModeWithoutBackingCredentialIsRejected(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	_, accountID := setupCredentialModeAccountFixture(t, db, r,
		`{"credentialMode":"apikey"}`, "", strPtrCredMode("sk-ant-key-1"))

	// No session credential anywhere on the row: a silent switch would leave an
	// account that reports session mode while holding nothing to authenticate
	// with, so the handler must refuse instead of persisting a broken state.
	resp := doPutJSON(t, r, "/api/accounts/"+itoa(accountID), map[string]any{
		"credentialMode": "session",
	})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("session switch without credential: got %d, want 400 (%s)", resp.Code, resp.Body.String())
	}
	if got := storedCredentialMode(t, db, accountID); got != "apikey" {
		t.Fatalf("stored credentialMode = %q, want unchanged apikey", got)
	}
}

func TestAccounts_Update_InvalidCredentialModeIsRejected(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	_, accountID := setupCredentialModeAccountFixture(t, db, r,
		`{"credentialMode":"apikey"}`, "", strPtrCredMode("sk-ant-key-1"))

	for _, mode := range []string{"password", "auto", "browser", ""} {
		resp := doPutJSON(t, r, "/api/accounts/"+itoa(accountID), map[string]any{
			"credentialMode": mode,
			"accessToken":    "session-jwt-1",
		})
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("credentialMode %q: got %d, want 400 (%s)", mode, resp.Code, resp.Body.String())
		}
	}
	if got := storedCredentialMode(t, db, accountID); got != "apikey" {
		t.Fatalf("stored credentialMode = %q, want unchanged apikey", got)
	}
}

func TestAccounts_Update_OmittedCredentialModeLeavesStoredModeAlone(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	_, accountID := setupCredentialModeAccountFixture(t, db, r,
		`{"credentialMode":"session","proxyUrl":"http://proxy.local"}`, "session-jwt-1", nil)

	resp := doPutJSON(t, r, "/api/accounts/"+itoa(accountID), map[string]any{
		"remark": "no mode in this patch",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("partial update: %d %s", resp.Code, resp.Body.String())
	}
	if got := storedCredentialMode(t, db, accountID); got != "session" {
		t.Fatalf("stored credentialMode = %q, want preserved session", got)
	}
}

func strPtrCredMode(s string) *string { return &s }
