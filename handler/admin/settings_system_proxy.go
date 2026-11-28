package admin

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
)

// POST /api/settings/system-proxy/test
func (h *settingsHandler) testSystemProxy(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProxyUrl  *string `json:"proxyUrl"`
		TargetUrl *string `json:"targetUrl"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	proxyURL := config.Runtime().SystemProxyUrl
	if body.ProxyUrl != nil {
		proxyURL = strings.TrimSpace(*body.ProxyUrl)
	}
	if proxyURL == "" {
		writeError(w, http.StatusBadRequest, "enter the system proxy address first")
		return
	}

	target := "https://www.gstatic.com/generate_204"
	if body.TargetUrl != nil && strings.TrimSpace(*body.TargetUrl) != "" {
		target = strings.TrimSpace(*body.TargetUrl)
		// Guard operator-supplied probe targets against non-http(s) and
		// cloud metadata / link-local SSRF first-hops.
		if service.IsForbiddenSiteTargetURL(target) || !service.IsValidHTTPURL(target) {
			writeError(w, http.StatusBadRequest, "Invalid targetUrl. Cloud metadata / link-local targets are not allowed; expected a valid http(s) URL.")
			return
		}
	}

	result := probeSystemProxy(r.Context(), proxyURL, target)
	writeJSON(w, http.StatusOK, result)
}

// probeSystemProxy performs a bounded HTTP GET via the given proxy URL.
// Injectable for tests via systemProxyProbeFn.
var systemProxyProbeFn = defaultSystemProxyProbe

func probeSystemProxy(ctx context.Context, proxyURL, target string) map[string]any {
	return systemProxyProbeFn(ctx, proxyURL, target)
}

func defaultSystemProxyProbe(ctx context.Context, proxyURL, target string) map[string]any {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return map[string]any{
			"success":   false,
			"proxyUrl":  proxyURL,
			"targetUrl": target,
			"reachable": false,
			"ok":        false,
			"message":   err.Error(),
		}
	}
	started := time.Now()
	resp, err := platform.DoWithProxy(ctx, req, &platform.ProxyConfig{ProxyURL: proxyURL})
	latency := time.Since(started).Milliseconds()
	if err != nil {
		return map[string]any{
			"success":   false,
			"proxyUrl":  proxyURL,
			"targetUrl": target,
			"reachable": false,
			"ok":        false,
			"latencyMs": latency,
			"message":   err.Error(),
		}
	}
	defer resp.Body.Close()
	ok := resp.StatusCode >= 200 && resp.StatusCode < 400
	return map[string]any{
		"success":    ok,
		"proxyUrl":   proxyURL,
		"targetUrl":  target,
		"reachable":  true,
		"ok":         ok,
		"statusCode": resp.StatusCode,
		"latencyMs":  latency,
	}
}
