package httpclient

import (
	"net/http"
	"net/url"
	"testing"
)

func TestNewTransportProxySelection(t *testing.T) {
	t.Run("nil keeps environment resolution", func(t *testing.T) {
		tr := NewTransport(Options{})
		if tr.Proxy == nil {
			t.Fatal("Options.Proxy nil must keep http.ProxyFromEnvironment, got nil Transport.Proxy")
		}
	})

	t.Run("NoProxy ignores environment proxies", func(t *testing.T) {
		t.Setenv("HTTP_PROXY", "http://proxy.invalid:3128")
		t.Setenv("HTTPS_PROXY", "http://proxy.invalid:3128")
		tr := NewTransport(Options{Proxy: NoProxy})
		req, err := http.NewRequest(http.MethodGet, "http://example.invalid", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		u, err := tr.Proxy(req)
		if err != nil {
			t.Fatalf("NoProxy returned error: %v", err)
		}
		if u != nil {
			t.Fatalf("NoProxy resolved %v despite environment proxies", u)
		}
	})

	t.Run("explicit proxy func is wired through", func(t *testing.T) {
		fixed, err := url.Parse("http://proxy.invalid:3128")
		if err != nil {
			t.Fatalf("parse fixed proxy URL: %v", err)
		}
		tr := NewTransport(Options{Proxy: http.ProxyURL(fixed)})
		req, err := http.NewRequest(http.MethodGet, "http://example.invalid", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		u, err := tr.Proxy(req)
		if err != nil {
			t.Fatalf("proxy func returned error: %v", err)
		}
		if u == nil || u.String() != fixed.String() {
			t.Fatalf("proxy func = %v, want %v", u, fixed)
		}
	})
}
