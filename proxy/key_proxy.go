package proxy

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/platform"
)

// ApplyKeyProxyOverride returns a ProxyConfig with ProxyURL replaced by the
// per-key egress proxy when keyProxyURL is non-empty after trim.

// base may be nil (no site/account proxy). Custom headers and other fields
// from base are preserved so site custom headers still apply under a key
// proxy override.

// Empty / whitespace keyProxyURL returns base unchanged (inherit).
func ApplyKeyProxyOverride(base *platform.ProxyConfig, keyProxyURL *string) *platform.ProxyConfig {
	if keyProxyURL == nil {
		return base
	}
	trimmed := strings.TrimSpace(*keyProxyURL)
	if trimmed == "" {
		return base
	}

	if base == nil {
		return &platform.ProxyConfig{ProxyURL: trimmed}
	}

	// Clone so callers can share base configs safely.
	out := *base
	out.ProxyURL = trimmed
	// Explicit key proxy is not the system proxy path.
	out.UseSystemProxy = false
	if base.CustomHeaders != nil {
		headers := make(map[string]string, len(base.CustomHeaders))
		for k, v := range base.CustomHeaders {
			headers[k] = v
		}
		out.CustomHeaders = headers
	}
	return &out
}
