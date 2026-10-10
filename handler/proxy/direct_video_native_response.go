package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
)

type directVideoUsageKey struct{}

func validDirectNativeVideoContentVariant(variant string) bool {
	return variant == "" || variant == "video" || variant == "last_frame"
}

func directNativeVideoResponseUsage(resp *http.Response) ParsedUsage {
	if resp != nil && resp.Request != nil {
		if usage, ok := resp.Request.Context().Value(directVideoUsageKey{}).(ParsedUsage); ok {
			return usage
		}
	}
	return ParsedUsage{Source: usageSourceUnknown}
}

// This sends each create exactly once. The dispatcher must disable retries for
// the POST before calling: even a broken response can follow a created task.
// Content requests first read the authorized task, then download its result
// without copying any upstream API credential or private header to the CDN.
func sendDirectNativeVideoRequest(cfg *UpstreamConfig, original *http.Request, proxyConfig *platform.ProxyConfig, firstByteTimeoutMs int64, profile, expectedTaskID string) (result *http.Response, err error) {
	if !isDirectNativeVideoProfile(profile) {
		return nil, fmt.Errorf("unsupported native video profile")
	}
	isContent := original.Method == http.MethodGet && expectedTaskID != "" &&
		strings.HasSuffix(original.URL.EscapedPath(), "/"+url.PathEscape(expectedTaskID)+"/content")
	req := original.Clone(original.Context())
	variant := req.URL.Query().Get("variant")
	if isContent {
		if !validDirectNativeVideoContentVariant(variant) {
			return nil, fmt.Errorf("unsupported native video content variant")
		}
		req.URL.Path = strings.TrimSuffix(req.URL.Path, "/content")
		req.URL.RawPath = strings.TrimSuffix(req.URL.RawPath, "/content")
		query := req.URL.Query()
		query.Del("variant")
		req.URL.RawQuery = query.Encode()
		req.Header.Del("Range")
		req.Header.Del("If-Range")
		req.Header.Set("Accept", "application/json")
	}
	started := time.Now()
	resp, err := sendUpstreamRequest(cfg, req, proxyConfig, firstByteTimeoutMs, false)
	firstByteMs := time.Since(started).Milliseconds()
	if err != nil {
		return nil, err
	}
	usage := ParsedUsage{Source: usageSourceUnknown}
	defer func() {
		if result != nil {
			ctx := context.WithValue(original.Context(), directMediaTimingKey{}, &firstByteMs)
			ctx = context.WithValue(ctx, directVideoUsageKey{}, usage)
			result.Request = original.WithContext(ctx)
		}
		if err != nil && usage.Found {
			err = &directMediaTransportError{cause: err, usage: usage}
		}
	}()
	body, err := readDirectMediaResponse(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return replayDirectMediaResponse(resp, body, false), nil
	}
	if original.Method == http.MethodDelete && len(bytes.TrimSpace(body)) == 0 {
		return replayDirectMediaResponse(resp, body, false), nil
	}
	normalized := proxy.NormalizeUpstreamBufferedBody(resp.Header, body)
	if !normalized.Readable {
		return directMediaFailure(resp, nil, "invalid_response", "video response encoding cannot be decoded"), nil
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(normalized.Bytes, &payload) != nil || payload == nil {
		return directMediaFailure(resp, nil, "invalid_response", "invalid native video response JSON"), nil
	}
	usage = ParseUsageFromBody(normalized.Bytes)
	if raw := payload["error"]; len(raw) > 0 && !bytes.Equal(raw, []byte("null")) && mediaString(payload, "id") == "" {
		return directMediaFailure(resp, payload, "native_video_error", "upstream video request failed"), nil
	}
	if original.Method == http.MethodDelete {
		// Seedance commonly returns an empty JSON object for successful deletion.
		if raw, exists := payload["error"]; exists && !bytes.Equal(raw, []byte("null")) {
			return directMediaFailure(resp, payload, "native_video_error", "upstream video deletion failed"), nil
		}
		payload["id"], _ = json.Marshal(expectedTaskID)
		payload["deleted"] = json.RawMessage("true")
		return directMediaJSON(resp, payload), nil
	}
	if err := normalizeNativeVideoResponse(payload, profile); err != nil {
		return directMediaFailure(resp, payload, "invalid_response", err.Error()), nil
	}
	if expectedTaskID != "" && mediaString(payload, "id") != expectedTaskID {
		return directMediaFailure(resp, payload, "invalid_response", "upstream returned a different video task"), nil
	}
	if expectedTaskID != "" {
		if account, ok := original.Context().Value(directVideoAccountingOperationKey{}).(func(ParsedUsage) error); ok {
			if err := account(usage); err != nil {
				return nil, err
			}
		}
	}
	if !isContent {
		return directMediaJSON(resp, payload), nil
	}
	if mediaString(payload, "status") != "completed" {
		return directMediaFailure(resp, payload, "video_not_ready", "video content is not available"), nil
	}
	key := "video_url"
	if variant == "last_frame" {
		key = "last_frame_url"
	}
	target := mediaString(payload, key)
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || service.IsForbiddenSiteTargetURL(target) {
		return directMediaFailure(resp, payload, "invalid_video_url", "video result URL is unavailable or invalid"), nil
	}
	download, err := http.NewRequestWithContext(original.Context(), http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	for _, header := range []string{"Range", "If-Range"} {
		if value := original.Header.Get(header); value != "" {
			download.Header.Set(header, value)
		}
	}
	download.Header.Set("Accept", "*/*")
	var downloadProxy *platform.ProxyConfig
	if proxyConfig != nil {
		copy := *proxyConfig
		copy.CustomHeaders = nil
		downloadProxy = &copy
	}
	result, err = sendUpstreamRequest(cfg, download, downloadProxy, firstByteTimeoutMs, true)
	if err != nil {
		return nil, err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		_ = result.Body.Close()
		return directMediaFailure(resp, payload, "video_download_failed", "upstream video content download failed"), nil
	}
	return result, nil
}

func normalizeNativeVideoResponse(payload map[string]json.RawMessage, profile string) error {
	if !validVideoUpstreamID(mediaString(payload, "id")) {
		return fmt.Errorf("native video response has no valid task ID")
	}
	status := strings.ToLower(strings.TrimSpace(mediaString(payload, "status")))
	if status == "" && profile == "seedance-video" {
		status = "queued"
	}
	switch status {
	case "queued":
	case "running":
		status = "in_progress"
	case "succeeded":
		status = "completed"
	case "failed", "cancelled", "expired":
		if profile == "zenmux-video" && status != "failed" {
			return fmt.Errorf("unsupported ZenMux video status")
		}
	default:
		return fmt.Errorf("unsupported native video status")
	}
	payload["status"], _ = json.Marshal(status)
	payload["object"] = json.RawMessage(`"video"`)
	if raw := payload["content"]; len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
		var content map[string]json.RawMessage
		if json.Unmarshal(raw, &content) != nil || content == nil {
			return fmt.Errorf("invalid native video content")
		}
		for _, key := range []string{"video_url", "last_frame_url"} {
			if value, ok := content[key]; ok {
				var target string
				if json.Unmarshal(value, &target) != nil {
					return fmt.Errorf("invalid native video result URL")
				}
				payload[key] = value
			}
		}
	}
	if raw, exists := payload["duration"]; exists {
		var duration json.Number
		if json.Unmarshal(raw, &duration) != nil {
			return fmt.Errorf("invalid native video duration")
		}
		payload["seconds"], _ = json.Marshal(duration.String())
	}
	if status == "completed" || status == "failed" {
		if updated, exists := payload["updated_at"]; exists {
			payload["completed_at"] = updated
		}
	}
	return nil
}
