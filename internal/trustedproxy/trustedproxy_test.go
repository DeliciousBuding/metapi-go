package trustedproxy

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustMarker(t *testing.T) {
	ctx := context.Background()
	if FromRequest(ctx) {
		t.Fatal("bare context must not be trusted")
	}
	if !FromRequest(WithTrust(ctx)) {
		t.Fatal("WithTrust context must be trusted")
	}
}

func TestIsHTTPS(t *testing.T) {
	plain := httptest.NewRequest(http.MethodGet, "/", nil)
	plain.Header.Set("X-Forwarded-Proto", "https")
	if IsHTTPS(plain) {
		t.Fatal("direct client X-Forwarded-Proto must be ignored")
	}
	if !IsHTTPS(plain.WithContext(WithTrust(plain.Context()))) {
		t.Fatal("trusted X-Forwarded-Proto:https must be honored")
	}

	httpReq := httptest.NewRequest(http.MethodGet, "/", nil)
	httpReq.Header.Set("X-Forwarded-Proto", "http")
	httpReq = httpReq.WithContext(WithTrust(httpReq.Context()))
	if IsHTTPS(httpReq) {
		t.Fatal("trusted X-Forwarded-Proto:http must not report HTTPS")
	}

	tlsReq := httptest.NewRequest(http.MethodGet, "/", nil)
	tlsReq.TLS = &tls.ConnectionState{}
	if !IsHTTPS(tlsReq) {
		t.Fatal("direct TLS must report HTTPS")
	}
}

func TestForwardedHostOnlyFromTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Host", "gw.example.com")
	if got := ForwardedHost(req); got != "" {
		t.Fatalf("direct client ForwardedHost = %q, want empty", got)
	}
	req = req.WithContext(WithTrust(req.Context()))
	if got := ForwardedHost(req); got != "gw.example.com" {
		t.Fatalf("trusted ForwardedHost = %q, want gw.example.com", got)
	}
}

func TestValidateHost(t *testing.T) {
	valid := map[string]string{
		"example.com":       "example.com",
		"example.com:8443":  "example.com:8443",
		"[2001:db8::1]:443": "[2001:db8::1]:443",
		" api.example.com ": "api.example.com",
	}
	for raw, want := range valid {
		if got := ValidateHost(raw); got != want {
			t.Errorf("ValidateHost(%q) = %q, want %q", raw, got, want)
		}
	}

	for _, raw := range []string{
		"", "user@example.com", "example.com/path", "example.com?q=1",
		"example.com#frag", "a.example,b.example", "exa mple.com",
	} {
		if got := ValidateHost(raw); got != "" {
			t.Errorf("ValidateHost(%q) = %q, want empty", raw, got)
		}
	}
}
