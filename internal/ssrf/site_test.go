package ssrf

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

type fakeNetIPResolver struct {
	answers map[string][]netip.Addr
	err     error
	lookups []string
}

func (r *fakeNetIPResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	r.lookups = append(r.lookups, host)
	if r.err != nil {
		return nil, r.err
	}
	return slices.Clone(r.answers[host]), nil
}

type recordingDialer struct {
	addresses []string
	err       error
}

func (d *recordingDialer) DialContext(_ context.Context, _ string, address string) (net.Conn, error) {
	d.addresses = append(d.addresses, address)
	if d.err != nil {
		return nil, d.err
	}
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func TestIsForbiddenSiteAddr_PreservesOperatorPrivateNetworks(t *testing.T) {
	allowed := []string{
		"127.0.0.1",
		"::1",
		"10.0.0.8",
		"172.16.0.9",
		"192.168.1.10",
		"fd12:3456:789a::1",
		"8.8.8.8",
	}
	for _, raw := range allowed {
		if IsForbiddenSiteAddr(netip.MustParseAddr(raw)) {
			t.Errorf("IsForbiddenSiteAddr(%s) = true, want false", raw)
		}
	}

	forbidden := []string{
		"0.0.0.0",
		"0.1.2.3",
		"169.254.169.254",
		"169.254.10.20",
		"100.100.100.200",
		"224.0.0.1",
		"255.255.255.255",
		"fe80::1",
		"ff02::1",
		"fd00:ec2::254",
		"::ffff:169.254.169.254",
	}
	for _, raw := range forbidden {
		if !IsForbiddenSiteAddr(netip.MustParseAddr(raw)) {
			t.Errorf("IsForbiddenSiteAddr(%s) = false, want true", raw)
		}
	}
}

func TestNewSiteDialContext_RejectsKnownMetadataHostnameBeforeDNS(t *testing.T) {
	resolver := &fakeNetIPResolver{answers: map[string][]netip.Addr{}}
	dialer := &recordingDialer{}
	dial := NewSiteDialContext(resolver, dialer.DialContext)

	_, err := dial(context.Background(), "tcp", "metadata.google.internal:80")
	if err == nil || !strings.Contains(err.Error(), "forbidden host") {
		t.Fatalf("error = %v, want forbidden host", err)
	}
	if len(resolver.lookups) != 0 {
		t.Fatalf("known metadata hostname unexpectedly resolved: %v", resolver.lookups)
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("underlying dialer called with %v", dialer.addresses)
	}
}

func TestNewSiteDialContext_RejectsDNSAliasToForbiddenAddress(t *testing.T) {
	resolver := &fakeNetIPResolver{answers: map[string][]netip.Addr{
		"169.254.169.254.nip.io": {netip.MustParseAddr("169.254.169.254")},
	}}
	dialer := &recordingDialer{}
	dial := NewSiteDialContext(resolver, dialer.DialContext)

	_, err := dial(context.Background(), "tcp", "169.254.169.254.nip.io:80")
	if err == nil || !strings.Contains(err.Error(), "forbidden resolved address") {
		t.Fatalf("error = %v, want forbidden resolved address", err)
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("underlying dialer called with %v", dialer.addresses)
	}
}

func TestNewSiteDialContext_RejectsIPv6LinkLocalAnswer(t *testing.T) {
	resolver := &fakeNetIPResolver{answers: map[string][]netip.Addr{
		"linklocal.example": {netip.MustParseAddr("fe80::1234")},
	}}
	dialer := &recordingDialer{}
	dial := NewSiteDialContext(resolver, dialer.DialContext)

	_, err := dial(context.Background(), "tcp", "linklocal.example:443")
	if err == nil || !strings.Contains(err.Error(), "fe80::1234") {
		t.Fatalf("error = %v, want IPv6 link-local rejection", err)
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("underlying dialer called with %v", dialer.addresses)
	}
}

func TestNewSiteDialContext_PinsValidatedDNSAnswer(t *testing.T) {
	resolver := &fakeNetIPResolver{answers: map[string][]netip.Addr{
		"private-upstream.example": {netip.MustParseAddr("10.20.30.40")},
	}}
	dialer := &recordingDialer{}
	dial := NewSiteDialContext(resolver, dialer.DialContext)

	conn, err := dial(context.Background(), "tcp", "private-upstream.example:8443")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
	if !slices.Equal(resolver.lookups, []string{"private-upstream.example"}) {
		t.Fatalf("resolver lookups = %v", resolver.lookups)
	}
	if !slices.Equal(dialer.addresses, []string{"10.20.30.40:8443"}) {
		t.Fatalf("dialed addresses = %v, want checked IP", dialer.addresses)
	}
}

func TestNewSiteDialContext_AllowsLoopbackLiteralWithoutDNS(t *testing.T) {
	resolver := &fakeNetIPResolver{answers: map[string][]netip.Addr{}}
	dialer := &recordingDialer{}
	dial := NewSiteDialContext(resolver, dialer.DialContext)

	conn, err := dial(context.Background(), "tcp", "127.0.0.1:39831")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
	if len(resolver.lookups) != 0 {
		t.Fatalf("literal IP unexpectedly resolved: %v", resolver.lookups)
	}
	if !slices.Equal(dialer.addresses, []string{"127.0.0.1:39831"}) {
		t.Fatalf("dialed addresses = %v", dialer.addresses)
	}
}

func TestNewSiteDialContext_RejectsMixedSafeAndForbiddenAnswers(t *testing.T) {
	resolver := &fakeNetIPResolver{answers: map[string][]netip.Addr{
		"rebinding.example": {
			netip.MustParseAddr("203.0.113.10"),
			netip.MustParseAddr("169.254.169.254"),
		},
	}}
	dialer := &recordingDialer{err: errors.New("should not dial")}
	dial := NewSiteDialContext(resolver, dialer.DialContext)

	_, err := dial(context.Background(), "tcp", "rebinding.example:80")
	if err == nil {
		t.Fatal("mixed safe/forbidden DNS answer was accepted")
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("underlying dialer called with %v", dialer.addresses)
	}
}
