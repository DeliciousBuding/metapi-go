package oauth

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

const (
	codexAuthURL             = "https://auth.openai.com/oauth/authorize"
	codexLoopbackPort        = 1455
	codexLoopbackPath        = "/auth/callback"
	codexLoopbackRedirectURI = "http://localhost:1455/auth/callback"
	codexUpstreamBaseURL     = "https://chatgpt.com/backend-api/codex"
)

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
		ParseAccessToken:          ParseCodexAccessToken,
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

func exchangeCodexAuthorizationCode(ctx context.Context, input ExchangeCodeInput) (*TokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", func() string { id, _ := requireCodexClientID(); return id }())
	form.Set("code", input.Code)
	form.Set("redirect_uri", input.RedirectURI)
	form.Set("code_verifier", input.CodeVerifier)

	payload, err := exchangeCodexToken(ctx, form, input.ProxyURL, false)
	if err != nil {
		return nil, err
	}
	if _, idErr := requireCodexClientID(); idErr != nil {
		return nil, idErr
	}
	return buildCodexTokenSetFromPayload(payload, "token exchange")
}

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
