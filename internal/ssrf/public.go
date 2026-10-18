package ssrf

import (
	"net/netip"
	"strings"
)

// Public-only target policy.
//
// The site policy (site.go) deliberately keeps loopback / RFC1918 / ULA
// connectivity because operators run upstreams on private networks. Outbound
// targets that a session or a non-operator input controls — notification
// channels, SMTP, catalog sources — must not reach those ranges at all: they
// are server-side fetches to an address the caller chose, which is the SSRF
// primitive. NewPublicOnlyDialContext resolves once, rejects any non-public
// answer, and dials the pinned IP, so a hostname cannot pass validation and
// then resolve privately at dial time.

// IsUnsafePublicAddr reports whether addr is outside the global unicast range
// accepted for user/admin-controlled outbound traffic: unspecified, loopback,
// RFC1918, ULA (fc00::/7, which also covers GCP's fd20:ce::254 metadata
// endpoint), link-local, multicast, and the whole 0.0.0.0/8 "this network"
// block. IsUnspecified alone only matches 0.0.0.0; addresses such as 0.1.2.3
// are routed to the local stack on some kernels.
func IsUnsafePublicAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr.Is4() && addr.As4()[0] == 0 {
		return true
	}
	return addr.IsUnspecified() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast()
}

// IsForbiddenPublicHost applies the hostname-level half of the public-only
// policy before DNS. Literal IPs are classified by IsUnsafePublicAddr;
// localhost aliases and zone-qualified literals are always rejected.
func IsForbiddenPublicHost(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || strings.Contains(host, "%") {
		return true
	}
	lower := strings.TrimSuffix(strings.ToLower(host), ".")
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return true
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return IsUnsafePublicAddr(addr)
	}
	return false
}

// NewPublicOnlyDialContext returns a DNS-pinning DialContext that only dials
// public addresses. Pass nil for resolver/dial to use the process defaults;
// tests inject fakes.
func NewPublicOnlyDialContext(resolver NetIPResolver, dial DialContextFunc) DialContextFunc {
	return newGuardedDialContext("outbound", resolver, dial, IsForbiddenPublicHost, IsUnsafePublicAddr)
}
