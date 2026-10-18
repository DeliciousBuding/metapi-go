// Package trustedproxy carries the per-request "arrived through an explicitly
// configured reverse proxy" signal from router.TrustedRealIP to downstream
// consumers that must not trust client-supplied forwarding headers on their
// own (cookie Secure flags, exported base URLs).
package trustedproxy

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

type contextKey struct{}

// WithTrust marks ctx as originating from a direct peer that matched
// TRUSTED_PROXY_CIDRS. Only router.TrustedRealIP should set it.
func WithTrust(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKey{}, true)
}

// FromRequest reports whether the request's direct peer matched
// TRUSTED_PROXY_CIDRS. It is false for direct clients, including direct TLS
// clients.
func FromRequest(ctx context.Context) bool {
	trusted, _ := ctx.Value(contextKey{}).(bool)
	return trusted
}

// ForwardedProto returns the X-Forwarded-Proto value only when the request
// arrived through a trusted proxy. Direct clients can send any value they
// like, so their header is ignored.
func ForwardedProto(r *http.Request) string {
	if !FromRequest(r.Context()) {
		return ""
	}
	return strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
}

// ForwardedHost returns a validated X-Forwarded-Host value only when the
// request arrived through a trusted proxy. Direct clients cannot steer it.
func ForwardedHost(r *http.Request) string {
	if !FromRequest(r.Context()) {
		return ""
	}
	return ValidateHost(r.Header.Get("X-Forwarded-Host"))
}

// IsHTTPS reports whether the browser-facing request used HTTPS: direct TLS
// always counts, and X-Forwarded-Proto: https is honored only behind a
// trusted proxy. Otherwise a direct client could spoof the header and either
// force Secure cookies over plain HTTP or (with a spoofed http value) weaken
// them.
func IsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(ForwardedProto(r), "https")
}

// ValidateHost normalizes a Host-style header value and rejects anything that
// is not a bare host[:port]: userinfo, paths, queries, fragments, whitespace,
// commas (forwarded lists) and empty values all return "". The result is safe
// to concatenate into "scheme://host".
func ValidateHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n,") {
		return ""
	}
	u, err := url.Parse("//" + raw)
	if err != nil || u.Host != raw || u.User != nil || u.Path != "" ||
		u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	return raw
}
