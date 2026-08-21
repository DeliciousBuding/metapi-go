package ssrf

import (
	"context"
	"net/netip"
	"testing"
)

// Pin the SSRF address classification: anything reachable by an attacker-
// controlled DNS answer must be refused. IPv4-mapped IPv6 addresses are
// un-mapped first, so 127.0.0.1 written as ::ffff:127.0.0.1 stays blocked.
func TestIsUnsafeAddr_BlocksNonGlobalRanges(t *testing.T) {
	unsafe := []string{
		"0.0.0.0", "0.0.0.1",            // unspecified
		"::",                             // unspecified v6
		"127.0.0.1", "127.1.2.3",         // loopback
		"::1",                            // loopback v6
		"10.0.0.1", "172.16.0.1", "192.168.1.1", // private
		"169.254.169.254",                // cloud metadata
		"fe80::1",                        // link-local unicast
		"ff02::1",                        // link-local multicast
		"224.0.0.1",                      // multicast
		"::ffff:127.0.0.1",               // IPv4-mapped loopback
		"::ffff:10.0.0.1",                // IPv4-mapped private
		"::ffff:169.254.169.254",         // IPv4-mapped metadata
	}
	for _, raw := range unsafe {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			t.Fatalf("test address %q failed to parse: %v", raw, err)
		}
		if !IsUnsafeAddr(addr) {
			t.Errorf("IsUnsafeAddr(%q) = false, want true", raw)
		}
	}

	safe := []string{
		"8.8.8.8", "1.1.1.1", "93.184.216.34",
		"2606:4700:4700::1111",  // public v6
		"2001:4860:4860::8888",  // public v6
		"::ffff:8.8.8.8",        // IPv4-mapped public
	}
	for _, raw := range safe {
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			t.Fatalf("test address %q failed to parse: %v", raw, err)
		}
		if IsUnsafeAddr(addr) {
			t.Errorf("IsUnsafeAddr(%q) = true, want false", raw)
		}
	}
}

// Pin hostname-level admission: "localhost" and *.localhost are denied before
// any DNS resolution; a bare hostname is allowed at this layer because the
// dial-time guard checks its resolved addresses.
func TestIsAllowedWebdavTargetHost_HostnameRules(t *testing.T) {
	denied := []string{
		"localhost",
		"localhost.",
		"foo.localhost",
		"a.b.localhost",
		"127.0.0.1",
		"[::1]",
		"::1",
		"10.1.2.3",
		"169.254.169.254",
		"",
	}
	for _, host := range denied {
		if IsAllowedWebdavTargetHost(host, false) {
			t.Errorf("IsAllowedWebdavTargetHost(%q, false) = true, want false", host)
		}
	}

	allowed := []string{
		"example.com",
		"webdav.example.net:8443",
		"8.8.8.8",
		"2001:4860:4860::8888",
		// "127.0.0.1:8080" parses as a hostname at this layer; the dial-time
		// guard splits host:port and refuses the loopback literal there.
		"127.0.0.1:8080",
	}
	for _, host := range allowed {
		if !IsAllowedWebdavTargetHost(host, false) {
			t.Errorf("IsAllowedWebdavTargetHost(%q, false) = false, want true", host)
		}
	}

	// Operator escape hatch must bypass everything.
	if !IsAllowedWebdavTargetHost("127.0.0.1", true) {
		t.Error("IsAllowedWebdavTargetHost(127.0.0.1, true) = false, want true")
	}
}

// Pin literal-IP detection used by the URL-validation layer: private and
// loopback literals are refused at parse time; hostnames are deferred to the
// dial-time resolution check (no TOCTOU-prone early resolution).
func TestIsPrivateOrLoopbackLiteral(t *testing.T) {
	private := []string{
		"127.0.0.1", "10.0.0.5", "172.16.4.4", "192.168.0.10",
		"169.254.169.254", "::1", "::ffff:10.0.0.1", "0.0.0.0",
	}
	for _, host := range private {
		if !IsPrivateOrLoopbackLiteral(host) {
			t.Errorf("IsPrivateOrLoopbackLiteral(%q) = false, want true", host)
		}
	}

	public := []string{
		"8.8.8.8", "1.1.1.1", "2001:4860:4860::8888",
		"example.com", "localhost", "", "[::1]:8443",
	}
	for _, host := range public {
		if IsPrivateOrLoopbackLiteral(host) {
			t.Errorf("IsPrivateOrLoopbackLiteral(%q) = true, want false", host)
		}
	}
}

// RejectUnsafeWebdavDialHost must refuse literal private targets — including
// bracketed and IPv4-mapped spellings — and refuse localhost outright.
func TestRejectUnsafeWebdavDialHost_RefusesPrivateAndLocalhost(t *testing.T) {
	cases := []string{
		"127.0.0.1",
		"[::1]",
		"localhost",
		"LOCALHOST",
		"foo.localhost",
		"10.0.0.1",
		"169.254.169.254",
		"::ffff:192.168.0.1",
	}
	for _, host := range cases {
		err := RejectUnsafeWebdavDialHost(context.Background(), host, false)
		if err == nil {
			t.Errorf("RejectUnsafeWebdavDialHost(%q, false) = nil, want error", host)
		}
	}
}

// The allowPrivate escape hatch must skip the guard entirely.
func TestRejectUnsafeWebdavDialHost_AllowPrivateBypasses(t *testing.T) {
	err := RejectUnsafeWebdavDialHost(context.Background(), "127.0.0.1", true)
	if err != nil {
		t.Errorf("RejectUnsafeWebdavDialHost(127.0.0.1, true) = %v, want nil", err)
	}
}

// RejectUnsafeWebdavDialHost must accept a public literal without touching DNS.
func TestRejectUnsafeWebdavDialHost_AcceptsPublicLiteral(t *testing.T) {
	err := RejectUnsafeWebdavDialHost(context.Background(), "8.8.8.8", false)
	if err != nil {
		t.Errorf("RejectUnsafeWebdavDialHost(8.8.8.8, false) = %v, want nil", err)
	}
}
