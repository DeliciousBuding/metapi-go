package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// setupCredsTest builds a router with both token routes (for /api/channels +
// /api/routes) and search routes (for /api/search) on a fresh in-memory DB.
func setupCredsTest(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	globalChannelsCache.clear()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := chi.NewRouter()
	RegisterTokenRoutesWithDeps(r, db.DB, TokenRoutesDeps{})
	RegisterSearchRoutes(r, db.DB)
	return db, r
}

// seedCredentialedAccount inserts a site + account carrying plaintext access_token
// and api_token, plus an account_token carrying a plaintext token. Returns the
// plaintext secrets so tests can assert they never appear in a response body.
func seedCredentialedAccount(t *testing.T, db *store.DB) (accessSecret, apiSecret, tokenSecret string, routeID, accountID, tokenID int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	accessSecret = "FAKE-access-PLAINTEXT-AAAA1111"
	apiSecret = "FAKE-api-PLAINTEXT-BBBB2222"
	tokenSecret = "FAKE-token-PLAINTEXT-CCCC3333"

	res, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('Cred Site', 'https://cred.example.com', 'openai', 'active', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := res.LastInsertId()

	res, err = db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, 'cred-user', ?, ?, 'active', TRUE, ?, ?)`,
		siteID, accessSecret, apiSecret, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ = res.LastInsertId()

	res, err = db.Exec(
		`INSERT INTO account_tokens (account_id, name, token, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, 'cred-token', ?, 'ready', 'manual', TRUE, TRUE, ?, ?)`,
		accountID, tokenSecret, now, now)
	if err != nil {
		t.Fatalf("insert account_token: %v", err)
	}
	tokenID, _ = res.LastInsertId()

	res, err = db.Exec(
		`INSERT INTO token_routes (model_pattern, enabled, created_at, updated_at) VALUES ('gpt-*', TRUE, ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	routeID, _ = res.LastInsertId()
	return
}

// TestListRoutes_ResponseOmitsPlaintextCredentials is the P1 security guard:
// GET /api/routes must never surface plaintext access_token/api_token in the
// response body — only the masked form rebuilt from length/prefix/suffix
// fragments may appear.
func TestListRoutes_ResponseOmitsPlaintextCredentials(t *testing.T) {
	db, r := setupCredsTest(t)
	accessSecret, apiSecret, _, routeID, accountID, tokenID := seedCredentialedAccount(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO route_channels (route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
		 VALUES (?, ?, ?, 'gpt-4o', 1, 10, TRUE, FALSE)`, routeID, accountID, tokenID); err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	_ = now

	rec := doGet(t, r, "/api/routes")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, accessSecret) {
		t.Fatalf("list routes leaked access_token plaintext: %s", body)
	}
	if strings.Contains(body, apiSecret) {
		t.Fatalf("list routes leaked api_token plaintext: %s", body)
	}

	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, route := range listed {
		if int64(route["id"].(float64)) != routeID {
			continue
		}
		chans, _ := route["channels"].([]any)
		if len(chans) != 1 {
			t.Fatalf("expected 1 channel, got %d", len(chans))
		}
		acc := chans[0].(map[string]any)["account"].(map[string]any)
		if _, ok := acc["accessToken"]; ok {
			t.Fatalf("accessToken key present: %#v", acc["accessToken"])
		}
		if _, ok := acc["apiToken"]; ok {
			t.Fatalf("apiToken key present: %#v", acc["apiToken"])
		}
		if acc["accessTokenMasked"] != maskSecret(accessSecret) {
			t.Fatalf("accessTokenMasked=%#v want %q", acc["accessTokenMasked"], maskSecret(accessSecret))
		}
		if acc["apiTokenMasked"] != maskSecret(apiSecret) {
			t.Fatalf("apiTokenMasked=%#v want %q", acc["apiTokenMasked"], maskSecret(apiSecret))
		}
	}
}

// TestGetRouteChannels_ResponseOmitsPlaintextCredentials guards the per-route
// channel list (GET /api/routes/:id/channels), which shares the same SELECT.
func TestGetRouteChannels_ResponseOmitsPlaintextCredentials(t *testing.T) {
	db, r := setupCredsTest(t)
	accessSecret, apiSecret, _, routeID, accountID, tokenID := seedCredentialedAccount(t, db)
	if _, err := db.Exec(
		`INSERT INTO route_channels (route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
		 VALUES (?, ?, ?, 'gpt-4o', 1, 10, TRUE, FALSE)`, routeID, accountID, tokenID); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	rec := doGet(t, r, "/api/routes/"+itoa(routeID)+"/channels")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, accessSecret) {
		t.Fatalf("getRouteChannels leaked access_token plaintext: %s", body)
	}
	if strings.Contains(body, apiSecret) {
		t.Fatalf("getRouteChannels leaked api_token plaintext: %s", body)
	}

	var chans []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &chans); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(chans) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(chans))
	}
	acc := chans[0]["account"].(map[string]any)
	if _, ok := acc["accessToken"]; ok {
		t.Fatalf("accessToken key present")
	}
	if acc["accessTokenMasked"] != maskSecret(accessSecret) {
		t.Fatalf("accessTokenMasked=%#v want %q", acc["accessTokenMasked"], maskSecret(accessSecret))
	}
	if acc["apiTokenMasked"] != maskSecret(apiSecret) {
		t.Fatalf("apiTokenMasked=%#v want %q", acc["apiTokenMasked"], maskSecret(apiSecret))
	}
}

// TestSearch_ResponseOmitsPlaintextCredentials guards POST /api/search: the
// accounts/account_tokens hits must never carry plaintext access_token,
// api_token, or token — only the masked forms rebuilt from fragments.
func TestSearch_ResponseOmitsPlaintextCredentials(t *testing.T) {
	db, r := setupCredsTest(t)
	accessSecret, apiSecret, tokenSecret, _, accountID, _ := seedCredentialedAccount(t, db)
	_ = accountID

	rec := doPostJSON(t, r, "/api/search", map[string]any{"query": "cred", "limit": 20})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, secret := range []string{accessSecret, apiSecret, tokenSecret} {
		if strings.Contains(body, secret) {
			t.Fatalf("search response leaked plaintext secret %q: %s", secret, body)
		}
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	accounts, _ := resp["accounts"].([]any)
	if len(accounts) == 0 {
		t.Fatalf("expected at least 1 account hit")
	}
	acc := accounts[0].(map[string]any)
	if _, ok := acc["accessToken"]; ok {
		t.Fatalf("account accessToken key present")
	}
	if _, ok := acc["apiToken"]; ok {
		t.Fatalf("account apiToken key present")
	}
	if acc["accessTokenMasked"] != maskSecret(accessSecret) {
		t.Fatalf("accessTokenMasked=%#v want %q", acc["accessTokenMasked"], maskSecret(accessSecret))
	}
	if acc["apiTokenMasked"] != maskSecret(apiSecret) {
		t.Fatalf("apiTokenMasked=%#v want %q", acc["apiTokenMasked"], maskSecret(apiSecret))
	}

	toks, _ := resp["accountTokens"].([]any)
	if len(toks) == 0 {
		t.Fatalf("expected at least 1 accountToken hit")
	}
	tok := toks[0].(map[string]any)
	if _, ok := tok["token"]; ok {
		t.Fatalf("accountToken token key present")
	}
	if tok["tokenMasked"] != maskSecret(tokenSecret) {
		t.Fatalf("tokenMasked=%#v want %q", tok["tokenMasked"], maskSecret(tokenSecret))
	}
}

// TestChannels_PaginationReturnsCorrectTotal verifies the COUNT(*) OVER ()
// total is accurate on every page, including the partial last page and a page
// past the end (which falls back to a plain COUNT(*)).
func TestChannels_PaginationReturnsCorrectTotal(t *testing.T) {
	db, r := setupCredsTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	now := time.Now().UTC().Format(time.RFC3339)
	_ = now

	// Insert 5 channels with distinct source models.
	for i := 0; i < 5; i++ {
		model := "gpt-pg-" + itoa(int64(i+1))
		if _, err := db.Exec(
			`INSERT INTO route_channels (route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
			 VALUES (?, ?, ?, ?, 0, 10, TRUE, FALSE)`, routeID, accountID, tokenID, model); err != nil {
			t.Fatalf("insert channel %d: %v", i, err)
		}
	}

	pageSize := 2
	pages := []struct {
		page, wantItems, wantTotal int
	}{
		{1, 2, 5},
		{2, 2, 5},
		{3, 1, 5},
		{4, 0, 5}, // past the end — total must still be 5 via fallback COUNT(*)
	}
	for _, tc := range pages {
		req := httptest.NewRequest(http.MethodGet, "/api/channels?page="+itoa(int64(tc.page))+"&pageSize="+itoa(int64(pageSize)), nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("page %d: status=%d body=%s", tc.page, rec.Code, rec.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("page %d decode: %v", tc.page, err)
		}
		if resp["total"] != float64(tc.wantTotal) {
			t.Fatalf("page %d total=%v want %d", tc.page, resp["total"], tc.wantTotal)
		}
		if resp["page"] != float64(tc.page) {
			t.Fatalf("page %d echoed=%v", tc.page, resp["page"])
		}
		if resp["pageSize"] != float64(pageSize) {
			t.Fatalf("page %d pageSize=%v want %d", tc.page, resp["pageSize"], pageSize)
		}
		items, _ := resp["items"].([]any)
		if len(items) != tc.wantItems {
			t.Fatalf("page %d items=%d want %d", tc.page, len(items), tc.wantItems)
		}
	}
}

// TestChannels_PageSizeClampedToMax verifies pageSize is clamped to the 200 cap.
func TestChannels_PageSizeClampedToMax(t *testing.T) {
	_, r := setupCredsTest(t)
	rec := doGet(t, r, "/api/channels?pageSize=9999")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["pageSize"] != float64(200) {
		t.Fatalf("pageSize=%v want 200 (clamped)", resp["pageSize"])
	}
}

// TestChannels_SnapshotCacheHitMissAndBypass verifies the 10s TTL snapshot
// cache: first request is a miss (populates), second is a hit (same bytes),
// and ?refresh=true forces a miss.
func TestChannels_SnapshotCacheHitMissAndBypass(t *testing.T) {
	_, r := setupCredsTest(t)

	first := doGet(t, r, "/api/channels")
	if first.Header().Get("x-channels-snapshot-cache") != "miss" {
		t.Fatalf("first request cache header=%q want miss", first.Header().Get("x-channels-snapshot-cache"))
	}

	second := doGet(t, r, "/api/channels")
	if second.Header().Get("x-channels-snapshot-cache") != "hit" {
		t.Fatalf("second request cache header=%q want hit", second.Header().Get("x-channels-snapshot-cache"))
	}
	if second.Body.String() != first.Body.String() {
		t.Fatalf("cached body differs from fresh body")
	}

	refreshed := doGet(t, r, "/api/channels?refresh=true")
	if refreshed.Header().Get("x-channels-snapshot-cache") != "miss" {
		t.Fatalf("refresh request cache header=%q want miss", refreshed.Header().Get("x-channels-snapshot-cache"))
	}
}

// TestChannels_MutationInvalidatesSnapshotCache verifies that a route_channels
// mutation clears the snapshot cache so the next list reflects the new row
// immediately (not up to the 10s TTL later).
func TestChannels_MutationInvalidatesSnapshotCache(t *testing.T) {
	db, r := setupCredsTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)

	before := doGet(t, r, "/api/channels")
	if before.Header().Get("x-channels-snapshot-cache") != "miss" {
		t.Fatalf("first request should be a miss")
	}
	var beforeResp map[string]any
	_ = json.Unmarshal(before.Body.Bytes(), &beforeResp)
	beforeItems, _ := beforeResp["items"].([]any)
	if len(beforeItems) != 0 {
		t.Fatalf("expected 0 channels before mutation, got %d", len(beforeItems))
	}

	// Add a channel — must invalidate the cache.
	add := doPostJSON(t, r, "/api/routes/"+itoa(routeID)+"/channels", map[string]any{
		"accountId":   accountID,
		"tokenId":     tokenID,
		"sourceModel": "gpt-4o",
		"priority":    1,
		"weight":      10,
	})
	if add.Code != http.StatusOK {
		t.Fatalf("add channel: %d %s", add.Code, add.Body.String())
	}

	after := doGet(t, r, "/api/channels")
	if after.Header().Get("x-channels-snapshot-cache") != "miss" {
		t.Fatalf("post-mutation request should be a miss (cache invalidated), got %q", after.Header().Get("x-channels-snapshot-cache"))
	}
	var afterResp map[string]any
	_ = json.Unmarshal(after.Body.Bytes(), &afterResp)
	afterItems, _ := afterResp["items"].([]any)
	if len(afterItems) != 1 {
		t.Fatalf("expected 1 channel after mutation, got %d", len(afterItems))
	}
}

// TestMaskSecretFromFragments_ParityWithMaskSecret is a table test asserting
// that rebuilding the masked form from prefix/suffix/length fragments produces
// byte-identical output to maskSecret(plaintext) across length boundaries.
func TestMaskSecretFromFragments_ParityWithMaskSecret(t *testing.T) {
	secrets := []string{
		"",
		"a",
		"ab",
		"abcd1234",  // exactly 8 -> "****"
		"abcd12345", // 9 -> first4+****+last4
		"FAKE-access-secret-ABCDEF",
		"FAKE-api-secret-XYZ12345",
	}
	for _, secret := range secrets {
		var prefix, suffix any
		var length int64
		if secret != "" {
			// SUBSTR(s,1,4) yields up to 4 chars; SUBSTR(s,-4) yields up to 4
			// from the end. Mimic that (and MapScan's []byte) for len<4 input.
			pEnd := 4
			if len(secret) < pEnd {
				pEnd = len(secret)
			}
			sStart := len(secret) - 4
			if sStart < 0 {
				sStart = 0
			}
			prefix = []byte(secret[:pEnd])
			suffix = []byte(secret[sStart:])
			length = int64(len(secret))
		}
		got := maskSecretFromFragments(prefix, suffix, length)
		want := maskSecret(secret)
		if got != want {
			t.Errorf("secret=%q (len=%d): got %q want %q", secret, len(secret), got, want)
		}
		// Masked form must never contain the full plaintext.
		if secret != "" && len(secret) > 8 && strings.Contains(got, secret) {
			t.Errorf("secret=%q: masked form leaked plaintext: %q", secret, got)
		}
	}
}

// TestUpdateAccount_ResponseOmitsPlaintextCredentials guards PUT /api/accounts/{id}.
// The list surface masks accessToken/apiToken and strips autoRelogin.passwordCipher,
// so a no-op update must not become a credential-harvest primitive that answers
// with the plaintext the list surface deliberately withholds. Revealing a secret
// stays an explicit act (GET /api/account-tokens/{id}/value).
func TestUpdateAccount_ResponseOmitsPlaintextCredentials(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	accessSecret, apiSecret, _, _, accountID, _ := seedCredentialedAccount(t, db)
	cipher := "enc:v1:FAKE-CIPHERTEXT-DO-NOT-LEAK"
	if _, err := db.Exec(
		`UPDATE accounts SET extra_config = ? WHERE id = ?`,
		`{"credentialMode":"session","autoRelogin":{"username":"cred-user","passwordCipher":"`+cipher+`"}}`,
		accountID); err != nil {
		t.Fatalf("seed extra_config: %v", err)
	}

	resp := doPutJSON(t, r, "/api/accounts/"+itoa(accountID), map[string]any{"sortOrder": 7})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, secret := range []string{accessSecret, apiSecret, cipher} {
		if strings.Contains(body, secret) {
			t.Fatalf("update account leaked plaintext secret %q: %s", secret, body)
		}
	}

	var updated map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := updated["accessToken"]; ok {
		t.Fatalf("accessToken key present: %#v", updated["accessToken"])
	}
	if _, ok := updated["apiToken"]; ok {
		t.Fatalf("apiToken key present: %#v", updated["apiToken"])
	}
	if updated["accessTokenMasked"] != maskSecret(accessSecret) {
		t.Fatalf("accessTokenMasked=%#v want %q", updated["accessTokenMasked"], maskSecret(accessSecret))
	}
	if updated["apiTokenMasked"] != maskSecret(apiSecret) {
		t.Fatalf("apiTokenMasked=%#v want %q", updated["apiTokenMasked"], maskSecret(apiSecret))
	}

	// Redaction is response-only: the write itself must still land.
	var sortOrder int64
	if err := db.QueryRow(`SELECT sort_order FROM accounts WHERE id = ?`, accountID).Scan(&sortOrder); err != nil {
		t.Fatalf("read sort_order: %v", err)
	}
	if sortOrder != 7 {
		t.Fatalf("sort_order = %d, want 7", sortOrder)
	}
}

// TestRebindSession_ResponseOmitsPlaintextCredentials guards
// POST /api/accounts/{id}/rebind-session: the response carries the stored
// apiToken (which the caller never supplied) plus the freshly bound session
// token, so it answers with the same masked contract as the list surface.
func TestRebindSession_ResponseOmitsPlaintextCredentials(t *testing.T) {
	db, r, _ := setupAccountsTest(t)
	_, apiSecret, _, _, accountID, _ := seedCredentialedAccount(t, db)
	newSecret := "FAKE-rebound-PLAINTEXT-DDDD4444"

	resp := doPostJSON(t, r, "/api/accounts/"+itoa(accountID)+"/rebind-session", map[string]any{"accessToken": newSecret})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, secret := range []string{newSecret, apiSecret} {
		if strings.Contains(body, secret) {
			t.Fatalf("rebind session leaked plaintext secret %q: %s", secret, body)
		}
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	account, ok := result["account"].(map[string]any)
	if !ok {
		t.Fatalf("account object missing: %s", body)
	}
	if _, ok := account["accessToken"]; ok {
		t.Fatalf("accessToken key present: %#v", account["accessToken"])
	}
	if _, ok := account["apiToken"]; ok {
		t.Fatalf("apiToken key present: %#v", account["apiToken"])
	}
	if account["accessTokenMasked"] != maskSecret(newSecret) {
		t.Fatalf("accessTokenMasked=%#v want %q", account["accessTokenMasked"], maskSecret(newSecret))
	}
	if account["apiTokenMasked"] != maskSecret(apiSecret) {
		t.Fatalf("apiTokenMasked=%#v want %q", account["apiTokenMasked"], maskSecret(apiSecret))
	}

	var stored string
	if err := db.QueryRow(`SELECT access_token FROM accounts WHERE id = ?`, accountID).Scan(&stored); err != nil {
		t.Fatalf("read access_token: %v", err)
	}
	if stored != newSecret {
		t.Fatalf("access_token = %q, want the rebound token persisted", stored)
	}
}
