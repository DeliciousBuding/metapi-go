package router

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

// cspNonceKey carries the per-request CSP nonce from SecurityHeaders to the
// SPA fallback handler, which injects it into the served index.html so
// runtime <style> injectors can stamp their elements with it.
type cspNonceKey struct{}

// sonnerToastStyleHash is the sha256 hash of the exact CSS string that
// sonner (the toast library) injects at import time via
// document.createElement("style") + text node. sonner exposes no nonce hook
// for that injection, so the policy allowlists the static string by hash
// instead of 'unsafe-inline'.
//
// Provenance: sha256 over the JS string literal passed to sonner's
// __insertCSS() call in the built bundle (web/dist/static/js/*.js). The
// string is unescaped JS; today it contains no escape sequences.
//
// Drift guard: TestSonnerStyleHashMatchesEmbeddedBundle re-derives the hash
// from the embedded dist and fails when a sonner upgrade changes the string,
// so the constant cannot silently go stale. Even if it ever did ship stale,
// the UI keeps working: web/src/components/ui/sonner.tsx also bundles
// sonner/dist/styles.css (same rules) as a static stylesheet, so a blocked
// runtime injection only costs a console violation, not toast styling.
//
// Verified against real engines (Playwright probes, 2026-08): sha256 sources
// in style-src are honored for DOM-inserted <style> elements on Chromium
// 151, Firefox 153 and WebKit 26.5.
const sonnerToastStyleHash = "'sha256-StEaX+se6YS7pqjzrzMIA0KaX9zF/8zAhvQXZAe5epY='"

// generateCSPNonce returns a cryptographically random nonce (16 random bytes,
// standard base64 — the CSP nonce-value grammar). A fresh value is minted
// per response so an injected element from one page load is useless on the
// next. crypto/rand is the only source; there is no deterministic fallback.
func generateCSPNonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand failing means the process has no entropy source left;
		// serving a guessable nonce would silently void the whole policy.
		panic("csp: crypto/rand unavailable: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// contentSecurityPolicy renders the CSP header value for one request.
//
// 'unsafe-inline' was removed from style-src (#1035 S2). The three runtime
// <style> injectors in the SPA are covered individually:
//
//   - chart color variables (web/src/components/ui/chart.tsx): React renders
//     the <style> element with a nonce attribute taken from the
//     <meta name="csp-nonce"> tag this server injects into index.html.
//   - dialog scroll lock (cmdk -> react-remove-scroll ->
//     react-style-singleton): main.tsx calls setNonce() from the get-nonce
//     package with the same meta value; the library stamps its injected
//     <style> with the nonce attribute.
//   - sonner toast styles: hashed, see sonnerToastStyleHash.
//
// script-src keeps no nonce: index.html has no inline scripts (theme
// bootstraps are static files) and none may be added without a review here.
func contentSecurityPolicy(nonce string) string {
	return "default-src 'self'; " +
		"script-src 'self' https://static.cloudflareinsights.com; " +
		"style-src 'self' 'nonce-" + nonce + "' " + sonnerToastStyleHash + "; " +
		"img-src 'self' https://api.dicebear.com; " +
		"connect-src 'self'; " +
		"frame-src 'self' https://check.linux.do; " +
		"frame-ancestors 'none'"
}

// SecurityHeaders adds standard security HTTP response headers.
// Applied globally to all routes including the admin SPA and API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce := generateCSPNonce()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy(nonce))
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), cspNonceKey{}, nonce)))
	})
}

// CSPNonceFromContext returns the per-request CSP nonce minted by
// SecurityHeaders, or "" when the middleware is not in the chain (test
// routers that mount handlers directly). Callers must treat "" as "nonce
// unavailable" and degrade instead of guessing.
func CSPNonceFromContext(ctx context.Context) string {
	nonce, _ := ctx.Value(cspNonceKey{}).(string)
	return nonce
}
