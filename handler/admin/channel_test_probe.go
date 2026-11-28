package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
)

// secretLikeRE redacts common secret-looking tokens from operator-visible bodies/errors.
var secretLikeRE = regexp.MustCompile(`(?i)(sk-[a-z0-9_\-]{8,}|Bearer\s+[A-Za-z0-9\-._~+/]+=*|api[_-]?key["']?\s*[:=]\s*["']?[A-Za-z0-9\-._]{8,})`)

func (h *channelTestHandler) executeProbe(
	ctx context.Context,
	target *channelTestTarget,
	mode, model, prompt string,
	timeoutMs int64,
	chat *chatTestOptions,
) map[string]any {
	upstreamPath := "/v1/models"
	method := http.MethodGet
	var bodyBytes []byte
	if mode == channelTestModeChat {
		upstreamPath = "/v1/chat/completions"
		method = http.MethodPost
		payload := map[string]any{
			"model": model,
			"messages": []map[string]string{
				{"role": "user", "content": prompt},
			},
			"max_tokens": 8,
			"stream":     false,
		}
		bodyBytes, _ = json.Marshal(payload)
		if chat != nil {
			var err error
			upstreamPath, bodyBytes, err = chat.request(ctx, model, prompt)
			if err != nil {
				return harnessFail(target, mode, model, upstreamPath, "", 0, 0, "", safeError(err))
			}
		}
	}

	upstreamURL := proxy.BuildUpstreamURL(target.Site.URL, upstreamPath)
	host := safeHost(upstreamURL)

	timeout := time.Duration(timeoutMs) * time.Millisecond
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, upstreamURL, bodyReader)
	if err != nil {
		return harnessFail(target, mode, model, upstreamPath, host, 0, 0, "", "build request: "+safeError(err))
	}
	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+target.TokenValue)
	credentialHeader := "Authorization"
	if chat != nil {
		switch chat.TargetFormat {
		case "claude":
			credentialHeader = "x-api-key"
			req.Header.Del("Authorization")
			req.Header.Set(credentialHeader, target.TokenValue)
			req.Header.Set("anthropic-version", "2023-06-01")
		case "gemini":
			credentialHeader = "x-goog-api-key"
			req.Header.Del("Authorization")
			req.Header.Set(credentialHeader, target.TokenValue)
		}
	}
	req.Header.Set("User-Agent", "metapi-go-admin-channel-test/1.0")

	proxyCfg := service.BuildPlatformProxyConfig(h.cfg, &target.Account, &target.Site)
	if proxyCfg != nil {
		for k, v := range proxyCfg.CustomHeaders {
			if proxy.IsMetapiControlHeader(k) {
				continue
			}
			// Never let site headers clobber the harness Authorization.
			if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, credentialHeader) {
				continue
			}
			req.Header.Set(k, v)
		}
	}

	started := time.Now()
	resp, err := h.doRequest(reqCtx, req, proxyCfg)
	latencyMs := time.Since(started).Milliseconds()
	if err != nil {
		return harnessFail(target, mode, model, upstreamPath, host, 0, latencyMs, "", safeError(err))
	}
	firstByteLatencyMs := latencyMs
	defer resp.Body.Close()

	// Bound body read tightly for operator summary.
	bodyLimit := channelTestMaxBodyBytes
	if chat != nil {
		bodyLimit = chatTestMaxBodyBytes
	}
	limited := io.LimitReader(resp.Body, int64(bodyLimit+1))
	raw, readErr := io.ReadAll(limited)
	if readErr != nil {
		failure := harnessFail(target, mode, model, upstreamPath, host, resp.StatusCode, latencyMs, "", "read body: "+safeError(readErr))
		failure["firstByteLatencyMs"] = firstByteLatencyMs
		return failure
	}
	truncated, wasTruncated := truncateAndRedact(raw, bodyLimit)
	if chat != nil {
		latencyMs = time.Since(started).Milliseconds()
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	out := map[string]any{
		"basis":              "forcedProbe",
		"firstByteLatencyMs": firstByteLatencyMs,
		"success":            success,
		"statusCode":         resp.StatusCode,
		"latencyMs":          latencyMs,
		"truncatedBody":      truncated,
		"error":              nil,
		"channelId":          target.ChannelID,
		"routeId":            target.RouteID,
		"siteId":             target.Site.ID,
		"siteName":           target.Site.Name,
		"accountId":          target.Account.ID,
		"model":              model,
		"mode":               mode,
		"upstreamPath":       upstreamPath,
		"upstreamHost":       host,
		"bodyTruncated":      wasTruncated,
	}
	var reported struct {
		Model string `json:"model"`
	}
	if !wasTruncated && json.Unmarshal(raw, &reported) == nil {
		if name := strings.TrimSpace(reported.Model); name != "" {
			out["upstreamReportedModel"] = secretLikeRE.ReplaceAllString(name, "[redacted]")
		}
	}
	if !success {
		out["error"] = fmt.Sprintf("upstream status %d", resp.StatusCode)
	}
	return out
}

func harnessFail(target *channelTestTarget, mode, model, path, host string, status int, latencyMs int64, body, errMsg string) map[string]any {
	return map[string]any{
		"basis":              "forcedProbe",
		"firstByteLatencyMs": nil,
		"success":            false,
		"statusCode":         status,
		"latencyMs":          latencyMs,
		"truncatedBody":      body,
		"error":              errMsg,
		"channelId":          target.ChannelID,
		"routeId":            target.RouteID,
		"siteId":             target.Site.ID,
		"siteName":           target.Site.Name,
		"accountId":          target.Account.ID,
		"model":              model,
		"mode":               mode,
		"upstreamPath":       path,
		"upstreamHost":       host,
		"bodyTruncated":      false,
	}
}

// harnessFallbackTransport bounds the dial/TLS/idle phases for channel-test
// requests that run without an injected transport or site proxy config. The
// deadline is owned by the caller context (per-test timeout), so the
// response-header phase stays unbounded here.
var harnessFallbackTransport = httpclient.NewTransport(httpclient.Options{})

func (h *channelTestHandler) doRequest(ctx context.Context, req *http.Request, proxyCfg *platform.ProxyConfig) (*http.Response, error) {
	if h.transport != nil {
		return h.transport.RoundTrip(req.WithContext(ctx))
	}
	if proxyCfg != nil && (proxyCfg.ProxyURL != "" || proxyCfg.InsecureSkipTLS) {
		return platform.DoWithProxy(ctx, req, proxyCfg)
	}
	// Context owns the deadline; refuse cross-origin redirects so a public
	// site URL cannot 302 into metadata/loopback (shared platform policy).
	client := &http.Client{
		Timeout:       0,
		Transport:     harnessFallbackTransport,
		CheckRedirect: platform.RejectCrossOriginRedirect,
	}
	return client.Do(req.WithContext(ctx))
}

func safeHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}

func truncateAndRedact(raw []byte, maxBytes int) (string, bool) {
	truncated := false
	if len(raw) > maxBytes {
		raw = raw[:maxBytes]
		truncated = true
	}
	// Ensure valid UTF-8 after hard cut.
	for !utf8.Valid(raw) && len(raw) > 0 {
		raw = raw[:len(raw)-1]
		truncated = true
	}
	s := string(raw)
	s = secretLikeRE.ReplaceAllString(s, "[redacted]")
	s = strings.ReplaceAll(s, "\r", "")
	if truncated {
		s += "…[truncated]"
	}
	return s, truncated
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	msg = secretLikeRE.ReplaceAllString(msg, "[redacted]")
	if strings.Contains(msg, "context deadline exceeded") {
		return "timeout waiting for upstream"
	}
	if strings.Contains(msg, "context canceled") {
		return "request canceled"
	}
	if utf8.RuneCountInString(msg) > 400 {
		runes := []rune(msg)
		msg = string(runes[:400]) + "…"
	}
	return msg
}
