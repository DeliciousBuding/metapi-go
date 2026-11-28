package oauth

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
)

func doHTTP(req *http.Request, proxyURL *string, client *http.Client) (*http.Response, error) {
	if client == nil {
		client = newOAuthHTTPClient(nil)
	}
	if proxyURL != nil && strings.TrimSpace(*proxyURL) != "" {
		proxy, err := url.Parse(strings.TrimSpace(*proxyURL))
		if err != nil {
			return nil, fmt.Errorf("invalid oauth proxy URL: %w", err)
		}
		if proxy.Scheme == "" || proxy.Host == "" {
			return nil, fmt.Errorf("invalid oauth proxy URL: missing scheme or host")
		}
		client.Transport = newOAuthHTTPTransport(http.ProxyURL(proxy))
	}
	return client.Do(req)
}

// oauthHTTPTimeout bounds the whole OAuth control-plane request; the same
// value caps the transport's response-header phase. It is passed explicitly
// to the httpclient baseline so package defaults can never shadow it.
const oauthHTTPTimeout = 30 * time.Second

func newOAuthHTTPClient(proxy func(*http.Request) (*url.URL, error)) *http.Client {
	return httpclient.NewClient(newOAuthHTTPTransport(proxy), oauthHTTPTimeout, platform.RejectCrossOriginRedirect)
}

// newOAuthHTTPTransport builds the OAuth control-plane transport on the
// shared baseline with its historical phase bounds (dial 10s, TLS 10s,
// header phase 30s, idle 30s). A nil proxy means NoProxy: OAuth token
// exchange must never inherit HTTP_PROXY/HTTPS_PROXY from the operator
// environment (locked by TestDoHTTPIgnoresEnvironmentProxyWithoutProxyURL).
func newOAuthHTTPTransport(proxy func(*http.Request) (*url.URL, error)) *http.Transport {
	if proxy == nil {
		proxy = httpclient.NoProxy
	}
	return httpclient.NewTransport(httpclient.Options{
		DialTimeout:           10 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: oauthHTTPTimeout,
		IdleConnTimeout:       30 * time.Second,
		Proxy:                 proxy,
	})
}
