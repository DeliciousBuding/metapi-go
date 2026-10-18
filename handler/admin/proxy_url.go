package admin

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/service"
)

const invalidProxyURLMessage = "Invalid proxyUrl. Expected an http, https, socks5, or socks5h URL with a host."

// normalizeProxyURLInput trims an optional proxyUrl from a request body and
// validates it. Empty (or absent) means "no proxy / inherit", which is always
// accepted. ok=false means the caller must answer 400.
func normalizeProxyURLInput(value *string) (normalized string, ok bool) {
	if value == nil {
		return "", true
	}
	normalized = strings.TrimSpace(*value)
	if normalized == "" {
		return "", true
	}
	if err := service.ValidateProxyURL(normalized); err != nil {
		return "", false
	}
	return normalized, true
}

// requireProxyURLReauth rejects a request that would persist a non-empty
// egress proxy without the master token re-confirmation header. The value must
// already be validated; callers skip the gate when the field is absent or
// explicitly cleared (clearing cannot redirect traffic).
func requireProxyURLReauth(w http.ResponseWriter, r *http.Request, normalized string) bool {
	if normalized == "" || auth.ReauthConfirmed(r) {
		return false
	}
	auth.WriteReauthRequired(w)
	return true
}

// gateOAuthProxyURL validates and reauth-gates a request-supplied OAuth
// proxyUrl. OAuth start/rebind/proxy-patch all persist the value into the
// account's extraConfig, which redirects the connection's upstream traffic.
// Returns the pointer to persist (nil when the field was absent) and ok=false
// when a response has already been written.
func gateOAuthProxyURL(w http.ResponseWriter, r *http.Request, input *string) (*string, bool) {
	if input == nil {
		return nil, true
	}
	normalized, valid := normalizeProxyURLInput(input)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": invalidProxyURLMessage})
		return nil, false
	}
	if requireProxyURLReauth(w, r, normalized) {
		return nil, false
	}
	return &normalized, true
}

// extraConfigProxyURL reports a non-empty proxyUrl nested in an account
// extraConfig patch. The account update handler merges that map verbatim, so a
// client can reach the same field without using the top-level proxyUrl.
func extraConfigProxyURL(extraConfig any) (patch map[string]any, normalized string, found bool) {
	patch, ok := extraConfig.(map[string]any)
	if !ok {
		return nil, "", false
	}
	raw, ok := patch["proxyUrl"]
	if !ok {
		return patch, "", false
	}
	value, ok := raw.(string)
	if !ok {
		return patch, "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return patch, "", false
	}
	return patch, value, true
}
