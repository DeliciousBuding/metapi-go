package oauth

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// TestHandleCallback_Success_Codex exercises the full callback → exchange →
// persist pipeline end-to-end against an in-memory SQLite DB and a mocked
// codex token endpoint. This covers HandleCallback's success path, the proxy
// resolution fallback, and the account-persistence rollback snapshot logic.
func TestHandleCallback_Success_Codex(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	previousHooks := workflowHooks
	SetOAuthWorkflowHooks(nil)
	t.Cleanup(func() { SetOAuthWorkflowHooks(previousHooks) })

	// Fresh session store so this test doesn't collide with other tests.
	previousStore := globalSessionStore
	SetSessionStore(NewMemoryOAuthSessionStore())
	t.Cleanup(func() { globalSessionStore = previousStore })

	// Build a valid codex id_token so the exchange succeeds.
	claims := codexJWTClaims{Email: "callback@example.com"}
	claims.Auth.ChatGPTAccountID = "acct-callback-1"
	claims.Auth.ChatGPTPlanType = "plus"
	idToken := makeCodexIDToken(t, claims)

	tokenServer := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		if r.PostFormValue("code") != "valid-auth-code" {
			t.Errorf("code = %q", r.PostFormValue("code"))
		}
		if r.PostFormValue("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.PostFormValue("grant_type"))
		}
		return 200, codexTokenResponse{
			AccessToken:  "callback-access-token",
			RefreshToken: "callback-refresh-token",
			IDToken:      idToken,
			ExpiresIn:    float64(3600),
		}
	})
	defer tokenServer.Close()
	withCodexTokenURLSwap(t, tokenServer.URL)

	// Create a session for codex.
	session, err := CreateSession(CreateSessionInput{
		Provider:    "codex",
		RedirectURI: "http://localhost:1455/auth/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	result, err := HandleCallback(CallbackInput{
		Provider: "codex",
		State:    session.State,
		Code:     "valid-auth-code",
	})
	if err != nil {
		t.Fatalf("HandleCallback failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.AccountID <= 0 {
		t.Errorf("AccountID = %d, want positive", result.AccountID)
	}
	if result.SiteID <= 0 {
		t.Errorf("SiteID = %d, want positive", result.SiteID)
	}

	// Verify the account was persisted in the DB.
	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", result.AccountID); err != nil {
		t.Fatalf("reload account: %v", err)
	}
	if account.AccessToken != "callback-access-token" {
		t.Errorf("DB access_token = %q, want callback-access-token", account.AccessToken)
	}
	if account.OAuthProvider == nil || *account.OAuthProvider != "codex" {
		t.Errorf("DB oauth_provider = %v, want codex", account.OAuthProvider)
	}
	if account.ExtraConfig == nil {
		t.Fatal("extraConfig should not be nil")
	}
	if !strings.Contains(*account.ExtraConfig, "callback-refresh-token") {
		t.Errorf("extraConfig should contain refresh token, got %q", *account.ExtraConfig)
	}
	if !strings.Contains(*account.ExtraConfig, "callback@example.com") {
		t.Errorf("extraConfig should contain email, got %q", *account.ExtraConfig)
	}

	// The session should be marked as success.
	got := GetSession(session.State)
	if got == nil || got.Status != SessionSuccess {
		t.Errorf("session status = %v, want success", got)
	}
}

// TestHandleCallback_Success_Codex_WithRebind exercises the rebind path where
// the callback rebinds to an existing account ID rather than creating a new one.
func TestHandleCallback_Success_Codex_WithRebind(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	previousHooks := workflowHooks
	SetOAuthWorkflowHooks(nil)
	t.Cleanup(func() { SetOAuthWorkflowHooks(previousHooks) })

	previousStore := globalSessionStore
	SetSessionStore(NewMemoryOAuthSessionStore())
	t.Cleanup(func() { globalSessionStore = previousStore })

	// Pre-create an account that the rebind will target.
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('rebind-site', 'https://rebind.example.com', 'codex', 'active', ?, ?)`,
		now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	provider := "codex"
	_, err = db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, status, oauth_provider, oauth_account_key, extra_config, created_at, updated_at)
		 VALUES (1, 'old-user', 'old-access', 'active', ?, 'old-acct-key', '{}', ?, ?)`,
		provider, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}

	claims := codexJWTClaims{Email: "rebind@example.com"}
	claims.Auth.ChatGPTAccountID = "old-acct-key"
	idToken := makeCodexIDToken(t, claims)

	tokenServer := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, codexTokenResponse{
			AccessToken:  "rebind-access",
			RefreshToken: "rebind-refresh",
			IDToken:      idToken,
			ExpiresIn:    float64(3600),
		}
	})
	defer tokenServer.Close()
	withCodexTokenURLSwap(t, tokenServer.URL)

	session, err := CreateSession(CreateSessionInput{
		Provider:        "codex",
		RedirectURI:     "http://localhost:1455/auth/callback",
		RebindAccountID: 1,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	result, err := HandleCallback(CallbackInput{
		Provider: "codex",
		State:    session.State,
		Code:     "rebind-code",
	})
	if err != nil {
		t.Fatalf("HandleCallback rebind failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.AccountID != 1 {
		t.Errorf("AccountID = %d, want 1 (rebound)", result.AccountID)
	}

	// The account should have the fresh access token.
	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = 1"); err != nil {
		t.Fatalf("reload account: %v", err)
	}
	if account.AccessToken != "rebind-access" {
		t.Errorf("DB access_token = %q, want rebind-access", account.AccessToken)
	}
}

// ---- SubmitManualCallback ----

func TestSubmitManualCallback_InvalidURLAndMissingCode(t *testing.T) {
	previousStore := globalSessionStore
	SetSessionStore(NewMemoryOAuthSessionStore())
	t.Cleanup(func() { globalSessionStore = previousStore })

	session, err := CreateSession(CreateSessionInput{
		Provider:   "codex",
		RedirectURI: "http://localhost:1455/auth/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// An empty/invalid callback URL should surface a parse error, not a
	// session-not-found or state-mismatch error.
	err = SubmitManualCallback(ManualCallbackInput{
		State:       session.State,
		CallbackURL: "",
	})
	if err == nil {
		t.Fatal("expected error for empty callback URL")
	}
	if !strings.Contains(err.Error(), "invalid oauth callback url") {
		t.Errorf("error = %v, want 'invalid oauth callback url'", err)
	}

	// A URL with state but no code and no error should also fail parsing.
	err = SubmitManualCallback(ManualCallbackInput{
		State:       session.State,
		CallbackURL: "http://localhost:1455/auth/callback?state=" + session.State,
	})
	if err == nil {
		t.Fatal("expected error for URL missing both code and error")
	}
}
