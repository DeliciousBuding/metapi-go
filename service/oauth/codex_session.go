package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// codexSessionBrowserUserAgent mimics a Chrome browser UA for the
	// session-cookie fallback. The session endpoint is a NextAuth route that
	// expects browser traffic, not CLI traffic.
	codexSessionBrowserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

	// codexSessionCookieName is the NextAuth cookie carrying the web session
	// token used by the fallback refresh path.
	codexSessionCookieName = "__Secure-next-auth.session-token"
)

// codexSessionURL is a package var (not a const) so tests can swap in a local
// httptest server for the session-cookie fallback path.
var codexSessionURL = "https://chatgpt.com/api/auth/session"

// refreshCodexWithSessionToken is the session-cookie fallback invoked by the
// refresh orchestrator after a non-retryable RT refresh failure (e.g. the
// refresh token was revoked but the ChatGPT web session is still valid). It
// hits the NextAuth session endpoint with the __Secure-next-auth.session-token
// cookie and recovers a fresh access token. Identity (email, account id, plan
// type) is parsed from the access_token JWT since the session endpoint does
// not return an id_token. Mirrors codex2api's RefreshWithSessionToken.
func refreshCodexWithSessionToken(ctx context.Context, input SessionTokenInput) (*TokenSet, error) {
	sessionToken := strings.TrimSpace(input.SessionToken)
	if sessionToken == "" {
		return nil, fmt.Errorf("codex session refresh requires a session token")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, codexSessionURL, nil)
	if err != nil {
		return nil, fmt.Errorf("codex session refresh: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", codexSessionBrowserUserAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.AddCookie(&http.Cookie{Name: codexSessionCookieName, Value: sessionToken})

	resp, err := doHTTP(req, input.ProxyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("codex session refresh: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("codex session refresh: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("codex session refresh failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var sessionResp struct {
		AccessToken string `json:"accessToken"`
		Expires     string `json:"expires"`
		User        struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &sessionResp); err != nil {
		return nil, fmt.Errorf("codex session refresh: parse response: %w", err)
	}

	accessToken := strings.TrimSpace(sessionResp.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("codex session refresh: response missing accessToken")
	}

	// The session endpoint returns no id_token and no refresh_token. Identity
	// is recovered from the access_token JWT (email under /profile, account id
	// and plan type under /auth). Email falls back to sessionResp.User.Email.
	identity := ParseCodexAccessToken(accessToken)
	if identity == nil {
		identity = &AccountIdentity{}
	}
	email := identity.Email
	if email == "" {
		email = strings.TrimSpace(sessionResp.User.Email)
	}

	accountID := identity.ChatGPTAccountID
	if accountID == "" {
		return nil, fmt.Errorf("codex session refresh: access_token missing chatgpt_account_id")
	}

	expiresAt := parseCodexSessionExpires(sessionResp.Expires)
	expiresIn := int64(time.Until(expiresAt).Seconds())
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	return &TokenSet{
		AccessToken:    accessToken,
		TokenExpiresAt: time.Now().UnixMilli() + expiresIn*1000,
		Email:          email,
		AccountID:      accountID,
		AccountKey:     accountID,
		PlanType:       identity.PlanType,
	}, nil
}

// parseCodexSessionExpires parses the RFC3339 expiry string returned by the
// NextAuth session endpoint. Returns time.Now()+1h on parse failure so the
// caller always gets a positive expiry.
func parseCodexSessionExpires(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().Add(time.Hour)
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed
		}
	}
	return time.Now().Add(time.Hour)
}
