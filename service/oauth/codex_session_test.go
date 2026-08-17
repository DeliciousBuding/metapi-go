package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// buildCodexAccessTokenForTest assembles an unsigned access_token JWT whose
// payload carries the /profile email and /auth account_id/plan_type claims.
func buildCodexAccessTokenForTest(t *testing.T, profileEmail, accountID, planType string) string {
	t.Helper()
	header := map[string]string{"alg": "none", "typ": "JWT"}
	auth := map[string]any{}
	if accountID != "" {
		auth["chatgpt_account_id"] = accountID
	}
	if planType != "" {
		auth["chatgpt_plan_type"] = planType
	}
	profile := map[string]any{}
	if profileEmail != "" {
		profile["email"] = profileEmail
	}
	claims := map[string]any{
		"https://api.openai.com/auth":    auth,
		"https://api.openai.com/profile": profile,
	}
	headerBytes, _ := json.Marshal(header)
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	encode := func(b []byte) string {
		return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")
	}
	return encode(headerBytes) + "." + encode(payloadBytes) + ".sig"
}

// withCodexSessionServer swaps codexSessionURL to a test server and restores
// it on cleanup.
func withCodexSessionServer(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	original := codexSessionURL
	server := httptest.NewServer(handler)
	codexSessionURL = server.URL
	t.Cleanup(func() {
		codexSessionURL = original
		server.Close()
	})
}

func TestRefreshCodexWithSessionToken_Success(t *testing.T) {
	accessToken := buildCodexAccessTokenForTest(t, "session-user@example.com", "acc_session", "plus")
	expires := time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339)
	withCodexSessionServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		cookie, err := r.Cookie(codexSessionCookieName)
		if err != nil || cookie == nil {
			t.Errorf("expected %s cookie, got %v", codexSessionCookieName, err)
		} else if cookie.Value != "st-valid" {
			t.Errorf("cookie value = %q, want %q", cookie.Value, "st-valid")
		}
		if ua := r.Header.Get("User-Agent"); ua != codexSessionBrowserUserAgent {
			t.Errorf("User-Agent = %q", ua)
		}
		if v := r.Header.Get("Sec-Fetch-Mode"); v != "cors" {
			t.Errorf("Sec-Fetch-Mode = %q", v)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken": accessToken,
			"expires":      expires,
			"user":         map[string]string{"email": "fallback@example.com"},
		})
	})

	token, err := refreshCodexWithSessionToken(context.Background(), SessionTokenInput{
		SessionToken: "st-valid",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if token.AccessToken != accessToken {
		t.Errorf("AccessToken mismatch")
	}
	if token.Email != "session-user@example.com" {
		t.Errorf("Email = %q, want from AT /profile", token.Email)
	}
	if token.AccountID != "acc_session" {
		t.Errorf("AccountID = %q", token.AccountID)
	}
	if token.PlanType != "plus" {
		t.Errorf("PlanType = %q", token.PlanType)
	}
	if token.TokenExpiresAt <= 0 {
		t.Errorf("TokenExpiresAt = %d, want positive", token.TokenExpiresAt)
	}
}

func TestRefreshCodexWithSessionToken_EmailFallbackFromUser(t *testing.T) {
	// AT has no /profile email → fall back to sessionResp.User.Email
	accessToken := buildCodexAccessTokenForTest(t, "", "acc_no_email", "pro")
	withCodexSessionServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken": accessToken,
			"expires":     time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			"user":        map[string]string{"email": "from-user@example.com"},
		})
	})

	token, err := refreshCodexWithSessionToken(context.Background(), SessionTokenInput{
		SessionToken: "st-valid",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if token.Email != "from-user@example.com" {
		t.Errorf("Email = %q, want user fallback", token.Email)
	}
}

func TestRefreshCodexWithSessionToken_MissingSessionToken(t *testing.T) {
	_, err := refreshCodexWithSessionToken(context.Background(), SessionTokenInput{})
	if err == nil {
		t.Fatal("expected error for empty session token")
	}
	if !strings.Contains(err.Error(), "session token") {
		t.Errorf("error should mention session token, got %q", err.Error())
	}
}

func TestRefreshCodexWithSessionToken_HTTPError(t *testing.T) {
	withCodexSessionServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":"unauthorized"}`)
	})
	_, err := refreshCodexWithSessionToken(context.Background(), SessionTokenInput{
		SessionToken: "st-revoked",
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention HTTP 401, got %q", err.Error())
	}
}

func TestRefreshCodexWithSessionToken_MissingAccessToken(t *testing.T) {
	withCodexSessionServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken": "",
			"user":         map[string]string{"email": "x@example.com"},
		})
	})
	_, err := refreshCodexWithSessionToken(context.Background(), SessionTokenInput{
		SessionToken: "st-valid",
	})
	if err == nil {
		t.Fatal("expected error for missing accessToken")
	}
	if !strings.Contains(err.Error(), "accessToken") {
		t.Errorf("error should mention accessToken, got %q", err.Error())
	}
}

func TestRefreshCodexWithSessionToken_ATMissingAccountID(t *testing.T) {
	accessToken := buildCodexAccessTokenForTest(t, "e@example.com", "", "plan")
	withCodexSessionServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accessToken": accessToken,
			"expires":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	})
	_, err := refreshCodexWithSessionToken(context.Background(), SessionTokenInput{
		SessionToken: "st-valid",
	})
	if err == nil {
		t.Fatal("expected error for missing chatgpt_account_id")
	}
	if !strings.Contains(err.Error(), "chatgpt_account_id") {
		t.Errorf("error should mention chatgpt_account_id, got %q", err.Error())
	}
}

func TestParseCodexSessionExpires_EmptyReturnsDefault(t *testing.T) {
	got := parseCodexSessionExpires("")
	if got.IsZero() {
		t.Error("expected non-zero time for empty input")
	}
	if time.Until(got) > time.Hour+time.Second || time.Until(got) < time.Hour-time.Second {
		t.Errorf("expected ~1h default, got %v", time.Until(got))
	}
}

func TestParseCodexSessionExpires_InvalidReturnsDefault(t *testing.T) {
	got := parseCodexSessionExpires("not-a-date")
	if got.IsZero() {
		t.Error("expected non-zero time for invalid input")
	}
}

func TestParseCodexSessionExpires_ValidRFC3339(t *testing.T) {
	raw := "2026-12-31T23:59:59Z"
	got := parseCodexSessionExpires(raw)
	if got.IsZero() {
		t.Fatal("expected parsed time")
	}
	// Round-trip check
	if got.UTC().Format(time.RFC3339) != raw {
		t.Errorf("round-trip mismatch: got %v", got.UTC().Format(time.RFC3339))
	}
}

func TestCodexProviderDefinition_RegistersSessionCapability(t *testing.T) {
	def := GetProviderDefinition("codex")
	if def == nil {
		t.Fatal("codex provider not registered")
	}
	if def.RefreshWithSessionToken == nil {
		t.Error("RefreshWithSessionToken should be registered for codex")
	}
	if def.ParseAccessToken == nil {
		t.Error("ParseAccessToken should be registered for codex")
	}
}
