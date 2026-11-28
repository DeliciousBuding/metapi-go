package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/trustedproxy"
)

const monitorAuthCookie = "meta_monitor_auth"

// monitorCookiePath is scoped to the iframe proxy surface only.
// Path=/ is intentionally avoided so a stolen cookie cannot be presented
// to other same-origin endpoints. Iframe + "open proxy" targets are both
// under /monitor-proxy/, so this does not break the embed flow.
const monitorCookiePath = "/monitor-proxy/"

// monitorSessionMaxAge matches the previous 2h cookie lifetime.
const monitorSessionMaxAge = 7200

// monitorSessionPurpose is the fixed HMAC message. Changing it rotates all
// outstanding monitor cookies without touching AuthToken itself.
const monitorSessionPurpose = "metapi-monitor-session-v1"

// POST /api/monitor/session
// Mints an opaque monitor-proxy cookie derived from AuthToken via HMAC.
// The raw AUTH_TOKEN is never written into the cookie value: cookie theft
// must not yield a usable admin Bearer credential for /api/*.
func (h *monitorHandler) createSession(w http.ResponseWriter, r *http.Request) {
	session := deriveMonitorSessionToken(config.Runtime().AuthToken)
	http.SetCookie(w, newMonitorAuthCookie(session, monitorSessionMaxAge, isMonitorHTTPS(r)))
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// DELETE /api/monitor/session
// Clears the HttpOnly meta_monitor_auth cookie with Path matching createSession.
// Admin logout must call this while the Bearer is still valid: after frontend
// clears localStorage, the opaque cookie would otherwise remain usable for
// /monitor-proxy/* until Max-Age (AuthToken is unchanged on logout).
func (h *monitorHandler) clearSession(w http.ResponseWriter, r *http.Request) {
	clearMonitorAuthCookies(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// newMonitorAuthCookie builds the shared cookie attributes for mint + clear so
// Path/HttpOnly/SameSite/Secure stay in lockstep (browsers only clear when the
// Path matches the cookie that was originally set).
func newMonitorAuthCookie(value string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     monitorAuthCookie,
		Value:    value,
		Path:     monitorCookiePath,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
		Secure:   secure,
	}
}

// clearMonitorAuthCookies expires meta_monitor_auth for the scoped path used
// by createSession. Also expires a legacy Path=/ cookie in case an older
// release minted one before the /monitor-proxy/ scope landed.
//
// net/http serializes MaxAge < 0 as Max-Age=0 (immediate expiry).
func clearMonitorAuthCookies(w http.ResponseWriter, r *http.Request) {
	secure := isMonitorHTTPS(r)
	http.SetCookie(w, newMonitorAuthCookie("", -1, secure))
	// Legacy Path=/ known limitation from createSession.
	http.SetCookie(w, &http.Cookie{
		Name:     monitorAuthCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Secure:   secure,
	})
}

func (h *monitorHandler) ensureMonitorAuth(r *http.Request) bool {
	cookies := parseCookieHeader(r.Header.Get("Cookie"))
	got := cookies[monitorAuthCookie]
	if got == "" {
		return false
	}
	// AuthToken is runtime-mutable (rotation hot-applies via UpdateRuntime):
	// take one snapshot so the raw-token rejection and the derived-session
	// check agree on the same value.
	authToken := config.Runtime().AuthToken
	// Never accept the raw AuthToken as a monitor session.
	if subtle.ConstantTimeCompare([]byte(got), []byte(authToken)) == 1 {
		return false
	}
	expected := deriveMonitorSessionToken(authToken)
	if len(got) != len(expected) {
		// Length mismatch: still run a dummy compare to keep timing flat-ish,
		// then deny.
		_ = subtle.ConstantTimeCompare([]byte(expected), []byte(expected))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// deriveMonitorSessionToken returns an opaque, AuthToken-bound session value.
// HMAC-SHA256(key=AuthToken, msg=purpose) means:
// - cookie value is never the live AUTH_TOKEN
// - AuthToken rotation automatically invalidates outstanding cookies
// - no server-side session store is required
func deriveMonitorSessionToken(authToken string) string {
	mac := hmac.New(sha256.New, []byte(authToken))
	_, _ = mac.Write([]byte(monitorSessionPurpose))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// isMonitorHTTPS reports whether the browser-facing request used HTTPS:
// direct TLS, or X-Forwarded-Proto: https from a peer inside
// TRUSTED_PROXY_CIDRS. Direct clients cannot spoof the header to force (or
// weaken) the Secure flag (audit L26). Cookie.Secure is set when true;
// deployments that terminate TLS at a proxy must list that proxy in
// TRUSTED_PROXY_CIDRS and forward the header.
func isMonitorHTTPS(r *http.Request) bool {
	return trustedproxy.IsHTTPS(r)
}

func parseCookieHeader(raw string) map[string]string {
	result := map[string]string{}
	if raw == "" {
		return result
	}
	for _, part := range strings.Split(raw, ";") {
		entry := strings.TrimSpace(part)
		if entry == "" {
			continue
		}
		idx := strings.Index(entry, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(entry[:idx])
		value := strings.TrimSpace(entry[idx+1:])
		if key == "" {
			continue
		}
		result[key] = value
	}
	return result
}
