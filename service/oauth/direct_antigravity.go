package oauth

import (
	"strings"
	"unicode"

	"github.com/deliciousbuding/metapi-go/store"
)

// ParseAntigravityImportMaterial reads the source's refresh-token|project
// credential. It performs no exchange and does not manufacture project identity.
func ParseAntigravityImportMaterial(material string) (store.DirectOAuthState, error) {
	parts := strings.Split(material, "|")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return store.DirectOAuthState{}, ErrInvalidDirectCredential
	}
	for _, part := range parts {
		if strings.IndexFunc(part, unicode.IsControl) >= 0 || strings.TrimSpace(part) != part {
			return store.DirectOAuthState{}, ErrInvalidDirectCredential
		}
	}
	if len(parts[1]) > 255 || strings.ContainsAny(parts[1], "/\\?#") {
		return store.DirectOAuthState{}, ErrInvalidDirectCredential
	}
	return store.DirectOAuthState{RefreshToken: parts[0], ProjectID: parts[1], ClientID: antigravityClientID}, nil
}
