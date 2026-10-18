package service

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"

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

// proxyURLSchemes are the egress proxy schemes admin write APIs accept.
// platform.supportedProxySchemes also carries the legacy socks/socks4/socks4a
// spellings for URLs already persisted by older deployments; new writes are
// restricted to the schemes we document.
var proxyURLSchemes = map[string]bool{
	"http": true, "https": true, "socks5": true, "socks5h": true,
}

// ValidateProxyURL validates an optional egress proxy URL. Empty means "no
// proxy" and is valid. A non-empty value needs a supported scheme, a host, and
// no whitespace or control characters: storing a value the transport cannot
// dial would silently disable egress, and control characters can split the
// URL across transport layers. Private/loopback targets are intentionally
// allowed here — internal proxies are a legitimate deployment; outbound
// destination guarding is enforced by the site dial guard instead.
func ValidateProxyURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	for _, r := range trimmed {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return errors.New("proxy URL contains whitespace or control characters")
		}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("proxy URL is malformed: %w", err)
	}
	if !proxyURLSchemes[strings.ToLower(parsed.Scheme)] {
		return errors.New("proxy URL scheme must be http, https, socks5, or socks5h")
	}
	if parsed.Hostname() == "" {
		return errors.New("proxy URL is missing a host")
	}
	return nil
}

// IsValidProxyURL checks whether a string is a valid http(s)/socks proxy URL.
func IsValidProxyURL(raw string) bool {
	return ValidateProxyURL(raw) == nil
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

// IsForbiddenPublicTargetURL applies the public-only outbound policy for
// targets supplied by an upstream response or another non-operator party.
// Unlike IsForbiddenSiteTargetURL it also rejects loopback, RFC1918, ULA and
// 0.0.0.0/8, and fails closed on unparseable or credential-bearing URLs.
func IsForbiddenPublicTargetURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return true
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return true
	}
	return ssrf.IsForbiddenPublicHost(parsed.Hostname())
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
