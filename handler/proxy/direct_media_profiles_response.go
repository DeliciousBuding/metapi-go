package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
)

type directMediaTimingKey struct{}

type directMediaTransportError struct {
	cause error
	usage ParsedUsage
}

func (e *directMediaTransportError) Error() string { return e.cause.Error() }
func (e *directMediaTransportError) Unwrap() error { return e.cause }

func directMediaFailureUsage(err error) ParsedUsage {
	var failure *directMediaTransportError
	if errors.As(err, &failure) {
		return failure.usage
	}
	return ParsedUsage{Source: usageSourceUnknown}
}

func directMediaFirstByteLatencyMs(resp *http.Response) *int64 {
	if resp == nil || resp.Request == nil {
		return nil
	}
	value, _ := resp.Request.Context().Value(directMediaTimingKey{}).(*int64)
	return value
}

// A task submission must not be replayed by a generic HTTP retry after polling
// or downloading fails. The dispatcher owns that retry boundary and accounting.
func sendDirectMediaProfileRequest(cfg *UpstreamConfig, req *http.Request, proxyConfig *platform.ProxyConfig, firstByteTimeoutMs int64, profile string) (*http.Response, error) {
	operation := directMediaOperation{cfg: cfg, proxyConfig: proxyConfig, firstByteTimeoutMs: firstByteTimeoutMs, pollInterval: 2 * time.Second}
	return operation.run(req, profile)
}

type directMediaOperation struct {
	cfg                *UpstreamConfig
	proxyConfig        *platform.ProxyConfig
	firstByteTimeoutMs int64
	pollInterval       time.Duration
}

func (op directMediaOperation) run(original *http.Request, profile string) (result *http.Response, err error) {
	if !isDirectMediaProfile(profile) {
		return nil, fmt.Errorf("unsupported direct media profile")
	}
	ctx, cancel := context.WithTimeout(original.Context(), proxy.DefaultRequestCeiling)
	defer cancel()
	req := original.Clone(ctx)
	started := time.Now()
	resp, err := sendUpstreamRequest(op.cfg, req, op.proxyConfig, op.firstByteTimeoutMs, false)
	firstByteMs := time.Since(started).Milliseconds()
	if err != nil {
		return nil, err
	}
	defer func() {
		if result != nil {
			result.Request = original.WithContext(context.WithValue(original.Context(), directMediaTimingKey{}, &firstByteMs))
		}
	}()
	body, err := readDirectMediaResponse(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return replayDirectMediaResponse(resp, body, false), nil
	}
	payload, err := decodeDirectMediaResponse(resp, body)
	if err != nil {
		return directMediaFailure(resp, nil, "invalid_response", err.Error()), nil
	}
	if profile == "minimax-image" {
		return finishMiniMaxImage(resp, payload), nil
	}
	return op.finishModelScope(ctx, req, resp, payload)
}

func readDirectMediaResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return proxy.ReadBufferedResponseBody(resp.Body)
}

func decodeDirectMediaResponse(resp *http.Response, body []byte) (map[string]json.RawMessage, error) {
	normalized := proxy.NormalizeUpstreamBufferedBody(resp.Header, body)
	if !normalized.Readable {
		return nil, fmt.Errorf("image response encoding cannot be decoded")
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(normalized.Bytes, &payload) != nil || payload == nil {
		return nil, fmt.Errorf("invalid image response JSON")
	}
	return payload, nil
}

func replayDirectMediaResponse(resp *http.Response, body []byte, converted bool) *http.Response {
	copy := *resp
	copy.Header = resp.Header.Clone()
	if converted {
		for _, key := range []string{"Content-Encoding", "Content-Length", "ETag", "Content-MD5", "Digest"} {
			copy.Header.Del(key)
		}
		copy.Header.Set("Content-Type", "application/json")
	}
	copy.Body = io.NopCloser(bytes.NewReader(body))
	copy.ContentLength = int64(len(body))
	return &copy
}

func directMediaJSON(resp *http.Response, payload map[string]json.RawMessage) *http.Response {
	body, _ := json.Marshal(payload)
	if int64(len(body)) > proxy.MaxBufferedResponseBodyBytes() {
		return directMediaFailure(resp, nil, "response_too_large", "image response exceeds buffered limit")
	}
	return replayDirectMediaResponse(resp, body, true)
}

func directMediaFailure(resp *http.Response, payload map[string]json.RawMessage, code any, message string) *http.Response {
	if payload == nil {
		payload = map[string]json.RawMessage{}
	}
	if existing := payload["error"]; len(existing) == 0 || bytes.Equal(existing, []byte("null")) {
		payload["error"], _ = json.Marshal(map[string]any{"code": code, "message": message, "type": "upstream_error"})
	}
	body, _ := json.Marshal(payload)
	result := replayDirectMediaResponse(resp, body, true)
	result.StatusCode = http.StatusBadGateway
	result.Status = "502 Bad Gateway"
	return result
}

func mediaString(payload map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(payload[key], &value)
	return value
}

func ensureDirectImageCreated(payload map[string]json.RawMessage) {
	var created int64
	if json.Unmarshal(payload["created"], &created) != nil || created == 0 {
		payload["created"], _ = json.Marshal(time.Now().Unix())
	}
}

func carryDirectMediaUsage(resp *http.Response, body []byte, earlier map[string]json.RawMessage) *http.Response {
	if len(earlier["usage"]) == 0 {
		return replayDirectMediaResponse(resp, body, false)
	}
	// Decode a clone: an unreadable error must retain its original encoding.
	copy := *resp
	copy.Header = resp.Header.Clone()
	payload, err := decodeDirectMediaResponse(&copy, body)
	if err != nil {
		return replayDirectMediaResponse(resp, body, false)
	}
	if _, exists := payload["usage"]; !exists {
		payload["usage"] = earlier["usage"]
	}
	return directMediaJSON(&copy, payload)
}
