package platform

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// setProxyTimeoutConfig swaps the global config singleton for cfg and
// restores the unloaded (nil) state when the test finishes so no other
// platform test observes the mutation. Tests that touch the global config
// must not use t.Parallel.
func setProxyTimeoutConfig(t *testing.T, cfg *config.Config) {
	t.Helper()
	config.Set(cfg)
	t.Cleanup(func() { config.Set(nil) })
}

func TestEffectiveProxyTimeoutsDefaultsWithoutConfig(t *testing.T) {
	config.Set(nil)
	got := effectiveProxyTimeouts()
	want := proxyTimeoutSet{
		connect:        2 * time.Second,
		tlsHandshake:   10 * time.Second,
		responseHeader: 30 * time.Second,
		idleConn:       90 * time.Second,
		request:        30 * time.Second,
	}
	if got != want {
		t.Fatalf("effectiveProxyTimeouts() without config = %+v, want historical defaults %+v", got, want)
	}
}

func TestEffectiveProxyTimeoutsFromConfig(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{
		ProxyConnectTimeoutSec:        7,
		ProxyTLSHandshakeTimeoutSec:   11,
		ProxyResponseHeaderTimeoutSec: 45,
		ProxyIdleConnTimeoutSec:       120,
		ProxyRequestTimeoutSec:        60,
	})
	got := effectiveProxyTimeouts()
	want := proxyTimeoutSet{
		connect:        7 * time.Second,
		tlsHandshake:   11 * time.Second,
		responseHeader: 45 * time.Second,
		idleConn:       120 * time.Second,
		request:        60 * time.Second,
	}
	if got != want {
		t.Fatalf("effectiveProxyTimeouts() = %+v, want %+v", got, want)
	}
}

func TestEffectiveProxyTimeoutsZeroValueConfigKeepsDefaults(t *testing.T) {
	// A hand-constructed zero-value Config (e.g. from other packages' tests)
	// must behave exactly like no config at all — every field non-positive
	// falls back to the historical default.
	setProxyTimeoutConfig(t, &config.Config{})
	got := effectiveProxyTimeouts()
	if got.connect != defaultProxyConnectTimeout ||
		got.tlsHandshake != defaultProxyTLSHandshakeTimeout ||
		got.responseHeader != defaultProxyResponseHeaderTimeout ||
		got.idleConn != defaultProxyIdleConnTimeout ||
		got.request != defaultProxyRequestTimeout {
		t.Fatalf("effectiveProxyTimeouts() with zero-value config = %+v, want all historical defaults", got)
	}
}

func TestNewSiteDialerUsesConfiguredConnectTimeout(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{ProxyConnectTimeoutSec: 7})
	if got := newSiteDialer().Timeout; got != 7*time.Second {
		t.Fatalf("newSiteDialer().Timeout = %v, want 7s", got)
	}
}

func TestNewPooledTransportTimeoutsWithoutConfig(t *testing.T) {
	config.Set(nil)
	tr := newPooledTransport(nil, false)
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %v, want historical default 10s", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 30*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want historical default 30s", tr.ResponseHeaderTimeout)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Fatalf("IdleConnTimeout = %v, want historical default 90s", tr.IdleConnTimeout)
	}
	if client := newProxyClient(tr); client.Timeout != 30*time.Second {
		t.Fatalf("newProxyClient Timeout = %v, want historical default 30s", client.Timeout)
	}
}

func TestNewPooledTransportUsesConfiguredTimeouts(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{
		ProxyTLSHandshakeTimeoutSec:   11,
		ProxyResponseHeaderTimeoutSec: 45,
		ProxyIdleConnTimeoutSec:       120,
	})
	tr := newPooledTransport(nil, false)
	if tr.TLSHandshakeTimeout != 11*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %v, want 11s", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 45*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want 45s", tr.ResponseHeaderTimeout)
	}
	if tr.IdleConnTimeout != 120*time.Second {
		t.Fatalf("IdleConnTimeout = %v, want 120s", tr.IdleConnTimeout)
	}
}

func TestNewUTLSTransportUsesConfiguredTimeouts(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{
		ProxyTLSHandshakeTimeoutSec:   11,
		ProxyResponseHeaderTimeoutSec: 45,
		ProxyIdleConnTimeoutSec:       120,
	})
	tr := newUTLSTransport(nil, false)
	if tr.DialTLSContext == nil {
		t.Fatal("newUTLSTransport lost its DialTLSContext override")
	}
	if tr.TLSHandshakeTimeout != 11*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %v, want 11s", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 45*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want 45s", tr.ResponseHeaderTimeout)
	}
	if tr.IdleConnTimeout != 120*time.Second {
		t.Fatalf("IdleConnTimeout = %v, want 120s", tr.IdleConnTimeout)
	}
}

func TestNewProxyClientUsesConfiguredRequestTimeout(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{ProxyRequestTimeoutSec: 60})
	client := newProxyClient(newPooledTransport(nil, false))
	if client.Timeout != 60*time.Second {
		t.Fatalf("newProxyClient Timeout = %v, want 60s", client.Timeout)
	}
}

func TestSiteProxyClientsUseConfiguredTimeouts(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{
		ProxyTLSHandshakeTimeoutSec:   11,
		ProxyResponseHeaderTimeoutSec: 45,
		ProxyRequestTimeoutSec:        60,
	})
	sp := NewSiteProxy("")
	for name, client := range map[string]*http.Client{
		"httpClient":      sp.httpClient,
		"httpClientNoTLS": sp.httpClientNoTLS,
	} {
		if client.Timeout != 60*time.Second {
			t.Fatalf("%s.Timeout = %v, want 60s", name, client.Timeout)
		}
		tr, ok := client.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("%s.Transport is %T, want *http.Transport", name, client.Transport)
		}
		if tr.TLSHandshakeTimeout != 11*time.Second {
			t.Fatalf("%s TLSHandshakeTimeout = %v, want 11s", name, tr.TLSHandshakeTimeout)
		}
		if tr.ResponseHeaderTimeout != 45*time.Second {
			t.Fatalf("%s ResponseHeaderTimeout = %v, want 45s", name, tr.ResponseHeaderTimeout)
		}
	}
}

// TestPooledTransportResponseHeaderTimeoutEndToEnd proves the configured
// ResponseHeaderTimeout is really wired into a live request: the upstream
// stalls past the 1s header budget and the request fails fast.
func TestPooledTransportResponseHeaderTimeoutEndToEnd(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{ProxyResponseHeaderTimeoutSec: 1})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second) // longer than the 1s response-header budget
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newProxyClient(newPooledTransport(nil, false))
	start := time.Now()
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected response-header timeout error, got success")
	}
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Fatalf("request took %v; configured 1s ResponseHeaderTimeout not applied", elapsed)
	}
}

// TestNewProxyClientRequestTimeoutEndToEnd proves the configured whole-request
// client timeout fires even after headers arrive: the upstream flushes headers
// immediately, then stalls the body past the 1s request budget. The timeout
// surfaces while reading the response body (http.Client.Timeout covers the
// whole request including the body read).
func TestNewProxyClientRequestTimeoutEndToEnd(t *testing.T) {
	setProxyTimeoutConfig(t, &config.Config{ProxyRequestTimeoutSec: 1})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush() // deliver headers so ResponseHeaderTimeout is satisfied
		}
		time.Sleep(3 * time.Second) // stall the body past the 1s request budget
		_, _ = w.Write([]byte("late body"))
	}))
	defer srv.Close()

	client := newProxyClient(newPooledTransport(nil, false))
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get failed before headers: %v", err)
	}
	defer resp.Body.Close()
	start := time.Now()
	_, err = io.Copy(io.Discard, resp.Body)
	if err == nil {
		t.Fatal("expected client request timeout while reading body, got success")
	}
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Fatalf("body read took %v; configured 1s request timeout not applied", elapsed)
	}
}
