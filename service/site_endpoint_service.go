package service

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
)

// NormalizeSiteAPIEndpointBaseUrl normalizes a site API endpoint URL.
// Mirrors TS normalizeSiteApiEndpointBaseUrl(): parse URL, strip search/hash, remove trailing slash.
func NormalizeSiteAPIEndpointBaseUrl(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return strings.TrimRight(trimmed, "/")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

// IsValidAPIEndpointURL checks whether a normalized URL is a valid http(s) URL.
// Also rejects cloud metadata / link-local targets via IsForbiddenSiteTargetURL
// so any future caller is safe by default (parity with).
func IsValidAPIEndpointURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, ok := parseSiteHTTPURL(trimmed)
	if !ok || ssrf.IsForbiddenSiteHostname(parsed.Hostname()) {
		return false
	}
	return true
}

// IsValidProxyURL checks whether a string is a valid http(s)/socks proxy URL.
func IsValidProxyURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true // empty/null is valid (no proxy)
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return false
	}
	switch parsed.Scheme {
	case "http", "https", "socks", "socks5", "socks5h":
		return true
	}
	return false
}

// IsValidHTTPURL checks whether a string is a valid http/https URL (not socks).
// Empty is valid (optional field). Also rejects cloud metadata / link-local
// targets via IsForbiddenSiteTargetURL so externalCheckin and similar callers
// cannot store first-hop SSRF URLs (extends).
func IsValidHTTPURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true
	}
	parsed, ok := parseSiteHTTPURL(trimmed)
	if !ok || ssrf.IsForbiddenSiteHostname(parsed.Hostname()) {
		return false
	}
	return true
}

func parseSiteHTTPURL(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.Host == "" || parsed.Hostname() == "" {
		return nil, false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return nil, false
	}
	if strings.HasSuffix(parsed.Host, ":") {
		return nil, false
	}
	if port := parsed.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, false
		}
	}
	return parsed, true
}

// IsForbiddenSiteTargetURL reports whether a site/endpoint URL must be rejected
// as a first-hop SSRF risk to cloud metadata / link-local targets.
// RFC1918 private and localhost remain allowed for lab/docker operators.
func IsForbiddenSiteTargetURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	return ssrf.IsForbiddenSiteHostname(parsed.Hostname())
}
