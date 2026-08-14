package oauth

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadOAuthJSONResponseBodyAllowsLimit(t *testing.T) {
	body := strings.Repeat("x", oauthJSONResponseBodyLimit)

	got, err := readOAuthJSONResponseBody(strings.NewReader(body))
	if err != nil {
		t.Fatalf("readOAuthJSONResponseBody: %v", err)
	}
	if len(got) != oauthJSONResponseBodyLimit {
		t.Fatalf("len = %d, want %d", len(got), oauthJSONResponseBodyLimit)
	}
}

func TestReadOAuthJSONResponseBodyRejectsOversized(t *testing.T) {
	body := strings.Repeat("x", oauthJSONResponseBodyLimit+1)

	_, err := readOAuthJSONResponseBody(strings.NewReader(body))
	if err == nil {
		t.Fatal("readOAuthJSONResponseBody succeeded for oversized response")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want exceeds", err)
	}
}

func TestReadOAuthErrorResponseBodyCapsOutput(t *testing.T) {
	body := bytes.Repeat([]byte("e"), oauthErrorResponseBodyLimit+1024)

	got := readOAuthErrorResponseBody(bytes.NewReader(body))
	if len(got) != oauthErrorResponseBodyLimit {
		t.Fatalf("len = %d, want %d", len(got), oauthErrorResponseBodyLimit)
	}
	if !bytes.Equal(got, bytes.Repeat([]byte("e"), oauthErrorResponseBodyLimit)) {
		t.Fatal("error response body was not capped to the leading bytes")
	}
}

func TestDoHTTPRejectsInvalidProxyURLWithoutRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	proxyURL := "://bad"

	_, err = doHTTP(req, &proxyURL, nil)
	if err == nil {
		t.Fatal("doHTTP succeeded, want invalid proxy URL error")
	}
	if called {
		t.Fatal("request was sent despite invalid proxy URL")
	}
}

func TestDoHTTPIgnoresEnvironmentProxyWithoutProxyURL(t *testing.T) {
	proxyCalled := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.invalid/oauth", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	_, _ = doHTTP(req, nil, nil)

	if proxyCalled {
		t.Fatal("doHTTP without proxy URL used HTTP_PROXY from environment")
	}
}

func TestOAuthHTTPClientRejectsCrossOriginRedirect(t *testing.T) {
	targetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/landing", http.StatusFound)
	}))
	t.Cleanup(source.Close)

	req, err := http.NewRequest(http.MethodGet, source.URL+"/oauth/token", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := doHTTP(req, nil, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("cross-origin redirect was allowed")
	}
	if !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("error = %v, want cross-origin", err)
	}
	if targetCalled {
		t.Fatal("cross-origin redirect target was called")
	}
}

func TestNewOAuthHTTPClientWiresCheckRedirect(t *testing.T) {
	client := newOAuthHTTPClient(nil)
	if client.CheckRedirect == nil {
		t.Fatal("newOAuthHTTPClient must set CheckRedirect")
	}
}

// ---- readOAuthErrorResponseBody error branch ----

// erroringReader is an io.Reader that always returns an error on Read.
type erroringReader struct{}

func (erroringReader) Read(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestReadOAuthErrorResponseBody_ReturnsErrorTextOnReadFailure(t *testing.T) {
	got := readOAuthErrorResponseBody(erroringReader{})
	if len(got) == 0 {
		t.Fatal("erroring reader should yield non-empty error text")
	}
	if !bytes.Contains(got, []byte("unexpected")) {
		t.Errorf("expected error text to mention the read failure, got %q", string(got))
	}
}

// ---- doHTTP with a valid proxy URL ----

func TestDoHTTPWithValidProxyURLRoutesThroughProxy(t *testing.T) {
	// Set up a proxy server that records the request. The doHTTP function
	// wires http.ProxyURL into the transport; for httptest the proxy handler
	// just needs to respond so the request succeeds.
	proxyHit := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(proxy.Close)

	req, err := http.NewRequest(http.MethodGet, "http://example.invalid/oauth/token", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	proxyURL := proxy.URL
	resp, err := doHTTP(req, &proxyURL, nil)
	if err != nil {
		t.Fatalf("doHTTP with proxy: %v", err)
	}
	defer resp.Body.Close()
	if !proxyHit {
		t.Error("request should have gone through the proxy")
	}
}

func TestDoHTTPRejectsProxyURLMissingSchemeOrHost(t *testing.T) {
	// A proxy URL like "localhost:8080" (no scheme) should be rejected before
	// any request is sent.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	proxyURL := "localhost:8080"
	_, err = doHTTP(req, &proxyURL, nil)
	if err == nil {
		t.Fatal("expected error for proxy URL missing scheme/host")
	}
	if !strings.Contains(err.Error(), "missing scheme or host") {
		t.Errorf("error = %v, want 'missing scheme or host'", err)
	}
}
