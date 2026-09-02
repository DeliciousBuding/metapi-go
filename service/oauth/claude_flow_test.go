package oauth

import (
	"context"
	"encoding/json"
	"github.com/deliciousbuding/metapi-go/platform"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// withClaudeTokenURLSwap redirects claudeTokenURL to the given URL for the
// duration of the test and restores it on cleanup. Mirrors the
// withCodexTokenURLSwap / withGrokEndpointSwap pattern.
func withClaudeTokenURLSwap(t *testing.T, tokenURL string) {
	t.Helper()
	original := claudeTokenURL
	claudeTokenURL = tokenURL
	t.Cleanup(func() { claudeTokenURL = original })
}

// claudeFullTokenResponseJSON returns the raw JSON body for a successful
// Anthropic token endpoint response, including the organization + account
// nested objects that exchangeClaudeAuthorizationCode reads.
func claudeFullTokenResponseJSON(t *testing.T, access, refresh, orgUUID, orgName, acctUUID, email string, expiresIn int) []byte {
	t.Helper()
	full := map[string]interface{}{
		"access_token":  access,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"organization": map[string]interface{}{
			"uuid": orgUUID,
			"name": orgName,
		},
		"account": map[string]interface{}{
			"uuid":          acctUUID,
			"email_address": email,
		},
	}
	out, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal claude payload: %v", err)
	}
	return out
}

// ---- parseClaudeExpiresAt ----

func TestParseClaudeExpiresAt_FloatPositive(t *testing.T) {
	before := time.Now().UnixMilli()
	got := parseClaudeExpiresAt(float64(3600))
	after := time.Now().UnixMilli()
	if got <= before {
		t.Errorf("float64 3600 should yield future ts > %d, got %d", before, got)
	}
	if got > after+3600*1000+10 {
		t.Errorf("future ts too far: %d (after=%d)", got, after)
	}
}

func TestParseClaudeExpiresAt_FloatZeroOrNegative(t *testing.T) {
	if got := parseClaudeExpiresAt(float64(0)); got != 0 {
		t.Errorf("zero float should yield 0, got %d", got)
	}
	if got := parseClaudeExpiresAt(float64(-5)); got != 0 {
		t.Errorf("negative float should yield 0, got %d", got)
	}
}

func TestParseClaudeExpiresAt_StringPositive(t *testing.T) {
	before := time.Now().UnixMilli()
	got := parseClaudeExpiresAt("1800")
	after := time.Now().UnixMilli()
	if got <= before {
		t.Errorf("string 1800 should yield future ts > %d, got %d", before, got)
	}
	if got > after+1800*1000+10 {
		t.Errorf("future ts too far: %d (after=%d)", got, after)
	}
}

func TestParseClaudeExpiresAt_StringNonNumeric(t *testing.T) {
	if got := parseClaudeExpiresAt("abc"); got != 0 {
		t.Errorf("non-numeric string should yield 0, got %d", got)
	}
}

func TestParseClaudeExpiresAt_UnsupportedType(t *testing.T) {
	if got := parseClaudeExpiresAt(int64(3600)); got != 0 {
		t.Errorf("int64 should yield 0 (unsupported), got %d", got)
	}
}

func TestParseClaudeExpiresAt_NilValue(t *testing.T) {
	if got := parseClaudeExpiresAt(nil); got != 0 {
		t.Errorf("nil should yield 0, got %d", got)
	}
}

// ---- buildClaudeProxyHeaders ----

func TestBuildClaudeProxyHeaders_SetsAnthropicVersion(t *testing.T) {
	headers := buildClaudeProxyHeaders(context.Background(), ProxyHeaderInput{})
	if headers["anthropic-version"] != platform.ClaudeDefaultAnthropicVersion {
		t.Errorf("anthropic-version = %q, want %q",
			headers["anthropic-version"], platform.ClaudeDefaultAnthropicVersion)
	}
	if len(headers) != 1 {
		t.Errorf("claude proxy headers should set exactly one header, got %d", len(headers))
	}
}

func TestBuildClaudeProxyHeaders_IgnoresOAuthContext(t *testing.T) {
	// Claude's proxy header hook is stateless: the anthropic-version is the
	// only header it emits, regardless of the account/identity in the input.
	headers := buildClaudeProxyHeaders(context.Background(), ProxyHeaderInput{
		OAuth: ProxyHeaderOAuth{
			AccountID:  "acct-1",
			AccountKey: "key-1",
			Provider:   "claude",
		},
	})
	if headers["anthropic-version"] != platform.ClaudeDefaultAnthropicVersion {
		t.Errorf("anthropic-version = %q", headers["anthropic-version"])
	}
}

// ---- Exchange: success ----

func TestExchangeClaudeAuthorizationCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		bodyBytes, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			t.Errorf("request body is not JSON: %v", err)
		}
		if body["grant_type"] != "authorization_code" {
			t.Errorf("grant_type = %v", body["grant_type"])
		}
		if body["client_id"] != "test-claude-client-id" {
			t.Errorf("client_id = %v", body["client_id"])
		}
		if body["code"] != "auth-code-xyz" {
			t.Errorf("code = %v", body["code"])
		}
		if body["code_verifier"] != "verifier-abc" {
			t.Errorf("code_verifier = %v", body["code_verifier"])
		}
		if body["state"] != "state-123" {
			t.Errorf("state = %v", body["state"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(claudeFullTokenResponseJSON(t,
			"claude-access", "claude-refresh",
			"org-uuid-1", "My Org",
			"acct-uuid-1", "user@example.com",
			3600,
		))
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	token, err := exchangeClaudeAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code:         "auth-code-xyz",
		State:        "state-123",
		RedirectURI:  "http://localhost:54545/callback",
		CodeVerifier: "verifier-abc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "claude-access" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "claude-refresh" {
		t.Errorf("RefreshToken = %q", token.RefreshToken)
	}
	if token.Email != "user@example.com" {
		t.Errorf("Email = %q", token.Email)
	}
	if token.AccountID != "acct-uuid-1" {
		t.Errorf("AccountID = %q", token.AccountID)
	}
	if token.AccountKey != "acct-uuid-1" {
		t.Errorf("AccountKey = %q, should equal AccountID", token.AccountKey)
	}
	if token.TokenExpiresAt <= 0 {
		t.Error("TokenExpiresAt should be set")
	}
	if token.ProviderData["organizationId"] != "org-uuid-1" {
		t.Errorf("ProviderData.organizationId = %v", token.ProviderData["organizationId"])
	}
	if token.ProviderData["organizationName"] != "My Org" {
		t.Errorf("ProviderData.organizationName = %v", token.ProviderData["organizationName"])
	}
}

// ---- Exchange: success with account-key falling back to email ----

func TestExchangeClaudeAuthorizationCode_AccountKeyFallsBackToEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No account.uuid returned — AccountKey must fall back to email.
		full := map[string]interface{}{
			"access_token":  "access",
			"refresh_token": "refresh",
			"expires_in":    3600,
			"account": map[string]interface{}{
				"email_address": "fallback@example.com",
			},
		}
		out, _ := json.Marshal(full)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(out)
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	token, err := exchangeClaudeAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// exchangeClaudeAuthorizationCode only backfills AccountKey from email;
	// AccountID stays empty and is backfilled later by account.go's
	// GetOauthInfoFromExtraConfig (see asNonEmptyString logic).
	if token.AccountKey != "fallback@example.com" {
		t.Errorf("AccountKey = %q, want email fallback", token.AccountKey)
	}
	if token.Email != "fallback@example.com" {
		t.Errorf("Email = %q", token.Email)
	}
}

// ---- Exchange: error / malformed / network ----

func TestExchangeClaudeAuthorizationCode_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad code"}`))
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	_, err := exchangeClaudeAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "bad-code",
	})
	if err == nil {
		t.Fatal("expected error for 4xx response")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should surface provider body, got %q", err.Error())
	}
}

func TestExchangeClaudeAuthorizationCode_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	_, err := exchangeClaudeAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "invalid payload") {
		t.Errorf("error should mention invalid payload, got %q", err.Error())
	}
}

func TestExchangeClaudeAuthorizationCode_MissingAccessToken(t *testing.T) {
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, map[string]interface{}{
			"refresh_token": "refresh-only",
			"expires_in":    3600,
		}
	})
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	_, err := exchangeClaudeAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
	if !strings.Contains(err.Error(), "missing access token") {
		t.Errorf("error should mention missing access token, got %q", err.Error())
	}
}

func TestExchangeClaudeAuthorizationCode_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	_, err := exchangeClaudeAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestExchangeClaudeAuthorizationCode_UnreachableServer(t *testing.T) {
	withClaudeTokenURLSwap(t, "http://127.0.0.1:1/oauth/token")
	_, err := exchangeClaudeAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected connection error")
	}
}

// ---- Refresh: success ----

func TestRefreshClaudeAccessToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		var body map[string]interface{}
		_ = json.Unmarshal(bodyBytes, &body)
		if body["grant_type"] != "refresh_token" {
			t.Errorf("grant_type = %v", body["grant_type"])
		}
		if body["refresh_token"] != "old-refresh" {
			t.Errorf("refresh_token = %v", body["refresh_token"])
		}
		if body["client_id"] != "test-claude-client-id" {
			t.Errorf("client_id = %v", body["client_id"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(claudeFullTokenResponseJSON(t,
			"new-access", "new-refresh",
			"org-2", "Refreshed Org",
			"acct-2", "refresh@example.com",
			7200,
		))
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	token, err := refreshClaudeAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "old-refresh",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q", token.RefreshToken)
	}
	if token.Email != "refresh@example.com" {
		t.Errorf("Email = %q", token.Email)
	}
	if token.AccountID != "acct-2" {
		t.Errorf("AccountID = %q", token.AccountID)
	}
	if token.ProviderData["organizationName"] != "Refreshed Org" {
		t.Errorf("ProviderData.organizationName = %v", token.ProviderData["organizationName"])
	}
}

// ---- Refresh: error / network ----

func TestRefreshClaudeAccessToken_ExpiredRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token expired"}`))
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	_, err := refreshClaudeAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "dead-refresh",
	})
	if err == nil {
		t.Fatal("expected error for expired refresh_token")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should mention invalid_grant, got %q", err.Error())
	}
}

func TestRefreshClaudeAccessToken_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{broken`))
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	_, err := refreshClaudeAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "any",
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "invalid payload") {
		t.Errorf("error should mention invalid payload, got %q", err.Error())
	}
}

func TestRefreshClaudeAccessToken_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	_, err := refreshClaudeAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "any",
	})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestRefreshClaudeAccessToken_MissingAccessToken(t *testing.T) {
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, map[string]interface{}{
			"refresh_token": "rt-only",
			"expires_in":    3600,
		}
	})
	defer server.Close()
	withClaudeTokenURLSwap(t, server.URL)

	_, err := refreshClaudeAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "any",
	})
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
	if !strings.Contains(err.Error(), "missing access token") {
		t.Errorf("error should mention missing access token, got %q", err.Error())
	}
}
