package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

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
	payload, err := postOAuthForm(callCtx, grokDeviceCodeURL, form, nil)
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

	ttl := time.Duration(device.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = grokDeviceCodeTTL
	}
	storeDeviceFlow(input.State, device.DeviceCode, device.UserCode, device.VerificationURI, interval, ttl)

	if strings.TrimSpace(device.VerificationURIComplete) != "" {
		return device.VerificationURIComplete, nil
	}
	return device.VerificationURI, nil
}

// exchangeGrokAuthorizationCode polls the xAI token endpoint using the
// device_code stashed during buildGrokAuthorizationURL. This performs a single
// poll attempt (mirroring the reference implementation's pollDevice); the
// flow layer is responsible for retrying on authorization_pending.
//
// TODO: flow.go currently drives completion via HandleCallback, which requires
// a redirect `code`. Device OAuth has no callback — completion must instead be
// triggered by a poll-based status endpoint that calls this function until it
// resolves. Wiring that poll trigger is intentionally not done yet.
func exchangeGrokAuthorizationCode(ctx context.Context, input ExchangeCodeInput) (*TokenSet, error) {
	state := strings.TrimSpace(input.State)
	if state == "" {
		return nil, fmt.Errorf("grok device oauth: missing state for token exchange")
	}

	entry, ok := lookupDeviceFlow(state)
	if !ok {
		return nil, fmt.Errorf("grok device oauth: no device_code for state %q (expired or not started)", state)
	}

	callCtx, cancel := context.WithTimeout(ctx, grokHTTPTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("client_id", grokClientID)
	form.Set("device_code", entry.deviceCode)

	payload, err := postOAuthForm(callCtx, grokTokenURL, form, input.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("grok device oauth: token request failed: %w", err)
	}

	var token grokTokenResponse
	if err := json.Unmarshal(payload, &token); err != nil {
		return nil, fmt.Errorf("grok device oauth: invalid token payload: %w", err)
	}

	if token.Error != "" {
		switch token.Error {
		case "authorization_pending":
			return nil, &DeviceFlowPendingError{
				Reason:     "authorization_pending",
				RetryAfter: entry.interval,
			}
		case "slow_down":
			retryAfter := slowDownDeviceFlow(state)
			return nil, &DeviceFlowPendingError{
				Reason:     "slow_down",
				RetryAfter: retryAfter,
			}
		default:
			msg := token.Error
			if token.ErrorDescription != "" {
				msg = msg + ": " + token.ErrorDescription
			}
			clearDeviceFlow(state)
			return nil, fmt.Errorf("grok device oauth: %s", msg)
		}
	}

	if strings.TrimSpace(token.AccessToken) == "" {
		clearDeviceFlow(state)
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

	clearDeviceFlow(state)

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

// resolveGrokPollInterval exposes the stashed poll interval for tests; the
// production flow uses sessionStatusResult's shared projection.
func resolveGrokPollInterval(state string) time.Duration {
	entry, ok := lookupDeviceFlow(state)
	if !ok {
		return grokDefaultPollInterval
	}
	if entry.interval <= 0 {
		return grokDefaultPollInterval
	}
	return entry.interval
}
