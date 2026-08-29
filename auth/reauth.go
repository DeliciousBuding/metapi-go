// metapi-go/auth — sensitive-operation re-confirmation (#1034).
//
// A live admin session (or a Bearer master token) authorizes ordinary admin
// API calls. Destructive/irreversible surfaces additionally require the
// operator to present the master token again in X-Admin-Confirm-Token, so a
// walk-away session or a hijacked tab cannot silently exfiltrate backups,
// downstream keys, or rotate the master token itself. The header value is
// compared hash-then-constant-time and never logged.
package auth

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

// ReauthConfirmHeader carries the master token for sensitive operations.
const ReauthConfirmHeader = "X-Admin-Confirm-Token"

// sensitiveAdminPath reports whether the request path needs master-token
// re-confirmation. Keep this list explicit and tiny — it mirrors the
// sensitive operations enumerated by #1034:
//   - backup export (download + WebDAV push): full database contents
//   - downstream key export: plaintext API key profiles
//   - master token rotation: credential takeover if unattended
//
// The downstream-key export path carries an {id} segment, hence the
// prefix+suffix match; chi does not clean request paths, so a ".." segment
// disqualifies the match the same way isPublicAPIRoute does.
func sensitiveAdminPath(urlPath string) bool {
	if containsDotDotSegment(urlPath) {
		return false
	}
	switch urlPath {
	case "/api/settings/backup/export",
		"/api/settings/backup/webdav/export",
		"/api/settings/auth/change":
		return true
	}
	if strings.HasPrefix(urlPath, "/api/downstream-keys/") && strings.HasSuffix(urlPath, "/export") {
		// Exactly one numeric {id} segment between the prefix and /export
		// (chi's registered shape is /api/downstream-keys/{id}/export with an
		// integer id; anything else is not the export route).
		middle := strings.TrimSuffix(strings.TrimPrefix(urlPath, "/api/downstream-keys/"), "/export")
		if isNumericSegment(middle) {
			return true
		}
	}
	return false
}

// RequireReauth gates sensitive admin endpoints behind master-token
// re-confirmation. Non-sensitive paths pass through untouched. Rejection is
// a machine-readable 403 with reauthRequired=true so the UI can prompt for
// the token and replay the request once.
func RequireReauth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !sensitiveAdminPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			confirm := strings.TrimSpace(r.Header.Get(ReauthConfirmHeader))
			rt := config.RuntimeSafe()
			if rt == nil || confirm == "" || !constantTimeTokenEqual(confirm, rt.AuthToken) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"Sensitive operation requires master token confirmation","reauthRequired":true}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isNumericSegment reports whether s is a non-empty all-digit path segment.
func isNumericSegment(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
