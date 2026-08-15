package admin

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
)

// disallowPrivateWebdavTargetsForTest explicitly pins the test-only
// allowPrivateWebdavTargets flag to false so the local URL-validation layer
// exercises the SSRF guards. The flag defaults to false, but other tests in
// this package flip it to true (see allowPrivateWebdavTargetsForTest), so we
// restore the prior value via t.Cleanup to avoid cross-test contamination.
func disallowPrivateWebdavTargetsForTest(t *testing.T) {
	t.Helper()
	previous := allowPrivateWebdavTargets
	allowPrivateWebdavTargets = false
	t.Cleanup(func() { allowPrivateWebdavTargets = previous })
}

// TestIsPrivateOrLoopbackLiteral exercises the literal-IP classification used
// by the URL-validation layer. Hostnames that are not literal IPs return false
// because DNS resolution is deferred to the dial-time guard to avoid TOCTOU.
func TestIsPrivateOrLoopback(t *testing.T) {
	disallowPrivateWebdavTargetsForTest(t)

	tests := []struct {
		name string
		host string
		want bool
	}{
		{"localhost is not a literal IP", "localhost", false},
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv4 loopback high octet", "127.255.255.254", true},
		{"cloud metadata link-local", "169.254.169.254", true},
		{"IPv6 loopback", "::1", true},
		{"IPv6 loopback bracketed", "[::1]", true},
		{"private 10/8", "10.0.0.1", true},
		{"private 172.16/12", "172.16.0.1", true},
		{"private 192.168/16", "192.168.1.1", true},
		{"unspecified 0.0.0.0", "0.0.0.0", true},
		{"unspecified IPv6", "::", true},
		{"public IPv4 8.8.8.8", "8.8.8.8", false},
		{"public IPv4 1.1.1.1", "1.1.1.1", false},
		{"public hostname is not a literal IP", "example.com", false},
		{"empty string is not a literal IP", "", false},
		{"garbage is not a literal IP", "not-an-ip", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ssrf.IsPrivateOrLoopbackLiteral(tt.host); got != tt.want {
				t.Fatalf("ssrf.IsPrivateOrLoopbackLiteral(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

// TestIsAllowedWebdavTargetHost verifies the hostname-level allowlist that runs
// before any DNS resolution. It rejects localhost and unsafe literal IPs while
// permitting public IPs and arbitrary hostnames (whose resolution is deferred).
func TestIsAllowedWebdavTargetHost(t *testing.T) {
	disallowPrivateWebdavTargetsForTest(t)

	tests := []struct {
		name string
		host string
		want bool
	}{
		{"localhost rejected", "localhost", false},
		{"localhost subdomain rejected", "api.localhost", false},
		{"localhost trailing dot rejected", "localhost.", false},
		{"IPv4 loopback rejected", "127.0.0.1", false},
		{"cloud metadata rejected", "169.254.169.254", false},
		{"IPv6 loopback rejected", "::1", false},
		{"private 10/8 rejected", "10.0.0.1", false},
		{"private 172.16/12 rejected", "172.16.0.1", false},
		{"private 192.168/16 rejected", "192.168.1.1", false},
		{"public IPv4 allowed", "8.8.8.8", true},
		{"public DNS hostname allowed", "example.com", true},
		{"empty host rejected", "", false},
		{"zone-index host rejected", "fe80::1%eth0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ssrf.IsAllowedWebdavTargetHost(tt.host, false); got != tt.want {
				t.Fatalf("ssrf.IsAllowedWebdavTargetHost(%q, false) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

// TestIsAllowedWebdavTargetHost_FlagBypassed confirms that when the test-only
// allowPrivateWebdavTargets flag is true, the hostname guard is disabled. This
// documents why the positive SSRF tests above must run with the flag false.
func TestIsAllowedWebdavTargetHost_FlagBypassed(t *testing.T) {
	allowPrivateWebdavTargetsForTest(t) // flips to true

	if !ssrf.IsAllowedWebdavTargetHost("127.0.0.1", true) {
		t.Fatal("with allowPrivate=true, 127.0.0.1 should be allowed")
	}
	if !ssrf.IsAllowedWebdavTargetHost("localhost", true) {
		t.Fatal("with allowPrivate=true, localhost should be allowed")
	}
}

// TestRejectUnsafeWebdavDialHost exercises the dial-time guard. Literal IPs and
// localhost are deterministic (no DNS required). Public literal IPs are parsed
// directly and allowed without resolution.
func TestRejectUnsafeWebdavDialHost(t *testing.T) {
	disallowPrivateWebdavTargetsForTest(t)

	tests := []struct {
		name     string
		host     string
		wantErr  bool
		dialSkip bool // true => behaviour depends on external DNS, skip assertion
	}{
		{"localhost rejected before DNS", "localhost", true, false},
		{"IPv4 loopback rejected", "127.0.0.1", true, false},
		{"cloud metadata rejected", "169.254.169.254", true, false},
		{"IPv6 loopback rejected", "::1", true, false},
		{"private 10/8 rejected", "10.0.0.1", true, false},
		{"private 172.16/12 rejected", "172.16.0.1", true, false},
		{"private 192.168/16 rejected", "192.168.1.1", true, false},
		{"unspecified rejected", "0.0.0.0", true, false},
		{"public IPv4 allowed (literal, no DNS)", "8.8.8.8", false, false},
		{"public IPv6 allowed (literal, no DNS)", "2606:4700:4700::1111", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := ssrf.RejectUnsafeWebdavDialHost(ctx, tt.host, false)
			if tt.dialSkip {
				t.Logf("ssrf.RejectUnsafeWebdavDialHost(%q, false) err=%v (DNS-dependent, not asserted)", tt.host, err)
				return
			}
			if tt.wantErr && err == nil {
				t.Fatalf("ssrf.RejectUnsafeWebdavDialHost(%q, false) = nil, want error", tt.host)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ssrf.RejectUnsafeWebdavDialHost(%q, false) = %v, want nil", tt.host, err)
			}
		})
	}
}

// TestRejectUnsafeWebdavDialHost_PublicHostname covers a public DNS hostname.
// This requires live DNS resolution; if the resolver is unavailable the test
// is skipped rather than failing, so it remains robust in offline CI.
func TestRejectUnsafeWebdavDialHost_PublicHostname(t *testing.T) {
	disallowPrivateWebdavTargetsForTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := ssrf.RejectUnsafeWebdavDialHost(ctx, "example.com", false)
	if err == nil {
		// DNS resolved to one or more public addresses — allowed as expected.
		return
	}
	// A DNS lookup failure means the environment has no resolver; skip rather
	// than fail. Any other error (resolved to a private address) is a real bug.
	if isDNSLookupError(err) {
		t.Skipf("skipping: no DNS resolver available: %v", err)
	}
	t.Fatalf("ssrf.RejectUnsafeWebdavDialHost(example.com, false) = %v, want nil", err)
}

// isDNSLookupError heuristically detects resolver-unavailable errors so the
// public-hostname dial test can skip in offline environments.
func isDNSLookupError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	contains := func(sub string) bool {
		return len(msg) >= len(sub) && stringContains(msg, sub)
	}
	return contains("no such host") || contains("lookup") || contains("no IP addresses") || contains("server misbehaving")
}

// stringContains is a tiny strings.Contains shim to avoid importing strings
// just for the heuristic above.
func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestIsValidWebdavFileURL_SSRFGuard ensures the URL-validation layer rejects
// private/loopback targets when the allowPrivateWebdavTargets flag is false,
// and that the guard composes correctly with the ssrf package functions.
func TestIsValidWebdavFileURL_SSRFGuard(t *testing.T) {
	disallowPrivateWebdavTargetsForTest(t)

	tests := []struct {
		name   string
		rawURL string
		wantOK bool
	}{
		{"https localhost rejected", "https://localhost/path", false},
		{"https 127.0.0.1 rejected", "https://127.0.0.1/path", false},
		{"https metadata rejected", "https://169.254.169.254/latest", false},
		{"https private 10/8 rejected", "https://10.0.0.1/path", false},
		{"https public IP allowed", "https://8.8.8.8/path", true},
		{"https public host allowed", "https://example.com/path", true},
		{"http public host allowed", "http://example.com/path", true},
		{"ftp scheme rejected", "ftp://example.com/file", false},
		{"empty host rejected", "https:///path", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidWebdavFileURL(tt.rawURL); got != tt.wantOK {
				t.Fatalf("isValidWebdavFileURL(%q) = %v, want %v", tt.rawURL, got, tt.wantOK)
			}
		})
	}
}
