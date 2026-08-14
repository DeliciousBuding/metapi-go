package platform

import (
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

	first := getCachedTransport(proxy, false)
	second := getCachedTransport(proxy, false)
	if first != second {
		t.Fatal("same (proxy, insecure) must return the same *http.Transport pointer")
	}

	// The insecure variant maps to a different cache key and must be distinct.
	insecure := getCachedTransport(proxy, true)
	if insecure == first {
		t.Fatal("insecure variant should be a distinct transport")
	}
}

// TestGetCachedTransport_DifferentKeysReturnDifferentPointers verifies that
// different proxy URLs do not alias to the same pooled transport.
func TestGetCachedTransport_DifferentKeysReturnDifferentPointers(t *testing.T) {
	proxyA, _ := url.Parse("http://distinct-a.example:8080")
	proxyB, _ := url.Parse("http://distinct-b.example:8080")

	transportA := getCachedTransport(http.ProxyURL(proxyA), false)
	transportB := getCachedTransport(http.ProxyURL(proxyB), false)
	if transportA == transportB {
		t.Fatal("different proxy URLs must map to different transports")
	}
}

// TestGetCachedTransport_NilProxyDirectConnection covers the direct-connection
// path used by DoWithProxy when no ProxyURL is configured. The Proxy field on a
// direct transport must be nil so environment proxies (HTTP_PROXY) are ignored.
func TestGetCachedTransport_NilProxyDirectConnection(t *testing.T) {
	direct := getCachedTransport(nil, false)
	if direct == nil {
		t.Fatal("direct transport must not be nil")
	}
	if direct.Proxy != nil {
		t.Fatal("direct transport must have nil Proxy so env proxies are ignored")
	}
	// Same nil-proxy + insecure combo must be stable across calls.
	directAgain := getCachedTransport(nil, false)
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
	transport := getCachedTransport(http.ProxyURL(proxyURL), false)
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
			pointers[i] = getCachedTransport(proxy, false)
		}()
	}
	close(start)
	wg.Wait()

	baseline := getCachedTransport(proxy, false)
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

	if got, want := transportCacheKey(proxy, false), "http://key.example:8080|0"; got != want {
		t.Errorf("secure key = %q, want %q", got, want)
	}
	if got, want := transportCacheKey(proxy, true), "http://key.example:8080|1"; got != want {
		t.Errorf("insecure key = %q, want %q", got, want)
	}
	if got, want := transportCacheKey(nil, false), "|0"; got != want {
		t.Errorf("nil-proxy secure key = %q, want %q", got, want)
	}
	if got, want := transportCacheKey(nil, true), "|1"; got != want {
		t.Errorf("nil-proxy insecure key = %q, want %q", got, want)
	}
}
