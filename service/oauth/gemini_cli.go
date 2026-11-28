package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

const (
	geminiCliAuthURL                   = "https://accounts.google.com/o/oauth2/v2/auth"
	geminiCliTokenURL                  = "https://oauth2.googleapis.com/token"
	geminiCliUserinfoURL               = "https://www.googleapis.com/oauth2/v1/userinfo?alt=json"
	geminiCliProjectsURL               = "https://cloudresourcemanager.googleapis.com/v1/projects"
	geminiCliServiceUsageURL           = "https://serviceusage.googleapis.com/v1"
	geminiCliLoopbackPort              = 8085
	geminiCliLoopbackPath              = "/oauth2callback"
	geminiCliLoopbackRedirectURI       = "http://localhost:8085/oauth2callback"
	geminiCliGoogleAPIClient           = "google-genai-sdk/1.41.0 gl-node/v22.19.0"
	geminiCliUserAgent                 = "GeminiCLI/0.31.0/unknown (win32; x64)"
	geminiCliRequiredService           = "cloudaicompanion.googleapis.com"
	geminiCliInternalAPIVersion        = "v1internal"
	geminiCliAutoOnboardPollIntervalMs = 2000
	geminiCliAutoOnboardMaxAttempts    = 15
	geminiCliOnboardPollIntervalMs     = 5000
	geminiCliOnboardMaxAttempts        = 6
)

// geminiCliUpstreamBaseURL is a package var (not a const) so tests can point the
// cloudcode internal API at a local httptest server, mirroring the
// antigravityUpstreamBaseURL / grok endpoint seams.
var geminiCliUpstreamBaseURL = "https://cloudcode-pa.googleapis.com"

var geminiCliScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
}

func init() {
	RegisterProvider(&OAuthProviderDefinition{
		Metadata: ProviderMetadata{
			Provider:                     ProviderGeminiCli,
			Label:                        "Gemini CLI",
			Platform:                     "gemini-cli",
			Enabled:                      true,
			LoginType:                    "oauth",
			RequiresProjectId:            true,
			SupportsDirectAccountRouting: true,
			SupportsCloudValidation:      true,
			SupportsNativeProxy:          true,
		},
		Site: ProviderSiteConfig{
			Name:     "Google Gemini CLI OAuth",
			URL:      geminiCliUpstreamBaseURL,
			Platform: "gemini-cli",
		},
		Loopback: LoopbackConfig{
			Host:        "127.0.0.1",
			Port:        geminiCliLoopbackPort,
			Path:        geminiCliLoopbackPath,
			RedirectURI: geminiCliLoopbackRedirectURI,
		},
		BuildAuthorizationURL:     buildGeminiCliAuthorizationURL,
		ExchangeAuthorizationCode: exchangeGeminiCliAuthorizationCode,
		RefreshAccessToken:        refreshGeminiCliAccessToken,
		BuildProxyHeaders:         buildGeminiCliProxyHeaders,
	})
}

func requireGeminiCliOAuthConfig() (clientID, clientSecret string, err error) {
	clientID = strings.TrimSpace(config.Get().GeminiCliClientId)
	if clientID == "" {
		return "", "", fmt.Errorf("GEMINI_CLI_CLIENT_ID is not configured")
	}
	clientSecret = strings.TrimSpace(config.Get().GeminiCliClientSecret)
	if clientSecret == "" {
		return "", "", fmt.Errorf("GEMINI_CLI_CLIENT_SECRET is not configured")
	}
	return
}

// ---- Auth URL (no PKCE) ----

func buildGeminiCliAuthorizationURL(ctx context.Context, input BuildAuthURLInput) (string, error) {
	clientID, _, err := requireGeminiCliOAuthConfig()
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", input.RedirectURI)
	params.Set("response_type", "code")
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	params.Set("scope", strings.Join(geminiCliScopes, " "))
	params.Set("state", input.State)
	return geminiCliAuthURL + "?" + params.Encode(), nil
}

// ---- Token Exchange ----

type geminiOAuthTokenPayload struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    interface{} `json:"expires_in"`
	Scope        string      `json:"scope"`
	Expiry       string      `json:"expiry"`
}

func postGeminiToken(form url.Values, proxyURL *string) (*TokenSet, error) {
	return postGeminiTokenContext(context.Background(), form, proxyURL)
}

func postGeminiTokenContext(ctx context.Context, form url.Values, proxyURL *string) (*TokenSet, error) {
	refresh := form.Get("grant_type") == "refresh_token"
	req, err := http.NewRequestWithContext(ctx, "POST", geminiCliTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, refreshPreparationFailure(refresh, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := doRefreshHTTP(req, proxyURL, refresh)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody := readOAuthErrorResponseBody(resp.Body)
		return nil, refreshHTTPFailure(ctx, refresh, resp.StatusCode, respBody, fmt.Errorf("%s", string(respBody)))
	}

	respBody, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, successfulRefreshResponseError(refresh, "response read failed", err)
	}

	var payload geminiOAuthTokenPayload
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, successfulRefreshResponseError(refresh, "invalid payload", fmt.Errorf("gemini token exchange returned invalid payload"))
	}

	accessToken := strings.TrimSpace(payload.AccessToken)
	if accessToken == "" {
		return nil, successfulRefreshResponseError(refresh, "missing access token", fmt.Errorf("gemini token exchange response missing access token"))
	}

	providerData := make(map[string]interface{})
	if tt := strings.TrimSpace(payload.TokenType); tt != "" {
		providerData["tokenType"] = tt
	}
	if sc := strings.TrimSpace(payload.Scope); sc != "" {
		providerData["scope"] = sc
	}

	return &TokenSet{
		AccessToken:    accessToken,
		RefreshToken:   strings.TrimSpace(payload.RefreshToken),
		TokenExpiresAt: parseTokenExpiresAt(payload.ExpiresIn, payload.Expiry),
		ProviderData:   providerData,
	}, nil
}

func exchangeGeminiCliAuthorizationCode(ctx context.Context, input ExchangeCodeInput) (*TokenSet, error) {
	clientID, clientSecret, err := requireGeminiCliOAuthConfig()
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("code", input.Code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("redirect_uri", input.RedirectURI)
	form.Set("grant_type", "authorization_code")

	token, err := postGeminiToken(form, input.ProxyURL)
	if err != nil {
		return nil, err
	}

	resolvedProjectID, err := ensureGeminiProjectAndOnboard(ctx, token.AccessToken, input.ProjectID, input.ProxyURL)
	if err != nil {
		return nil, err
	}

	if err := checkCloudAIAPIEnabled(token.AccessToken, resolvedProjectID, input.ProxyURL); err != nil {
		return nil, err
	}

	email, _ := fetchGeminiUserEmail(token.AccessToken, input.ProxyURL)

	return &TokenSet{
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		TokenExpiresAt: token.TokenExpiresAt,
		Email:          email,
		AccountKey:     email,
		AccountID:      email,
		ProjectID:      resolvedProjectID,
		ProviderData:   token.ProviderData,
	}, nil
}

// ---- Token Refresh ----

func refreshGeminiCliAccessToken(ctx context.Context, input RefreshTokenInput) (*TokenSet, error) {
	clientID, clientSecret, err := requireGeminiCliOAuthConfig()
	if err != nil {
		return nil, refreshNotConsumed(err)
	}
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", input.RefreshToken)

	token, err := postGeminiTokenContext(ctx, form, input.ProxyURL)
	if err != nil {
		return nil, err
	}

	var nextProjectID string
	if input.OAuth != nil && input.OAuth.ProjectID != "" {
		nextProjectID = input.OAuth.ProjectID
	} else {
		var err error
		nextProjectID, err = ensureGeminiProjectAndOnboard(ctx, token.AccessToken, "", input.ProxyURL)
		if err != nil {
			return nil, err
		}
	}

	if err := checkCloudAIAPIEnabled(token.AccessToken, nextProjectID, input.ProxyURL); err != nil {
		return nil, err
	}

	email, _ := fetchGeminiUserEmail(token.AccessToken, input.ProxyURL)

	return &TokenSet{
		AccessToken:    token.AccessToken,
		RefreshToken:   token.RefreshToken,
		TokenExpiresAt: token.TokenExpiresAt,
		Email:          email,
		AccountKey:     email,
		AccountID:      email,
		ProjectID:      nextProjectID,
		ProviderData:   token.ProviderData,
	}, nil
}

// ---- Proxy Headers ----

func buildGeminiCliProxyHeaders(ctx context.Context, input ProxyHeaderInput) map[string]string {
	return map[string]string{
		"User-Agent":        geminiCliUserAgent,
		"X-Goog-Api-Client": geminiCliGoogleAPIClient,
	}
}
