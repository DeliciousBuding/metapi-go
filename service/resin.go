package service

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// This file implements the Resin sticky-proxy-pool integration (Tier 1).
//
// Resin is an external proxy pool that gives each business identity a stable
// outbound IP. metapi-go routes its outbound traffic through Resin so each
// account keeps a stable egress IP.
//
// Two access modes:
//
//  1. Forward proxy (HTTP) — used for all HTTP/SSE traffic. The proxy URL
//     embeds proxy-auth credentials {Platform}.{Account}:{token}; Go's
//     http.Transport derives Proxy-Authorization automatically from the
//     proxy-URL userinfo.
//
//  2. Reverse proxy — used only for native wss dials that bypass the HTTP
//     forward proxy. The target URL is rewritten to
//     ws://{host}:{port}/{token}/{Platform}/{protocol}/{targetHost}{path}
//     and the X-Resin-Account header carries the Account identity.
//
// Account identity rules (Resin splits proxy-auth on first '.' and last ':'):
//   - Stable (post-creation):  acc-{Account.ID}
//   - Pre-account (verify/login/create):  tmp-{sha1(siteID:token)[:16]}
//
// See doc/integration-prompt.md in the resin repo for the full contract.

// resinAccountPrefixStable is the prefix for stable post-creation identities.
const resinAccountPrefixStable = "acc-"

// resinAccountPrefixTemp is the prefix for deterministic pre-account identities.
const resinAccountPrefixTemp = "tmp-"

// Enabled reports whether the Resin sticky proxy is configured and active.
// Resin requires both RESIN_ENABLED=true and a non-empty RESIN_URL.
func ResinEnabled(cfg *config.Config) bool {
	if cfg == nil || !cfg.ResinEnabled {
		return false
	}
	return strings.TrimSpace(cfg.ResinURL) != ""
}

// AccountFor returns the stable Resin account identity for an existing account.
// The identity is acc-{Account.ID} using the immutable auto-increment PK.
// Returns "" when the account row has no ID yet (pre-account flows should use
// TempAccountFor instead).
func AccountFor(account *store.Account, site *store.Site) string {
	if account != nil && account.ID > 0 {
		return fmt.Sprintf("%s%d", resinAccountPrefixStable, account.ID)
	}
	return ""
}

// TempAccountFor returns a deterministic temp Resin identity for pre-account
// flows (verify/login/create) so all requests for the same token land on the
// same egress IP. The identity is tmp-{sha1(siteID:token) first 16 hex chars}.
// siteID may be 0; the identity remains deterministic per token.
//
// The identity NEVER contains '.' or ':' (sha1 hex output is [0-9a-f]) so it
// is safe for Resin's proxy-auth splitting.
func TempAccountFor(siteID int64, token string) string {
	digest := sha1.Sum([]byte(fmt.Sprintf("%d:%s", siteID, token)))
	return resinAccountPrefixTemp + hex.EncodeToString(digest[:])[:16]
}

// ForwardProxyURL builds the HTTP forward-proxy URL with embedded proxy-auth
// userinfo. The username is "{Platform}.{Account}" and the password is the
// resin token. Go's http.Transport reads the proxy URL userinfo and emits
// the Proxy-Authorization: Basic header automatically — no manual header needed.
//
// Returns ("", false) when Resin is misconfigured or platform/account empty.
func ForwardProxyURL(cfg *config.Config, platform, account string) (string, bool) {
	host, token, ok := parseResinURL(cfg)
	if !ok {
		return "", false
	}
	platform = strings.TrimSpace(platform)
	account = strings.TrimSpace(account)
	if platform == "" || account == "" {
		return "", false
	}
	// url.UserPassword stores the raw values; http.Transport extracts the
	// unescaped forms when building the Proxy-Authorization header.
	proxyURL := &url.URL{
		Scheme: "http",
		Host:   host,
		User:   url.UserPassword(platform+"."+account, token),
	}
	return proxyURL.String(), true
}

// RewriteWSURL rewrites a target wss/ws URL into Resin reverse-proxy form:
//
//	ws://{host}:{port}/{token}/{Platform}/{protocol}/{targetHost}{path}?query
//
// where protocol is "https" for wss targets and "http" for ws targets
// (Resin requires the client→resin leg to always be ws). The Account identity
// is NOT embedded in the URL — it is sent via the X-Resin-Account header
// (handled by the caller). Returns "" when Resin is misconfigured, the
// platform/account is empty, or the target URL is not ws/wss.
func RewriteWSURL(targetURL string, cfg *config.Config, platform, account string) string {
	host, token, ok := parseResinURL(cfg)
	if !ok {
		return ""
	}
	platform = strings.TrimSpace(platform)
	account = strings.TrimSpace(account)
	if platform == "" || account == "" {
		return ""
	}
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || parsed == nil {
		return ""
	}
	var protocol string
	switch strings.ToLower(parsed.Scheme) {
	case "wss":
		protocol = "https"
	case "ws":
		protocol = "http"
	default:
		return ""
	}
	// targetHost carries port if present (e.g. api.example.com:443).
	targetHost := parsed.Host
	if targetHost == "" {
		return ""
	}
	// Build path: /{token}/{Platform}/{protocol}/{targetHost}{originalPath}
	pathSegments := []string{"", token, platform, protocol, targetHost}
	rewrittenPath := strings.Join(pathSegments, "/")
	if parsed.Path != "" && parsed.Path != "/" {
		rewrittenPath += "/" + strings.TrimPrefix(parsed.Path, "/")
	}
	resinURL := &url.URL{
		Scheme:   "ws",
		Host:     host,
		Path:     rewrittenPath,
		RawQuery: parsed.RawQuery,
	}
	// account is intentionally NOT in the URL; caller sets X-Resin-Account.
	_ = account
	return resinURL.String()
}

// ResolveResinPlatform resolves the Platform identity: explicit config value
// wins, otherwise fall back to the site's platform. Returns "" when neither
// is available.
func ResolveResinPlatform(cfg *config.Config, site *store.Site) string {
	if cfg != nil {
		if name := strings.TrimSpace(cfg.ResinPlatformName); name != "" {
			return name
		}
	}
	if site != nil {
		if platform := strings.TrimSpace(site.Platform); platform != "" {
			return platform
		}
	}
	return ""
}

// parseResinURL extracts the host (host:port) and token from cfg.ResinURL.
// The token is the entire path-after-host (e.g. "my-token" for
// http://resin.local:2260/my-token). Returns ok=false when ResinURL is empty,
// unparseable, missing a host, or missing the token segment — in those cases
// Resin is misconfigured and the caller should fall back to direct.
func parseResinURL(cfg *config.Config) (host, token string, ok bool) {
	if cfg == nil {
		return "", "", false
	}
	raw := strings.TrimSpace(cfg.ResinURL)
	if raw == "" {
		return "", "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		slog.Warn("resin: invalid RESIN_URL, falling back to direct", "resin_url", raw, "err", err)
		return "", "", false
	}
	host = parsed.Host
	if host == "" {
		slog.Warn("resin: RESIN_URL missing host, falling back to direct", "resin_url", raw)
		return "", "", false
	}
	// The token is the whole path-after-host (minus the leading slash).
	token = strings.TrimPrefix(parsed.Path, "/")
	if token == "" {
		slog.Warn("resin: RESIN_URL missing token segment, falling back to direct", "resin_url", raw)
		return "", "", false
	}
	return host, token, true
}
