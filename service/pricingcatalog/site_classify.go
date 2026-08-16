package pricingcatalog

import (
	"net/url"
	"strings"
)

// SiteClass labels a site's vendor status for catalog provenance.
type SiteClass string

const (
	// SiteClassOfficial marks a site pointing at the vendor's own API host.
	SiteClassOfficial SiteClass = "official"
	// SiteClassRelay marks a third-party relay (or anything we cannot prove
	// is the vendor). Catalog prices for relays are estimates only.
	SiteClassRelay SiteClass = "relay"
)

// SiteSnapshot is the minimal site metadata needed for classification.
type SiteSnapshot struct {
	Platform string
	URL      string
}

// officialHosts maps canonical MetAPI platform ids (see platform.PlatformAliases)
// to the vendor's official API hosts. Anything not listed classifies as relay
// (honest default): models.dev prices are official list prices and must not be
// claimed as the real payment price of a third-party relay.
var officialHosts = map[string]map[string]struct{}{
	"openai":     hostSet("api.openai.com"),
	"codex":      hostSet("api.openai.com", "chatgpt.com"),
	"claude":     hostSet("api.anthropic.com"),
	"gemini":     hostSet("generativelanguage.googleapis.com"),
	"gemini-cli": hostSet("generativelanguage.googleapis.com"),
	"grok":       hostSet("api.x.ai"),
}

func hostSet(hosts ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		set[strings.ToLower(host)] = struct{}{}
	}
	return set
}

// ClassifySite returns official only when BOTH the platform and the site URL
// host match a known vendor endpoint. Unknown platform, empty URL, or any
// other host (including relays fronting the vendor's models) → relay.
func ClassifySite(site SiteSnapshot) SiteClass {
	hosts, ok := officialHosts[strings.ToLower(strings.TrimSpace(site.Platform))]
	if !ok {
		return SiteClassRelay
	}
	host := siteHost(site.URL)
	if host == "" {
		return SiteClassRelay
	}
	if _, ok := hosts[strings.ToLower(host)]; ok {
		return SiteClassOfficial
	}
	return SiteClassRelay
}

// siteHost extracts the hostname from a site URL, tolerating scheme-less
// inputs like "api.openai.com/v1" that operators sometimes store.
func siteHost(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err == nil {
		if host := parsed.Hostname(); host != "" {
			return host
		}
	}
	rest := trimmed
	if idx := strings.Index(rest, "://"); idx >= 0 {
		rest = rest[idx+3:]
	}
	host := rest
	if idx := strings.IndexAny(rest, "/?#"); idx >= 0 {
		host = rest[:idx]
	}
	if idx := strings.Index(host, ":"); idx >= 0 {
		host = host[:idx]
	}
	return host
}
