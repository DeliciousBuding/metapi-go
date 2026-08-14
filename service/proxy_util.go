package service

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
)

// ProxyAwareHTTPClient creates an *http.Client that optionally routes through a proxy.
// If proxyURL is empty, returns a standard client.
//
// The client always wires platform.RejectCrossOriginRedirect so callers
// (notably notify/telegram) cannot follow a public-origin 302 onto a
// different host.
func ProxyAwareHTTPClient(proxyURL string, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
	}

	if proxyURL != "" {
		proxyURL = strings.TrimSpace(proxyURL)
		parsed, err := url.Parse(proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}

	return &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: platform.RejectCrossOriginRedirect,
	}
}
