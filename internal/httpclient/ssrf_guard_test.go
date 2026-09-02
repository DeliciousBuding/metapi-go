package httpclient

import (
	"net/http"
	"net/http/httptest"
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
