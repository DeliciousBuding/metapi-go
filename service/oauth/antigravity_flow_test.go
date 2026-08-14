package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// withAntigravityEndpointSwap redirects the antigravity token, userinfo and
// upstream (internal API) URLs to the given servers for the duration of the
// test and restores them on cleanup. Mirrors the grok.go swap pattern; a nil
// argument means "leave that endpoint at its current value".
func withAntigravityEndpointSwap(t *testing.T, tokenURL, userinfoURL, upstreamURL string) {
	t.Helper()
	originalToken := antigravityTokenURL
	originalUserinfo := antigravityUserinfoURL
	originalUpstream := antigravityUpstreamBaseURL
	if tokenURL != "" {
		antigravityTokenURL = tokenURL
	}
	if userinfoURL != "" {
		antigravityUserinfoURL = userinfoURL
	}
	if upstreamURL != "" {
		antigravityUpstreamBaseURL = upstreamURL
	}
	t.Cleanup(func() {
		antigravityTokenURL = originalToken
		antigravityUserinfoURL = originalUserinfo
		antigravityUpstreamBaseURL = originalUpstream
	})
}

// antigravityTokenJSON returns the raw JSON for a Google OAuth token response.
func antigravityTokenJSON(t *testing.T, access, refresh string, expiresIn int) []byte {
	t.Helper()
	full := map[string]interface{}{
		"access_token":  access,
		"refresh_token": refresh,
		"token_type":    "Bearer",
		"expires_in":    expiresIn,
		"scope":         "https://www.googleapis.com/auth/cloud-platform",
	}
	out, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("marshal antigravity payload: %v", err)
	}
	return out
}

// antigravityUserInfoJSON returns the raw JSON for a Google userinfo response.
func antigravityUserInfoJSON(t *testing.T, email string) []byte {
	t.Helper()
	out, err := json.Marshal(map[string]interface{}{"email": email})
	if err != nil {
		t.Fatalf("marshal userinfo: %v", err)
	}
	return out
}

// ---- parseAntigravityExpiresAt ----

func TestParseAntigravityExpiresAt_FloatPositive(t *testing.T) {
	payload := &antigravityOAuthTokenPayload{ExpiresIn: float64(3600)}
	got := parseAntigravityExpiresAt(payload)
	if got <= 0 {
		t.Errorf("float64 3600 should yield future ts, got %d", got)
	}
}

func TestParseAntigravityExpiresAt_FloatZero(t *testing.T) {
	payload := &antigravityOAuthTokenPayload{ExpiresIn: float64(0)}
	if got := parseAntigravityExpiresAt(payload); got != 0 {
		t.Errorf("zero float should yield 0, got %d", got)
	}
}

func TestParseAntigravityExpiresAt_StringPositive(t *testing.T) {
	payload := &antigravityOAuthTokenPayload{ExpiresIn: "1800"}
	got := parseAntigravityExpiresAt(payload)
	if got <= 0 {
		t.Errorf("string 1800 should yield future ts, got %d", got)
	}
}

func TestParseAntigravityExpiresAt_ExpiryRFC3339(t *testing.T) {
	// Expiry field is an absolute RFC3339 timestamp; ExpiresIn absent.
	payload := &antigravityOAuthTokenPayload{Expiry: "2099-01-01T00:00:00Z"}
	got := parseAntigravityExpiresAt(payload)
	if got <= 0 {
		t.Errorf("future RFC3339 expiry should yield non-zero ts, got %d", got)
	}
}

func TestParseAntigravityExpiresAt_InvalidExpiry(t *testing.T) {
	payload := &antigravityOAuthTokenPayload{Expiry: "not-a-date"}
	if got := parseAntigravityExpiresAt(payload); got != 0 {
		t.Errorf("invalid expiry should yield 0, got %d", got)
	}
}

func TestParseAntigravityExpiresAt_UnsupportedType(t *testing.T) {
	payload := &antigravityOAuthTokenPayload{ExpiresIn: int64(3600)}
	if got := parseAntigravityExpiresAt(payload); got != 0 {
		t.Errorf("int64 ExpiresIn should yield 0 (unsupported), got %d", got)
	}
}

// ---- buildAntigravityProxyHeaders ----

func TestBuildAntigravityProxyHeaders_SetsIdentityHeaders(t *testing.T) {
	headers := buildAntigravityProxyHeaders(context.Background(), ProxyHeaderInput{})
	if headers["User-Agent"] != antigravityUserAgent {
		t.Errorf("User-Agent = %q, want %q", headers["User-Agent"], antigravityUserAgent)
	}
	if headers["X-Goog-Api-Client"] != antigravityGoogleAPIClient {
		t.Errorf("X-Goog-Api-Client = %q", headers["X-Goog-Api-Client"])
	}
	if headers["Client-Metadata"] != antigravityClientMetadata {
		t.Errorf("Client-Metadata = %q", headers["Client-Metadata"])
	}
	if len(headers) != 3 {
		t.Errorf("antigravity proxy headers should set exactly 3 headers, got %d", len(headers))
	}
}

func TestBuildAntigravityProxyHeaders_Stateless(t *testing.T) {
	// The antigravity proxy header hook is stateless: it ignores the OAuth
	// context entirely and always returns the same 3 identity headers.
	h1 := buildAntigravityProxyHeaders(context.Background(), ProxyHeaderInput{})
	h2 := buildAntigravityProxyHeaders(context.Background(), ProxyHeaderInput{
		OAuth: ProxyHeaderOAuth{AccountID: "acct", ProjectID: "proj"},
	})
	if h1["User-Agent"] != h2["User-Agent"] {
		t.Error("proxy headers should be identical regardless of OAuth context")
	}
}

// ---- fetchAntigravityUserEmail ----

func TestFetchAntigravityUserEmail_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer ag-access" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(antigravityUserInfoJSON(t, "user@example.com"))
	}))
	defer server.Close()
	withAntigravityEndpointSwap(t, "", server.URL, "")

	email, err := fetchAntigravityUserEmail("ag-access", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "user@example.com" {
		t.Errorf("email = %q", email)
	}
}

func TestFetchAntigravityUserEmail_Non200ReturnsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	withAntigravityEndpointSwap(t, "", server.URL, "")

	email, _ := fetchAntigravityUserEmail("bad-token", nil)
	if email != "" {
		t.Errorf("non-200 should yield empty email, got %q", email)
	}
}

func TestFetchAntigravityUserEmail_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{broken`))
	}))
	defer server.Close()
	withAntigravityEndpointSwap(t, "", server.URL, "")

	// fetchAntigravityUserEmail intentionally swallows the json.Unmarshal
	// error (`_ = json.Unmarshal`), returning empty email + nil error so a
	// transient upstream JSON corruption never breaks the exchange flow.
	email, err := fetchAntigravityUserEmail("ag-access", nil)
	if err != nil {
		t.Fatalf("malformed JSON error should be swallowed, got %v", err)
	}
	if email != "" {
		t.Errorf("malformed JSON should yield empty email, got %q", email)
	}
}

// ---- callAntigravityInternalAPI ----

func TestCallAntigravityInternalAPI_Non200ReturnsNilNoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	withAntigravityEndpointSwap(t, "", "", server.URL)

	result, err := callAntigravityInternalAPI("ag-access", "loadCodeAssist", map[string]interface{}{}, nil)
	if err != nil {
		t.Fatalf("non-200 should not return transport error, got %v", err)
	}
	if result != nil {
		t.Errorf("non-200 should yield nil result, got %v", result)
	}
}

func TestCallAntigravityInternalAPI_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The method is embedded in the path (cloudcode-pa/v1internal:method).
		if !strings.Contains(r.URL.Path, "loadCodeAssist") {
			t.Errorf("expected path to contain loadCodeAssist, got %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ag-access" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("User-Agent") != antigravityUserAgent {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"proj-from-load","allowedTiers":[{"id":"t1","isDefault":true}]}`))
	}))
	defer server.Close()
	withAntigravityEndpointSwap(t, "", "", server.URL)

	result, err := callAntigravityInternalAPI("ag-access", "loadCodeAssist", map[string]interface{}{}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["cloudaicompanionProject"] != "proj-from-load" {
		t.Errorf("cloudaicompanionProject = %v", result["cloudaicompanionProject"])
	}
}

// ---- fetchAntigravityProjectID ----

func TestFetchAntigravityProjectID_LoadReturnsProject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"load-project-123"}`))
	}))
	defer server.Close()
	withAntigravityEndpointSwap(t, "", "", server.URL)

	projectID, _ := fetchAntigravityProjectID("ag-access", nil)
	if projectID != "load-project-123" {
		t.Errorf("projectID = %q, want load-project-123", projectID)
	}
}

func TestFetchAntigravityProjectID_OnboardCompletes(t *testing.T) {
	// loadCodeAssist returns no project; onboardUser returns done=true with a
	// response.cloudaicompanionProject. The polling loop should resolve on
	// the first attempt.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "loadCodeAssist") {
			_, _ = w.Write([]byte(`{"allowedTiers":[{"id":"free-tier","isDefault":true}]}`))
			return
		}
		// onboardUser
		_, _ = w.Write([]byte(`{"done":true,"response":{"cloudaicompanionProject":"onboarded-project"}}`))
	}))
	defer server.Close()
	withAntigravityEndpointSwap(t, "", "", server.URL)

	projectID, _ := fetchAntigravityProjectID("ag-access", nil)
	if projectID != "onboarded-project" {
		t.Errorf("projectID = %q, want onboarded-project", projectID)
	}
}

func TestFetchAntigravityProjectID_OnboardNeverCompletes(t *testing.T) {
	// onboardUser keeps returning done=false → loop exhausts attempts and
	// returns empty.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"done":false}`))
	}))
	defer server.Close()
	withAntigravityEndpointSwap(t, "", "", server.URL)

	projectID, _ := fetchAntigravityProjectID("ag-access", nil)
	if projectID != "" {
		t.Errorf("projectID = %q, want empty when onboarding never completes", projectID)
	}
}

// ---- Exchange: success ----

func TestExchangeAntigravityAuthorizationCode_Success(t *testing.T) {
	// Stand up three mock servers: token, userinfo, internal API (load+onboard).
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", ct)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.PostForm.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("client_id") != antigravityClientID {
			t.Errorf("client_id = %q", r.PostForm.Get("client_id"))
		}
		if r.PostForm.Get("client_secret") != antigravityClientSecret {
			t.Errorf("client_secret = %q", r.PostForm.Get("client_secret"))
		}
		if r.PostForm.Get("code") != "ag-code" {
			t.Errorf("code = %q", r.PostForm.Get("code"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(antigravityTokenJSON(t, "ag-access", "ag-refresh", 3600))
	}))
	defer tokenServer.Close()

	userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(antigravityUserInfoJSON(t, "ag-user@example.com"))
	}))
	defer userinfoServer.Close()

	internalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"ag-project-from-load"}`))
	}))
	defer internalServer.Close()

	withAntigravityEndpointSwap(t, tokenServer.URL, userinfoServer.URL, internalServer.URL)

	token, err := exchangeAntigravityAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code:        "ag-code",
		RedirectURI: "http://localhost:51121/oauth-callback",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "ag-access" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "ag-refresh" {
		t.Errorf("RefreshToken = %q", token.RefreshToken)
	}
	if token.Email != "ag-user@example.com" {
		t.Errorf("Email = %q", token.Email)
	}
	if token.AccountID != "ag-user@example.com" {
		t.Errorf("AccountID = %q", token.Email)
	}
	if token.AccountKey != "ag-user@example.com" {
		t.Errorf("AccountKey = %q", token.AccountKey)
	}
	if token.ProjectID != "ag-project-from-load" {
		t.Errorf("ProjectID = %q", token.ProjectID)
	}
	if token.TokenExpiresAt <= 0 {
		t.Error("TokenExpiresAt should be set")
	}
	if token.ProviderData["scope"] == nil {
		t.Error("ProviderData should include scope")
	}
}

// ---- Exchange: error / malformed / network ----

func TestExchangeAntigravityAuthorizationCode_TokenErrorResponse(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer tokenServer.Close()
	withAntigravityEndpointSwap(t, tokenServer.URL, "", "")

	_, err := exchangeAntigravityAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "bad-code",
	})
	if err == nil {
		t.Fatal("expected error for 4xx token response")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should surface provider body, got %q", err.Error())
	}
}

func TestExchangeAntigravityAuthorizationCode_MalformedJSON(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer tokenServer.Close()
	withAntigravityEndpointSwap(t, tokenServer.URL, "", "")

	_, err := exchangeAntigravityAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "invalid payload") {
		t.Errorf("error should mention invalid payload, got %q", err.Error())
	}
}

func TestExchangeAntigravityAuthorizationCode_MissingAccessToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"refresh_token":"rt-only","expires_in":3600}`))
	}))
	defer tokenServer.Close()
	withAntigravityEndpointSwap(t, tokenServer.URL, "", "")

	_, err := exchangeAntigravityAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
	if !strings.Contains(err.Error(), "missing access token") {
		t.Errorf("error should mention missing access token, got %q", err.Error())
	}
}

func TestExchangeAntigravityAuthorizationCode_NetworkError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer tokenServer.Close()
	withAntigravityEndpointSwap(t, tokenServer.URL, "", "")

	_, err := exchangeAntigravityAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err == nil {
		t.Fatal("expected network error")
	}
}

// ---- Refresh: success ----

func TestRefreshAntigravityAccessToken_Success(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.PostForm.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh_token = %q", r.PostForm.Get("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(antigravityTokenJSON(t, "new-ag-access", "new-ag-refresh", 7200))
	}))
	defer tokenServer.Close()

	userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(antigravityUserInfoJSON(t, "ag-refresh@example.com"))
	}))
	defer userinfoServer.Close()

	internalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"refresh-project"}`))
	}))
	defer internalServer.Close()

	withAntigravityEndpointSwap(t, tokenServer.URL, userinfoServer.URL, internalServer.URL)

	token, err := refreshAntigravityAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "old-refresh",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "new-ag-access" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "new-ag-refresh" {
		t.Errorf("RefreshToken = %q", token.RefreshToken)
	}
	if token.Email != "ag-refresh@example.com" {
		t.Errorf("Email = %q", token.Email)
	}
	if token.ProjectID != "refresh-project" {
		t.Errorf("ProjectID = %q", token.ProjectID)
	}
}

func TestRefreshAntigravityAccessToken_PreservesProjectIDFromInput(t *testing.T) {
	// When OAuth.ProjectID is set, the refresh path must reuse it and NOT call
	// fetchAntigravityProjectID. We assert the internal API is never hit.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(antigravityTokenJSON(t, "access", "refresh", 3600))
	}))
	defer tokenServer.Close()

	userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(antigravityUserInfoJSON(t, "user@example.com"))
	}))
	defer userinfoServer.Close()

	var internalCalls int32
	internalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&internalCalls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cloudaicompanionProject":"should-not-be-used"}`))
	}))
	defer internalServer.Close()

	withAntigravityEndpointSwap(t, tokenServer.URL, userinfoServer.URL, internalServer.URL)

	token, err := refreshAntigravityAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "any-refresh",
		OAuth: &RefreshOAuthContext{
			ProjectID: "preserved-project",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.ProjectID != "preserved-project" {
		t.Errorf("ProjectID = %q, want preserved-project", token.ProjectID)
	}
	if atomic.LoadInt32(&internalCalls) != 0 {
		t.Errorf("internal API should not be called when OAuth.ProjectID is set, got %d calls", internalCalls)
	}
}

// ---- Refresh: error / network ----

func TestRefreshAntigravityAccessToken_ExpiredRefreshToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh token expired"}`))
	}))
	defer tokenServer.Close()
	withAntigravityEndpointSwap(t, tokenServer.URL, "", "")

	_, err := refreshAntigravityAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "dead-refresh",
	})
	if err == nil {
		t.Fatal("expected error for expired refresh_token")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error should mention invalid_grant, got %q", err.Error())
	}
}

func TestRefreshAntigravityAccessToken_MalformedJSON(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{broken`))
	}))
	defer tokenServer.Close()
	withAntigravityEndpointSwap(t, tokenServer.URL, "", "")

	_, err := refreshAntigravityAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "any",
	})
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "invalid payload") {
		t.Errorf("error should mention invalid payload, got %q", err.Error())
	}
}

func TestRefreshAntigravityAccessToken_NetworkError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer tokenServer.Close()
	withAntigravityEndpointSwap(t, tokenServer.URL, "", "")

	_, err := refreshAntigravityAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "any",
	})
	if err == nil {
		t.Fatal("expected network error")
	}
}

// ---- postAntigravityToken (direct helper tests) ----

func TestPostAntigravityToken_MissingAccessToken(t *testing.T) {
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, map[string]interface{}{
			"refresh_token": "rt",
			"expires_in":    3600,
		}
	})
	defer server.Close()
	withAntigravityEndpointSwap(t, server.URL, "", "")

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("code", "any")

	_, err := postAntigravityToken(form, nil)
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
	if !strings.Contains(err.Error(), "missing access token") {
		t.Errorf("error should mention missing access token, got %q", err.Error())
	}
}

// ---- Sanity: antigravity exchange tolerates userinfo/internal failures ----

func TestExchangeAntigravityAuthorizationCode_ToleratesUserinfoFailure(t *testing.T) {
	// fetchAntigravityUserEmail + fetchAntigravityProjectID swallow errors so
	// the exchange still succeeds with empty email/projectID.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(antigravityTokenJSON(t, "access", "refresh", 3600))
	}))
	defer tokenServer.Close()

	// userinfo returns 500, internal API returns 500 — both swallowed.
	brokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer brokenServer.Close()
	withAntigravityEndpointSwap(t, tokenServer.URL, brokenServer.URL, brokenServer.URL)

	token, err := exchangeAntigravityAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code: "any-code",
	})
	if err != nil {
		t.Fatalf("exchange should succeed even with broken aux endpoints: %v", err)
	}
	if token.AccessToken != "access" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.Email != "" {
		t.Errorf("Email should be empty when userinfo fails, got %q", token.Email)
	}
	if token.ProjectID != "" {
		t.Errorf("ProjectID should be empty when internal API fails, got %q", token.ProjectID)
	}
}
