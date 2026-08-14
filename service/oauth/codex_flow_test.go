package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// withCodexTokenURLSwap redirects codexTokenURL to the given URL for the
// duration of the test and restores it on cleanup. Mirrors the
// withGrokEndpointSwap pattern in grok_test.go.
func withCodexTokenURLSwap(t *testing.T, tokenURL string) {
	t.Helper()
	original := codexTokenURL
	codexTokenURL = tokenURL
	t.Cleanup(func() { codexTokenURL = original })
}

// makeCodexIDToken builds a synthetic OpenAI-style JWT id_token whose payload
// is the marshalled claims. The header and signature are opaque — only the
// claims payload needs to decode for parseJWTClaims to succeed.
func makeCodexIDToken(t *testing.T, claims codexJWTClaims) string {
	t.Helper()
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(claimsBytes)
	return "header." + encodedPayload + ".signature"
}

// ---- parseJWTClaims ----

func TestParseJWTClaims_Valid(t *testing.T) {
	claims := codexJWTClaims{Email: "user@example.com"}
	claims.Auth.ChatGPTAccountID = "acct-123"
	claims.Auth.ChatGPTPlanType = "plus"

	parsed, err := parseJWTClaims(makeCodexIDToken(t, claims))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Email != "user@example.com" {
		t.Errorf("Email = %q", parsed.Email)
	}
	if parsed.Auth.ChatGPTAccountID != "acct-123" {
		t.Errorf("ChatGPTAccountID = %q", parsed.Auth.ChatGPTAccountID)
	}
	if parsed.Auth.ChatGPTPlanType != "plus" {
		t.Errorf("ChatGPTPlanType = %q", parsed.Auth.ChatGPTPlanType)
	}
}

func TestParseJWTClaims_NotThreeParts(t *testing.T) {
	if _, err := parseJWTClaims("not-a-jwt"); err == nil {
		t.Fatal("expected error for non-JWT string")
	}
	if _, err := parseJWTClaims("only.two"); err == nil {
		t.Fatal("expected error for two-part token")
	}
}

func TestParseJWTClaims_InvalidBase64(t *testing.T) {
	// Payload contains characters that cannot be base64-decoded even after
	// padding/replacement logic. base64.URLEncoding is strict about length
	// mod 4, and an odd-length payload triggers a decode error.
	_, err := parseJWTClaims("header.@@@.signature")
	if err == nil {
		t.Fatal("expected error for invalid base64 payload")
	}
}

func TestParseJWTClaims_InvalidJSON(t *testing.T) {
	// Valid base64 but invalid JSON payload.
	encoded := base64.RawURLEncoding.EncodeToString([]byte("not json"))
	if _, err := parseJWTClaims("header." + encoded + ".sig"); err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
}

// ---- base64Decode / base64DecodeRaw ----

func TestBase64Decode_StandardPadding(t *testing.T) {
	// base64.StdEncoding of "hello" is "aGVsbG8=".
	decoded, err := base64Decode("aGVsbG8=")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(decoded) != "hello" {
		t.Errorf("got %q, want hello", string(decoded))
	}
}

func TestBase64Decode_URLSafeNoPadding(t *testing.T) {
	// base64.RawURLEncoding of "payload" — no padding, URL-safe alphabet.
	encoded := base64.RawURLEncoding.EncodeToString([]byte("payload"))
	decoded, err := base64Decode(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(decoded) != "payload" {
		t.Errorf("got %q, want payload", string(decoded))
	}
}

func TestBase64Decode_PadsShortInput(t *testing.T) {
	// Two-mod-three lengths must be padded before StdEncoding can decode.
	// "YWJj" (abc) is length 4 (fine), but trimming to "YWJ" (len 3) exercises
	// the padding branch.
	decoded, err := base64DecodeRaw("YWJj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(decoded) != "abc" {
		t.Errorf("got %q, want abc", string(decoded))
	}
}

// ---- getHeaderValue ----

func TestGetHeaderValue_StringMatch(t *testing.T) {
	headers := map[string]interface{}{
		"Originator": "codex_cli_rs",
	}
	if got := getHeaderValue(headers, "originator"); got != "codex_cli_rs" {
		t.Errorf("got %q", got)
	}
}

func TestGetHeaderValue_CaseInsensitive(t *testing.T) {
	headers := map[string]interface{}{
		"ORIGINATOR": "from-header",
	}
	if got := getHeaderValue(headers, "originator"); got != "from-header" {
		t.Errorf("got %q", got)
	}
}

func TestGetHeaderValue_ArrayReturnsFirstNonEmpty(t *testing.T) {
	headers := map[string]interface{}{
		"originator": []interface{}{"  ", "codex_cli_go", "second"},
	}
	if got := getHeaderValue(headers, "Originator"); got != "codex_cli_go" {
		t.Errorf("got %q", got)
	}
}

func TestGetHeaderValue_NilHeaders(t *testing.T) {
	if got := getHeaderValue(nil, "originator"); got != "" {
		t.Errorf("nil headers should yield empty, got %q", got)
	}
}

func TestGetHeaderValue_MissingKey(t *testing.T) {
	headers := map[string]interface{}{"other": "value"}
	if got := getHeaderValue(headers, "originator"); got != "" {
		t.Errorf("missing key should yield empty, got %q", got)
	}
}

func TestGetHeaderValue_WhitespaceOnlyTreatedAsEmpty(t *testing.T) {
	headers := map[string]interface{}{
		"originator": "   ",
		"other":      "fallback",
	}
	// Whitespace-only string is skipped; the lookup does NOT fall through to
	// other keys, so the result is empty (the loop returns the first matching
	// key, even if its value is empty after trim).
	if got := getHeaderValue(headers, "originator"); got != "" {
		t.Errorf("whitespace-only should yield empty, got %q", got)
	}
}

// ---- buildCodexProxyHeaders ----

func TestBuildCodexProxyHeaders_UsesAccountID(t *testing.T) {
	headers := buildCodexProxyHeaders(context.Background(), ProxyHeaderInput{
		OAuth: ProxyHeaderOAuth{AccountID: "acct-1"},
	})
	if headers["Chatgpt-Account-Id"] != "acct-1" {
		t.Errorf("Chatgpt-Account-Id = %q", headers["Chatgpt-Account-Id"])
	}
	if headers["Originator"] != "codex_cli_rs" {
		t.Errorf("default Originator = %q, want codex_cli_rs", headers["Originator"])
	}
}

func TestBuildCodexProxyHeaders_FallsBackToAccountKey(t *testing.T) {
	headers := buildCodexProxyHeaders(context.Background(), ProxyHeaderInput{
		OAuth: ProxyHeaderOAuth{AccountKey: "key-2"},
	})
	if headers["Chatgpt-Account-Id"] != "key-2" {
		t.Errorf("should fall back to AccountKey, got %q", headers["Chatgpt-Account-Id"])
	}
}

func TestBuildCodexProxyHeaders_NoAccount(t *testing.T) {
	headers := buildCodexProxyHeaders(context.Background(), ProxyHeaderInput{})
	if _, hasAccount := headers["Chatgpt-Account-Id"]; hasAccount {
		t.Error("should not set Chatgpt-Account-Id when both empty")
	}
	if headers["Originator"] != "codex_cli_rs" {
		t.Errorf("default Originator = %q", headers["Originator"])
	}
}

func TestBuildCodexProxyHeaders_DownstreamOriginatorWins(t *testing.T) {
	headers := buildCodexProxyHeaders(context.Background(), ProxyHeaderInput{
		OAuth:             ProxyHeaderOAuth{AccountID: "acct-3"},
		DownstreamHeaders: map[string]interface{}{"originator": "custom-originator"},
	})
	if headers["Originator"] != "custom-originator" {
		t.Errorf("downstream Originator should win, got %q", headers["Originator"])
	}
}

func TestBuildCodexProxyHeaders_DownstreamOriginatorCaseInsensitive(t *testing.T) {
	headers := buildCodexProxyHeaders(context.Background(), ProxyHeaderInput{
		DownstreamHeaders: map[string]interface{}{"ORIGINATOR": "upper-origin"},
	})
	if headers["Originator"] != "upper-origin" {
		t.Errorf("case-insensitive lookup failed, got %q", headers["Originator"])
	}
}

// ---- Exchange: success ----

func TestExchangeCodexAuthorizationCode_Success(t *testing.T) {
	claims := codexJWTClaims{Email: "codex-user@example.com"}
	claims.Auth.ChatGPTAccountID = "acct-codex-1"
	claims.Auth.ChatGPTPlanType = "pro"
	idToken := makeCodexIDToken(t, claims)

	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.PostForm.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("client_id") != "test-codex-client-id" {
			t.Errorf("client_id = %q", r.PostForm.Get("client_id"))
		}
		if r.PostForm.Get("code") != "auth-code-abc" {
			t.Errorf("code = %q", r.PostForm.Get("code"))
		}
		if r.PostForm.Get("code_verifier") != "verifier-123" {
			t.Errorf("code_verifier = %q", r.PostForm.Get("code_verifier"))
		}
		return 200, codexTokenResponse{
			AccessToken:  "codex-access-token",
			RefreshToken: "codex-refresh-token",
			IDToken:      idToken,
			ExpiresIn:    float64(3600),
		}
	})
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	token, err := exchangeCodexAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code:         "auth-code-abc",
		RedirectURI:  "http://localhost:1455/auth/callback",
		CodeVerifier: "verifier-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "codex-access-token" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "codex-refresh-token" {
		t.Errorf("RefreshToken = %q", token.RefreshToken)
	}
	if token.IDToken != idToken {
		t.Errorf("IDToken mismatch")
	}
	if token.Email != "codex-user@example.com" {
		t.Errorf("Email = %q", token.Email)
	}
	if token.AccountID != "acct-codex-1" {
		t.Errorf("AccountID = %q", token.AccountID)
	}
	if token.AccountKey != "acct-codex-1" {
		t.Errorf("AccountKey = %q, should equal AccountID", token.AccountKey)
	}
	if token.PlanType != "pro" {
		t.Errorf("PlanType = %q", token.PlanType)
	}
	if token.TokenExpiresAt <= 0 {
		t.Error("TokenExpiresAt should be set")
	}
}

// ---- Exchange: error / malformed / network ----

func TestExchangeCodexAuthorizationCode_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"bad code"}`))
	}))
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	_, err := exchangeCodexAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "bad-code",
	})
	if err == nil {
		t.Fatal("expected error for 4xx response")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should surface provider body, got %q", err.Error())
	}
}

func TestExchangeCodexAuthorizationCode_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	_, err := exchangeCodexAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "invalid payload") {
		t.Errorf("error should mention invalid payload, got %q", err.Error())
	}
}

func TestExchangeCodexAuthorizationCode_MissingRequiredFields(t *testing.T) {
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		// Missing refresh_token + id_token.
		return 200, map[string]interface{}{
			"access_token": "only-access",
			"expires_in":   3600,
		}
	})
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	_, err := exchangeCodexAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
	if !strings.Contains(err.Error(), "missing required fields") {
		t.Errorf("error should mention missing fields, got %q", err.Error())
	}
}

func TestExchangeCodexAuthorizationCode_InvalidIDToken(t *testing.T) {
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, codexTokenResponse{
			AccessToken:  "access",
			RefreshToken: "refresh",
			IDToken:      "not-a-jwt",
			ExpiresIn:    float64(3600),
		}
	})
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	_, err := exchangeCodexAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected error for invalid id_token")
	}
	if !strings.Contains(err.Error(), "invalid id_token") {
		t.Errorf("error should mention invalid id_token, got %q", err.Error())
	}
}

func TestExchangeCodexAuthorizationCode_MissingChatGPTAccountID(t *testing.T) {
	// Valid JWT shape but the auth claim is absent → missing chatgpt_account_id.
	idToken := makeCodexIDToken(t, codexJWTClaims{Email: "user@example.com"})
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, codexTokenResponse{
			AccessToken:  "access",
			RefreshToken: "refresh",
			IDToken:      idToken,
			ExpiresIn:    float64(3600),
		}
	})
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	_, err := exchangeCodexAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected error for missing chatgpt_account_id")
	}
	if !strings.Contains(err.Error(), "chatgpt_account_id") {
		t.Errorf("error should mention chatgpt_account_id, got %q", err.Error())
	}
}

func TestExchangeCodexAuthorizationCode_NetworkError(t *testing.T) {
	// A server that closes the connection immediately causes a network error
	// surfaced by the http client. Use httptest with a handler that errors.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack and close the connection to simulate a mid-stream network failure.
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	_, err := exchangeCodexAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestExchangeCodexAuthorizationCode_UnreachableServer(t *testing.T) {
	// Point at a port that nothing listens on — guaranteed connection refused.
	withCodexTokenURLSwap(t, "http://127.0.0.1:1/oauth/token")

	_, err := exchangeCodexAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected connection error")
	}
}

// ---- Refresh: success ----

func TestRefreshCodexAccessToken_Success(t *testing.T) {
	claims := codexJWTClaims{Email: "refresh-user@example.com"}
	claims.Auth.ChatGPTAccountID = "acct-refresh-1"
	claims.Auth.ChatGPTPlanType = "team"
	idToken := makeCodexIDToken(t, claims)

	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh_token = %q", r.PostForm.Get("refresh_token"))
		}
		if r.PostForm.Get("scope") != "openid profile email" {
			t.Errorf("scope = %q", r.PostForm.Get("scope"))
		}
		return 200, codexTokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			IDToken:      idToken,
			ExpiresIn:    float64(7200),
		}
	})
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	token, err := refreshCodexAccessToken(context.Background(), RefreshTokenInput{
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
	if token.AccountID != "acct-refresh-1" {
		t.Errorf("AccountID = %q", token.AccountID)
	}
	if token.PlanType != "team" {
		t.Errorf("PlanType = %q", token.PlanType)
	}
}

// ---- Refresh: error / network ----

func TestRefreshCodexAccessToken_ExpiredRefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token expired"}`))
	}))
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	_, err := refreshCodexAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "dead-refresh",
	})
	if err == nil {
		t.Fatal("expected error for expired refresh_token")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should mention invalid_grant, got %q", err.Error())
	}
}

func TestRefreshCodexAccessToken_NetworkError(t *testing.T) {
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
	withCodexTokenURLSwap(t, server.URL)

	_, err := refreshCodexAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "any",
	})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestRefreshCodexAccessToken_MissingChatGPTAccountID(t *testing.T) {
	idToken := makeCodexIDToken(t, codexJWTClaims{Email: "user@example.com"})
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, codexTokenResponse{
			AccessToken:  "access",
			RefreshToken: "refresh",
			IDToken:      idToken,
			ExpiresIn:    float64(3600),
		}
	})
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)

	_, err := refreshCodexAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "any",
	})
	if err == nil {
		t.Fatal("expected error for missing chatgpt_account_id")
	}
}

// ---- parseExpiresIn extra coverage ----

func TestParseExpiresIn_StringPositive(t *testing.T) {
	v, ok := parseExpiresIn("1800")
	if !ok || v != 1800 {
		t.Errorf("string 1800 → (%d, %v)", v, ok)
	}
}

func TestParseExpiresIn_StringNegative(t *testing.T) {
	if _, ok := parseExpiresIn("-5"); ok {
		t.Error("negative string should not be accepted")
	}
}

func TestParseExpiresIn_StringNonNumeric(t *testing.T) {
	if _, ok := parseExpiresIn("abc"); ok {
		t.Error("non-numeric string should not be accepted")
	}
}

func TestParseExpiresIn_FloatZero(t *testing.T) {
	if _, ok := parseExpiresIn(float64(0)); ok {
		t.Error("zero float should not be accepted")
	}
}

func TestParseExpiresIn_UnsupportedType(t *testing.T) {
	if _, ok := parseExpiresIn(int64(3600)); ok {
		t.Error("int64 should not be accepted (only float64/string)")
	}
}

// ---- newJSONTestServer helper ----

// newJSONTestServer returns an httptest.Server that JSON-encodes the value
// returned by the handler. If the handler returns a non-200 status, the body
// is still JSON-encoded. Allows table-driven provider mocks to share a fixture.
// Defined here but shared by claude_flow_test.go and antigravity_flow_test.go
// (all _test.go files in the oauth package share the same scope).
func newJSONTestServer(t *testing.T, handler func(r *http.Request) (status int, body interface{})) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := handler(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			_, _ = io.WriteString(w, `{"error":"encoder failure"}`)
		}
	}))
}
