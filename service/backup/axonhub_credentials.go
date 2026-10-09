package backup

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

type AxonHubSourceOAuth struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ClientID     string    `json:"client_id"`
	IDToken      string    `json:"id_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	Scopes       []string  `json:"scopes"`
}

func decodeAxonHubOAuthCredentials(raw json.RawMessage, dst *AxonHubSourceCredentials) error {
	if len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "null" {
		var parsed AxonHubSourceOAuth
		if err := json.Unmarshal(raw, &parsed); err != nil {
			return axonHubErr("credentials.oauth is invalid")
		}
		if strings.TrimSpace(parsed.AccessToken) != "" {
			dst.OAuth = true
			dst.OAuthCredentials = &parsed
			return nil
		}
	}
	// The legacy APIKey field is itself OAuth JSON on exported older channels.
	if strings.HasPrefix(strings.TrimSpace(dst.APIKey), "{") && strings.Contains(dst.APIKey, "access_token") {
		var parsed AxonHubSourceOAuth
		if err := json.Unmarshal([]byte(dst.APIKey), &parsed); err != nil || strings.TrimSpace(parsed.AccessToken) == "" {
			return axonHubErr("legacy OAuth credentials are invalid")
		}
		dst.OAuth = true
		dst.OAuthCredentials = &parsed
		dst.APIKey = ""
	}
	return nil
}

func axonHubOAuthProvider(provider string) bool {
	return provider == "codex" || provider == "fenno" || provider == "claudecode"
}

func axonHubOAuthState(credentials *AxonHubSourceOAuth) *store.DirectOAuthState {
	if credentials == nil {
		return nil
	}
	state := &store.DirectOAuthState{RefreshToken: credentials.RefreshToken, ClientID: credentials.ClientID, IDToken: credentials.IDToken}
	if !credentials.ExpiresAt.IsZero() {
		state.ExpiresAt = credentials.ExpiresAt.UnixMilli()
	}
	return state
}
