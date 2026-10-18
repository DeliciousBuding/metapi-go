package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestSearchResponseStripsExtraConfigCredentials is the HTTP-level guard for
// H7's search half: POST /api/search must apply the same credential policy as
// GET /api/accounts, including nested extraConfig credentials, and must be
// uncacheable.
func TestSearchResponseStripsExtraConfigCredentials(t *testing.T) {
	db, r := setupCredsTest(t)
	_, _, _, _, accountID, _ := seedCredentialedAccount(t, db)

	extraConfig := `{"credentialMode":"session","proxyUrl":"http://proxy.example:8080",` +
		`"oauth":{"provider":"codex","email":"operator@example.com","refreshToken":"synthetic-search-refresh",` +
		`"idToken":"synthetic-search-id","sessionToken":"synthetic-search-session","tokenExpiresAt":1760000000000},` +
		`"sub2apiAuth":{"refreshToken":"synthetic-search-sub2api","tokenExpiresAt":1760000000000},` +
		`"autoRelogin":{"username":"operator@example.com","passwordCipher":"synthetic-search-cipher"},` +
		`"apiKey":"synthetic-search-api-key"}`
	if _, err := db.Exec("UPDATE accounts SET extra_config = ? WHERE id = ?", extraConfig, accountID); err != nil {
		t.Fatalf("seed extra_config: %v", err)
	}
	const siteAPIKey = "sk-SEARCH-SITE-PLAINTEXT-1234"
	const siteCFClearance = "cf-SEARCH-CLEARANCE-PLAINTEXT-5678"
	if _, err := db.Exec(
		"UPDATE sites SET api_key = ?, cf_clearance = ?",
		siteAPIKey, siteCFClearance,
	); err != nil {
		t.Fatalf("seed site credentials: %v", err)
	}

	resp := doPostJSON(t, r, "/api/search", map[string]any{"query": "cred"})
	if resp.Code != http.StatusOK {
		t.Fatalf("POST /api/search: %d %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Cache-Control"); got != noStoreCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, noStoreCacheControl)
	}

	for _, secret := range []string{
		"synthetic-search-refresh",
		"synthetic-search-id",
		"synthetic-search-session",
		"synthetic-search-sub2api",
		"synthetic-search-cipher",
		"synthetic-search-api-key",
		"FAKE-access-PLAINTEXT",
		"FAKE-api-PLAINTEXT",
		siteAPIKey,
		siteCFClearance,
	} {
		if strings.Contains(resp.Body.String(), secret) {
			t.Fatalf("search response leaked %q: %s", secret, resp.Body.String())
		}
	}

	var payload struct {
		Accounts []map[string]any `json:"accounts"`
		Sites    []map[string]any `json:"sites"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload.Sites) != 1 {
		t.Fatalf("sites = %d, want 1", len(payload.Sites))
	}
	if _, ok := payload.Sites[0]["cfClearance"]; ok {
		t.Fatalf("search site row carries cfClearance: %#v", payload.Sites[0])
	}
	if payload.Sites[0]["hasApiKey"] != true || payload.Sites[0]["apiKeyMasked"] != "sk-S****1234" {
		t.Fatalf("search site row projection = %#v", payload.Sites[0])
	}
	if len(payload.Accounts) != 1 {
		t.Fatalf("accounts = %d, want 1", len(payload.Accounts))
	}
	account := payload.Accounts[0]
	if account["accessTokenMasked"] == nil || account["apiTokenMasked"] == nil {
		t.Fatalf("masked projections missing: %#v", account)
	}
	ec, _ := account["extraConfig"].(string)
	var nested map[string]any
	if err := json.Unmarshal([]byte(ec), &nested); err != nil {
		t.Fatalf("extraConfig is no longer valid JSON: %v", err)
	}
	if nested["proxyUrl"] != "http://proxy.example:8080" || nested["credentialMode"] != "session" {
		t.Fatalf("non-credential extraConfig was dropped: %#v", nested)
	}
	oauth, _ := nested["oauth"].(map[string]any)
	if oauth["provider"] != "codex" || oauth["email"] != "operator@example.com" || oauth["tokenExpiresAt"] != float64(1760000000000) {
		t.Fatalf("oauth display metadata was dropped: %#v", nested)
	}
}
