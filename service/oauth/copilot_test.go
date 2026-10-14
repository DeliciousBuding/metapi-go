package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func withCopilotEndpointSwap(t *testing.T, deviceURL, tokenURL, exchangeURL, userURL string) {
	t.Helper()
	oldDeviceURL := copilotDeviceCodeURL
	oldTokenURL := copilotTokenURL
	oldExchangeURL := copilotTokenExchangeURL
	oldUserURL := copilotUserURL
	if deviceURL != "" {
		copilotDeviceCodeURL = deviceURL
	}
	if tokenURL != "" {
		copilotTokenURL = tokenURL
	}
	if exchangeURL != "" {
		copilotTokenExchangeURL = exchangeURL
	}
	if userURL != "" {
		copilotUserURL = userURL
	}
	t.Cleanup(func() {
		copilotDeviceCodeURL = oldDeviceURL
		copilotTokenURL = oldTokenURL
		copilotTokenExchangeURL = oldExchangeURL
		copilotUserURL = oldUserURL
	})
}

func TestCopilotProviderMetadata(t *testing.T) {
	def := GetProviderDefinition(string(ProviderCopilot))
	if def == nil {
		t.Fatal("github-copilot provider not registered")
	}
	if def.Metadata.Platform != "github-copilot" {
		t.Errorf("Platform = %q", def.Metadata.Platform)
	}
	if !def.Metadata.SupportsDeviceFlow {
		t.Error("github-copilot must declare Device Flow support")
	}
	if def.Site.URL != copilotUpstreamBaseURL {
		t.Errorf("Site.URL = %q", def.Site.URL)
	}
}

func TestBuildCopilotAuthorizationURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.PostForm.Get("client_id") != copilotClientID {
			t.Errorf("client_id = %q", r.PostForm.Get("client_id"))
		}
		if r.PostForm.Get("scope") != copilotScope {
			t.Errorf("scope = %q", r.PostForm.Get("scope"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(copilotDeviceCodeResponse{
			DeviceCode:              "device-code",
			UserCode:                "ABCD-1234",
			VerificationURI:         "https://github.com/login/device",
			VerificationURIComplete: "https://github.com/login/device?user_code=ABCD-1234",
			Interval:                5,
			ExpiresIn:               900,
		})
	}))
	defer server.Close()
	withCopilotEndpointSwap(t, server.URL, "", "", "")

	urlStr, err := buildCopilotAuthorizationURL(context.Background(), BuildAuthURLInput{State: "copilot-state"})
	if err != nil {
		t.Fatalf("build authorization URL: %v", err)
	}
	if urlStr != "https://github.com/login/device?user_code=ABCD-1234" {
		t.Errorf("authorization URL = %q", urlStr)
	}
	entry, ok := lookupDeviceFlow("copilot-state")
	if !ok {
		t.Fatal("device flow state missing")
	}
	if entry.deviceCode != "device-code" || entry.userCode != "ABCD-1234" {
		t.Errorf("stored entry = %+v", entry)
	}
	if entry.interval != 5*time.Second {
		t.Errorf("interval = %v", entry.interval)
	}
	clearDeviceFlow("copilot-state")
}

func TestExchangeCopilotAuthorizationCode_PendingKeepsDeviceCode(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(copilotGitHubTokenResponse{Error: "authorization_pending"})
	}))
	defer tokenServer.Close()
	withCopilotEndpointSwap(t, "", tokenServer.URL, "", "")
	storeDeviceFlow("copilot-pending", "device-code", "ABCD-1234", "https://github.com/login/device", 5*time.Second, time.Minute)
	defer clearDeviceFlow("copilot-pending")

	_, err := exchangeCopilotAuthorizationCode(context.Background(), ExchangeCodeInput{State: "copilot-pending"})
	if err == nil {
		t.Fatal("expected pending error")
	}
	var pending *DeviceFlowPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("expected DeviceFlowPendingError, got %T", err)
	}
	if pending.RetryAfter != 5*time.Second {
		t.Errorf("RetryAfter = %v", pending.RetryAfter)
	}
	if _, ok := lookupDeviceFlow("copilot-pending"); !ok {
		t.Fatal("pending must keep device_code")
	}
}

func TestGetSessionStatus_CopilotPendingPoll(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(copilotGitHubTokenResponse{Error: "authorization_pending"})
	}))
	defer tokenServer.Close()
	withCopilotEndpointSwap(t, "", tokenServer.URL, "", "")

	session, err := CreateSession(CreateSessionInput{Provider: string(ProviderCopilot)})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	storeDeviceFlow(session.State, "device-code", "ABCD-1234", "https://github.com/login/device", 5*time.Second, time.Minute)
	defer clearDeviceFlow(session.State)

	status := GetSessionStatus(context.Background(), session.State)
	if status == nil {
		t.Fatal("session status missing")
	}
	if status.Status != SessionPending {
		t.Fatalf("Status = %q, want pending", status.Status)
	}
	if status.RetryAfterSeconds != 5 {
		t.Errorf("RetryAfterSeconds = %d", status.RetryAfterSeconds)
	}
	if got := GetSession(session.State); got == nil || got.Status != SessionPending {
		t.Fatalf("session was incorrectly settled: %+v", got)
	}
}

func TestExchangeCopilotAuthorizationCode_Success(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.PostFormValue("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
			t.Errorf("grant_type = %q", r.PostFormValue("grant_type"))
		}
		if r.PostFormValue("device_code") != "device-code" {
			t.Errorf("device_code = %q", r.PostFormValue("device_code"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(copilotGitHubTokenResponse{
			AccessToken: "github-access",
			TokenType:   "bearer",
			Scope:       "read:user",
		})
	}))
	defer tokenServer.Close()

	exchangeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token github-access" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(copilotTokenResponse{
			Token:     "copilot-access",
			ExpiresAt: time.Now().Add(time.Hour).Unix(),
		})
	}))
	defer exchangeServer.Close()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token github-access" {
			t.Errorf("user Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(copilotUserResponse{Login: "octocat", ID: 123})
	}))
	defer userServer.Close()

	withCopilotEndpointSwap(t, "", tokenServer.URL, exchangeServer.URL, userServer.URL)
	storeDeviceFlow("copilot-success", "device-code", "ABCD-1234", "https://github.com/login/device", 5*time.Second, time.Minute)

	token, err := exchangeCopilotAuthorizationCode(context.Background(), ExchangeCodeInput{State: "copilot-success"})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if token.AccessToken != "copilot-access" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "github-access" {
		t.Errorf("RefreshToken = %q", token.RefreshToken)
	}
	if token.AccountKey != "octocat" || token.AccountID != "123" {
		t.Errorf("identity = %q/%q", token.AccountKey, token.AccountID)
	}
	if token.TokenExpiresAt <= time.Now().UnixMilli() {
		t.Error("TokenExpiresAt must be in milliseconds")
	}
	if _, ok := lookupDeviceFlow("copilot-success"); ok {
		t.Error("successful exchange must consume device_code")
	}
}

func TestRefreshCopilotAccessToken(t *testing.T) {
	exchangeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "token github-access" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(copilotTokenResponse{
			Token:     "fresh-copilot-access",
			ExpiresAt: time.Now().Add(30 * time.Minute).Unix(),
		})
	}))
	defer exchangeServer.Close()
	withCopilotEndpointSwap(t, "", "", exchangeServer.URL, "")

	token, err := refreshCopilotAccessToken(context.Background(), RefreshTokenInput{
		RefreshToken: "github-access",
		OAuth: &RefreshOAuthContext{ProviderData: map[string]interface{}{
			"githubScope": "read:user",
		}},
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if token.AccessToken != "fresh-copilot-access" {
		t.Errorf("AccessToken = %q", token.AccessToken)
	}
	if token.RefreshToken != "github-access" {
		t.Errorf("durable GitHub token was not preserved: %q", token.RefreshToken)
	}
	if token.ProviderData["githubScope"] != "read:user" {
		t.Errorf("ProviderData = %+v", token.ProviderData)
	}
}

func TestExchangeCopilotToken_RejectsIncompletePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"copilot-token"}`))
	}))
	defer server.Close()
	withCopilotEndpointSwap(t, "", "", server.URL, "")

	_, err := exchangeCopilotToken(context.Background(), "github-access", nil)
	if err == nil || !strings.Contains(err.Error(), "expires_at") {
		t.Fatalf("expected missing expires_at error, got %v", err)
	}
}

func TestExchangeCopilotToken_AuthFailureIsNonRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()
	withCopilotEndpointSwap(t, "", "", server.URL, "")

	_, err := exchangeCopilotToken(context.Background(), "invalid-github-access", nil)
	if err == nil || !IsNonRetryable(err) {
		t.Fatalf("401 token exchange must be non-retryable, got %v", err)
	}
}

func TestBuildCopilotProxyHeaders(t *testing.T) {
	headers := buildCopilotProxyHeaders(context.Background(), ProxyHeaderInput{})
	if headers["User-Agent"] != "GitHubCopilotChat/0.26.7" {
		t.Errorf("User-Agent = %q", headers["User-Agent"])
	}
	if headers["X-Initiator"] != "user" {
		t.Errorf("default X-Initiator = %q", headers["X-Initiator"])
	}
	headers = buildCopilotProxyHeaders(context.Background(), ProxyHeaderInput{
		DownstreamHeaders: map[string]interface{}{"X-Initiator": "agent"},
	})
	if headers["X-Initiator"] != "agent" {
		t.Errorf("downstream X-Initiator was not honored: %q", headers["X-Initiator"])
	}
	if _, hasAuthorization := headers["Authorization"]; hasAuthorization {
		t.Error("provider headers must not set Authorization; the proxy layer owns it")
	}
}
