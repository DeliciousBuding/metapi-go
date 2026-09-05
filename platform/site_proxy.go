package platform

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/ssrf"
	utls "github.com/refraction-networking/utls"
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

var supportedProxySchemes = map[string]bool{
	"http": true, "https": true,
	"socks": true, "socks4": true, "socks4a": true,
	"socks5": true, "socks5h": true,
}

// transportCache pools *http.Transport instances by (proxyURL, insecureSkipTLS)
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
	key := transportCacheKey(proxy, insecureSkipTLS, useUTLS)
	if cached, ok := transportCache.Load(key); ok {
		return cached.(*http.Transport)
	}
	var transport *http.Transport
	if useUTLS {
		transport = newUTLSTransport(proxy, insecureSkipTLS)
	} else {
		transport = newPooledTransport(proxy, insecureSkipTLS)
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

// newSiteDialer builds the shared outbound dialer. The connect timeout is
// configurable via PROXY_CONNECT_TIMEOUT_SEC (#1009); the keep-alive period
// stays fixed.
func newSiteDialer() *net.Dialer {
	return &net.Dialer{
		Timeout:   effectiveProxyTimeouts().connect,
		KeepAlive: defaultProxyKeepAliveInitial,
	}
}

func newSiteDialContext() ssrf.DialContextFunc {
	return ssrf.NewSiteDialContext(net.DefaultResolver, newSiteDialer().DialContext)
}

// newUTLSTransport builds a pooled *http.Transport whose DialTLSContext
// performs a uTLS handshake with HelloChrome_Auto (the latest Chrome
// ClientHello spec), masking the JA3/JA4 fingerprint that Cloudflare and
// similar WAFs use to block Go's default crypto/tls ClientHello.
//
// On uTLS handshake failure the connection falls back to standard Go TLS
// (tls.Client + HandshakeContext) so a transient uTLS incompatibility never
// breaks the request — the operator can opt in knowing it is best-effort.
//
// Note: DialTLSContext is honored for non-proxied HTTPS connections (per the
// net/http contract). When an HTTP/HTTPS proxy is configured the transport
// still dials through it and uses TLSClientConfig for the post-CONNECT TLS
// handshake; uTLS masking applies to direct connections. SOCKS proxies are
// handled at the dial layer so DialTLSContext covers them as well.
func newUTLSTransport(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS bool) *http.Transport {
	transport := newPooledTransport(proxy, insecureSkipTLS)
	transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialUTLSContext(ctx, network, addr, insecureSkipTLS)
	}
	return transport
}

// dialUTLSContext performs the uTLS Chrome-ClientHello handshake with a
// standard-TLS fallback. It dials the raw TCP connection, attempts the uTLS
// handshake, and on failure redials and retries with crypto/tls. Both paths
// honor insecureSkipTLS for sites with self-signed or expired certificates.
func dialUTLSContext(ctx context.Context, network, addr string, insecureSkipTLS bool) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	dialContext := newSiteDialContext()

	rawConn, err := dialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("utls dial: %w", err)
	}

	// uTLS handshake with Chrome ClientHello spec.
	uConn := utls.UClient(rawConn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: insecureSkipTLS,
	}, utls.HelloChrome_Auto)
	if err := uConn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		// Fallback to standard Go TLS on uTLS failure — don't let a uTLS
		// incompatibility break the request.
		fallbackConn, dialErr := dialContext(ctx, network, addr)
		if dialErr != nil {
			return nil, fmt.Errorf("utls handshake: %w; fallback dial: %v", err, dialErr)
		}
		stdConn := tls.Client(fallbackConn, &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: insecureSkipTLS,
		})
		if handshakeErr := stdConn.HandshakeContext(ctx); handshakeErr != nil {
			fallbackConn.Close()
			return nil, fmt.Errorf("utls handshake: %w; fallback tls handshake: %v", err, handshakeErr)
		}
		return stdConn, nil
	}
	return uConn, nil
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
// total duration while chunks keep flowing. The header phase stays bounded
// by the transport's ResponseHeaderTimeout; body-phase liveness is enforced
// per chunk by the relay's idle guard (PROXY_STREAM_IDLE_TIMEOUT_SEC).
func newStreamProxyClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport:     transport,
		CheckRedirect: RejectCrossOriginRedirect,
	}
}

// SiteProxyConfig holds proxy configuration for a site.
type SiteProxyConfig struct {
	ProxyURL       string
	UseSystemProxy bool
	CustomHeaders  map[string]string
}

// SiteProxy is the outbound HTTP client with SOCKS/HTTP proxy support.
type SiteProxy struct {
	systemProxyURL  string
	siteConfigs     map[string]*SiteProxyConfig
	httpClient      *http.Client
	httpClientNoTLS *http.Client
}

// NewSiteProxy creates a new SiteProxy.
func NewSiteProxy(systemProxyURL string) *SiteProxy {
	sp := &SiteProxy{
		systemProxyURL: systemProxyURL,
		siteConfigs:    make(map[string]*SiteProxyConfig),
	}
	sp.buildClients()
	return sp
}

func (sp *SiteProxy) buildClients() {
	timeouts := effectiveProxyTimeouts()
	transport := &http.Transport{
		Proxy:                 sp.proxyFunc,
		DialContext:           newSiteDialContext(),
		TLSHandshakeTimeout:   timeouts.tlsHandshake,
		ResponseHeaderTimeout: timeouts.responseHeader,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       timeouts.idleConn,
	}

	sp.httpClient = &http.Client{
		Transport:     transport,
		Timeout:       timeouts.request,
		CheckRedirect: RejectCrossOriginRedirect,
	}

	transportNoTLS := &http.Transport{
		Proxy:                 sp.proxyFunc,
		DialContext:           newSiteDialContext(),
		TLSHandshakeTimeout:   timeouts.tlsHandshake,
		ResponseHeaderTimeout: timeouts.responseHeader,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       timeouts.idleConn,
	}

	sp.httpClientNoTLS = &http.Client{
		Transport:     transportNoTLS,
		Timeout:       timeouts.request,
		CheckRedirect: RejectCrossOriginRedirect,
	}
}

func (sp *SiteProxy) proxyFunc(req *http.Request) (*url.URL, error) {
	if sp.systemProxyURL != "" {
		return url.Parse(sp.systemProxyURL)
	}
	return nil, nil
}

// Do executes an HTTP request through the site proxy layer.
func (sp *SiteProxy) Do(ctx context.Context, req *http.Request, proxyConfig *ProxyConfig) (*http.Response, error) {
	req = req.WithContext(ctx)

	// Apply custom headers from proxy config (deny-list filters identity/hop-by-hop).
	if proxyConfig != nil {
		ApplyCustomHeadersWithOptions(req, proxyConfig.CustomHeaders, ApplyCustomHeadersOptions{
			OverrideRequest: proxyConfig.CustomHeadersOverrideRequest,
		})
	}

	// If specific proxy URL is given, use a dedicated transport
	if proxyConfig != nil && proxyConfig.ProxyURL != "" {
		return sp.doWithExplicitProxy(ctx, req, proxyConfig)
	}

	// Use default client
	client := sp.httpClient
	if proxyConfig != nil && proxyConfig.InsecureSkipTLS {
		client = sp.httpClientNoTLS
	}
	return client.Do(req)
}

func (sp *SiteProxy) doWithExplicitProxy(ctx context.Context, req *http.Request, proxyConfig *ProxyConfig) (*http.Response, error) {
	proxyURL, err := url.Parse(proxyConfig.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	scheme := strings.ToLower(proxyURL.Scheme)
	if !supportedProxySchemes[scheme] {
		return nil, fmt.Errorf("unsupported proxy scheme: %s", scheme)
	}

	// Reuse a pooled transport so repeated requests through the same proxy
	// share keep-alive connections instead of re-handshaking TLS every call.
	// Only the *http.Transport is pooled; the *http.Client wrapper (timeout +
	// redirect policy) is constructed per call and is cheap.
	transport := getCachedTransport(http.ProxyURL(proxyURL), proxyConfig.InsecureSkipTLS, proxyConfig.UseUTLS)
	client := newProxyClient(transport)
	return client.Do(req)
}

// DoWithProxy is a convenience function that works without a SiteProxy instance.
func DoWithProxy(ctx context.Context, req *http.Request, proxyConfig *ProxyConfig) (*http.Response, error) {
	return doWithProxy(ctx, req, proxyConfig, false)
}

// DoWithProxyStream is the SSE variant of DoWithProxy: the dispatched client
// carries no whole-request timeout so a healthy stream is not capped by
// PROXY_REQUEST_TIMEOUT_SEC while chunks keep flowing. Header wait remains
// bounded by the pooled transport's ResponseHeaderTimeout; body-phase
// liveness is enforced by the relay's idle guard. Non-streaming callers must
// keep using DoWithProxy.
func DoWithProxyStream(ctx context.Context, req *http.Request, proxyConfig *ProxyConfig) (*http.Response, error) {
	return doWithProxy(ctx, req, proxyConfig, true)
}

func doWithProxy(ctx context.Context, req *http.Request, proxyConfig *ProxyConfig, stream bool) (*http.Response, error) {
	if proxyConfig != nil {
		// Deny-list sensitive / hop-by-hop / metapi-control headers.
		// Honor CustomHeadersOverrideRequest (default request-wins).
		ApplyCustomHeadersWithOptions(req, proxyConfig.CustomHeaders, ApplyCustomHeadersOptions{
			OverrideRequest: proxyConfig.CustomHeadersOverrideRequest,
		})
	}

	newClient := newProxyClient
	if stream {
		newClient = newStreamProxyClient
	}

	insecureSkipTLS := proxyConfig != nil && proxyConfig.InsecureSkipTLS
	if proxyConfig != nil && proxyConfig.ProxyURL != "" {
		proxyURL, err := url.Parse(proxyConfig.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		scheme := strings.ToLower(proxyURL.Scheme)
		if !supportedProxySchemes[scheme] {
			return nil, fmt.Errorf("unsupported proxy scheme: %s", scheme)
		}

		client := newClient(getCachedTransport(http.ProxyURL(proxyURL), insecureSkipTLS, proxyConfig.UseUTLS))
		return client.Do(req.WithContext(ctx))
	}

	useUTLS := proxyConfig != nil && proxyConfig.UseUTLS
	client := newClient(getCachedTransport(nil, insecureSkipTLS, useUTLS))
	return client.Do(req.WithContext(ctx))
}

// RejectCrossOriginRedirect is the shared CheckRedirect policy for outbound
// HTTP clients that talk to operator-configured upstream sites.
//
// It refuses:
//   - more than 5 redirects
//   - https → non-https scheme downgrades
//   - any host change (blocks 302 to metadata / loopback / private SSRF targets)
//
// Same-origin redirects remain allowed so normal upstream path hops still work.
// Used by platform.DoWithProxy, proxy.RuntimeExecutor, and residual bare clients
// (health probe / admin harness / defaultUpstreamClient).
func RejectCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("stopped after %d redirects", len(via))
	}
	if len(via) == 0 {
		return nil
	}
	previous := via[len(via)-1].URL
	if previous.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect from https to %s", req.URL.Scheme)
	}
	if !strings.EqualFold(previous.Host, req.URL.Host) {
		return fmt.Errorf("refusing cross-origin redirect from %s to %s", previous.Host, req.URL.Host)
	}
	return nil
}

// WithTimeout creates a context with timeout for quick probes.
func withProbeTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, 5*time.Second)
}
