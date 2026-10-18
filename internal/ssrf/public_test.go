package ssrf

import (
	"context"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func TestIsUnsafePublicAddr(t *testing.T) {
	unsafe := []string{
		"0.0.0.0",
		"0.1.2.3", // 0.0.0.0/8 non-zero address (audit L11)
		"127.0.0.1",
		"::1",
		"10.0.0.5",
		"172.16.0.5",
		"192.168.1.5",
		"fd12:3456:789a::1", // ULA, covers the GCP metadata endpoint
		"fd20:ce::254",      // GCP IMDS IPv6 (audit M9)
		"169.254.169.254",
		"fe80::1",
		"224.0.0.1",
		"ff02::1",
	}
	for _, raw := range unsafe {
		if !IsUnsafePublicAddr(netip.MustParseAddr(raw)) {
			t.Errorf("IsUnsafePublicAddr(%s) = false, want true", raw)
		}
	}

	safe := []string{"8.8.8.8", "203.0.113.10", "2606:4700:4700::1111"}
	for _, raw := range safe {
		if IsUnsafePublicAddr(netip.MustParseAddr(raw)) {
			t.Errorf("IsUnsafePublicAddr(%s) = true, want false", raw)
		}
	}
}

func TestIsForbiddenPublicHost(t *testing.T) {
	for _, host := range []string{
		"localhost", "localhost.", "app.localhost", "127.0.0.1", "[::1]",
		"0.1.2.3", "10.0.0.1", "[fd20:ce::254]", "fe80::1%eth0",
	} {
		if !IsForbiddenPublicHost(host) {
			t.Errorf("IsForbiddenPublicHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"api.telegram.org", "8.8.8.8", "example.com"} {
		if IsForbiddenPublicHost(host) {
			t.Errorf("IsForbiddenPublicHost(%q) = true, want false", host)
		}
	}
}

func TestNewPublicOnlyDialContext_RejectsPrivateLiteralBeforeDial(t *testing.T) {
	dialer := &recordingDialer{}
	dial := NewPublicOnlyDialContext(&fakeNetIPResolver{answers: map[string][]netip.Addr{}}, dialer.DialContext)

	for _, address := range []string{"127.0.0.1:443", "[fd20:ce::254]:80", "0.1.2.3:25"} {
		if _, err := dial(context.Background(), "tcp", address); err == nil ||
			!strings.Contains(err.Error(), "forbidden host") {
			t.Errorf("dial(%s) error = %v, want forbidden host", address, err)
		}
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("underlying dialer called with %v", dialer.addresses)
	}
}

func TestNewPublicOnlyDialContext_PinsPublicAnswerAndRejectsPrivateAlias(t *testing.T) {
	resolver := &fakeNetIPResolver{answers: map[string][]netip.Addr{
		"public.example":  {netip.MustParseAddr("203.0.113.10")},
		"private.example": {netip.MustParseAddr("10.20.30.40")},
	}}
	dialer := &recordingDialer{}
	dial := NewPublicOnlyDialContext(resolver, dialer.DialContext)

	conn, err := dial(context.Background(), "tcp", "public.example:443")
	if err != nil {
		t.Fatalf("public dial: %v", err)
	}
	_ = conn.Close()
	if !slices.Equal(dialer.addresses, []string{"203.0.113.10:443"}) {
		t.Fatalf("dialed addresses = %v, want the checked public IP", dialer.addresses)
	}

	_, err = dial(context.Background(), "tcp", "private.example:443")
	if err == nil || !strings.Contains(err.Error(), "forbidden resolved address") {
		t.Fatalf("private alias error = %v, want forbidden resolved address", err)
	}
}
