package proxy

import "github.com/deliciousbuding/metapi-go/service"

// IsSessionScopedChannel checks if a channel uses session-scoped credentials.
func IsSessionScopedChannel(extraConfig *string, oauthProvider *string) bool {
	if service.GetCredentialModeFromExtraConfig(extraConfig) == service.CredentialModeSession {
		return true
	}
	return oauthProvider != nil && *oauthProvider != ""
}
