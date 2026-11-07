package proxyhandler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
)

// A query-aware content hook asks for one authorized metadata GET, then emits
// a credential-free public download URL. It cannot start another vendor job.
func sendDirectProviderContent(cfg *UpstreamConfig, original *http.Request, proxyCtx *Ctx, selected *routing.SelectedChannel, wire *directProviderWire, proxyConfig *platform.ProxyConfig, budgetMs int64) (result *http.Response, err error) {
	query := original.Clone(original.Context())
	variant := original.URL.Query().Get("variant")
	if len(variant) > 128 || strings.ContainsAny(variant, "\r\n\x00") {
		return nil, channelplugin.ErrResult
	}
	query.Header = original.Header.Clone()
	query.Header.Del("Range")
	query.Header.Del("If-Range")
	query.Header.Set("Accept", "application/json")
	parameters := query.URL.Query()
	parameters.Del("variant")
	query.URL.RawQuery = parameters.Encode()
	started := time.Now()
	response, err := sendUpstreamRequest(cfg, query, proxyConfig, budgetMs, false)
	firstByteMs := time.Since(started).Milliseconds()
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
	if err != nil {
		return nil, err
	}
	body, err := readDirectMediaResponse(response)
	if err != nil {
		return nil, err
	}
	normalized := proxy.NormalizeUpstreamBufferedBody(response.Header, body)
	if !normalized.Readable {
		return directMediaFailure(response, nil, "invalid_response", "video response encoding cannot be decoded"), nil
	}
	observed, _, observeErr := observeDirectDriverBody(wire, normalized.Bytes, false)
	usage = observed
	if observeErr != nil {
		return nil, channelplugin.ErrResult
	}
	if err = captureDirectMediaTaskUsage(original.Context(), proxyCtx, selected, wire, usage); err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return directMediaFailure(response, nil, "video_query_failed", "video content query failed"), nil
	}
	native, _, err := decodeDirectDriverPayload(original.Context(), wire, normalized.Bytes, false, response.StatusCode)
	if err != nil {
		return nil, err
	}
	var payload map[string]json.RawMessage
	if requestoverride.Decode(native, &payload) != nil || mediaString(payload, "status") != "completed" {
		return directMediaFailure(response, nil, "video_not_ready", "video content is not available"), nil
	}
	// Legacy decoders may have no declarative ID selector. A successfully
	// validated native task result can still establish the original relation.
	if !proxyCtx.videoUsageVerified {
		proxyCtx.videoUsageVerified = true
		if _, err = accountDirectVideoUsage(original.Context(), proxyCtx, selected, usage); err != nil {
			return nil, err
		}
	}
	state, _ := json.Marshal(map[string]string{"variant": variant})
	raw, err := wire.Driver.Call(original.Context(), "content", channelplugin.DriverInput{Body: string(normalized.Bytes), Model: wire.PluginModel, TaskID: wire.DriverTaskID, StatusCode: response.StatusCode, State: state})
	if err != nil {
		return nil, err
	}
	var download struct {
		URL string `json:"url"`
	}
	if requestoverride.Decode(raw, &download) != nil || download.URL == "" {
		return nil, channelplugin.ErrResult
	}
	return downloadDirectVideoContent(cfg, original, proxyConfig, budgetMs, download.URL, response, payload)
}
