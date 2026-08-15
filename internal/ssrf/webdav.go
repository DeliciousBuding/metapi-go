// Package ssrf provides shared Server-Side Request Forgery (SSRF) guards for
// outbound WebDAV / backup transports.
//
// Both the admin settings handler (handler/admin) and the backup scheduler
// previously carried byte-identical copies of this logic under different
// names (rejectUnsafeWebdavDialHost / rejectUnsafeBackupWebdavDialHost, etc.).
// Centralizing it here keeps the two call sites from drifting: a fix to the
// unsafe-range predicate or the TOCTOU-aware dial check lands in one place.
//
// Each call site passes its own allowPrivate flag so operator/test escape
// hatches (e.g. pointing a WebDAV backup at a localhost test server) remain
// independent per feature without coupling the two packages.
package ssrf

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// RejectUnsafeWebdavDialHost validates a host immediately before a network
// dial. It applies hostname-level allow/deny rules and then resolves the host
// to verify that none of the resolved IPs fall in an unsafe range, preventing
// the TOCTOU race where a hostname resolves to a safe IP at validation time
// but a private IP by dial time (and vice versa).
//
// allowPrivate=true disables the guard entirely (operator escape hatch /
// test isolation for httptest servers on 127.0.0.1).
func RejectUnsafeWebdavDialHost(ctx context.Context, host string, allowPrivate bool) error {
	if !IsAllowedWebdavTargetHost(host, allowPrivate) {
		return fmt.Errorf("refusing WebDAV request to unsafe host %q", host)
	}
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return nil
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("no IP addresses found for WebDAV host %q", host)
	}
	for _, ip := range ips {
		if IsUnsafeAddr(ip) {
			return fmt.Errorf("refusing WebDAV request to unsafe resolved address %s", ip)
		}
	}
	return nil
}

// IsAllowedWebdavTargetHost applies hostname-level allow/deny rules before DNS
// resolution. A bare hostname is allowed (its IPs are checked at dial time by
// RejectUnsafeWebdavDialHost). Literal IP literals are allowed only when they
// are not in an unsafe range. "localhost" and *.localhost are always denied.
// allowPrivate=true permits all hosts (operator escape hatch).
func IsAllowedWebdavTargetHost(host string, allowPrivate bool) bool {
	if allowPrivate {
		return true
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" || strings.Contains(host, "%") {
		return false
	}
	lower := strings.TrimSuffix(strings.ToLower(host), ".")
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return !IsUnsafeAddr(addr)
	}
	return true
}

// IsUnsafeAddr reports whether an IP address falls in an unsafe range for
// outbound SSRF-sensitive traffic: unspecified, loopback, private,
// link-local unicast, link-local multicast, or multicast.
func IsUnsafeAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsUnspecified() ||
		addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast()
}

// IsPrivateOrLoopbackLiteral reports whether host is a literal IP address in a
// private, loopback, link-local, multicast, or unspecified range. This covers:
//   - Loopback: 127.0.0.0/8, ::1
//   - Link-local: 169.254.0.0/16 (incl. cloud metadata 169.254.169.254), fe80::/10
//   - Private: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
//   - Unspecified: 0.0.0.0/8, ::
//
// Hostnames that are not literal IPs return false: DNS resolution for
// hostnames is deferred to the dial-time guard (RejectUnsafeWebdavDialHost)
// to avoid TOCTOU races (a hostname that resolves to a safe IP at validation
// time could resolve to a private IP by dial time, and vice versa).
func IsPrivateOrLoopbackLiteral(host string) bool {
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return false
	}
	return IsUnsafeAddr(addr)
}
