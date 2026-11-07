package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/ssrf"
	"github.com/deliciousbuding/metapi-go/platform"
)

type videoDownloadError struct{ cause error }

func (e *videoDownloadError) Error() string { return "video content download failed" }
func (e *videoDownloadError) Unwrap() error { return e.cause }

func pinnedPublicMediaTarget(ctx context.Context, raw string, resolver ssrf.NetIPResolver) (*url.URL, string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u.Host == "" || u.User != nil || u.Fragment != "" || strings.ContainsAny(raw, "\r\n\x00") || (u.Scheme != "http" && u.Scheme != "https") || ssrf.IsForbiddenPublicHost(u.Hostname()) || ssrf.IsForbiddenSiteHostname(u.Hostname()) {
		return nil, "", "", fmt.Errorf("invalid public media target")
	}
	originalHost, serverName := u.Host, u.Hostname()
	var addresses []netip.Addr
	if ip, err := netip.ParseAddr(serverName); err == nil {
		addresses = []netip.Addr{ip}
	} else {
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		addresses, err = resolver.LookupNetIP(ctx, "ip", serverName)
		if err != nil {
			return nil, "", "", fmt.Errorf("public media target resolution failed")
		}
	}
	if len(addresses) == 0 {
		return nil, "", "", fmt.Errorf("public media target resolution failed")
	}
	for _, ip := range addresses {
		if ssrf.IsUnsafePublicAddr(ip) || ssrf.IsForbiddenSiteAddr(ip) {
			return nil, "", "", fmt.Errorf("non-public media target")
		}
	}
	ip := addresses[0].Unmap()
	if port := u.Port(); port != "" {
		u.Host = net.JoinHostPort(ip.String(), port)
	} else {
		u.Host = ip.String()
		if ip.Is6() {
			u.Host = "[" + ip.String() + "]"
		}
	}
	return u, originalHost, serverName, nil
}

// Downloads share the existing media/transport owners. Factory credentials and
// headers never become download headers; query signatures never enter errors.
func downloadDirectVideoContent(cfg *UpstreamConfig, original *http.Request, proxyConfig *platform.ProxyConfig, budgetMs int64, target string, queryResponse *http.Response, payload map[string]json.RawMessage) (*http.Response, error) {
	safe := make(map[string]json.RawMessage, len(payload))
	for key, value := range payload {
		if key != "video_url" && key != "last_frame_url" && key != "content" {
			safe[key] = value
		}
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || len(target) > 8192 || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return directMediaFailure(queryResponse, safe, "invalid_video_url", "video result URL is unavailable or invalid"), nil
	}
	download, _ := http.NewRequestWithContext(original.Context(), http.MethodGet, target, nil)
	for _, key := range []string{"Range", "If-Range"} {
		if value := original.Header.Get(key); value != "" {
			download.Header.Set(key, value)
		}
	}
	download.Header.Set("Accept", "*/*")
	var response *http.Response
	if mediaTargetAllowPrivate {
		// This existing test-only escape hatch is never enabled in production.
		var clean *platform.ProxyConfig
		if proxyConfig != nil {
			copy := *proxyConfig
			copy.CustomHeaders = nil
			copy.InsecureSkipTLS = false
			clean = &copy
		}
		response, err = sendUpstreamRequest(cfg, download, clean, budgetMs, true)
	} else {
		var host, serverName string
		download.URL, host, serverName, err = pinnedPublicMediaTarget(original.Context(), target, nil)
		if err != nil {
			return directMediaFailure(queryResponse, safe, "invalid_video_url", "video result URL is unavailable or invalid"), nil
		}
		download.Host = host
		response, err = platform.DoPinnedDownloadStreamBudget(original.Context(), download, proxyConfig, serverName, budgetMs)
	}
	if err != nil {
		return nil, &videoDownloadError{cause: err}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_ = response.Body.Close()
		return directMediaFailure(queryResponse, safe, "video_download_failed", "upstream video content download failed"), nil
	}
	return response, nil
}
