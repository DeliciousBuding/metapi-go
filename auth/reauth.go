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
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/go-chi/chi/v5"
)

// ReauthConfirmHeader carries the master token for sensitive operations.
const ReauthConfirmHeader = "X-Admin-Confirm-Token"

// sensitiveAdminOperation reports whether the routed admin operation needs
// master-token re-confirmation.
//
// It keys on the chi route pattern, not on r.URL.Path: the pattern is the
// registered shape (`/api/downstream-keys/{id}/export`), so path-encoding
// tricks (`+9`, `%209`, NBSP) cannot make the middleware and the handler
// disagree about which route was hit. Callers replaying a request must use
// the same method and route.
func sensitiveAdminOperation(method, routePattern string) bool {
	switch routePattern {
	// Full-database or plaintext-credential exports.
	case "/api/settings/backup/export",
		"/api/settings/backup/webdav/export",
		"/api/downstream-keys/{id}/export",
		"/api/account-tokens/{id}/value":
		return true

	// Credential-bearing writes: import can plant routes/settings/ledgers;
	// WebDAV configuration makes the scheduler push the whole database;
	// database migration copies every table to an arbitrary DSN.
	case "/api/settings/backup/import",
		"/api/settings/backup/webdav/import":
		return method == http.MethodPost
	case "/api/settings/backup/webdav":
		return method == http.MethodPut
	case "/api/settings/database/migrate",
		"/api/settings/database/test-connection",
		"/api/settings/system-proxy/test":
		return method == http.MethodPost

	// Destructive maintenance and master-token rotation.
	case "/api/settings/auth/change",
		"/api/settings/maintenance/factory-reset",
		"/api/settings/maintenance/clear-usage":
		return true

	// Downstream keys are long-lived bearer credentials: minting or rotating
	// one from a session creates a credential that survives session revocation.
	// PUT is NOT gated here: the handler only demands re-confirmation when the
	// request actually rotates the secret or sets a per-key egress proxyUrl;
	// ordinary metadata edits stay usable from a live session.
	case "/api/downstream-keys":
		return method == http.MethodPost
	case "/api/downstream-keys/batch":
		return method == http.MethodPost
	}
	return false
}

// sensitiveAdminPath is the fallback used when no chi route pattern is
// available. It canonicalizes the path before matching: percent-encoding is
// decoded up to two rounds and empty/`.` segments are collapsed, and any
// `..` segment fails closed. Parameter positions are matched as wildcards, so
// encoding tricks (`+9`, `%209`, NBSP, `%2f`) can only ever make a path MORE
// likely to require confirmation, never less.
func sensitiveAdminPath(method, urlPath string) bool {
	segments := canonicalPathSegments(urlPath)
	if segments == nil {
		return true // traversal or undecodable near an admin API: fail closed
	}
	match := func(want ...string) bool {
		if len(segments) != len(want) {
			return false
		}
		for i := range want {
			if want[i] == "*" || strings.EqualFold(segments[i], want[i]) {
				continue
			}
			return false
		}
		return true
	}
	switch method {
	case http.MethodGet:
		return match("api", "settings", "backup", "export") ||
			match("api", "settings", "backup", "webdav", "export") ||
			match("api", "downstream-keys", "*", "export") ||
			match("api", "account-tokens", "*", "value")
	case http.MethodPost:
		return match("api", "settings", "backup", "import") ||
			match("api", "settings", "backup", "webdav", "import") ||
			match("api", "settings", "database", "migrate") ||
			match("api", "settings", "database", "test-connection") ||
			match("api", "settings", "system-proxy", "test") ||
			match("api", "settings", "maintenance", "factory-reset") ||
			match("api", "settings", "maintenance", "clear-usage") ||
			match("api", "settings", "auth", "change") ||
			match("api", "downstream-keys") ||
			match("api", "downstream-keys", "batch")
	case http.MethodPut:
		return match("api", "settings", "backup", "webdav")
	}
	return false
}

// canonicalPathSegments decodes and normalizes a request path. It returns nil
// when a traversal segment is present or the path cannot be decoded: callers
// treat that as sensitive.
func canonicalPathSegments(raw string) []string {
	path := raw
	for i := 0; i < 2; i++ {
		decoded, err := url.PathUnescape(path)
		if err != nil {
			return nil
		}
		if decoded == path {
			break
		}
		path = decoded
	}
	var segments []string
	for _, segment := range strings.Split(path, "/") {
		segment = strings.TrimSpace(segment)
		switch segment {
		case "", ".":
			continue
		case "..":
			return nil
		}
		segments = append(segments, segment)
	}
	return segments
}

func reauthRequiredPath(r *http.Request) bool {
	if pattern := routePattern(r); pattern != "" {
		if sensitiveAdminOperation(r.Method, pattern) {
			return true
		}
	}
	return sensitiveAdminPath(r.Method, r.URL.Path) || sensitiveAdminPath(r.Method, r.URL.EscapedPath())
}

func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		return rctx.RoutePattern()
	}
	return ""
}

// ReauthConfirmed reports whether the request carries the current master
// token in the confirmation header. It is exported for handlers that gate a
// single field of an otherwise ordinary request (runtime settings).
func ReauthConfirmed(r *http.Request) bool {
	confirm := strings.TrimSpace(r.Header.Get(ReauthConfirmHeader))
	rt := config.RuntimeSafe()
	return rt != nil && confirm != "" && constantTimeTokenEqual(confirm, rt.AuthToken)
}

// WriteReauthRequired emits the machine-readable rejection shared by the
// middleware and field-level gates.
func WriteReauthRequired(w http.ResponseWriter) {
	noStore(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	// errorCode is additive: the frontend branches on reauthRequired; the
	// code only feeds the i18n toast map.
	_, _ = w.Write([]byte(`{"error":"Sensitive operation requires master token confirmation","reauthRequired":true,"errorCode":"authReauthRequired"}`))
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

// RequireReauth gates sensitive admin endpoints behind master-token
// re-confirmation. Non-sensitive paths pass through untouched. Rejection is
// a machine-readable 403 with reauthRequired=true so the UI can prompt for
// the token and replay the request once. Sensitive responses (success or
// rejection) are never cacheable.
func RequireReauth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !reauthRequiredPath(r) {
				next.ServeHTTP(w, r)
				return
			}
			noStore(w)
			if !ReauthConfirmed(r) {
				WriteReauthRequired(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
