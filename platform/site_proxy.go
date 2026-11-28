package platform

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
)

var supportedProxySchemes = map[string]bool{
	"http": true, "https": true,
	"socks": true, "socks4": true, "socks4a": true,
	"socks5": true, "socks5h": true,
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
	return doWithProxy(ctx, req, proxyConfig, false, 0)
}

// WebsocketHTTPClient uses the same explicit proxy, guarded dialing and
// cross-origin redirect policy as HTTP dispatch. The caller bounds upgrade
// and socket lifetime with contexts, rather than a whole-request timeout.
func WebsocketHTTPClient(proxyConfig *ProxyConfig) (*http.Client, error) {
	var proxy func(*http.Request) (*url.URL, error)
	if proxyConfig != nil && proxyConfig.ProxyURL != "" {
		target, err := url.Parse(proxyConfig.ProxyURL)
		if err != nil || !supportedProxySchemes[strings.ToLower(target.Scheme)] {
			return nil, fmt.Errorf("invalid websocket proxy")
		}
		proxy = http.ProxyURL(target)
	}
	return newStreamProxyClient(getCachedTransportForMode(proxy,
		proxyConfig != nil && proxyConfig.InsecureSkipTLS, proxyConfig != nil && proxyConfig.UseUTLS, true)), nil
}

// DoWithProxyStreamBudget uses one header timer, not the buffered transport's
// header or whole-request timeout. firstByteTimeoutMs is milliseconds; values
// <= 0 use httpclient.DefaultRequestCeiling. After headers, the caller and relay
// idle guard own the body lifetime. Callers must close resp.Body.
func DoWithProxyStreamBudget(ctx context.Context, req *http.Request, proxyConfig *ProxyConfig, firstByteTimeoutMs int64) (*http.Response, error) {
	return doWithProxy(ctx, req, proxyConfig, true, firstByteTimeoutMs)
}

func doWithProxy(ctx context.Context, req *http.Request, proxyConfig *ProxyConfig, stream bool, firstByteTimeoutMs int64) (*http.Response, error) {
	if proxyConfig != nil {
		// Deny-list sensitive / hop-by-hop / metapi-control headers.
		// Honor CustomHeadersOverrideRequest (default request-wins).
		ApplyCustomHeadersWithOptions(req, proxyConfig.CustomHeaders, ApplyCustomHeadersOptions{
			OverrideRequest: proxyConfig.CustomHeadersOverrideRequest,
		})
	}

	var proxy func(*http.Request) (*url.URL, error)
	if proxyConfig != nil && proxyConfig.ProxyURL != "" {
		proxyURL, err := url.Parse(proxyConfig.ProxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %w", err)
		}
		scheme := strings.ToLower(proxyURL.Scheme)
		if !supportedProxySchemes[scheme] {
			return nil, fmt.Errorf("unsupported proxy scheme: %s", scheme)
		}
		proxy = http.ProxyURL(proxyURL)
	}

	insecureSkipTLS := proxyConfig != nil && proxyConfig.InsecureSkipTLS
	useUTLS := proxyConfig != nil && proxyConfig.UseUTLS
	transport := getCachedTransportForMode(proxy, insecureSkipTLS, useUTLS, stream)
	req = req.WithContext(ctx)
	if stream {
		return httpclient.DoWithHeaderBudget(req, firstByteTimeoutMs, newStreamProxyClient(transport).Do)
	}
	return newProxyClient(transport).Do(req)
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
	if requestRedirectsDisabled(req, via) {
		return http.ErrUseLastResponse
	}
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
