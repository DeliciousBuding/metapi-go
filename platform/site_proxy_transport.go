package platform

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

const (
	// Keep short: multi-candidate user-id probes must fail fast on dead hosts.
	defaultProxyConnectTimeout   = 2 * time.Second
	defaultProxyKeepAliveInitial = 60 * time.Second

	// Historical outbound timeout defaults. #1009 made them configurable via
	// the PROXY_*_TIMEOUT_SEC env vars; effectiveProxyTimeouts resolves the
	// live values, falling back to these exact constants so unset deployments
	// keep the pre-#1009 behavior.
	defaultProxyTLSHandshakeTimeout   = 10 * time.Second
	defaultProxyResponseHeaderTimeout = 30 * time.Second
	defaultProxyIdleConnTimeout       = 90 * time.Second
	defaultProxyRequestTimeout        = 30 * time.Second
)

// transportCache pools *http.Transport instances by proxy, TLS and stream mode
// so repeated DoWithProxy / SiteProxy.Do calls to the same proxy (or direct)
// reuse the underlying keep-alive connection pool instead of forcing a fresh
// TLS handshake + dial on every request. Probe loops and stream traffic are
// the main beneficiaries: handshake amplification and fingerprint exposure at
// shield-protected upstreams drop to one-per-(proxy,tls)-tuple per process.
//
// Entries live for the process lifetime; pooled transports hold idle
// connections bounded by MaxIdleConns / MaxIdleConnsPerHost / IdleConnTimeout
// (see newPooledTransport). The proxy func captured by a cached transport must
// be effectively constant with respect to the request (http.ProxyURL,
// SiteProxy.proxyFunc, or nil) so the cache key stays stable.
var transportCache sync.Map // map[string]*http.Transport

// getCachedTransport returns a pooled *http.Transport for the given
// (proxy, insecureSkipTLS, useUTLS) tuple, creating and storing it on first
// miss. Concurrency-safe: concurrent first-miss callers race via
// LoadOrStore; the loser's transport is unreferenced and garbage-collected
// with no connections.
//
// When useUTLS is true the transport is built by newUTLSTransport (Chrome
// ClientHello fingerprint masking via uTLS); otherwise the standard
// newPooledTransport is used. The two variants occupy separate cache keys so
// they never alias — the uTLS DialTLSContext is only paid for by sites that
// opt in.
func getCachedTransport(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS, useUTLS bool) *http.Transport {
	return getCachedTransportForMode(proxy, insecureSkipTLS, useUTLS, false)
}

// Stream mode gets its own stable pool: its header phase belongs to
// DoWithHeaderBudget. Never mutate a transport already used by buffered calls.
func getCachedTransportForMode(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS, useUTLS, stream bool) *http.Transport {
	key := transportCacheKey(proxy, insecureSkipTLS, useUTLS)
	if stream {
		key += "|stream"
	}
	if cached, ok := transportCache.Load(key); ok {
		return cached.(*http.Transport)
	}
	var transport *http.Transport
	if useUTLS {
		transport = newUTLSTransport(proxy, insecureSkipTLS)
	} else {
		transport = newPooledTransport(proxy, insecureSkipTLS)
	}
	if stream {
		transport.ResponseHeaderTimeout = 0
	}
	actual, _ := transportCache.LoadOrStore(key, transport)
	return actual.(*http.Transport)
}

// transportCacheKey derives the cache key from the proxy func by resolving it
// against a sentinel request. proxy may be nil (direct connection), in which
// case the proxy portion of the key is the empty string. When useUTLS is true
// a "|utls" suffix is appended so the uTLS-backed transport never aliases
// with the standard crypto/tls transport for the same (proxy, insecure) tuple.
func transportCacheKey(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS, useUTLS bool) string {
	proxyURL := ""
	if proxy != nil {
		sentinel := &http.Request{URL: &url.URL{Scheme: "http", Host: "transport-cache.sentinel.invalid"}}
		if resolved, err := proxy(sentinel); err == nil && resolved != nil {
			proxyURL = resolved.String()
		}
	}
	insecure := "0"
	if insecureSkipTLS {
		insecure = "1"
	}
	key := proxyURL + "|" + insecure
	if useUTLS {
		key += "|utls"
	}
	return key
}

// proxyTimeoutSet holds the effective outbound proxy / upstream HTTP timeout
// set (#1009).
type proxyTimeoutSet struct {
	connect        time.Duration // TCP dial (connect)
	tlsHandshake   time.Duration // TLS handshake
	responseHeader time.Duration // wait for upstream response headers
	idleConn       time.Duration // idle keep-alive connection TTL
	request        time.Duration // whole-request http.Client timeout
}

// effectiveProxyTimeouts resolves the outbound timeout set from the loaded
// config (PROXY_*_TIMEOUT_SEC env vars). When no config is loaded (unit
// tests) or a field is non-positive, the matching historical default applies,
// so unset deployments keep the exact pre-#1009 behavior. Config is frozen at
// startup, so callers resolve once per transport/client construction; cached
// transports therefore never see a mid-process timeout change.
func effectiveProxyTimeouts() proxyTimeoutSet {
	set := proxyTimeoutSet{
		connect:        defaultProxyConnectTimeout,
		tlsHandshake:   defaultProxyTLSHandshakeTimeout,
		responseHeader: defaultProxyResponseHeaderTimeout,
		idleConn:       defaultProxyIdleConnTimeout,
		request:        defaultProxyRequestTimeout,
	}
	cfg := config.GetSafe()
	if cfg == nil {
		return set
	}
	if cfg.ProxyConnectTimeoutSec > 0 {
		set.connect = time.Duration(cfg.ProxyConnectTimeoutSec) * time.Second
	}
	if cfg.ProxyTLSHandshakeTimeoutSec > 0 {
		set.tlsHandshake = time.Duration(cfg.ProxyTLSHandshakeTimeoutSec) * time.Second
	}
	if cfg.ProxyResponseHeaderTimeoutSec > 0 {
		set.responseHeader = time.Duration(cfg.ProxyResponseHeaderTimeoutSec) * time.Second
	}
	if cfg.ProxyIdleConnTimeoutSec > 0 {
		set.idleConn = time.Duration(cfg.ProxyIdleConnTimeoutSec) * time.Second
	}
	if cfg.ProxyRequestTimeoutSec > 0 {
		set.request = time.Duration(cfg.ProxyRequestTimeoutSec) * time.Second
	}
	return set
}

// newPooledTransport builds a fresh *http.Transport configured for connection
// reuse. Called only on cache misses inside getCachedTransport.
func newPooledTransport(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS bool) *http.Transport {
	timeouts := effectiveProxyTimeouts()
	transport := &http.Transport{
		Proxy:                 proxy,
		DialContext:           newSiteDialContext(),
		TLSHandshakeTimeout:   timeouts.tlsHandshake,
		ResponseHeaderTimeout: timeouts.responseHeader,
		ExpectContinueTimeout: 1 * time.Second,
		// Pool sizing: keep idle sockets warm so repeated probes/streams to the
		// same upstream reuse the same TCP+TLS pair instead of re-dialing.
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     timeouts.idleConn,
	}
	if insecureSkipTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return transport
}

// newProxyClient wraps a (pooled) *http.Transport with the shared timeout and
// cross-origin redirect policy used by outbound site-proxy traffic. The
// *http.Client is cheap to construct per request; only the Transport is pooled.
// The whole-request timeout is configurable via PROXY_REQUEST_TIMEOUT_SEC
// (#1009).
func newProxyClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport:     transport,
		Timeout:       effectiveProxyTimeouts().request,
		CheckRedirect: RejectCrossOriginRedirect,
	}
}

// newStreamProxyClient wraps a (pooled) *http.Transport for SSE stream
// dispatch: identical to newProxyClient but without the whole-request
// timeout, so PROXY_REQUEST_TIMEOUT_SEC does not cap a healthy stream's
// total duration while chunks keep flowing. Dispatch must use DoWithHeaderBudget
// with a stream-mode transport; the relay owns body-idle liveness.
func newStreamProxyClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport:     transport,
		CheckRedirect: RejectCrossOriginRedirect,
	}
}
