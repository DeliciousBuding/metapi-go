package oauth

import "github.com/deliciousbuding/metapi-go/store"

// ValidateDirectCredentialMaterial exposes the credential owner's validation to
// workflows that must insert a complete catalog graph in their own transaction.
// Callers must not log or return the secret or OAuth state.
func ValidateDirectCredentialMaterial(provider string, endpoints store.DirectEndpoints, kind string, key *string, oauth *DirectOAuthReplacement) (string, string, store.DirectOAuthState, error) {
	// The connect flow is API-key/preset shaped; gcp_service_account material is
	// only accepted through the per-channel credential management endpoints.
	secret, resolvedKind, state, _, err := directCredentialMaterial(provider, endpoints, kind, key, oauth, nil)
	return secret, resolvedKind, state, err
}
