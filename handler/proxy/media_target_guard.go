// metapi-go/handler/proxy — outbound media target policy.
//
// The gateway re-fetches image/video URLs found in upstream responses. Those
// URLs are chosen by the upstream (a relay may be malicious or compromised),
// so they get the public-only policy instead of the site policy that permits
// operator-run private networks. mediaTargetAllowPrivate is a test-only switch
// for the fake CDNs the handler tests stand up on loopback; production never
// sets it.

package proxyhandler

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
	"github.com/deliciousbuding/metapi-go/service"
)

var mediaTargetAllowPrivate bool

// SetMediaTargetAllowPrivateForTest lets tests in other packages that stand up
// a loopback CDN exercise the media path. It returns a restore function and is
// never called from production code.
func SetMediaTargetAllowPrivateForTest(allow bool) func() {
	previous := mediaTargetAllowPrivate
	mediaTargetAllowPrivate = allow
	return func() { mediaTargetAllowPrivate = previous }
}

// allowedPublicMediaTarget reports whether the gateway may fetch raw. It
// rejects unsafe literal addresses, then resolves hostnames once and rejects
// any answer outside the global unicast range; the actual dial still goes
// through the normal upstream client, so a fully rebinding-proof fetch would
// additionally pin the IP (tracked as a follow-up for proxied downloads).
func allowedPublicMediaTarget(ctx context.Context, raw string) bool {
	if mediaTargetAllowPrivate {
		return true
	}
	if service.IsForbiddenPublicTargetURL(raw) {
		return false
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		return !ssrf.IsUnsafePublicAddr(ip)
	}
	// A hostname that cannot be resolved must not be attempted: fail closed.
	resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(resolved) == 0 {
		return false
	}
	for _, addr := range resolved {
		if ssrf.IsUnsafePublicAddr(addr) {
			return false
		}
	}
	return true
}
