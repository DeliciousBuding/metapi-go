package notify

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
)

const (
	notifyHTTPTimeout         = 10 * time.Second
	notifyMaxResponseBodySize = 64 << 10
)

var notifyHTTPClient = newNotifyHTTPClient()

func newNotifyHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   notifyHTTPTimeout,
		Transport: newNotifyHTTPTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after %d notification redirects", len(via))
			}
			return validateNotifyRequestURL(req.URL)
		},
	}
}

func newNotifyHTTPTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Transport{
		Proxy: nil,
		// The guard resolves the host once, rejects any non-public answer
		// (including 0.0.0.0/8 non-zero addresses — audit L11) and pins the
		// checked IP, so a DNS answer cannot change between validation and
		// dial.
		DialContext:           ssrf.NewPublicOnlyDialContext(net.DefaultResolver, dialer.DialContext),
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: notifyHTTPTimeout,
		IdleConnTimeout:       30 * time.Second,
	}
}

func doNotifyRequest(req *http.Request) (*http.Response, error) {
	if err := validateNotifyRequestURL(req.URL); err != nil {
		return nil, err
	}
	resp, err := notifyHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func readNotifyResponseBody(body io.Reader) ([]byte, error) {
	limited := io.LimitReader(body, notifyMaxResponseBodySize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > notifyMaxResponseBodySize {
		return nil, fmt.Errorf("notification response body exceeds %d bytes", notifyMaxResponseBodySize)
	}
	return data, nil
}

func validateNotifyRequestURL(u *url.URL) error {
	if u == nil {
		return fmt.Errorf("notification URL is empty")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("notification URL scheme %q is not allowed", u.Scheme)
	}
	if u.Host == "" || u.User != nil {
		return fmt.Errorf("notification URL host is invalid")
	}
	if port := u.Port(); port != "" {
		if _, err := net.LookupPort("tcp", port); err != nil {
			return fmt.Errorf("notification URL port is invalid: %w", err)
		}
	}
	if !ssrf.IsForbiddenPublicHost(u.Hostname()) {
		return nil
	}
	return fmt.Errorf("refusing notification request to unsafe host %q", u.Hostname())
}
