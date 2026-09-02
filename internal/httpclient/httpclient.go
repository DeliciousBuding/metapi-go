// Package httpclient is the shared construction point for outbound HTTP
// transports and clients outside the platform site-proxy regime.
//
// Two regimes coexist by design:
//
//   - Platform adapter traffic (login/checkin/balance/model calls through
//     platform.DoWithProxy) uses the pooled transport cache in
//     platform/site_proxy.go, whose timeouts are operator-tunable via the
//     PROXY_*_TIMEOUT_SEC env vars.
//   - Every other outbound path (proxy executor, channel health probes,
//     admin channel test harness, monitor LDOH proxy, notifications, pricing
//     catalog fetch, OAuth token/session exchange, Codex websocket dial,
//     server healthcheck) builds its transport here so each client carries
//     explicit dial / TLS-handshake / idle bounds and a sized connection
//     pool instead of riding http.DefaultTransport.
//
// Baseline values mirror the axonhub HTTP client baseline
// (docs/internal/analysis/competitor-study-2026-08.md): dial 30s,
// TLS handshake 10s, MaxIdleConns 100, IdleConnTimeout 90s.
package httpclient

import (
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
)

// Baseline defaults shared by every outbound transport built in this package.
const (
	DefaultDialTimeout           = 30 * time.Second
	DefaultKeepAlive             = 30 * time.Second
	DefaultTLSHandshakeTimeout   = 10 * time.Second
	DefaultIdleConnTimeout       = 90 * time.Second
	DefaultMaxIdleConns          = 100
	DefaultMaxIdleConnsPerHost   = 20
	DefaultExpectContinueTimeout = 1 * time.Second

	// sharedResponseHeaderTimeout caps the header phase on SharedTransport.
	// It must stay at or above the whole-request timeout of every client that
	// rides SharedTransport (all are <= 30s today) so it never pre-empts a
	// request that would otherwise finish within its own timeout.
	sharedResponseHeaderTimeout = 60 * time.Second
)

// Options configures NewTransport. Zero values select the baseline defaults.
// ResponseHeaderTimeout 0 leaves the header phase unbounded: use that only
// where a caller context or the client Timeout already owns the deadline
// (SSE stream relays, context-driven probes).
type Options struct {
	DialTimeout           time.Duration
	TLSHandshakeTimeout   time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	// Proxy overrides the transport's proxy resolution. Nil keeps
	// http.ProxyFromEnvironment, the behavior these paths historically
	// inherited from http.DefaultTransport. Set NoProxy on paths that must
	// never ride an operator-configured HTTP_PROXY (e.g. OAuth token
	// exchange).
	Proxy func(*http.Request) (*url.URL, error)
	// SiteDialGuard wraps DialContext with the shared SSRF site dial guard
	// (internal/ssrf): the hostname is resolved exactly once, every answer is
	// checked against the metadata / link-local / multicast / reserved deny
	// set, and the connection is pinned to an already-validated IP — closing
	// the DNS-rebinding window between validation and dial. Loopback, RFC1918
	// and IPv6 ULA stay reachable because Metapi upstreams are commonly
	// operator-hosted on private networks.
	//
	// Set it on transports whose target comes from operator-configured site
	// URLs (proxy data plane, channel health probes). Note that when a proxy
	// is resolved for the request, DialContext dials the proxy address, so the
	// guard then validates the proxy rather than the final origin.
	SiteDialGuard bool
}

// NoProxy is an Options.Proxy value that disables proxy resolution
// entirely: HTTP_PROXY/HTTPS_PROXY/NO_PROXY environment variables are
// ignored and every request dials its target directly. It is equivalent to
// a nil Transport.Proxy but can be passed explicitly through Options.
func NoProxy(*http.Request) (*url.URL, error) { return nil, nil }

// NewTransport builds an *http.Transport with explicit phase timeouts and a
// sized idle pool. Proxy resolution keeps http.ProxyFromEnvironment, the
// behavior these paths historically inherited from http.DefaultTransport.
func NewTransport(opts Options) *http.Transport {
	dialTimeout := opts.DialTimeout
	if dialTimeout <= 0 {
		dialTimeout = DefaultDialTimeout
	}
	tlsHandshakeTimeout := opts.TLSHandshakeTimeout
	if tlsHandshakeTimeout <= 0 {
		tlsHandshakeTimeout = DefaultTLSHandshakeTimeout
	}
	idleConnTimeout := opts.IdleConnTimeout
	if idleConnTimeout <= 0 {
		idleConnTimeout = DefaultIdleConnTimeout
	}
	maxIdleConns := opts.MaxIdleConns
	if maxIdleConns <= 0 {
		maxIdleConns = DefaultMaxIdleConns
	}
	maxIdleConnsPerHost := opts.MaxIdleConnsPerHost
	if maxIdleConnsPerHost <= 0 {
		maxIdleConnsPerHost = DefaultMaxIdleConnsPerHost
	}
	proxy := opts.Proxy
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: DefaultKeepAlive,
	}
	dialContext := dialer.DialContext
	if opts.SiteDialGuard {
		dialContext = ssrf.NewSiteDialContext(net.DefaultResolver, dialContext)
	}
	return &http.Transport{
		Proxy:                 proxy,
		DialContext:           dialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
		ExpectContinueTimeout: DefaultExpectContinueTimeout,
		MaxIdleConns:          maxIdleConns,
		MaxIdleConnsPerHost:   maxIdleConnsPerHost,
		IdleConnTimeout:       idleConnTimeout,
	}
}

// NewClient wraps transport with a whole-request timeout and a redirect
// policy. A zero timeout leaves the total request unbounded (streaming
// callers); the transport's phase timeouts still apply.
func NewClient(transport http.RoundTripper, timeout time.Duration, checkRedirect func(*http.Request, []*http.Request) error) *http.Client {
	return &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: checkRedirect,
	}
}

var (
	sharedOnce      sync.Once
	sharedTransport *http.Transport
)

// SharedTransport returns the process-wide baseline transport for outbound
// clients whose whole-request timeout is at most sharedResponseHeaderTimeout
// (telegram notifications, pricing catalog fetch, Codex websocket upgrade,
// server healthcheck). Callers needing a different header-phase cap must
// build their own transport via NewTransport instead of mutating this one.
func SharedTransport() *http.Transport {
	sharedOnce.Do(func() {
		sharedTransport = NewTransport(Options{
			ResponseHeaderTimeout: sharedResponseHeaderTimeout,
		})
	})
	return sharedTransport
}
