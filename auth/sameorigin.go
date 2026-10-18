// metapi-go/auth — cross-site request guard for cookie-authenticated admin
// surfaces.
//
// The admin UI authenticates with a SameSite=Strict cookie, which blocks
// cross-site requests but not same-site ones: a sibling subdomain (or another
// service on the same host) is "same-site" and can still drive state-changing
// requests. Browsers send Origin on unsafe requests and Sec-Fetch-Site on all
// fetch/navigation requests, so comparing them closes the sibling-subdomain
// and same-host-other-port gaps without affecting non-browser clients
// (curl/scripts send neither header).
package auth

import (
	"net/http"
	"net/url"
	"strings"
)

// SameOriginGuard rejects state-changing requests whose declaring origin does
// not match the request host. allowedOrigins is an explicit list of extra
// origins (scheme://host[:port]) accepted for deployments that front the API
// with a different public host; comparison is exact on the serialized origin.
func SameOriginGuard(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if normalized, ok := normalizeOrigin(origin); ok {
			allowed[normalized] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
				if !originAllowed(origin, r.Host, allowed) {
					writeCrossSiteRejected(w)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			// No Origin: browsers still declare same-site vs same-origin via
			// Fetch Metadata. Reject same-site (sibling subdomain, another port
			// on the same registrable domain); older browsers omit the header
			// and fall through to the SameSite=Strict cookie as before.
			switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
			case "cross-site", "same-site":
				writeCrossSiteRejected(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin, requestHost string, allowed map[string]bool) bool {
	if strings.EqualFold(strings.TrimSpace(origin), "null") {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	if allowed[strings.ToLower(parsed.Scheme+"://"+parsed.Host)] {
		return true
	}
	return strings.EqualFold(parsed.Host, requestHost)
}

func normalizeOrigin(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host), true
}

func writeCrossSiteRejected(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error":"Cross-site request rejected","errorCode":"authCrossSiteRejected"}`))
}
