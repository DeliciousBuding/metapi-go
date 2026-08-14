package platform

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"
)

// TestGetCachedTransport_SameKeyReturnsSamePointer verifies the core pooling
// invariant: two lookups with the same (proxy, insecure) tuple must return the
// same *http.Transport pointer, while the insecure variant is distinct.
func TestGetCachedTransport_SameKeyReturnsSamePointer(t *testing.T) {
	proxyURL, err := url.Parse("http://same-key.example:8080")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	proxy := http.ProxyURL(proxyURL)

	first := getCachedTransport(proxy, false, false)
	second := getCachedTransport(proxy, false, false)
	if first != second {
		t.Fatal("same (proxy, insecure) must return the same *http.Transport pointer")
	}

	// The insecure variant maps to a different cache key and must be distinct.
	insecure := getCachedTransport(proxy, true, false)
	if insecure == first {
		t.Fatal("insecure variant should be a distinct transport")
	}
}

// TestGetCachedTransport_DifferentKeysReturnDifferentPointers verifies that
// different proxy URLs do not alias to the same pooled transport.
func TestGetCachedTransport_DifferentKeysReturnDifferentPointers(t *testing.T) {
	proxyA, _ := url.Parse("http://distinct-a.example:8080")
	proxyB, _ := url.Parse("http://distinct-b.example:8080")

	transportA := getCachedTransport(http.ProxyURL(proxyA), false, false)
	transportB := getCachedTransport(http.ProxyURL(proxyB), false, false)
	if transportA == transportB {
		t.Fatal("different proxy URLs must map to different transports")
	}
}

// TestGetCachedTransport_NilProxyDirectConnection covers the direct-connection
// path used by DoWithProxy when no ProxyURL is configured. The Proxy field on a
// direct transport must be nil so environment proxies (HTTP_PROXY) are ignored.
func TestGetCachedTransport_NilProxyDirectConnection(t *testing.T) {
	direct := getCachedTransport(nil, false, false)
	if direct == nil {
		t.Fatal("direct transport must not be nil")
	}
	if direct.Proxy != nil {
		t.Fatal("direct transport must have nil Proxy so env proxies are ignored")
	}
	// Same nil-proxy + insecure combo must be stable across calls.
	directAgain := getCachedTransport(nil, false, false)
	if direct != directAgain {
		t.Fatal("nil proxy must be cached and stable across calls")
	}
}

// TestGetCachedTransport_PoolingSettingsApplied asserts the pooled transport is
// configured for real connection reuse (the whole point of the cache).
func TestGetCachedTransport_PoolingSettingsApplied(t *testing.T) {
	// Unique socks5 URL so this test does not collide with cache entries
	// populated by other tests in the package.
	proxyURL, _ := url.Parse("socks5://pool-settings.example:1080")
	transport := getCachedTransport(http.ProxyURL(proxyURL), false, false)
	if got, want := transport.MaxIdleConns, 100; got != want {
		t.Errorf("MaxIdleConns = %d, want %d", got, want)
	}
	if got, want := transport.MaxIdleConnsPerHost, 20; got != want {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", got, want)
	}
	if got, want := transport.IdleConnTimeout, 90*time.Second; got != want {
		t.Errorf("IdleConnTimeout = %v, want %v", got, want)
	}
}

// TestGetCachedTransport_ConcurrentSameKey confirms the cache is concurrency
// safe: many goroutines racing on the same first-miss key all observe the same
// transport pointer (no duplicate entries, no panics).
func TestGetCachedTransport_ConcurrentSameKey(t *testing.T) {
	proxyURL, _ := url.Parse("http://concurrent.example:8080")
	proxy := http.ProxyURL(proxyURL)

	const goroutines = 64
	pointers := make([]*http.Transport, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			pointers[i] = getCachedTransport(proxy, false, false)
		}()
	}
	close(start)
	wg.Wait()

	baseline := getCachedTransport(proxy, false, false)
	for i, got := range pointers {
		if got != baseline {
			t.Fatalf("goroutine %d got %p, want %p (cache must dedupe under concurrency)", i, got, baseline)
		}
	}
}

// TestTransportCacheKey_StableAndDistinct pins the key format so future
// refactors of the proxy-func introspection do not silently change caching
// behavior (e.g. merging the direct and proxied paths).
func TestTransportCacheKey_StableAndDistinct(t *testing.T) {
	proxyURL, _ := url.Parse("http://key.example:8080")
	proxy := http.ProxyURL(proxyURL)

	if got, want := transportCacheKey(proxy, false, false), "http://key.example:8080|0"; got != want {
		t.Errorf("secure key = %q, want %q", got, want)
	}
	if got, want := transportCacheKey(proxy, true, false), "http://key.example:8080|1"; got != want {
		t.Errorf("insecure key = %q, want %q", got, want)
	}
	if got, want := transportCacheKey(nil, false, false), "|0"; got != want {
		t.Errorf("nil-proxy secure key = %q, want %q", got, want)
	}
	if got, want := transportCacheKey(nil, true, false), "|1"; got != want {
		t.Errorf("nil-proxy insecure key = %q, want %q", got, want)
	}
}

// TestTransportCacheKey_UTLSSuffix verifies that the uTLS variant gets a
// distinct cache key with the "|utls" suffix, so the Chrome-ClientHello
// transport never aliases with the standard crypto/tls transport for the
// same (proxy, insecure) tuple.
func TestTransportCacheKey_UTLSSuffix(t *testing.T) {
	proxyURL, _ := url.Parse("http://utls-key.example:8080")
	proxy := http.ProxyURL(proxyURL)

	standard := transportCacheKey(proxy, false, false)
	utlsKey := transportCacheKey(proxy, false, true)
	if standard == utlsKey {
		t.Fatal("utls key must differ from standard key")
	}
	if got, want := utlsKey, "http://utls-key.example:8080|0|utls"; got != want {
		t.Errorf("utls key = %q, want %q", got, want)
	}

	// Insecure + utls
	insecureUTLS := transportCacheKey(proxy, true, true)
	if got, want := insecureUTLS, "http://utls-key.example:8080|1|utls"; got != want {
		t.Errorf("insecure utls key = %q, want %q", got, want)
	}

	// Nil proxy + utls (direct connection)
	nilUTLS := transportCacheKey(nil, false, true)
	if got, want := nilUTLS, "|0|utls"; got != want {
		t.Errorf("nil-proxy utls key = %q, want %q", got, want)
	}
}

// TestGetCachedTransport_UTLSDistinctFromStandard verifies that the uTLS
// transport is a distinct pointer from the standard transport for the same
// (proxy, insecure) tuple and that it has DialTLSContext set.
func TestGetCachedTransport_UTLSDistinctFromStandard(t *testing.T) {
	proxyURL, _ := url.Parse("http://utls-distinct.example:8080")
	proxy := http.ProxyURL(proxyURL)

	standard := getCachedTransport(proxy, false, false)
	utlsTransport := getCachedTransport(proxy, false, true)
	if standard == utlsTransport {
		t.Fatal("utls transport must be distinct from standard transport")
	}
	if utlsTransport.DialTLSContext == nil {
		t.Fatal("utls transport must have DialTLSContext set")
	}
	if standard.DialTLSContext != nil {
		t.Fatal("standard transport must NOT have DialTLSContext set")
	}
}

// TestNewUTLSTransport_PoolingSettingsApplied verifies the uTLS transport
// inherits the same connection-reuse pool sizing as the standard transport so
// it does not regress on keep-alive behavior.
func TestNewUTLSTransport_PoolingSettingsApplied(t *testing.T) {
	transport := newUTLSTransport(nil, false)
	if transport == nil {
		t.Fatal("newUTLSTransport must not return nil")
	}
	if transport.DialTLSContext == nil {
		t.Fatal("DialTLSContext must be set on uTLS transport")
	}
	if got, want := transport.MaxIdleConns, 100; got != want {
		t.Errorf("MaxIdleConns = %d, want %d", got, want)
	}
	if got, want := transport.MaxIdleConnsPerHost, 20; got != want {
		t.Errorf("MaxIdleConnsPerHost = %d, want %d", got, want)
	}
}

// TestDialUTLSContext_FallbackOnNonTLSServer verifies the fallback path: when
// the uTLS handshake fails (here against a plain TCP server that sends no TLS),
// the function redials and retries with standard Go TLS, which also fails, so
// the caller receives a combined error mentioning the uTLS handshake. The
// important assertion is that no panic occurs and an error is returned.
func TestDialUTLSContext_FallbackOnNonTLSServer(t *testing.T) {
	// A plain TCP listener that immediately accepts and closes — the uTLS
	// handshake will get EOF, triggering the fallback path.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		// Read a byte then close so the handshake gets an unexpected EOF.
		buf := make([]byte, 1)
		_, _ = conn.Read(buf)
		_ = conn.Close()
	}()

	addr := ln.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = dialUTLSContext(ctx, "tcp", addr, true)
	if err == nil {
		t.Fatal("expected error from dialUTLSContext against non-TLS server")
	}
}
