package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GitHub Copilot uses RFC 8628 Device Flow to obtain a GitHub OAuth access
// token, then exchanges that token for a short-lived Copilot token. The
// account's access_token stores the Copilot token; its refresh_token stores
// the durable GitHub OAuth token used for later exchanges.
var (
	copilotDeviceCodeURL    = "https://github.com/login/device/code"
	copilotTokenURL         = "https://github.com/login/oauth/access_token"
	copilotTokenExchangeURL = "https://api.github.com/copilot_internal/v2/token"
	copilotUserURL          = "https://api.github.com/user"
)

const (
	copilotClientID        = "Iv1.b507a08c87ecfe98"
	copilotScope           = "read:user"
	copilotUpstreamBaseURL = "https://api.githubcopilot.com"
	copilotHTTPTimeout     = 30 * time.Second
	copilotSessionTTL      = 15 * time.Minute
)

type copilotDeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

type copilotGitHubTokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	Scope            string `json:"scope"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type copilotTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

type copilotUserResponse struct {
	Login string `json:"login"`
	ID    int64  `json:"id"`
}

func init() {
	RegisterProvider(&OAuthProviderDefinition{
		Metadata: ProviderMetadata{
			Provider:                     ProviderCopilot,
			Label:                        "GitHub Copilot",
			Platform:                     "github-copilot",
			Enabled:                      true,
			LoginType:                    "oauth",
			RequiresProjectId:            false,
			SupportsDirectAccountRouting: true,
			SupportsCloudValidation:      false,
			SupportsNativeProxy:          true,
			SupportsDeviceFlow:           true,
		},
		Site: ProviderSiteConfig{
			Name:     "GitHub Copilot",
			URL:      copilotUpstreamBaseURL,
			Platform: "github-copilot",
		},
		Loopback: LoopbackConfig{
			Host: "127.0.0.1",
			Port: 0,
		},
		SessionTTL:                copilotSessionTTL,
		BuildAuthorizationURL:     buildCopilotAuthorizationURL,
		ExchangeAuthorizationCode: exchangeCopilotAuthorizationCode,
		RefreshAccessToken:        refreshCopilotAccessToken,
		BuildProxyHeaders:         buildCopilotProxyHeaders,
	})
}

func buildCopilotAuthorizationURL(ctx context.Context, input BuildAuthURLInput) (string, error) {
	if strings.TrimSpace(input.State) == "" {
		return "", fmt.Errorf("github copilot device oauth: missing state")
	}

	callCtx, cancel := context.WithTimeout(ctx, copilotHTTPTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("client_id", copilotClientID)
	form.Set("scope", copilotScope)
	payload, err := postOAuthForm(callCtx, copilotDeviceCodeURL, form, nil)
	if err != nil {
		return "", fmt.Errorf("github copilot device oauth: device-code request failed: %w", err)
	}

	var device copilotDeviceCodeResponse
	if err := json.Unmarshal(payload, &device); err != nil {
		return "", fmt.Errorf("github copilot device oauth: invalid device-code payload: %w", err)
	}
	if device.DeviceCode == "" || device.VerificationURI == "" || device.UserCode == "" {
		return "", fmt.Errorf("github copilot device oauth: device-code response missing required fields")
	}

	interval := 5 * time.Second
	if device.Interval > 0 {
		interval = time.Duration(device.Interval) * time.Second
	}
	ttl := time.Duration(device.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = copilotSessionTTL
	}
	storeDeviceFlow(input.State, device.DeviceCode, device.UserCode, device.VerificationURI, interval, ttl)

	if strings.TrimSpace(device.VerificationURIComplete) != "" {
		return device.VerificationURIComplete, nil
	}
	return device.VerificationURI, nil
}

func exchangeCopilotAuthorizationCode(ctx context.Context, input ExchangeCodeInput) (*TokenSet, error) {
	state := strings.TrimSpace(input.State)
	entry, ok := lookupDeviceFlow(state)
	if !ok {
		return nil, fmt.Errorf("github copilot device oauth: no device_code for state (expired or not started)")
	}

	callCtx, cancel := context.WithTimeout(ctx, copilotHTTPTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("client_id", copilotClientID)
	form.Set("device_code", entry.deviceCode)
	payload, err := postOAuthForm(callCtx, copilotTokenURL, form, input.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("github copilot device oauth: token request failed: %w", err)
	}

	var githubToken copilotGitHubTokenResponse
	if err := json.Unmarshal(payload, &githubToken); err != nil {
		return nil, fmt.Errorf("github copilot device oauth: invalid token payload: %w", err)
	}
	if githubToken.Error != "" {
		switch githubToken.Error {
		case "authorization_pending":
			return nil, &DeviceFlowPendingError{Reason: "authorization_pending", RetryAfter: entry.interval}
		case "slow_down":
			retryAfter := slowDownDeviceFlow(state)
			return nil, &DeviceFlowPendingError{Reason: "slow_down", RetryAfter: retryAfter}
		default:
			clearDeviceFlow(state)
			msg := githubToken.Error
			if githubToken.ErrorDescription != "" {
				msg += ": " + githubToken.ErrorDescription
			}
			return nil, fmt.Errorf("github copilot device oauth: %s", msg)
		}
	}
	if strings.TrimSpace(githubToken.AccessToken) == "" {
		clearDeviceFlow(state)
		return nil, fmt.Errorf("github copilot device oauth: token response missing access_token")
	}

	copilotToken, err := exchangeCopilotToken(ctx, githubToken.AccessToken, input.ProxyURL)
	if err != nil {
		clearDeviceFlow(state)
		return nil, err
	}

	login, githubID := fetchCopilotIdentity(ctx, githubToken.AccessToken, input.ProxyURL)
	accountID := strconv.FormatInt(githubID, 10)
	if accountID == "0" {
		accountID = login
	}
	clearDeviceFlow(state)

	providerData := map[string]interface{}{}
	if githubToken.TokenType != "" {
		providerData["githubTokenType"] = githubToken.TokenType
	}
	if githubToken.Scope != "" {
		providerData["githubScope"] = githubToken.Scope
	}

	return &TokenSet{
		AccessToken:    copilotToken.Token,
		RefreshToken:   githubToken.AccessToken,
		TokenExpiresAt: copilotToken.ExpiresAt * 1000,
		Email:          login,
		AccountKey:     login,
		AccountID:      accountID,
		ProviderData:   providerData,
	}, nil
}

func refreshCopilotAccessToken(ctx context.Context, input RefreshTokenInput) (*TokenSet, error) {
	githubToken := strings.TrimSpace(input.RefreshToken)
	if githubToken == "" {
		return nil, fmt.Errorf("github copilot oauth: missing github access token")
	}

	copilotToken, err := exchangeCopilotToken(ctx, githubToken, input.ProxyURL)
	if err != nil {
		return nil, err
	}
	providerData := map[string]interface{}{}
	if input.OAuth != nil && input.OAuth.ProviderData != nil {
		for key, value := range input.OAuth.ProviderData {
			providerData[key] = value
		}
	}
	return &TokenSet{
		AccessToken:    copilotToken.Token,
		RefreshToken:   githubToken,
		TokenExpiresAt: copilotToken.ExpiresAt * 1000,
		ProviderData:   providerData,
	}, nil
}

func exchangeCopilotToken(ctx context.Context, githubAccessToken string, proxyURL *string) (*copilotTokenResponse, error) {
	callCtx, cancel := context.WithTimeout(ctx, copilotHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, copilotTokenExchangeURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "token "+githubAccessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doHTTP(req, proxyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("github copilot token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readOAuthErrorResponseBody(resp.Body)
		message := fmt.Sprintf("github copilot token exchange returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, &NonRetryableError{Message: message}
		}
		return nil, fmt.Errorf("%s", message)
	}

	body, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}
	var token copilotTokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("github copilot token exchange returned invalid payload: %w", err)
	}
	if token.Token == "" || token.ExpiresAt <= 0 {
		return nil, fmt.Errorf("github copilot token exchange response missing token or expires_at")
	}
	return &token, nil
}

func fetchCopilotIdentity(ctx context.Context, githubAccessToken string, proxyURL *string) (login string, id int64) {
	callCtx, cancel := context.WithTimeout(ctx, copilotHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, copilotUserURL, nil)
	if err != nil {
		return "", 0
	}
	req.Header.Set("Authorization", "token "+githubAccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := doHTTP(req, proxyURL, nil)
	if err != nil || resp == nil {
		return "", 0
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0
	}
	body, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return "", 0
	}
	var user copilotUserResponse
	if err := json.Unmarshal(body, &user); err != nil {
		return "", 0
	}
	return strings.TrimSpace(user.Login), user.ID
}

func buildCopilotProxyHeaders(ctx context.Context, input ProxyHeaderInput) map[string]string {
	initiator := "user"
	if value, ok := input.DownstreamHeaders["X-Initiator"].(string); ok {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "user" || normalized == "agent" {
			initiator = normalized
		}
	}
	return map[string]string{
		"Editor-Version":                      "vscode/1.95.0",
		"Editor-Plugin-Version":               "copilot-chat/0.26.7",
		"User-Agent":                          "GitHubCopilotChat/0.26.7",
		"Copilot-Integration-Id":              "vscode-chat",
		"Openai-Intent":                       "conversation-edits",
		"X-GitHub-Api-Version":                "2025-04-01",
		"X-Vscode-User-Agent-Library-Version": "electron-fetch",
		"X-Initiator":                         initiator,
	}
}
