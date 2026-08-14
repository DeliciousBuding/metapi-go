package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// xAI/Grok Device OAuth constants.
//
// Endpoints and the public client_id are sourced from the grok2api-chenyme
// reference implementation (backend/internal/infra/provider/cli/oauth.go).
// xAI exposes a public Device OAuth client for the Grok CLI; the client_id is
// not secret and is hardcoded here (mirrors the Antigravity placeholder pattern
// for vendor-published public clients).
// grokDeviceCodeURL and grokTokenURL are package vars (not consts) so tests
// can swap in a local httptest server without monkey-patching net.Dialer.
var (
	grokDeviceCodeURL = "https://auth.x.ai/oauth2/device/code"
	grokTokenURL       = "https://auth.x.ai/oauth2/token"
)

const (
	grokClientID            = "b1a00492-073a-47ea-816f-4c329264a828"
	grokDefaultScope        = "openid profile email offline_access grok-cli:access api:access"
	grokUpstreamBaseURL     = "https://api.x.ai"
	grokUserAgent           = "grok-cli/1.0 (oauth)"
	grokLoopbackPort        = 0 // Device OAuth has no redirect callback.
	grokLoopbackPath        = ""
	grokLoopbackRedirectURI = "" // Device OAuth does not use a redirect URI.
	// grokHTTPTimeout bounds each outbound call to the xAI auth endpoints.
	grokHTTPTimeout = 30 * time.Second
	// grokDeviceCodeTTL bounds how long a device_code remains usable in our
	// side-channel store. xAI typically returns expires_in=1800 (30 min);
	// we keep a small margin above that.
	grokDeviceCodeTTL = 35 * time.Minute
	// grokDefaultPollInterval is the fallback poll interval when xAI does not
	// return an explicit `interval` field.
	grokDefaultPollInterval = 5 * time.Second
)

// ---- Device-code side-channel store ----
//
// Device OAuth splits the flow across two provider hooks:
//   1. BuildAuthorizationURL  — POST /device/code, hand the user the
//      verification URI, and stash the device_code for the exchange step.
//   2. ExchangeAuthorizationCode — POST /token with the stashed device_code.
//
// The shared session record has no field for a device_code, so we keep a
// short-lived side-channel map keyed by the OAuth `state`. Entries are pruned
// by TTL to avoid unbounded growth from abandoned flows.
var (
	grokDeviceCodes   = make(map[string]grokDeviceCodeEntry)
	grokDeviceCodesMu sync.Mutex
)

type grokDeviceCodeEntry struct {
	deviceCode      string
	verificationURI string
	interval        time.Duration
	expiresAt       time.Time
}

func storeGrokDeviceCode(state, deviceCode, verificationURI string, interval time.Duration) {
	grokDeviceCodesMu.Lock()
	defer grokDeviceCodesMu.Unlock()
	grokDeviceCodes[state] = grokDeviceCodeEntry{
		deviceCode:      deviceCode,
		verificationURI: verificationURI,
		interval:        interval,
		expiresAt:       time.Now().Add(grokDeviceCodeTTL),
	}
}

func lookupGrokDeviceCode(state string) (grokDeviceCodeEntry, bool) {
	grokDeviceCodesMu.Lock()
	defer grokDeviceCodesMu.Unlock()
	pruneExpiredGrokDeviceCodes(time.Now())
	entry, ok := grokDeviceCodes[state]
	return entry, ok
}

func clearGrokDeviceCode(state string) {
	grokDeviceCodesMu.Lock()
	defer grokDeviceCodesMu.Unlock()
	delete(grokDeviceCodes, state)
}

func pruneExpiredGrokDeviceCodes(now time.Time) {
	for state, entry := range grokDeviceCodes {
		if !entry.expiresAt.After(now) {
			delete(grokDeviceCodes, state)
		}
	}
}

// ---- Registration ----

func init() {
	RegisterProvider(&OAuthProviderDefinition{
		Metadata: ProviderMetadata{
			Provider:                     ProviderGrok,
			Label:                        "Grok",
			Platform:                     "grok",
			Enabled:                      true,
			LoginType:                    "oauth",
			RequiresProjectId:            false,
			SupportsDirectAccountRouting: true,
			SupportsCloudValidation:      false,
			SupportsNativeProxy:          true,
		},
		Site: ProviderSiteConfig{
			Name:     "xAI Grok OAuth",
			URL:      grokUpstreamBaseURL,
			Platform: "grok",
		},
		// Loopback is intentionally zero-valued: Device OAuth has no redirect
		// callback. The shared flow layer still starts a loopback listener for
		// every provider, but for grok it simply never receives a callback —
		// completion is driven by polling the token endpoint.
		Loopback: LoopbackConfig{
			Host:        "127.0.0.1",
			Port:        grokLoopbackPort,
			Path:        grokLoopbackPath,
			RedirectURI: grokLoopbackRedirectURI,
		},
		BuildAuthorizationURL:     buildGrokAuthorizationURL,
		ExchangeAuthorizationCode: exchangeGrokAuthorizationCode,
		RefreshAccessToken:        refreshGrokAccessToken,
		BuildProxyHeaders:         buildGrokProxyHeaders,
	})
}

// ---- Device-code start (BuildAuthorizationURL) ----

// grokDeviceCodeResponse models the xAI /oauth2/device/code response.
type grokDeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

// buildGrokAuthorizationURL starts the Device OAuth flow by requesting a
// device code from xAI, stashing the device_code keyed by the session state,
// and returning the verification URI the user must visit. When xAI returns
// verification_uri_complete (embeds the user_code), that is preferred;
// otherwise the bare verification_uri is returned.
func buildGrokAuthorizationURL(ctx context.Context, input BuildAuthURLInput) (string, error) {
	if strings.TrimSpace(input.State) == "" {
		return "", fmt.Errorf("grok device oauth: missing state")
	}

	callCtx, cancel := context.WithTimeout(ctx, grokHTTPTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("client_id", grokClientID)
	form.Set("scope", grokDefaultScope)

	// BuildAuthURLInput carries no proxy URL (the shared flow layer resolves
	// the proxy later, at exchange time). The device-code start call therefore
	// goes direct; if a per-flow proxy is configured it will apply to the
	// subsequent token-exchange poll instead.
	payload, err := postGrokForm(callCtx, grokDeviceCodeURL, form, nil)
	if err != nil {
		return "", fmt.Errorf("grok device oauth: device-code request failed: %w", err)
	}

	var device grokDeviceCodeResponse
	if err := json.Unmarshal(payload, &device); err != nil {
		return "", fmt.Errorf("grok device oauth: invalid device-code payload: %w", err)
	}
	if device.DeviceCode == "" || device.VerificationURI == "" {
		return "", fmt.Errorf("grok device oauth: device-code response missing required fields")
	}

	interval := grokDefaultPollInterval
	if device.Interval > 0 {
		interval = time.Duration(device.Interval) * time.Second
	}

	storeGrokDeviceCode(input.State, device.DeviceCode, device.VerificationURI, interval)

	if strings.TrimSpace(device.VerificationURIComplete) != "" {
		return device.VerificationURIComplete, nil
	}
	return device.VerificationURI, nil
}

// ---- Token exchange (poll) ----

// grokTokenResponse models the xAI /oauth2/token response. The same shape is
// reused for device-code grant and refresh-token grant.
type grokTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	// Error / RFC 8628 polling error fields.
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// grokIDTokenClaims is the subset of JWT claims we read from the xAI id_token.
type grokIDTokenClaims struct {
	Email string `json:"email"`
	Sub   string `json:"sub"`
	Name  string `json:"name"`
}

// exchangeGrokAuthorizationCode polls the xAI token endpoint using the
// device_code stashed during buildGrokAuthorizationURL. This performs a single
// poll attempt (mirroring the reference implementation's pollDevice); the
// flow layer is responsible for retrying on authorization_pending.
//
// TODO: flow.go currently drives completion via HandleCallback, which requires
// a redirect `code`. Device OAuth has no callback — completion must instead be
// triggered by a poll-based status endpoint that calls this function until it
// resolves. Wiring that poll trigger is intentionally out of Tier 1 scope.
func exchangeGrokAuthorizationCode(ctx context.Context, input ExchangeCodeInput) (*TokenSet, error) {
	state := strings.TrimSpace(input.State)
	if state == "" {
		return nil, fmt.Errorf("grok device oauth: missing state for token exchange")
	}

	entry, ok := lookupGrokDeviceCode(state)
	if !ok {
		return nil, fmt.Errorf("grok device oauth: no device_code for state %q (expired or not started)", state)
	}

	callCtx, cancel := context.WithTimeout(ctx, grokHTTPTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("client_id", grokClientID)
	form.Set("device_code", entry.deviceCode)

	payload, err := postGrokForm(callCtx, grokTokenURL, form, input.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("grok device oauth: token request failed: %w", err)
	}

	var token grokTokenResponse
	if err := json.Unmarshal(payload, &token); err != nil {
		return nil, fmt.Errorf("grok device oauth: invalid token payload: %w", err)
	}

	if token.Error != "" {
		// Surface RFC 8628 polling errors verbatim so the flow layer can decide
		// whether to retry (authorization_pending / slow_down) or abort
		// (access_denied / expired_token).
		msg := token.Error
		if token.ErrorDescription != "" {
			msg = msg + ": " + token.ErrorDescription
		}
		clearGrokDeviceCode(state)
		return nil, fmt.Errorf("grok device oauth: %s", msg)
	}

	if strings.TrimSpace(token.AccessToken) == "" {
		clearGrokDeviceCode(state)
		return nil, fmt.Errorf("grok device oauth: token response missing access_token")
	}

	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	email, sub := parseGrokIDToken(token.IDToken)
	accountKey := email
	if accountKey == "" {
		accountKey = sub
	}

	providerData := map[string]interface{}{}
	if token.TokenType != "" {
		providerData["tokenType"] = token.TokenType
	}
	if token.Scope != "" {
		providerData["scope"] = token.Scope
	}
	if token.IDToken != "" {
		providerData["idToken"] = token.IDToken
	}

	clearGrokDeviceCode(state)

	return &TokenSet{
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		TokenExpiresAt: time.Now().UnixMilli() + int64(expiresIn)*1000,
		Email:          email,
		AccountKey:     accountKey,
		AccountID:      sub,
		IDToken:        token.IDToken,
		ProviderData:   providerData,
	}, nil
}

// ---- Token refresh ----

func refreshGrokAccessToken(ctx context.Context, input RefreshTokenInput) (*TokenSet, error) {
	refreshToken := strings.TrimSpace(input.RefreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("grok oauth: missing refresh_token")
	}

	callCtx, cancel := context.WithTimeout(ctx, grokHTTPTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", grokClientID)
	form.Set("refresh_token", refreshToken)

	payload, err := postGrokForm(callCtx, grokTokenURL, form, input.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("grok oauth: refresh request failed: %w", err)
	}

	var token grokTokenResponse
	if err := json.Unmarshal(payload, &token); err != nil {
		return nil, fmt.Errorf("grok oauth: invalid refresh payload: %w", err)
	}
	if token.Error != "" {
		msg := token.Error
		if token.ErrorDescription != "" {
			msg = msg + ": " + token.ErrorDescription
		}
		return nil, fmt.Errorf("grok oauth: %s", msg)
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, fmt.Errorf("grok oauth: refresh response missing access_token")
	}

	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	email, sub := parseGrokIDToken(token.IDToken)
	accountKey := email
	if accountKey == "" {
		accountKey = sub
	}
	// Preserve provider data from the previous token set when the refresh
	// response omits it.
	if input.OAuth != nil && input.OAuth.ProviderData != nil {
		if _, ok := providerDataGet(input.OAuth.ProviderData, "scope"); !ok && token.Scope != "" {
			providerDataSet(input.OAuth.ProviderData, "scope", token.Scope)
		}
		merged := mergeGrokProviderData(input.OAuth.ProviderData, token)
		return &TokenSet{
			AccessToken:    token.AccessToken,
			RefreshToken:   firstGrokNonEmpty(token.RefreshToken, refreshToken),
			TokenExpiresAt: time.Now().UnixMilli() + int64(expiresIn)*1000,
			Email:          email,
			AccountKey:     accountKey,
			AccountID:      sub,
			IDToken:        token.IDToken,
			ProviderData:   merged,
		}, nil
	}

	providerData := map[string]interface{}{}
	if token.TokenType != "" {
		providerData["tokenType"] = token.TokenType
	}
	if token.Scope != "" {
		providerData["scope"] = token.Scope
	}
	if token.IDToken != "" {
		providerData["idToken"] = token.IDToken
	}

	return &TokenSet{
		AccessToken:    token.AccessToken,
		RefreshToken:   firstGrokNonEmpty(token.RefreshToken, refreshToken),
		TokenExpiresAt: time.Now().UnixMilli() + int64(expiresIn)*1000,
		Email:          email,
		AccountKey:     accountKey,
		AccountID:      sub,
		IDToken:        token.IDToken,
		ProviderData:   providerData,
	}, nil
}

// ---- Proxy headers ----

func buildGrokProxyHeaders(ctx context.Context, input ProxyHeaderInput) map[string]string {
	// The Authorization: Bearer <access_token> header is applied centrally by
	// the proxy layer from the account's stored access token. Here we only add
	// the xAI/Grok client identity headers that the upstream expects on every
	// chat-completion request.
	return map[string]string{
		"User-Agent": grokUserAgent,
	}
}

// ---- HTTP + helpers ----

// postGrokForm POSTs an x-www-form-urlencoded body to an xAI endpoint and
// returns the raw JSON payload. Non-2xx responses are returned as errors with
// the trimmed body so callers can surface xAI's error description.
func postGrokForm(ctx context.Context, endpoint string, form url.Values, proxyURL *string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := doHTTP(req, proxyURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readOAuthErrorResponseBody(resp.Body)
		return nil, fmt.Errorf("xAI returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// parseGrokIDToken decodes the JWT id_token returned by xAI and extracts the
// email and sub claims. Returns empty strings (no error) when the id_token is
// absent or not a parseable JWT — callers treat absence as a soft fallback.
func parseGrokIDToken(idToken string) (email, sub string) {
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return "", ""
	}
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", ""
	}
	decoded, err := base64Decode(parts[1])
	if err != nil {
		return "", ""
	}
	var claims grokIDTokenClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return "", ""
	}
	return strings.TrimSpace(claims.Email), strings.TrimSpace(claims.Sub)
}

func firstGrokNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func providerDataGet(data map[string]interface{}, key string) (interface{}, bool) {
	if data == nil {
		return nil, false
	}
	value, ok := data[key]
	return value, ok
}

func providerDataSet(data map[string]interface{}, key string, value interface{}) {
	if data == nil {
		return
	}
	data[key] = value
}

func mergeGrokProviderData(existing map[string]interface{}, token grokTokenResponse) map[string]interface{} {
	merged := make(map[string]interface{})
	for key, value := range existing {
		merged[key] = value
	}
	if token.TokenType != "" {
		merged["tokenType"] = token.TokenType
	}
	if token.Scope != "" {
		merged["scope"] = token.Scope
	}
	if token.IDToken != "" {
		merged["idToken"] = token.IDToken
	}
	return merged
}

// resolveGrokPollInterval exposes the stashed poll interval for a state so the
// (future) flow-layer poller can pace retries. Kept unexported but package-
// visible for tests + the eventual poll trigger.
func resolveGrokPollInterval(state string) time.Duration {
	entry, ok := lookupGrokDeviceCode(state)
	if !ok {
		return grokDefaultPollInterval
	}
	if entry.interval <= 0 {
		return grokDefaultPollInterval
	}
	return entry.interval
}
