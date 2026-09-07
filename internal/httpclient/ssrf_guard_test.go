package httpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewTransport_SiteDialGuardRefusesForbiddenTargets(t *testing.T) {
	client := &http.Client{
		Transport: NewTransport(Options{SiteDialGuard: true, Proxy: NoProxy}),
		Timeout:   3 * time.Second,
	}
	for _, target := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://100.100.100.200/latest/meta-data/",
		"http://metadata/latest/meta-data/",
	} {
		resp, err := client.Get(target)
		if err == nil {
			resp.Body.Close()
			t.Fatalf("%s: expected refusal, got a response", target)
		}
		if !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("%s: expected a forbidden-target error, got %v", target, err)
		}
	}
}

// Loopback and private-range upstreams must keep working: Metapi proxies
// operator-hosted gateways on localhost/RFC1918 by design.
func TestNewTransport_SiteDialGuardAllowsLoopback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: NewTransport(Options{SiteDialGuard: true, Proxy: NoProxy}),
		Timeout:   3 * time.Second,
	}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("loopback upstream must stay reachable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestNewTransport_WithoutSiteDialGuardKeepsPlainDialer(t *testing.T) {
	transport := NewTransport(Options{})
	if transport.DialContext == nil {
		t.Fatal("DialContext must always be set (gate R4)")
	}
}

// Proxy callbacks run before HTTP forwarding or CONNECT. The guard must reject
// the URL host before proxy selection, without resolving proxy-only DNS names.
func TestNewTransport_SiteDialGuardValidatesTargetBeforeProxyResolution(t *testing.T) {
	calls := 0
	transport := NewTransport(Options{
		SiteDialGuard: true,
		Proxy: func(*http.Request) (*url.URL, error) {
			calls++
			return nil, nil
		},
	})
	for _, target := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"https://METADATA.GOOGLE.INTERNAL./computeMetadata/v1/",
		"http://[::ffff:169.254.169.254]/",
	} {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "allowed.test.invalid" // The URL, not the Host header, owns the destination.
		proxyURL, err := transport.Proxy(req)
		if proxyURL != nil || err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("%s: proxy=%v error=%v, want forbidden target", target, proxyURL, err)
		}
	}
	if calls != 0 {
		t.Errorf("proxy resolver called %d times for forbidden targets", calls)
	}
}

func TestNewTransport_SiteDialGuardPreservesProxyDecision(t *testing.T) {
	fixed := &url.URL{Scheme: "http", Host: "proxy.test.invalid:3128"}
	proxyErr := errors.New("proxy selection failed")
	for _, tc := range []struct {
		name string
		url  *url.URL
		err  error
	}{
		{name: "direct"},
		{name: "proxy", url: fixed},
		{name: "error", err: proxyErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://proxy-only.test.invalid/", nil)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			transport := NewTransport(Options{
				SiteDialGuard: true,
				Proxy: func(got *http.Request) (*url.URL, error) {
					calls++
					if got != req {
						t.Error("proxy resolver received a different request")
					}
					return tc.url, tc.err
				},
			})
			got, err := transport.Proxy(req)
			if got != tc.url || !errors.Is(err, tc.err) || calls != 1 {
				t.Errorf("proxy=%v error=%v calls=%d, want %v/%v/1", got, err, calls, tc.url, tc.err)
			}
		})
	}
}
