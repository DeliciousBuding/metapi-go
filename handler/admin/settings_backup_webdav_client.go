package admin

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
)

func isValidWebdavFileURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" || parsed.User != nil {
		return false
	}
	if port := parsed.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	// Explicit SSRF guard: reject literal IPs in private/loopback/link-local/
	// unspecified ranges at the URL-validation layer. This complements the
	// dial-time DNS resolution check in ssrf.RejectUnsafeWebdavDialHost and the
	// hostname-level guard in ssrf.IsAllowedWebdavTargetHost. The
	// allowPrivateWebdavTargets flag (test-only) bypasses this guard so
	// httptest servers on 127.0.0.1 work.
	host := parsed.Hostname()
	if !allowPrivateWebdavTargets && ssrf.IsPrivateOrLoopbackLiteral(host) {
		return false
	}
	return ssrf.IsAllowedWebdavTargetHost(host, allowPrivateWebdavTargets)
}

func newWebdavHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   backupWebdavFetchTimeout,
		Transport: newWebdavHTTPTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			if !isValidWebdavFileURL(req.URL.String()) {
				return fmt.Errorf("refusing WebDAV redirect to unsafe target")
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing WebDAV redirect from https to %s", req.URL.Scheme)
			}
			return nil
		},
	}
}

func newWebdavHTTPTransport() *http.Transport {
	return newWebdavHTTPTransportWithGuard(net.DefaultResolver, nil)
}

// newWebdavHTTPTransportWithGuard builds the admin WebDAV transport with an
// injectable resolver/dialer so tests can prove the resolved answer is pinned.
// Validating the hostname and then handing it back to the dialer leaves a
// DNS-rebinding TOCTOU window (audit L24); the checked IP is dialed instead.
func newWebdavHTTPTransportWithGuard(resolver ssrf.NetIPResolver, dial ssrf.DialContextFunc) *http.Transport {
	if dial == nil {
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		dial = dialer.DialContext
	}
	dialContext := ssrf.NewPublicOnlyDialContext(resolver, dial)
	if allowPrivateWebdavTargets {
		dialContext = ssrf.NewSiteDialContext(resolver, dial)
	}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           dialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: backupWebdavFetchTimeout,
		IdleConnTimeout:       30 * time.Second,
	}
}
