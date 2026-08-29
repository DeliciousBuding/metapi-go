package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
)

const (
	codexAuthURL             = "https://auth.openai.com/oauth/authorize"
	codexLoopbackPort        = 1455
	codexLoopbackPath        = "/auth/callback"
	codexLoopbackRedirectURI = "http://localhost:1455/auth/callback"
	codexUpstreamBaseURL     = "https://chatgpt.com/backend-api/codex"

	// codexCLIUserAgent mirrors the Codex CLI client fingerprint used by the
	// upstream OAuth/token endpoints. Without it, OpenAI's auth gateway is
	// more likely to classify requests as non-CLI traffic.
	codexCLIUserAgent = "codex-cli/0.91.0"

	// codexSessionBrowserUserAgent mimics a Chrome browser UA for the
	// session-cookie fallback. The session endpoint is a NextAuth route that
	// expects browser traffic, not CLI traffic.
	codexSessionBrowserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

	// codexSessionCookieName is the NextAuth cookie carrying the web session
	// token used by the fallback refresh path.
	codexSessionCookieName = "__Secure-next-auth.session-token"
)

// codexTokenURL is a package var (not a const) so tests can swap in a local
// httptest server via withCodexEndpointSwap, mirroring the grok.go pattern.
var codexTokenURL = "https://auth.openai.com/oauth/token"

// codexSessionURL is a package var (not a const) so tests can swap in a local
// httptest server for the session-cookie fallback path.
var codexSessionURL = "https://chatgpt.com/api/auth/session"

func init() {
	RegisterProvider(&OAuthProviderDefinition{
		Metadata: ProviderMetadata{
			Provider:                     ProviderCodex,
			Label:                        "Codex",
			Platform:                     "codex",
			Enabled:                      true,
			LoginType:                    "oauth",
			RequiresProjectId:            false,
			SupportsDirectAccountRouting: true,
			SupportsCloudValidation:      true,
			SupportsNativeProxy:          true,
		},
		Site: ProviderSiteConfig{
			Name:     "ChatGPT Codex OAuth",
			URL:      codexUpstreamBaseURL,
			Platform: "codex",
		},
		Loopback: LoopbackConfig{
			Host:        "127.0.0.1",
			Port:        codexLoopbackPort,
			Path:        codexLoopbackPath,
			RedirectURI: codexLoopbackRedirectURI,
		},
		BuildAuthorizationURL:     buildCodexAuthorizationURL,
		ExchangeAuthorizationCode: exchangeCodexAuthorizationCode,
		RefreshAccessToken:        refreshCodexAccessToken,
		RefreshWithSessionToken:   refreshCodexWithSessionToken,
		BuildProxyHeaders:         buildCodexProxyHeaders,
		ParseAccessToken:           ParseCodexAccessToken,
	})
}

func requireCodexClientID() (string, error) {
	id := strings.TrimSpace(config.Get().CodexClientId)
	if id == "" {
		return "", fmt.Errorf("CODEX_CLIENT_ID is not configured")
	}
	return id, nil
}

// ---- Auth URL ----

func buildCodexAuthorizationURL(ctx context.Context, input BuildAuthURLInput) (string, error) {
	params := url.Values{}
	clientID, err := requireCodexClientID()
	if err != nil {
		return "", err
	}
	params.Set("client_id", clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", input.RedirectURI)
	params.Set("scope", "openid email profile offline_access")
	params.Set("state", input.State)
	params.Set("code_challenge", CreatePKCEChallenge(input.CodeVerifier))
	params.Set("code_challenge_method", "S256")
	params.Set("prompt", "login")
	params.Set("id_token_add_organizations", "true")
	params.Set("codex_cli_simplified_flow", "true")
	return codexAuthURL + "?" + params.Encode(), nil
}

// ---- Token Exchange ----

type codexJWTClaims struct {
	Email string `json:"email"`
	Auth  struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		ChatGPTPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

type codexTokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	IDToken      string      `json:"id_token"`
	ExpiresIn    interface{} `json:"expires_in"`
}

func parseJWTClaims(token string) (*codexJWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT")
	}
	decoded, err := base64Decode(parts[1])
	if err != nil {
		return nil, err
	}
	var claims codexJWTClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func parseExpiresIn(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return int64(v), true
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}

func exchangeCodexToken(form url.Values, proxyURL *string) (*codexTokenResponse, error) {
	req, err := http.NewRequest("POST", codexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", codexCLIUserAgent)

	resp, err := doHTTP(req, proxyURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body := readOAuthErrorResponseBody(resp.Body)
		return nil, fmt.Errorf("%s", string(body))
	}

	body, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload codexTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("codex token exchange returned invalid payload")
	}

	accessToken := strings.TrimSpace(payload.AccessToken)
	refreshToken := strings.TrimSpace(payload.RefreshToken)
	idToken := strings.TrimSpace(payload.IDToken)
	expiresIn, hasExpiresIn := parseExpiresIn(payload.ExpiresIn)

	if accessToken == "" || refreshToken == "" || idToken == "" || !hasExpiresIn || expiresIn <= 0 {
		return nil, fmt.Errorf("codex token exchange response missing required fields")
	}
	return &payload, nil
}

func exchangeCodexAuthorizationCode(ctx context.Context, input ExchangeCodeInput) (*TokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", func() string { id, _ := requireCodexClientID(); return id }())
	form.Set("code", input.Code)
	form.Set("redirect_uri", input.RedirectURI)
	form.Set("code_verifier", input.CodeVerifier)

	payload, err := exchangeCodexToken(form, input.ProxyURL)
	if err != nil {
		return nil, err
	}
	if _, idErr := requireCodexClientID(); idErr != nil {
		return nil, idErr
	}
	return buildCodexTokenSetFromPayload(payload, "token exchange")
}

// ---- Token Refresh ----

func refreshCodexAccessToken(ctx context.Context, input RefreshTokenInput) (*TokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", func() string { id, _ := requireCodexClientID(); return id }())
	form.Set("refresh_token", input.RefreshToken)
	form.Set("scope", "openid profile email")

	payload, err := exchangeCodexToken(form, input.ProxyURL)
	if err != nil {
		return nil, err
	}
	return buildCodexTokenSetFromPayload(payload, "token refresh")
}

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
		Expires    string `json:"expires"`
		User       struct {
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
		PlanType:        identity.PlanType,
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

// buildCodexTokenSetFromPayload converts a raw codex token response into a
// TokenSet, applying the access_token identity fallback codex2api uses: when
// the id_token omits email/account_id/plan_type, the access_token JWT (which
// surfaces email under the /profile namespace) recovers them. account_id is
// still required after the merge so an anonymous token fails loudly.
func buildCodexTokenSetFromPayload(payload *codexTokenResponse, origin string) (*TokenSet, error) {
	claims, err := parseJWTClaims(payload.IDToken)
	if err != nil {
		return nil, fmt.Errorf("codex %s response invalid id_token", origin)
	}

	identity := &AccountIdentity{
		Email:             strings.TrimSpace(claims.Email),
		ChatGPTAccountID: strings.TrimSpace(claims.Auth.ChatGPTAccountID),
		PlanType:          strings.TrimSpace(claims.Auth.ChatGPTPlanType),
	}
	if identityIncomplete(identity) {
		identity = MergeCodexIdentity(identity, ParseCodexAccessToken(payload.AccessToken))
	}

	accountID := strings.TrimSpace(identity.ChatGPTAccountID)
	if accountID == "" {
		return nil, fmt.Errorf("codex %s response missing chatgpt_account_id", origin)
	}

	expiresIn, _ := parseExpiresIn(payload.ExpiresIn)
	return &TokenSet{
		AccessToken:    payload.AccessToken,
		RefreshToken:   payload.RefreshToken,
		TokenExpiresAt: time.Now().UnixMilli() + expiresIn*1000,
		Email:          identity.Email,
		AccountID:      accountID,
		AccountKey:     accountID,
		PlanType:       identity.PlanType,
		IDToken:        payload.IDToken,
	}, nil
}

// ---- Proxy Headers ----

func buildCodexProxyHeaders(ctx context.Context, input ProxyHeaderInput) map[string]string {
	accountID := input.OAuth.AccountID
	if accountID == "" {
		accountID = input.OAuth.AccountKey
	}
	originator := getHeaderValue(input.DownstreamHeaders, "originator")
	if originator == "" {
		originator = "codex_cli_rs"
	}
	headers := map[string]string{
		"Originator": originator,
	}
	if accountID != "" {
		headers["Chatgpt-Account-Id"] = accountID
	}
	return headers
}

// ---- Utilities ----

func base64Decode(encoded string) ([]byte, error) {
	// Handle standard and URL-safe base64.
	decoded, err := base64DecodeRaw(encoded)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func base64DecodeRaw(encoded string) ([]byte, error) {
	// Add padding if needed.
	switch len(encoded) % 4 {
	case 2:
		encoded += "=="
	case 3:
		encoded += "="
	}
	return base64.URLEncoding.DecodeString(
		strings.NewReplacer("-", "+", "_", "/").Replace(encoded),
	)
}

func getHeaderValue(headers map[string]interface{}, key string) string {
	if headers == nil {
		return ""
	}
	lowerKey := strings.ToLower(key)
	for k, v := range headers {
		if strings.ToLower(k) == lowerKey {
			if s, ok := v.(string); ok {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					return trimmed
				}
			}
			if arr, ok := v.([]interface{}); ok {
				for _, item := range arr {
					if s, ok := item.(string); ok {
						trimmed := strings.TrimSpace(s)
						if trimmed != "" {
							return trimmed
						}
					}
				}
			}
		}
	}
	return ""
}

func doHTTP(req *http.Request, proxyURL *string, client *http.Client) (*http.Response, error) {
	if client == nil {
		client = newOAuthHTTPClient(nil)
	}
	if proxyURL != nil && strings.TrimSpace(*proxyURL) != "" {
		proxy, err := url.Parse(strings.TrimSpace(*proxyURL))
		if err != nil {
			return nil, fmt.Errorf("invalid oauth proxy URL: %w", err)
		}
		if proxy.Scheme == "" || proxy.Host == "" {
			return nil, fmt.Errorf("invalid oauth proxy URL: missing scheme or host")
		}
		client.Transport = newOAuthHTTPTransport(http.ProxyURL(proxy))
	}
	return client.Do(req)
}

// oauthHTTPTimeout bounds the whole OAuth control-plane request; the same
// value caps the transport's response-header phase. It is passed explicitly
// to the httpclient baseline so package defaults can never shadow it.
const oauthHTTPTimeout = 30 * time.Second

func newOAuthHTTPClient(proxy func(*http.Request) (*url.URL, error)) *http.Client {
	return httpclient.NewClient(newOAuthHTTPTransport(proxy), oauthHTTPTimeout, platform.RejectCrossOriginRedirect)
}

// newOAuthHTTPTransport builds the OAuth control-plane transport on the
// shared baseline with its historical phase bounds (dial 10s, TLS 10s,
// header phase 30s, idle 30s). A nil proxy means NoProxy: OAuth token
// exchange must never inherit HTTP_PROXY/HTTPS_PROXY from the operator
// environment (locked by TestDoHTTPIgnoresEnvironmentProxyWithoutProxyURL).
func newOAuthHTTPTransport(proxy func(*http.Request) (*url.URL, error)) *http.Transport {
	if proxy == nil {
		proxy = httpclient.NoProxy
	}
	return httpclient.NewTransport(httpclient.Options{
		DialTimeout:           10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: oauthHTTPTimeout,
		IdleConnTimeout:       30 * time.Second,
		Proxy:                 proxy,
	})
}
