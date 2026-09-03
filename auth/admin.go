package auth

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

// ---------------------------------------------------------------------------
// AdminAuth — server-side session cookie + Bearer master token + IP CIDR
// allowlist middleware (#1034 session model). Applied to /api/* routes
// (except public endpoints).
//
// Order of checks (first failure returns immediately):
// 1. Extract client IP from RemoteAddr + normalize.
// 2. If allowlist is non-empty and IP is not allowed → 403
// 3. Session track: a metapi_session cookie is validated against
//    admin_sessions (sliding TTL). Valid → authenticated.
// 4. Bearer track: Authorization: Bearer <master token> still authenticates
//    (dual-track for external scripts/automation; the UI never stores the
//    master token client-side anymore).
// 5. No credentials → 401; stale cookie only → 401 Session expired;
//    wrong Bearer → 403 Invalid token.
//
// Public routes that bypass this middleware:
// - GET /api/oauth/callback/*
// - POST /api/auth/login, POST /api/auth/logout, GET /api/auth/session
//   (the session lifecycle surface; login is master-token + rate limited,
//   logout/session are idempotent/read-only and answer without a session)
// ---------------------------------------------------------------------------

// AdminAuth returns a chi-compatible middleware that enforces admin
// authentication using the given configuration. sessions may be nil (tests,
// DB-less boot): the cookie track then simply never authenticates and the
// Bearer track keeps working unchanged.
func AdminAuth(sessions *SessionManager) func(http.Handler) http.Handler {
	// Pre-parse the allowlist once at factory creation time so we don't
	// re-parse string entries on every request. Invalid entries are surfaced
	// here (startup time) so operators notice a typo'd allowlist instead of
	// believing the IP restriction is active when it silently dropped.
	// (Allowlist entries are read from the runtime-settings snapshot at
	// wiring time; changing adminIpAllowlist at runtime still requires a
	// restart to take effect, matching pre-C1 behavior.)
	parsedAllowlist, invalidEntries := parseAllowlistWithDiagnostics(config.Runtime().AdminIpAllowlist)
	for _, entry := range invalidEntries {
		slog.Warn("admin IP allowlist: skipping invalid entry",
			"entry", entry,
			"hint", "expected an exact IP or an IPv4 CIDR like 10.0.0.0/8")
	}
	if len(invalidEntries) > 0 {
		slog.Warn("admin IP allowlist: skipped invalid entries — the allowlist may not restrict traffic as intended",
			"skipped", len(invalidEntries),
			"valid", len(parsedAllowlist))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// ---- Public route bypass ----
			if isPublicAPIRoute(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// ---- 1. Extract client IP ----
			clientIP := extractClientIP(r)
			if !isIPAllowed(clientIP, parsedAllowlist) {
				writeJSON(w, http.StatusForbidden, jsonErrorWithCode("IP not allowed", ErrorCodeAuthIPBlocked))
				return
			}

			// ---- 2. Session track: HttpOnly cookie → admin_sessions row ----
			cookieToken := SessionCookieFromRequest(r)
			if cookieToken != "" {
				if sess := sessions.Validate(r.Context(), cookieToken); sess != nil {
					ctx := WithAdminAuth(r.Context())
					// Session hash prefix = stable, non-reversible audit actor.
					ctx = WithAdminSessionID(ctx, sess.TokenHash[:8])
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// ---- 3. Bearer track: master token (dual-track, #1034) ----
			auth := r.Header.Get("Authorization")
			if auth == "" {
				if cookieToken != "" {
					// A cookie was presented but is unknown/expired: the
					// session ended, say so (the frontend redirects to sign-in).
					writeJSON(w, http.StatusUnauthorized, jsonErrorWithCode("Session expired", ErrorCodeAuthSessionExpired))
					return
				}
				writeJSON(w, http.StatusUnauthorized, jsonErrorWithCode("Missing Authorization header", ErrorCodeAuthMissingCredential))
				return
			}

			// Case-sensitive simple replace of the first "Bearer " literal.
			// TS: token = auth.replace('Bearer ', '')
			token := strings.Replace(auth, "Bearer ", "", 1)
			if subtle.ConstantTimeCompare([]byte(token), []byte(config.Runtime().AuthToken)) != 1 {
				writeJSON(w, http.StatusForbidden, jsonErrorWithCode("Invalid token", ErrorCodeAuthInvalidToken))
				return
			}

			// ---- Store admin auth marker in context ----
			r = r.WithContext(WithAdminAuth(r.Context()))
			next.ServeHTTP(w, r)
		})
	}
}

// isPublicAPIRoute returns true for routes that do not require admin auth.
// Whitelist: /api/oauth/callback/* and the session
// lifecycle surface /api/auth/{login,logout,session} (#1034). ws-ticket is
// NOT public: minting a WS ticket requires a live session.
//
// A ".." path segment anywhere in the URL disqualifies the bypass. chi does
// not clean request paths, so today traversal strings never reach a
// registered handler — but the middleware predicate itself must not hand out
// a bypass for a path that merely starts with a public prefix, or a future
// path-normalizing refactor (before or instead of chi routing) would turn
// this prefix match into a full admin auth bypass.
func isPublicAPIRoute(urlPath string) bool {
	if containsDotDotSegment(urlPath) {
		return false
	}
	if strings.HasPrefix(urlPath, "/api/oauth/callback/") {
		return true
	}
	switch urlPath {
	case "/api/auth/login", "/api/auth/logout", "/api/auth/session":
		return true
	}
	return false
}

// containsDotDotSegment reports whether any "/"-separated segment is exactly
// "..". net/http decodes percent-escapes into r.URL.Path before routing, so
// "%2e%2e" has already become ".." by the time middleware sees the path.
func containsDotDotSegment(urlPath string) bool {
	for _, segment := range strings.Split(urlPath, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// IP extraction and normalization.
// ---------------------------------------------------------------------------

// extractClientIP extracts the client IP from RemoteAddr. Forwarded headers are
// intentionally ignored here; router.TrustedRealIP rewrites RemoteAddr only
// when the direct peer matches an explicitly configured trusted proxy CIDR.
func extractClientIP(r *http.Request) string {
	return normalizeIP(stripPort(r.RemoteAddr))
}

// stripPort removes the port suffix from an "IP:port" string.
// If there is no colon, returns the string as-is.
// For IPv6 addresses like "[::1]:1234", strips the brackets and port.
func stripPort(addr string) string {
	// Handle IPv6 bracket notation: [::1]:1234
	if strings.HasPrefix(addr, "[") {
		if idx := strings.LastIndex(addr, "]"); idx >= 0 {
			return addr[1:idx]
		}
		return addr
	}
	// IPv4: 1.2.3.4:1234
	if idx := strings.LastIndexByte(addr, ':'); idx >= 0 {
		return addr[:idx]
	}
	return addr
}

// normalizeIP normalizes an IP address string:
// - "::ffff:x.x.x.x" → "x.x.x.x" (IPv4-mapped IPv6 → pure IPv4)
// - "::1" → "127.0.0.1" (IPv6 loopback)
// - Other values are trimmed and returned as-is.
//
// Mirrors TS normalizeIp() exactly.
func normalizeIP(raw string) string {
	ip := strings.TrimSpace(raw)
	if ip == "" {
		return ""
	}
	if strings.HasPrefix(ip, "::ffff:") {
		return strings.TrimSpace(ip[len("::ffff:"):])
	}
	if ip == "::1" {
		return "127.0.0.1"
	}
	return ip
}

// ---------------------------------------------------------------------------
// IP allowlist parsing and matching.
// ---------------------------------------------------------------------------

// parsedAllowlistEntry represents a single parsed allowlist entry.
type parsedAllowlistEntry struct {
	kind       string // "exact" or "cidr"
	exactIP    string // normalized IP string for exact match
	cidrPrefix netip.Prefix
}

// parseAllowlist converts a list of raw allowlist strings into parsed entries.
// Invalid entries are silently skipped (matches TS parseAllowlistEntry → null).
// Use parseAllowlistWithDiagnostics when you need to surface skipped entries
// (e.g. startup warnings for a misconfigured ADMIN_IP_ALLOWLIST).
func parseAllowlist(entries []string) []parsedAllowlistEntry {
	parsed, _ := parseAllowlistWithDiagnostics(entries)
	return parsed
}

// parseAllowlistWithDiagnostics mirrors parseAllowlist but also returns the
// raw entries that were non-empty yet could not be parsed into an exact IP or
// IPv4 CIDR. Callers (e.g. AdminAuth) log these so operators notice a typo'd
// allowlist instead of believing the restriction is active. Whitespace-only
// entries are intentionally not reported — they are no-ops, not misconfig.
func parseAllowlistWithDiagnostics(entries []string) ([]parsedAllowlistEntry, []string) {
	result := make([]parsedAllowlistEntry, 0, len(entries))
	var invalid []string
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}

		slashIdx := strings.IndexByte(entry, '/')
		if slashIdx < 0 {
			// Exact IP match
			normalized := normalizeIP(entry)
			if normalized == "" {
				invalid = append(invalid, entry)
				continue
			}
			// Validate it's a real IP (or at least non-empty after normalization)
			if _, err := netip.ParseAddr(normalized); err != nil {
				invalid = append(invalid, entry)
				continue
			}
			result = append(result, parsedAllowlistEntry{
				kind:    "exact",
				exactIP: normalized,
			})
			continue
		}

		// CIDR match — only supports IPv4 CIDR (matches TS behavior)
		// Check for multiple slashes (invalid)
		if strings.IndexByte(entry[slashIdx+1:], '/') >= 0 {
			invalid = append(invalid, entry)
			continue
		}

		networkIP := normalizeIP(entry[:slashIdx])
		prefixText := strings.TrimSpace(entry[slashIdx+1:])

		// Must be a valid IPv4 address and a numeric prefix
		addr, err := netip.ParseAddr(networkIP)
		if err != nil || !addr.Is4() {
			invalid = append(invalid, entry)
			continue
		}

		// Build the CIDR string and parse
		prefix, err := netip.ParsePrefix(networkIP + "/" + prefixText)
		if err != nil {
			invalid = append(invalid, entry)
			continue
		}

		// TS only supports prefix 0-32 for IPv4
		if !prefix.Addr().Is4() {
			invalid = append(invalid, entry)
			continue
		}

		result = append(result, parsedAllowlistEntry{
			kind:       "cidr",
			cidrPrefix: prefix,
		})
	}
	return result, invalid
}

// isIPAllowed checks whether the given client IP is allowed by the allowlist.
// An empty allowlist means ALL IPs are allowed.
//
// Matching logic mirrors TS isIpAllowed():
// - exact entries: compare normalized IP strings
// - CIDR entries: only match IPv4 clients; pure IPv6 clients cannot pass CIDR checks
func isIPAllowed(clientIP string, allowlist []parsedAllowlistEntry) bool {
	if len(allowlist) == 0 {
		return true // empty allowlist = allow all
	}

	normalized := normalizeIP(clientIP)
	if normalized == "" {
		return false
	}

	clientAddr, clientParseErr := netip.ParseAddr(normalized)
	clientIsAddr := clientParseErr == nil

	for _, entry := range allowlist {
		switch entry.kind {
		case "exact":
			if entry.exactIP == normalized {
				return true
			}
		case "cidr":
			// CIDR entries only match IPv4 clients (matches TS: parseIpv4Value for CIDR)
			if !clientIsAddr || !clientAddr.Is4() {
				continue
			}
			if entry.cidrPrefix.Contains(clientAddr) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// JSON response helpers.
// ---------------------------------------------------------------------------

// Machine-readable errorCode values on auth-middleware rejections. Same
// convention as handler/admin/error_codes.go: stable camelCase, optional and
// additive — the human-readable "error" stays the display fallback, and the
// frontend keeps its load-bearing substring match ("invalid token") working.
// Registered in docs/api.md "errorCode convention and registry".
const (
	// ErrorCodeAuthSessionExpired: a cookie session was presented but is
	// unknown/expired (401; the frontend redirects to sign-in).
	ErrorCodeAuthSessionExpired = "authSessionExpired"
	// ErrorCodeAuthMissingCredential: no Authorization header and no session
	// cookie (401).
	ErrorCodeAuthMissingCredential = "authMissingCredential"
	// ErrorCodeAuthInvalidToken: Bearer master-token mismatch (403).
	ErrorCodeAuthInvalidToken = "authInvalidToken"
	// ErrorCodeAuthIPBlocked: client IP not on the admin allowlist (403).
	ErrorCodeAuthIPBlocked = "authIpBlocked"
)

type jsonErrorBody struct {
	Error string `json:"error"`
	// ErrorCode is additive; empty means "no registered code" and the field
	// is omitted from the wire body.
	ErrorCode string `json:"errorCode,omitempty"`
}

func jsonError(msg string) jsonErrorBody {
	return jsonErrorBody{Error: msg}
}

func jsonErrorWithCode(msg, code string) jsonErrorBody {
	return jsonErrorBody{Error: msg, ErrorCode: code}
}

func writeJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(body)
}
