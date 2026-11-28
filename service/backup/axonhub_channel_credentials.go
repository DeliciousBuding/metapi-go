package backup

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/gcpsa"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func compileAxonHubChannelCredentials(channel AxonHubSourceChannel) ([]string, *store.DirectOAuthState, string, []string) {
	var reasons []string
	if channel.Credentials.OAuth && !axonHubOAuthProvider(channel.Type) {
		reasons = append(reasons, "oauth_credentials_unsupported")
	}
	if channel.Credentials.OAuth && axonHubOAuthProvider(channel.Type) {
		for _, endpoint := range channel.Endpoints {
			if channel.Type == "codex" && endpoint.APIFormat == "openai/alpha_search" {
				continue
			}
			if (channel.Type == "codex" || channel.Type == "fenno") && (endpoint.APIFormat == "openai/responses" || endpoint.APIFormat == "openai/responses_websocket" || endpoint.APIFormat == "openai/image_generation" || endpoint.APIFormat == "openai/image_edit") {
				continue
			}
			if channel.Type == "github_copilot" {
				if strings.TrimSpace(endpoint.BaseURL) != "" || strings.TrimSpace(endpoint.Path) != "" {
					reasons = append(reasons, "oauth_custom_endpoint_unsupported")
				}
				continue
			}
			if channel.Type == "xai_subscription" {
				if strings.TrimSpace(endpoint.BaseURL) != "" || strings.TrimSpace(endpoint.Path) != "" {
					reasons = append(reasons, "oauth_custom_endpoint_unsupported")
				}
				continue
			}
			// Source custom endpoints use a static API-key provider except the
			// Codex Responses/image wrapper. Do not silently replace that contract.
			reasons = append(reasons, "oauth_custom_endpoint_unsupported")
		}
	}
	if channel.Type == "github_copilot" && !channel.Credentials.OAuth {
		reasons = append(reasons, "copilot_oauth_credential_required")
	}
	if channel.Type == "xai_subscription" {
		if !channel.Credentials.OAuth {
			reasons = append(reasons, "xai_subscription_oauth_credential_required")
		} else if credentials := channel.Credentials.OAuthCredentials; credentials == nil ||
			strings.TrimSpace(credentials.RefreshToken) == "" {
			reasons = append(reasons, "xai_subscription_refresh_token_required")
		} else if clientID := strings.TrimSpace(credentials.ClientID); clientID != "" && clientID != "b1a00492-073a-47ea-816f-4c329264a828" {
			reasons = append(reasons, "xai_subscription_client_id_unsupported")
		}
		if base := strings.TrimSpace(channel.BaseURL); base != "" && base != "https://cli-chat-proxy.grok.com/v1" {
			reasons = append(reasons, "xai_subscription_custom_base_url_unsupported")
		}
	}
	if channel.Credentials.Azure {
		reasons = append(reasons, "azure_credentials_unsupported")
	}
	if channel.Credentials.GCP != nil && channel.Type != "gemini_vertex" && channel.Type != "anthropic_gcp" {
		reasons = append(reasons, "gcp_credentials_unsupported")
	}
	for _, key := range channel.Credentials.UnsupportedFields {
		reasons = append(reasons, "credential_field_unsupported:"+key)
	}
	keys := credentialKeys(channel.Credentials)
	oauthState := axonHubOAuthState(channel.Credentials.OAuthCredentials)
	if channel.Type == "antigravity" {
		if len(channel.Credentials.APIKeys) > 0 || channel.Credentials.OAuth || channel.Credentials.GCP != nil || channel.Credentials.Azure {
			reasons = append(reasons, "antigravity_credential_form_unsupported")
		} else {
			state, err := oauth.ParseAntigravityImportMaterial(channel.Credentials.APIKey)
			if err != nil {
				reasons = append(reasons, "antigravity_refresh_token_project_required")
			} else {
				keys, oauthState = []string{""}, &state
			}
		}
		if len(channel.Endpoints) != 0 {
			reasons = append(reasons, "antigravity_custom_endpoint_unsupported")
		}
	}
	if channel.Credentials.OAuth && axonHubOAuthProvider(channel.Type) && channel.Credentials.OAuthCredentials != nil {
		keys = []string{channel.Credentials.OAuthCredentials.AccessToken}
		if tokenType := strings.ToLower(channel.Credentials.OAuthCredentials.TokenType); tokenType != "" && tokenType != "bearer" {
			reasons = append(reasons, "oauth_token_type_unsupported")
		}
		if channel.Type == "github_copilot" {
			// AxonHub exports the durable GitHub OAuth token. The short-lived
			// Copilot token is exchanged lazily; secret starts empty.
			keys = []string{""}
			oauthState = &store.DirectOAuthState{
				RefreshToken: channel.Credentials.OAuthCredentials.AccessToken,
				ClientID:     channel.Credentials.OAuthCredentials.ClientID,
			}
		}
	}
	if len(keys) == 0 && channel.Credentials.GCP != nil && (channel.Type == "gemini_vertex" || channel.Type == "anthropic_gcp") {
		keys = []string{""}
	}
	if len(keys) == 0 {
		if axonHubOptionalAuth(channel) {
			// An empty secret is the explicit no-auth credential. Never replace
			// configured but disabled keys with an anonymous grant.
			keys = []string{""}
		} else {
			reasons = append(reasons, "credential_missing")
		}
	}
	// gemini_vertex serves bearer tokens exchanged from the SA material only;
	// an API-key-shaped channel would fail at the upstream on every request.
	if (channel.Type == "gemini_vertex" || channel.Type == "anthropic_gcp") && channel.Credentials.GCP == nil {
		reasons = append(reasons, "gcp_credential_missing")
	}
	gcpMaterial := ""
	if gcp := channel.Credentials.GCP; gcp != nil && (channel.Type == "gemini_vertex" || channel.Type == "anthropic_gcp") {
		if _, _, err := gcpsa.Parse(gcp.JSONData); err != nil {
			reasons = append(reasons, "gcp_credential_invalid")
		} else if strings.TrimSpace(gcp.ProjectID) == "" {
			reasons = append(reasons, "gcp_project_missing")
		} else {
			gcpMaterial = gcp.JSONData
		}
	}
	return keys, oauthState, gcpMaterial, reasons
}

func credentialKeys(credentials AxonHubSourceCredentials) []string {
	var keys []string
	seen := map[string]bool{}
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if credentials.OAuth || credentials.Azure || credentials.GCP != nil {
		return nil
	}
	add(credentials.APIKey)
	for _, key := range credentials.APIKeys {
		add(key)
	}
	return keys
}
