package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---- Provider metadata / registration ----

func TestProviderMetadata_Grok(t *testing.T) {
	def := GetProviderDefinition(string(ProviderGrok))
	if def == nil {
		t.Fatal("grok provider not registered")
	}
	meta := def.Metadata
	if meta.Provider != ProviderGrok {
		t.Errorf("Provider = %q, want grok", meta.Provider)
	}
	if meta.Label != "Grok" {
		t.Errorf("Label = %q, want Grok", meta.Label)
	}
	if meta.Platform != "grok" {
		t.Errorf("Platform = %q, want grok", meta.Platform)
	}
	if !meta.Enabled {
		t.Error("grok should be enabled")
	}
	if meta.LoginType != "oauth" {
		t.Errorf("LoginType = %q, want oauth", meta.LoginType)
	}
	if meta.RequiresProjectId {
		t.Error("grok should not require project ID")
	}
	if !meta.SupportsDirectAccountRouting {
		t.Error("grok should support direct account routing")
	}
	if !meta.SupportsNativeProxy {
		t.Error("grok should support native proxy")
	}
	if meta.SupportsCloudValidation {
		t.Error("grok should NOT claim cloud validation (no xAI balance API)")
	}
}

func TestProviderSiteConfig_Grok(t *testing.T) {
	def := GetProviderDefinition(string(ProviderGrok))
	if def.Site.Name != "xAI Grok OAuth" {
		t.Errorf("Site.Name = %q", def.Site.Name)
	}
	if def.Site.URL != "https://api.x.ai" {
		t.Errorf("Site.URL = %q", def.Site.URL)
	}
	if def.Site.Platform != "grok" {
		t.Errorf("Site.Platform = %q", def.Site.Platform)
	}
}

func TestLoopbackConfig_Grok_ZeroValued(t *testing.T) {
	// Device OAuth has no redirect callback, so the loopback config must be
	// zero-valued (no port, no path, no redirect URI).
	def := GetProviderDefinition(string(ProviderGrok))
	if def.Loopback.Port != 0 {
		t.Errorf("Loopback.Port = %d, want 0 (device flow has no callback)", def.Loopback.Port)
	}
	if def.Loopback.Path != "" {
		t.Errorf("Loopback.Path = %q, want empty", def.Loopback.Path)
	}
	if def.Loopback.RedirectURI != "" {
		t.Errorf("Loopback.RedirectURI = %q, want empty", def.Loopback.RedirectURI)
	}
}

// ---- Proxy headers ----

func TestBuildProxyHeaders_Grok(t *testing.T) {
	def := GetProviderDefinition(string(ProviderGrok))
	headers := def.BuildProxyHeaders(context.Background(), ProxyHeaderInput{})
	if headers == nil {
		t.Fatal("expected non-nil headers")
	}
	if headers["User-Agent"] != grokUserAgent {
		t.Errorf("User-Agent = %q, want %q", headers["User-Agent"], grokUserAgent)
	}
	// The proxy layer applies the Authorization header centrally from the
	// stored access token; the provider hook must not double-set it.
	if _, hasAuth := headers["Authorization"]; hasAuth {
		t.Error("grok proxy headers should not set Authorization (applied centrally)")
	}
}

// ---- Pure helper tests ----

func TestGrokDeviceCodeStore_StoreLookupClear(t *testing.T) {
	// Use a unique state to avoid colliding with other tests.
	state := "test-store-state-" + t.Name()
	storeGrokDeviceCode(state, "device-xyz", "https://verify.example/abc", 5*time.Second)

	entry, ok := lookupGrokDeviceCode(state)
	if !ok {
		t.Fatal("expected device code to be present after store")
	}
	if entry.deviceCode != "device-xyz" {
		t.Errorf("deviceCode = %q", entry.deviceCode)
	}
	if entry.verificationURI != "https://verify.example/abc" {
		t.Errorf("verificationURI = %q", entry.verificationURI)
	}
	if entry.interval != 5*time.Second {
		t.Errorf("interval = %v", entry.interval)
	}

	clearGrokDeviceCode(state)
	if _, ok := lookupGrokDeviceCode(state); ok {
		t.Error("device code should be gone after clear")
	}
}

func TestGrokDeviceCodeStore_PrunesExpired(t *testing.T) {
	grokDeviceCodesMu.Lock()
	grokDeviceCodes["expired-state"] = grokDeviceCodeEntry{
		deviceCode:      "old",
		verificationURI: "https://verify.example/old",
		interval:        5 * time.Second,
		expiresAt:       time.Now().Add(-1 * time.Minute), // already expired
	}
	grokDeviceCodesMu.Unlock()

	if _, ok := lookupGrokDeviceCode("expired-state"); ok {
		t.Error("expired device code should be pruned on lookup")
	}
}

func TestParseGrokIDToken(t *testing.T) {
	// Empty token → empty claims, no error path.
	email, sub := parseGrokIDToken("")
	if email != "" || sub != "" {
		t.Errorf("empty id_token should yield empty claims, got email=%q sub=%q", email, sub)
	}

	// Malformed token (not 3 parts) → empty claims.
	email, sub = parseGrokIDToken("not-a-jwt")
	if email != "" || sub != "" {
		t.Errorf("malformed id_token should yield empty claims, got email=%q sub=%q", email, sub)
	}

	// Valid JWT with email + sub claims.
	claims := grokIDTokenClaims{Email: "user@example.com", Sub: "grok-user-123", Name: "Grok User"}
	claimsBytes, _ := json.Marshal(claims)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	// header.payload.signature
	jwt := "header." + encodedClaims + ".signature"
	email, sub = parseGrokIDToken(jwt)
	if email != "user@example.com" {
		t.Errorf("email = %q, want user@example.com", email)
	}
	if sub != "grok-user-123" {
		t.Errorf("sub = %q, want grok-user-123", sub)
	}
}

func TestMergeGrokProviderData(t *testing.T) {
	existing := map[string]interface{}{
		"tokenType": "Bearer",
		"scope":     "old-scope",
	}
	token := grokTokenResponse{
		TokenType: "Bearer",
		Scope:     "new-scope",
		IDToken:   "new-id-token",
	}
	merged := mergeGrokProviderData(existing, token)
	if merged["scope"] != "new-scope" {
		t.Errorf("scope should be overwritten: %v", merged["scope"])
	}
	if merged["idToken"] != "new-id-token" {
		t.Errorf("idToken should be set: %v", merged["idToken"])
	}
	if merged["tokenType"] != "Bearer" {
		t.Errorf("tokenType should be preserved: %v", merged["tokenType"])
	}
	// Original map should not be mutated (merge returns a new map).
	if existing["idToken"] != nil {
		t.Error("mergeGrokProviderData must not mutate the input map")
	}
}

func TestFirstGrokNonEmpty(t *testing.T) {
	if got := firstGrokNonEmpty("", "  ", "fallback"); got != "fallback" {
		t.Errorf("firstGrokNonEmpty = %q, want fallback", got)
	}
	if got := firstGrokNonEmpty("first", "second"); got != "first" {
		t.Errorf("firstGrokNonEmpty = %q, want first", got)
	}
	if got := firstGrokNonEmpty("", ""); got != "" {
		t.Errorf("firstGrokNonEmpty = %q, want empty", got)
	}
}

func TestResolveGrokPollInterval(t *testing.T) {
	// Unknown state → default interval.
	if got := resolveGrokPollInterval("definitely-unknown-state"); got != grokDefaultPollInterval {
		t.Errorf("resolveGrokPollInterval(unknown) = %v, want default %v", got, grokDefaultPollInterval)
	}

	// Known state → stashed interval.
	state := "test-interval-state-" + t.Name()
	storeGrokDeviceCode(state, "code", "https://verify.example", 11*time.Second)
	defer clearGrokDeviceCode(state)
	if got := resolveGrokPollInterval(state); got != 11*time.Second {
		t.Errorf("resolveGrokPollInterval(known) = %v, want 11s", got)
	}
}

// ---- Error-path tests (no HTTP) ----

func TestBuildGrokAuthorizationURL_MissingState(t *testing.T) {
	_, err := buildGrokAuthorizationURL(context.Background(), BuildAuthURLInput{State: ""})
	if err == nil || !strings.Contains(err.Error(), "missing state") {
		t.Errorf("expected missing-state error, got %v", err)
	}
}

func TestExchangeGrokAuthorizationCode_MissingState(t *testing.T) {
	_, err := exchangeGrokAuthorizationCode(context.Background(), ExchangeCodeInput{State: ""})
	if err == nil || !strings.Contains(err.Error(), "missing state") {
		t.Errorf("expected missing-state error, got %v", err)
	}
}

func TestExchangeGrokAuthorizationCode_UnknownState(t *testing.T) {
	_, err := exchangeGrokAuthorizationCode(context.Background(), ExchangeCodeInput{State: "never-started-state"})
	if err == nil || !strings.Contains(err.Error(), "no device_code") {
		t.Errorf("expected no-device_code error, got %v", err)
	}
}

func TestRefreshGrokAccessToken_MissingRefreshToken(t *testing.T) {
	_, err := refreshGrokAccessToken(context.Background(), RefreshTokenInput{RefreshToken: ""})
	if err == nil || !strings.Contains(err.Error(), "missing refresh_token") {
		t.Errorf("expected missing refresh_token error, got %v", err)
	}
}

// ---- End-to-end Device OAuth flow against a local mock ----

// withGrokEndpointSwap swaps the package-level endpoint URLs for the duration
// of a test and restores them on cleanup. Returns the device-code URL and the
// token URL the mock should serve.
func withGrokEndpointSwap(t *testing.T, deviceURL, tokenURL string) {
	t.Helper()
	originalDevice := grokDeviceCodeURL
	originalToken := grokTokenURL
	grokDeviceCodeURL = deviceURL
	grokTokenURL = tokenURL
	t.Cleanup(func() {
		grokDeviceCodeURL = originalDevice
		grokTokenURL = originalToken
	})
}

func TestBuildGrokAuthorizationURL_DeviceFlow_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("device-code endpoint method = %q, want POST", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if r.PostForm.Get("client_id") != grokClientID {
			t.Errorf("client_id = %q", r.PostForm.Get("client_id"))
		}
		if r.PostForm.Get("scope") != grokDefaultScope {
			t.Errorf("scope = %q", r.PostForm.Get("scope"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grokDeviceCodeResponse{
			DeviceCode:              "dc-123",
			UserCode:                "USER-ABC",
			VerificationURI:         "https://auth.x.ai/device",
			VerificationURIComplete: "https://auth.x.ai/device?user_code=USER-ABC",
			Interval:                5,
			ExpiresIn:               1800,
		})
	}))
	defer server.Close()

	withGrokEndpointSwap(t, server.URL, grokTokenURL)

	urlStr, err := buildGrokAuthorizationURL(context.Background(), BuildAuthURLInput{
		State: "flow-state",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// verification_uri_complete is preferred when present.
	if urlStr != "https://auth.x.ai/device?user_code=USER-ABC" {
		t.Errorf("authorization URL = %q", urlStr)
	}

	// The device_code must be stashed for the exchange step.
	entry, ok := lookupGrokDeviceCode("flow-state")
	if !ok {
		t.Fatal("expected device_code to be stashed for the flow state")
	}
	if entry.deviceCode != "dc-123" {
		t.Errorf("stashed deviceCode = %q", entry.deviceCode)
	}
	if entry.interval != 5*time.Second {
		t.Errorf("stashed interval = %v", entry.interval)
	}
	clearGrokDeviceCode("flow-state")
}

func TestBuildGrokAuthorizationURL_DeviceFlow_PrefersBareVerificationURI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No verification_uri_complete — must fall back to verification_uri.
		_ = json.NewEncoder(w).Encode(grokDeviceCodeResponse{
			DeviceCode:      "dc-456",
			UserCode:        "CODE",
			VerificationURI: "https://auth.x.ai/device",
			Interval:        0, // exercises default-interval fallback
		})
	}))
	defer server.Close()

	withGrokEndpointSwap(t, server.URL, grokTokenURL)

	urlStr, err := buildGrokAuthorizationURL(context.Background(), BuildAuthURLInput{State: "bare-state"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if urlStr != "https://auth.x.ai/device" {
		t.Errorf("expected bare verification_uri, got %q", urlStr)
	}
	entry, _ := lookupGrokDeviceCode("bare-state")
	if entry.interval != grokDefaultPollInterval {
		t.Errorf("expected default interval fallback, got %v", entry.interval)
	}
	clearGrokDeviceCode("bare-state")
}

func TestBuildGrokAuthorizationURL_DeviceFlow_MissingFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// DeviceCode present but VerificationURI absent → must error.
		_ = json.NewEncoder(w).Encode(grokDeviceCodeResponse{
			DeviceCode: "dc-789",
		})
	}))
	defer server.Close()

	withGrokEndpointSwap(t, server.URL, grokTokenURL)

	_, err := buildGrokAuthorizationURL(context.Background(), BuildAuthURLInput{State: "missing-state"})
	if err == nil || !strings.Contains(err.Error(), "missing required fields") {
		t.Errorf("expected missing-fields error, got %v", err)
	}
	// Nothing should be stashed on failure.
	if _, ok := lookupGrokDeviceCode("missing-state"); ok {
		t.Error("device_code must not be stashed when the response is incomplete")
	}
}

func TestExchangeGrokAuthorizationCode_DeviceFlow_Success(t *testing.T) {
	// Build a valid id_token JWT so parseGrokIDToken extracts email + sub.
	claims := grokIDTokenClaims{Email: "ellon@example.com", Sub: "xai-sub-42"}
	claimsBytes, _ := json.Marshal(claims)
	idToken := "header." + base64.RawURLEncoding.EncodeToString(claimsBytes) + ".sig"

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.PostFormValue("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("grant_type = %q", r.PostFormValue("grant_type"))
		}
		if r.PostFormValue("device_code") != "dc-exchange" {
			t.Errorf("device_code = %q, want dc-exchange", r.PostFormValue("device_code"))
		}
		if r.PostFormValue("client_id") != grokClientID {
			t.Errorf("client_id = %q", r.PostFormValue("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grokTokenResponse{
			AccessToken:  "access-token-abc",
			RefreshToken: "refresh-token-def",
			IDToken:      idToken,
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			Scope:        grokDefaultScope,
		})
	}))
	defer tokenServer.Close()

	withGrokEndpointSwap(t, grokDeviceCodeURL, tokenServer.URL)
	storeGrokDeviceCode("exchange-state", "dc-exchange", "https://auth.x.ai/device", 5*time.Second)

	token, err := exchangeGrokAuthorizationCode(context.Background(), ExchangeCodeInput{
		State: "exchange-state",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "access-token-abc" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "refresh-token-def" {
		t.Errorf("RefreshToken = %q", token.RefreshToken)
	}
	if token.Email != "ellon@example.com" {
		t.Errorf("Email = %q", token.Email)
	}
	if token.AccountID != "xai-sub-42" {
		t.Errorf("AccountID = %q", token.AccountID)
	}
	if token.AccountKey != "ellon@example.com" {
		t.Errorf("AccountKey = %q, want email", token.AccountKey)
	}
	if token.TokenExpiresAt <= 0 {
		t.Error("TokenExpiresAt should be set")
	}
	if token.ProviderData["scope"] != grokDefaultScope {
		t.Errorf("ProviderData.scope = %v", token.ProviderData["scope"])
	}
	if token.ProviderData["tokenType"] != "Bearer" {
		t.Errorf("ProviderData.tokenType = %v", token.ProviderData["tokenType"])
	}
	// Device code must be consumed (cleared) after a successful exchange.
	if _, ok := lookupGrokDeviceCode("exchange-state"); ok {
		t.Error("device_code should be cleared after successful exchange")
	}
}

func TestExchangeGrokAuthorizationCode_DeviceFlow_PendingError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// RFC 8628 polling error: authorization_pending. xAI returns 200 with
		// an error field rather than a non-2xx status in some flows.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grokTokenResponse{
			Error:            "authorization_pending",
			ErrorDescription: "the user has not yet authorized the device",
		})
	}))
	defer tokenServer.Close()

	withGrokEndpointSwap(t, grokDeviceCodeURL, tokenServer.URL)
	storeGrokDeviceCode("pending-state", "dc-pending", "https://auth.x.ai/device", 5*time.Second)

	_, err := exchangeGrokAuthorizationCode(context.Background(), ExchangeCodeInput{State: "pending-state"})
	if err == nil {
		t.Fatal("expected authorization_pending error")
	}
	if !strings.Contains(err.Error(), "authorization_pending") {
		t.Errorf("error should mention authorization_pending, got %v", err)
	}
	// Terminal/aborted errors should clear the device code so we don't retry
	// a stale code. (authorization_pending surfaces the error verbatim and the
	// device_code is cleared so the caller must restart the flow on a true
	// terminal state; for pending the flow layer is expected to re-poll by
	// re-exchanging — see TODO in grok.go.)
}

func TestExchangeGrokAuthorizationCode_DeviceFlow_MissingAccessToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grokTokenResponse{
			RefreshToken: "rt-only",
			ExpiresIn:    3600,
		})
	}))
	defer tokenServer.Close()

	withGrokEndpointSwap(t, grokDeviceCodeURL, tokenServer.URL)
	storeGrokDeviceCode("missing-access-state", "dc-missing", "https://auth.x.ai/device", 5*time.Second)

	_, err := exchangeGrokAuthorizationCode(context.Background(), ExchangeCodeInput{State: "missing-access-state"})
	if err == nil || !strings.Contains(err.Error(), "missing access_token") {
		t.Errorf("expected missing access_token error, got %v", err)
	}
}

func TestRefreshGrokAccessToken_Success(t *testing.T) {
	claims := grokIDTokenClaims{Email: "refresh@example.com", Sub: "refresh-sub"}
	claimsBytes, _ := json.Marshal(claims)
	idToken := "h." + base64.RawURLEncoding.EncodeToString(claimsBytes) + ".s"

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.PostFormValue("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.PostFormValue("grant_type"))
		}
		if r.PostFormValue("refresh_token") != "original-rt" {
			t.Errorf("refresh_token = %q", r.PostFormValue("refresh_token"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grokTokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			IDToken:      idToken,
			ExpiresIn:    7200,
		})
	}))
	defer tokenServer.Close()

	withGrokEndpointSwap(t, grokDeviceCodeURL, tokenServer.URL)

	token, err := refreshGrokAccessToken(context.Background(), RefreshTokenInput{RefreshToken: "original-rt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	// Refresh must prefer the new refresh_token, falling back to the original.
	if token.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q, want new-refresh", token.RefreshToken)
	}
	if token.Email != "refresh@example.com" {
		t.Errorf("Email = %q", token.Email)
	}
	if token.TokenExpiresAt <= 0 {
		t.Error("TokenExpiresAt should be set")
	}
}

func TestRefreshGrokAccessToken_PreservesRefreshTokenWhenOmitted(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// xAI may omit refresh_token on refresh (token rotation not guaranteed).
		_ = json.NewEncoder(w).Encode(grokTokenResponse{
			AccessToken: "new-access",
			ExpiresIn:   3600,
		})
	}))
	defer tokenServer.Close()

	withGrokEndpointSwap(t, grokDeviceCodeURL, tokenServer.URL)

	token, err := refreshGrokAccessToken(context.Background(), RefreshTokenInput{RefreshToken: "original-rt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.RefreshToken != "original-rt" {
		t.Errorf("RefreshToken = %q, want original preserved", token.RefreshToken)
	}
}

func TestRefreshGrokAccessToken_ErrorResponse(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(grokTokenResponse{
			Error:            "invalid_grant",
			ErrorDescription: "refresh token expired",
		})
	}))
	defer tokenServer.Close()

	withGrokEndpointSwap(t, grokDeviceCodeURL, tokenServer.URL)

	_, err := refreshGrokAccessToken(context.Background(), RefreshTokenInput{RefreshToken: "dead-rt"})
	if err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("expected invalid_grant error, got %v", err)
	}
}
