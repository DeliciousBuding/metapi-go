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
)

const (
	// Keep short: multi-candidate user-id probes must fail fast on dead hosts.
	defaultProxyConnectTimeout   = 2 * time.Second
	defaultProxyKeepAliveInitial = 60 * time.Second
	siteProxyCacheTTL            = 3 * time.Second
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
// (proxy, insecureSkipTLS) tuple, creating and storing it on first miss.
// Concurrency-safe: concurrent first-miss callers race via LoadOrStore; the
// loser's transport is unreferenced and garbage-collected with no connections.
func getCachedTransport(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS bool) *http.Transport {
	key := transportCacheKey(proxy, insecureSkipTLS)
	if cached, ok := transportCache.Load(key); ok {
		return cached.(*http.Transport)
	}
	transport := newPooledTransport(proxy, insecureSkipTLS)
	actual, _ := transportCache.LoadOrStore(key, transport)
	return actual.(*http.Transport)
}

// transportCacheKey derives the cache key from the proxy func by resolving it
// against a sentinel request. proxy may be nil (direct connection), in which
// case the proxy portion of the key is the empty string.
func transportCacheKey(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS bool) string {
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
	return proxyURL + "|" + insecure
}

// newPooledTransport builds a fresh *http.Transport configured for connection
// reuse. Called only on cache misses inside getCachedTransport.
func newPooledTransport(proxy func(*http.Request) (*url.URL, error), insecureSkipTLS bool) *http.Transport {
	transport := &http.Transport{
		Proxy: proxy,
		DialContext: (&net.Dialer{
			Timeout:   defaultProxyConnectTimeout,
			KeepAlive: defaultProxyKeepAliveInitial,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// Pool sizing: keep idle sockets warm so repeated probes/streams to the
		// same upstream reuse the same TCP+TLS pair instead of re-dialing.
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:    90 * time.Second,
	}
	if insecureSkipTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return transport
}

// newProxyClient wraps a (pooled) *http.Transport with the shared timeout and
// cross-origin redirect policy used by outbound site-proxy traffic. The
// *http.Client is cheap to construct per request; only the Transport is pooled.
func newProxyClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport:     transport,
		Timeout:       30 * time.Second,
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
	transport := &http.Transport{
		Proxy: sp.proxyFunc,
		DialContext: (&net.Dialer{
			Timeout:   defaultProxyConnectTimeout,
			KeepAlive: defaultProxyKeepAliveInitial,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	sp.httpClient = &http.Client{
		Transport:     transport,
		Timeout:       30 * time.Second,
		CheckRedirect: RejectCrossOriginRedirect,
	}

	transportNoTLS := &http.Transport{
		Proxy: sp.proxyFunc,
		DialContext: (&net.Dialer{
			Timeout:   defaultProxyConnectTimeout,
			KeepAlive: defaultProxyKeepAliveInitial,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	}

	sp.httpClientNoTLS = &http.Client{
		Transport:     transportNoTLS,
		Timeout:       30 * time.Second,
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
	transport := getCachedTransport(http.ProxyURL(proxyURL), proxyConfig.InsecureSkipTLS)
	client := newProxyClient(transport)
	return client.Do(req)
}

// DoWithProxy is a convenience function that works without a SiteProxy instance.
func DoWithProxy(ctx context.Context, req *http.Request, proxyConfig *ProxyConfig) (*http.Response, error) {
	if proxyConfig != nil {
		// Deny-list sensitive / hop-by-hop / metapi-control headers.
		// Honor CustomHeadersOverrideRequest (default request-wins).
		ApplyCustomHeadersWithOptions(req, proxyConfig.CustomHeaders, ApplyCustomHeadersOptions{
			OverrideRequest: proxyConfig.CustomHeadersOverrideRequest,
		})
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

		client := newProxyClient(getCachedTransport(http.ProxyURL(proxyURL), insecureSkipTLS))
		return client.Do(req.WithContext(ctx))
	}

	client := newProxyClient(getCachedTransport(nil, insecureSkipTLS))
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
