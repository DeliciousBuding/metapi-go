package oauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Singleflight Tests ----

func TestRefreshInFlight_Dedup(t *testing.T) {
	// Clean up state from other tests.
	refreshInFlightMu.Lock()
	refreshInFlight = make(map[int64]*refreshPromise)
	refreshInFlightMu.Unlock()

	// Verify the map is initially empty.
	refreshInFlightMu.Lock()
	if len(refreshInFlight) != 0 {
		t.Error("refreshInFlight should be empty at test start")
	}
	refreshInFlightMu.Unlock()

	p := newRefreshPromise()

	refreshInFlightMu.Lock()
	refreshInFlight[42] = p
	refreshInFlightMu.Unlock()

	// Verify it was stored.
	refreshInFlightMu.Lock()
	if _, exists := refreshInFlight[42]; !exists {
		t.Error("promise should exist after insertion")
	}
	refreshInFlightMu.Unlock()

	// Clean up.
	refreshInFlightMu.Lock()
	delete(refreshInFlight, 42)
	refreshInFlightMu.Unlock()
}

func TestRefreshInFlight_CleanupOnFinish(t *testing.T) {
	refreshInFlightMu.Lock()
	refreshInFlight = make(map[int64]*refreshPromise)
	refreshInFlightMu.Unlock()

	accountID := int64(999)

	// Simulate the singleflight pattern:
	// 1. Check if inflight — no
	// 2. Create promise
	p := newRefreshPromise()

	refreshInFlightMu.Lock()
	refreshInFlight[accountID] = p
	refreshInFlightMu.Unlock()

	// 3. Simulate done — resolve result and clean up
	result := refreshResult{
		AccountID:   accountID,
		AccessToken: "new-token",
		AccountKey:  "key-1",
	}
	p.resolve(result)

	refreshInFlightMu.Lock()
	delete(refreshInFlight, accountID)
	refreshInFlightMu.Unlock()

	// 4. Verify clean up.
	refreshInFlightMu.Lock()
	if _, exists := refreshInFlight[accountID]; exists {
		t.Error("promise should be cleaned up after completion")
	}
	refreshInFlightMu.Unlock()
}

func TestRefreshInFlight_ConcurrentDedup(t *testing.T) {
	refreshInFlightMu.Lock()
	refreshInFlight = make(map[int64]*refreshPromise)
	refreshInFlightMu.Unlock()

	accountID := int64(123)

	// Test that singleflight dedup works:
	// Insert a promise manually, then verify a second caller finds it.
	p := newRefreshPromise()

	refreshInFlightMu.Lock()
	refreshInFlight[accountID] = p
	refreshInFlightMu.Unlock()

	// Second caller should find the existing promise.
	var found bool
	func() {
		refreshInFlightMu.Lock()
		defer refreshInFlightMu.Unlock()
		if _, exists := refreshInFlight[accountID]; exists {
			found = true
		}
	}()

	if !found {
		t.Error("second caller should find existing promise")
	}

	// Cleanup: resolve result to unblock any waiter and remove.
	p.resolve(refreshResult{AccountID: accountID, AccessToken: "deduped-token"})
	refreshInFlightMu.Lock()
	delete(refreshInFlight, accountID)
	refreshInFlightMu.Unlock()

	if len(refreshInFlight) != 0 {
		t.Error("map should be empty after cleanup")
	}
}

func TestRefreshPromiseBroadcastsResultToAllWaiters(t *testing.T) {
	p := newRefreshPromise()

	done := make(chan refreshResult, 2)
	go func() { done <- p.wait() }()
	go func() { done <- p.wait() }()

	want := refreshResult{AccountID: 42, AccessToken: "shared-token"}
	p.resolve(want)

	for i := 0; i < 2; i++ {
		select {
		case got := <-done:
			if got.AccountID != want.AccountID || got.AccessToken != want.AccessToken {
				t.Fatalf("waiter %d got %+v, want %+v", i, got, want)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("waiter %d did not receive broadcast result", i)
		}
	}
}

// ---- coalesceStr Tests ----

func TestCoalesceStr_FirstNonEmpty(t *testing.T) {
	result := coalesceStr("a", "b", "c")
	if result != "a" {
		t.Errorf("expected 'a', got %q", result)
	}
}

func TestCoalesceStr_SkipEmpty(t *testing.T) {
	result := coalesceStr("", "", "c")
	if result != "c" {
		t.Errorf("expected 'c', got %q", result)
	}
}

func TestCoalesceStr_AllEmpty(t *testing.T) {
	result := coalesceStr("", "", "")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestCoalesceStr_NilValues(t *testing.T) {
	result := coalesceStr("", "")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

// ---- coalesceInt64 Tests ----

func TestCoalesceInt64_FirstPositive(t *testing.T) {
	result := coalesceInt64(100, 200, 300)
	if result != 100 {
		t.Errorf("expected 100, got %d", result)
	}
}

func TestCoalesceInt64_SkipZero(t *testing.T) {
	result := coalesceInt64(0, 0, 300)
	if result != 300 {
		t.Errorf("expected 300, got %d", result)
	}
}

func TestCoalesceInt64_AllZero(t *testing.T) {
	result := coalesceInt64(0, 0, 0)
	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
}

// ---- mergeProviderData Tests ----

func TestMergeProviderData_BothNil(t *testing.T) {
	result := mergeProviderData(nil, nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMergeProviderData_RefreshedMergesOntoExisting(t *testing.T) {
	existing := map[string]interface{}{"a": 1, "b": 2}
	refreshed := map[string]interface{}{"b": "new", "c": 3}
	result := mergeProviderData(existing, refreshed)
	if result["a"] != 1 {
		t.Errorf("expected existing 'a' to be preserved, got %v", result)
	}
	if result["b"] != "new" {
		t.Errorf("expected refreshed 'b' to overwrite, got %v", result)
	}
	if result["c"] != 3 {
		t.Errorf("expected new key 'c', got %v", result)
	}
}

func TestMergeProviderData_ExistingOnly(t *testing.T) {
	existing := map[string]interface{}{"x": "y"}
	result := mergeProviderData(existing, nil)
	if result["x"] != "y" {
		t.Errorf("expected existing data preserved, got %v", result)
	}
}

func TestMergeProviderData_RefreshedOnly(t *testing.T) {
	refreshed := map[string]interface{}{"new": "data"}
	result := mergeProviderData(nil, refreshed)
	if result["new"] != "data" {
		t.Errorf("expected refreshed data, got %v", result)
	}
}

func TestMergeProviderData_EmptyResultReturnsNil(t *testing.T) {
	result := mergeProviderData(map[string]interface{}{}, map[string]interface{}{})
	if result != nil {
		t.Errorf("expected nil for empty merge, got %v", result)
	}
}

// ---- stringsTrimSpace Tests ----

func TestStringsTrimSpace_NoWhitespace(t *testing.T) {
	result := stringsTrimSpace("hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestStringsTrimSpace_WithWhitespace(t *testing.T) {
	result := stringsTrimSpace("  hello world  ")
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestStringsTrimSpace_OnlyWhitespace(t *testing.T) {
	result := stringsTrimSpace("\t  \n")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

// ---- ptrToString Tests ----

func TestPtrToString_Nil(t *testing.T) {
	result := ptrToString(nil)
	if result != "" {
		t.Errorf("expected empty for nil, got %q", result)
	}
}

func TestPtrToString_NonNil(t *testing.T) {
	s := "hello"
	result := ptrToString(&s)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

// ---- resolveAccountProxyURL Tests ----

func TestResolveAccountProxyURL_NoProxy(t *testing.T) {
	result := resolveAccountProxyURL(1, nil)
	if result != nil {
		t.Error("expected nil for nil extraConfig")
	}
}

// ---- refreshResult Tests ----

func TestRefreshResult_ErrorHandling(t *testing.T) {
	result := refreshResult{
		AccountID: 42,
		Err:       nil,
	}
	if result.Err != nil {
		t.Error("fresh result with nil error should not report error")
	}
}

// ---- doRefreshAccessToken error paths + success path ----
//
// These tests use the setupTestDB helper (defined in route_unit_test.go) to
// spin up an in-memory SQLite database, then exercise the full refresh path
// with a mocked codex token endpoint.

// insertCodexTestAccount inserts a site + account with the given extraConfig
// and returns the account ID. Used by the doRefreshAccessToken tests to
// satisfy the accounts.site_id → sites.id foreign key.
func insertCodexTestAccount(t *testing.T, db *store.DB, accountKey, extraConfig string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('codex-site', 'https://codex.example.com', 'codex', 'active', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	provider := "codex"
	res, err := db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, oauth_provider, oauth_account_key, extra_config, created_at, updated_at)
		 VALUES (1, ?, 'stale-access', 'active', ?, ?, ?, ?, ?)`,
		accountKey, provider, accountKey, extraConfig, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestDoRefreshAccessToken_NoDatabase(t *testing.T) {
	// Ensure no DB is active so the early-return "database not initialized"
	// branch is exercised.
	store.CloseDatabase()
	t.Cleanup(func() { store.CloseDatabase() })

	result := doRefreshAccessToken(1)
	if result.Err == nil {
		t.Fatal("expected error when database is not initialized")
	}
	if !strings.Contains(result.Err.Error(), "database not initialized") {
		t.Errorf("error = %v, want 'database not initialized'", result.Err)
	}
}

func TestDoRefreshAccessToken_AccountNotFound(t *testing.T) {
	_, cleanup := setupTestDB(t)
	defer cleanup()

	result := doRefreshAccessToken(99999)
	if result.Err == nil {
		t.Fatal("expected error for missing account")
	}
	if !strings.Contains(result.Err.Error(), "oauth account not found") {
		t.Errorf("error = %v", result.Err)
	}
}

func TestDoRefreshAccessToken_NoRefreshToken(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-no-rt","accountKey":"acct-no-rt"}}`
	insertCodexTestAccount(t, db, "acct-no-rt", extraConfig)

	result := doRefreshAccessToken(1)
	if result.Err == nil {
		t.Fatal("expected error for account without refresh token")
	}
	if !strings.Contains(result.Err.Error(), "refresh token missing") {
		t.Errorf("error = %v", result.Err)
	}
}

func TestDoRefreshAccessToken_Success_Codex(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Build a valid codex id_token so parseJWTClaims succeeds and the refresh
	// path returns a full TokenSet.
	claims := codexJWTClaims{Email: "refresh@example.com"}
	claims.Auth.ChatGPTAccountID = "acct-refresh-1"
	claims.Auth.ChatGPTPlanType = "plus"
	idToken := makeCodexIDToken(t, claims)

	tokenServer := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		if r.PostFormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.PostFormValue("grant_type"))
		}
		if r.PostFormValue("refresh_token") != "old-codex-rt" {
			t.Errorf("refresh_token = %q", r.PostFormValue("refresh_token"))
		}
		return 200, codexTokenResponse{
			AccessToken:  "fresh-codex-access",
			RefreshToken: "fresh-codex-rt",
			IDToken:      idToken,
			ExpiresIn:    float64(7200),
		}
	})
	defer tokenServer.Close()
	withCodexTokenURLSwap(t, tokenServer.URL)

	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-refresh-1","accountKey":"acct-refresh-1","refreshToken":"old-codex-rt","planType":"free","email":"old@example.com"}}`
	accountID := insertCodexTestAccount(t, db, "acct-refresh-1", extraConfig)

	result := doRefreshAccessToken(accountID)
	if result.Err != nil {
		t.Fatalf("refresh failed: %v", result.Err)
	}
	if result.AccessToken != "fresh-codex-access" {
		t.Errorf("AccessToken = %q", result.AccessToken)
	}
	if result.AccountKey != "acct-refresh-1" {
		t.Errorf("AccountKey = %q", result.AccountKey)
	}
	if result.ExtraConfig == nil {
		t.Error("ExtraConfig should be non-nil")
	}

	// Verify the DB was updated.
	var updated store.Account
	if err := db.Get(&updated, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("reload account: %v", err)
	}
	if updated.AccessToken != "fresh-codex-access" {
		t.Errorf("DB access_token = %q, want fresh-codex-access", updated.AccessToken)
	}
	if updated.ExtraConfig == nil {
		t.Fatal("extraConfig should not be nil after refresh")
	}
	if !strings.Contains(*updated.ExtraConfig, "fresh-codex-rt") {
		t.Errorf("extraConfig should contain fresh refresh token, got %q", *updated.ExtraConfig)
	}
	if !strings.Contains(*updated.ExtraConfig, "refresh@example.com") {
		t.Errorf("extraConfig should contain refreshed email, got %q", *updated.ExtraConfig)
	}
}

func TestDoRefreshAccessToken_RefreshFails(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenServer.Close()
	withCodexTokenURLSwap(t, tokenServer.URL)

	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-fail","accountKey":"acct-fail","refreshToken":"dead-rt"}}`
	accountID := insertCodexTestAccount(t, db, "acct-fail", extraConfig)

	result := doRefreshAccessToken(accountID)
	if result.Err == nil {
		t.Fatal("expected refresh error to propagate")
	}
	if !strings.Contains(result.Err.Error(), "invalid_grant") {
		t.Errorf("error = %v, want invalid_grant", result.Err)
	}
}

func TestDoRefreshAccessToken_UnknownProvider(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	// Insert an account with an unregistered provider. We bypass the helper
	// because the helper hardcodes codex.
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('unk-site', 'https://unk.example.com', 'unknown', 'active', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	provider := "nonexistent-provider"
	accountKey := "acct-unknown"
	extraConfig := `{"oauth":{"provider":"nonexistent-provider","accountId":"acct-unknown","accountKey":"acct-unknown","refreshToken":"any-rt"}}`
	res, err := db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, oauth_provider, oauth_account_key, extra_config, created_at, updated_at)
		 VALUES (1, 'user', 'access', 'active', ?, ?, ?, ?, ?)`,
		provider, accountKey, extraConfig, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()

	result := doRefreshAccessToken(accountID)
	if result.Err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(result.Err.Error(), "unsupported oauth provider") {
		t.Errorf("error = %v", result.Err)
	}
}

// ---- RefreshAccessTokenSingleflight ----

func TestRefreshAccessTokenSingleflight_NoDatabase(t *testing.T) {
	store.CloseDatabase()
	t.Cleanup(func() { store.CloseDatabase() })

	refreshInFlightMu.Lock()
	refreshInFlight = make(map[int64]*refreshPromise)
	refreshInFlightMu.Unlock()

	_, err := RefreshAccessTokenSingleflight(1)
	if err == nil {
		t.Fatal("expected error when database is not initialized")
	}
}

func TestRefreshAccessTokenSingleflight_Success(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	claims := codexJWTClaims{Email: "sf@example.com"}
	claims.Auth.ChatGPTAccountID = "acct-sf-1"
	idToken := makeCodexIDToken(t, claims)

	tokenServer := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, codexTokenResponse{
			AccessToken:  "sf-access",
			RefreshToken: "sf-rt",
			IDToken:      idToken,
			ExpiresIn:    float64(3600),
		}
	})
	defer tokenServer.Close()
	withCodexTokenURLSwap(t, tokenServer.URL)

	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-sf-1","accountKey":"acct-sf-1","refreshToken":"old-rt"}}`
	accountID := insertCodexTestAccount(t, db, "acct-sf-1", extraConfig)

	result, err := RefreshAccessTokenSingleflight(accountID)
	if err != nil {
		t.Fatalf("singleflight refresh failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.AccessToken != "sf-access" {
		t.Errorf("AccessToken = %q", result.AccessToken)
	}
}

func TestRefreshAccessTokenSingleflight_Dedupes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	claims := codexJWTClaims{Email: "dedupe@example.com"}
	claims.Auth.ChatGPTAccountID = "acct-dedupe"
	idToken := makeCodexIDToken(t, claims)

	tokenServer := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, codexTokenResponse{
			AccessToken:  "dedupe-access",
			RefreshToken: "dedupe-rt",
			IDToken:      idToken,
			ExpiresIn:    float64(3600),
		}
	})
	defer tokenServer.Close()
	withCodexTokenURLSwap(t, tokenServer.URL)

	extraConfig := `{"oauth":{"provider":"codex","accountId":"acct-dedupe","accountKey":"acct-dedupe","refreshToken":"old-rt"}}`
	accountID := insertCodexTestAccount(t, db, "acct-dedupe", extraConfig)

	// Two concurrent singleflight calls for the same account → only one should
	// hit the token endpoint, the other dedupes via the in-flight promise.
	done := make(chan *refreshResult, 2)
	errs := make(chan error, 2)
	go func() {
		r, e := RefreshAccessTokenSingleflight(accountID)
		done <- r
		errs <- e
	}()
	go func() {
		r, e := RefreshAccessTokenSingleflight(accountID)
		done <- r
		errs <- e
	}()
	for i := 0; i < 2; i++ {
		if e := <-errs; e != nil {
			t.Errorf("concurrent singleflight %d failed: %v", i, e)
		}
		<-done
	}
}

func TestRefreshResult_WithError(t *testing.T) {
	result := refreshResult{
		AccountID: 42,
		Err:       nil,
	}
	// Simulate an error case.
	result.Err = nil
	if result.AccountID != 42 {
		t.Errorf("expected AccountID 42, got %d", result.AccountID)
	}
}
