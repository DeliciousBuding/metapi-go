package service

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
)

// ProxyAwareHTTPClient creates an *http.Client that optionally routes through
// a proxy. If proxyURL is empty or fails to parse, the client dials directly
// and never consults HTTP_PROXY/HTTPS_PROXY environment variables.
//
// The client always wires platform.RejectCrossOriginRedirect so callers
// (notably notify/telegram) cannot follow a public-origin 302 onto a
// different host. Phase bounds keep this helper's historical values (dial
// 10s, TLS 10s, response-header phase = timeout) on the shared httpclient
// baseline; the whole-request timeout is the caller's value.
func ProxyAwareHTTPClient(proxyURL string, timeout time.Duration) *http.Client {
	proxy := httpclient.NoProxy
	if proxyURL != "" {
		// Invalid proxy URLs fall back to direct requests, the historical
		// behavior of this helper.
		if parsed, err := url.Parse(strings.TrimSpace(proxyURL)); err == nil {
			proxy = http.ProxyURL(parsed)
		}
	}
	transport := httpclient.NewTransport(httpclient.Options{
		DialTimeout:           10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		Proxy:                 proxy,
	})
	return httpclient.NewClient(transport, timeout, platform.RejectCrossOriginRedirect)
}
