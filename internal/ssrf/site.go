// Package ssrf provides shared SSRF guards for outbound transports.
package ssrf

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// NetIPResolver is the subset of net.Resolver used by the site dial guard.
// It is intentionally injectable so tests can exercise DNS alias behavior
// without relying on public DNS.
type NetIPResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// DialContextFunc matches net.Dialer's DialContext method.
type DialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

var forbiddenSitePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),      // this-network / unspecified
	netip.MustParsePrefix("169.254.0.0/16"), // IPv4 link-local and common metadata endpoints
	netip.MustParsePrefix("224.0.0.0/4"),    // multicast
	netip.MustParsePrefix("240.0.0.0/4"),    // reserved / limited broadcast
	netip.MustParsePrefix("fe80::/10"),      // IPv6 link-local unicast
	netip.MustParsePrefix("ff00::/8"),       // IPv6 multicast
}

var explicitMetadataAddrs = map[netip.Addr]struct{}{
	netip.MustParseAddr("100.100.100.200"): {}, // Alibaba Cloud instance metadata
	netip.MustParseAddr("fd00:ec2::254"):   {}, // AWS IMDS IPv6 endpoint
	netip.MustParseAddr("fd20:ce::254"):    {}, // GCP IMDS IPv6 endpoint (inside ULA)
}

// IsForbiddenSiteHostname reports hostnames that are metadata service aliases
// independent of DNS. Literal IPs are classified by IsForbiddenSiteAddr.
// Loopback and RFC1918/ULA addresses deliberately remain allowed because
// Metapi supports operator-hosted upstreams on local networks.
func IsForbiddenSiteHostname(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return IsForbiddenSiteAddr(addr)
	}
	lower := strings.TrimSuffix(strings.ToLower(host), ".")
	switch lower {
	case "metadata", "instance-data", "metadata.google.internal":
		return true
	default:
		return false
	}
}

// IsForbiddenSiteAddr applies the product's site-upstream policy at dial time.
// It rejects link-local, multicast, unspecified/reserved, and explicit metadata
// addresses while preserving loopback, RFC1918, and IPv6 ULA connectivity.
func IsForbiddenSiteAddr(addr netip.Addr) bool {
	addr = addr.Unmap()
	if _, blocked := explicitMetadataAddrs[addr]; blocked {
		return true
	}
	for _, prefix := range forbiddenSitePrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return addr.IsUnspecified() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast()
}

// NewSiteDialContext returns a DNS-pinning DialContext for operator-configured
// site upstreams. Hostnames are resolved exactly once, every answer is checked,
// and the connection is made to the already-checked IP rather than asking the
// underlying dialer to resolve the hostname again. This closes the usual DNS
// rebinding / validation-to-dial window.
func NewSiteDialContext(resolver NetIPResolver, dial DialContextFunc) DialContextFunc {
	return newGuardedDialContext("site", resolver, dial, IsForbiddenSiteHostname, IsForbiddenSiteAddr)
}

// newGuardedDialContext builds the resolve-once, check-every-answer,
// dial-the-pinned-IP transport shared by the site and public-only policies.
// kind only names the policy in error messages.
func newGuardedDialContext(
	kind string,
	resolver NetIPResolver,
	dial DialContextFunc,
	forbiddenHost func(string) bool,
	forbiddenAddr func(netip.Addr) bool,
) DialContextFunc {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dial == nil {
		dialer := &net.Dialer{}
		dial = dialer.DialContext
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("invalid %s dial address %q: %w", kind, address, err)
		}
		if forbiddenHost(host) {
			return nil, fmt.Errorf("refusing %s request to forbidden host %q", kind, host)
		}

		var addrs []netip.Addr
		if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
			addrs = []netip.Addr{literal}
		} else {
			addrs, err = resolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve %s host %q: %w", kind, host, err)
			}
		}
		if len(addrs) == 0 {
			return nil, fmt.Errorf("no IP addresses found for %s host %q", kind, host)
		}

		for _, addr := range addrs {
			if forbiddenAddr(addr) {
				return nil, fmt.Errorf("refusing %s request to forbidden resolved address %s for host %q", kind, addr, host)
			}
		}

		var dialErrs []error
		compatible := 0
		for _, addr := range addrs {
			addr = addr.Unmap()
			if !addressMatchesNetwork(addr, network) {
				continue
			}
			compatible++
			pinnedAddress := net.JoinHostPort(addr.String(), port)
			conn, dialErr := dial(ctx, network, pinnedAddress)
			if dialErr == nil {
				return conn, nil
			}
			dialErrs = append(dialErrs, fmt.Errorf("%s: %w", pinnedAddress, dialErr))
			if ctx.Err() != nil {
				break
			}
		}
		if compatible == 0 {
			return nil, fmt.Errorf("no %s addresses found for %s host %q", network, kind, host)
		}
		return nil, fmt.Errorf("dial %s host %q: %w", kind, host, errors.Join(dialErrs...))
	}
}

func addressMatchesNetwork(addr netip.Addr, network string) bool {
	switch network {
	case "tcp4":
		return addr.Is4()
	case "tcp6":
		return addr.Is6()
	default:
		return true
	}
}
