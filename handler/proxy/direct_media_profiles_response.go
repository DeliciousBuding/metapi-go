package proxyhandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
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

func finishMiniMaxImage(resp *http.Response, payload map[string]json.RawMessage) *http.Response {
	var base struct {
		Code    int64  `json:"status_code"`
		Message string `json:"status_msg"`
	}
	if raw, ok := payload["base_resp"]; ok && json.Unmarshal(raw, &base) != nil {
		return directMediaFailure(resp, payload, "invalid_response", "invalid MiniMax base_resp")
	}
	if base.Code != 0 {
		return directMediaFailure(resp, payload, base.Code, base.Message)
	}
	if raw := payload["error"]; len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
		return directMediaFailure(resp, payload, "upstream_error", "MiniMax image request failed")
	}
	var data struct {
		URLs   []string `json:"image_urls"`
		Base64 []string `json:"image_base64"`
	}
	if json.Unmarshal(payload["data"], &data) != nil {
		return directMediaFailure(resp, payload, "invalid_response", "invalid MiniMax image data")
	}
	images := make([]map[string]string, 0, len(data.URLs)+len(data.Base64))
	for _, value := range data.URLs {
		if value != "" {
			images = append(images, map[string]string{"url": value})
		}
	}
	for _, value := range data.Base64 {
		if value != "" {
			images = append(images, map[string]string{"b64_json": value})
		}
	}
	if len(images) == 0 {
		return directMediaFailure(resp, payload, "empty_images", "MiniMax returned no images")
	}
	payload["data"], _ = json.Marshal(images)
	ensureDirectImageCreated(payload)
	return directMediaJSON(resp, payload)
}

func ensureDirectImageCreated(payload map[string]json.RawMessage) {
	var created int64
	if json.Unmarshal(payload["created"], &created) != nil || created == 0 {
		payload["created"], _ = json.Marshal(time.Now().Unix())
	}
}

func (op directMediaOperation) finishModelScope(ctx context.Context, submission *http.Request, resp *http.Response, payload map[string]json.RawMessage) (result *http.Response, err error) {
	defer func() {
		if err != nil {
			body, _ := json.Marshal(payload)
			if usage := ParseUsageFromBody(body); usage.Found {
				err = &directMediaTransportError{cause: err, usage: usage}
			}
		}
	}()
	taskID := mediaString(payload, "task_id")
	var taskURL string
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		state := strings.ToUpper(strings.TrimSpace(mediaString(payload, "task_status")))
		if state == "FAILED" || state == "CANCELED" || state == "CANCELLED" || len(payload["errors"]) > 0 && string(payload["errors"]) != "null" || len(payload["error"]) > 0 && string(payload["error"]) != "null" {
			code, message := modelScopeFailure(payload)
			return directMediaFailure(resp, payload, code, message), nil
		}
		if state == "SUCCEED" || state == "SUCCESS" || state == "" && (len(payload["data"]) > 0 || len(payload["output_images"]) > 0) {
			return op.modelScopeImages(ctx, resp, payload)
		}
		if taskID == "" {
			return directMediaFailure(resp, payload, "missing_task", "ModelScope response has no images or task_id"), nil
		}
		if state != "" && state != "PENDING" && state != "RUNNING" && state != "QUEUED" && state != "PROCESSING" {
			return directMediaFailure(resp, payload, "invalid_task_status", "ModelScope returned an unknown task status"), nil
		}
		if taskURL == "" {
			var err error
			taskURL, err = modelScopeTaskURL(submission.URL, taskID)
			if err != nil {
				return directMediaFailure(resp, payload, "invalid_task", err.Error()), nil
			}
		} else {
			timer := time.NewTimer(op.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		query, err := http.NewRequestWithContext(ctx, http.MethodGet, taskURL, nil)
		if err != nil {
			return nil, err
		}
		query.Header = submission.Header.Clone()
		for _, key := range []string{"Content-Type", "Content-Length", "X-ModelScope-Async-Mode"} {
			query.Header.Del(key)
		}
		query.Header.Set("Accept", "application/json")
		query.Header.Set("X-ModelScope-Task-Type", "image_generation")
		// Authentication is copied from the actual submitting request. Never
		// resolve the selected credential a second time during this operation.
		poll, err := sendUpstreamRequest(op.cfg, query, op.proxyConfig, op.firstByteTimeoutMs, false)
		if err != nil {
			return nil, err
		}
		body, err := readDirectMediaResponse(poll)
		if err != nil {
			return nil, err
		}
		if poll.StatusCode < 200 || poll.StatusCode >= 300 {
			return carryDirectMediaUsage(poll, body, payload), nil
		}
		next, err := decodeDirectMediaResponse(poll, body)
		if err != nil {
			return directMediaFailure(poll, payload, "invalid_response", err.Error()), nil
		}
		// Retain actual usage/metadata from submission when the task endpoint
		// omits them. New observations take precedence; never synthesize tokens.
		for _, key := range []string{"usage", "metadata", "id", "task_id", "created"} {
			if _, exists := next[key]; !exists && len(payload[key]) > 0 {
				next[key] = payload[key]
			}
		}
		payload, resp = next, poll
	}
}

func modelScopeTaskURL(submission *url.URL, taskID string) (string, error) {
	if taskID == "." || taskID == ".." || !strings.HasSuffix(submission.Path, "/images/generations") {
		return "", fmt.Errorf("cannot derive ModelScope task URL from the configured endpoint")
	}
	task := *submission
	task.Path = strings.TrimSuffix(submission.Path, "/images/generations") + "/tasks/" + taskID
	task.RawPath = strings.TrimSuffix(submission.EscapedPath(), "/images/generations") + "/tasks/" + url.PathEscape(taskID)
	return task.String(), nil
}

func modelScopeFailure(payload map[string]json.RawMessage) (any, string) {
	code := any("task_failed")
	if raw := payload["code"]; len(raw) > 0 {
		code = raw
	}
	message := mediaString(payload, "message")
	for _, key := range []string{"errors", "error"} {
		var details map[string]json.RawMessage
		if json.Unmarshal(payload[key], &details) == nil {
			if raw := details["code"]; len(raw) > 0 {
				code = raw
			}
			if message == "" {
				message = mediaString(details, "message")
			}
		}
	}
	if message == "" {
		message = "ModelScope image task failed"
	}
	return code, message
}

func (op directMediaOperation) modelScopeImages(ctx context.Context, resp *http.Response, payload map[string]json.RawMessage) (*http.Response, error) {
	var entries []json.RawMessage
	raw := payload["data"]
	if len(payload["output_images"]) > 0 {
		raw = payload["output_images"]
	}
	if json.Unmarshal(raw, &entries) != nil || len(entries) == 0 {
		return directMediaFailure(resp, payload, "empty_images", "ModelScope returned no output images"), nil
	}
	images := make([]map[string]json.RawMessage, 0, len(entries))
	var totalImageBytes int64
	for _, entry := range entries {
		image := map[string]json.RawMessage{}
		var imageURL string
		if json.Unmarshal(entry, &imageURL) == nil {
			image["url"], _ = json.Marshal(imageURL)
		} else if json.Unmarshal(entry, &image) != nil || image == nil {
			return directMediaFailure(resp, payload, "invalid_image", "invalid ModelScope output image"), nil
		}
		imageURL = mediaString(image, "url")
		if mediaString(image, "b64_json") == "" {
			if imageURL == "" {
				return directMediaFailure(resp, payload, "empty_image", "ModelScope output image is empty"), nil
			}
			encoded, failure, err := op.downloadModelScopeImage(ctx, imageURL)
			if err != nil {
				return nil, err
			}
			if failure != nil {
				body, err := readDirectMediaResponse(failure)
				if err != nil {
					return nil, err
				}
				return carryDirectMediaUsage(failure, body, payload), nil
			}
			image["b64_json"], _ = json.Marshal(encoded)
		}
		encoded, _ := json.Marshal(image)
		totalImageBytes += int64(len(encoded))
		if totalImageBytes > proxy.MaxBufferedResponseBodyBytes() {
			return directMediaFailure(resp, nil, "response_too_large", "image response exceeds buffered limit"), nil
		}
		images = append(images, image)
	}
	payload["data"], _ = json.Marshal(images)
	delete(payload, "output_images")
	ensureDirectImageCreated(payload)
	return directMediaJSON(resp, payload), nil
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

func (op directMediaOperation) downloadModelScopeImage(ctx context.Context, imageURL string) (string, *http.Response, error) {
	if strings.HasPrefix(imageURL, "data:") {
		encoded, err := directImageDataURL(imageURL)
		return encoded, nil, err
	}
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || service.IsForbiddenSiteTargetURL(imageURL) {
		return "", nil, fmt.Errorf("invalid ModelScope output image URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "image/*")
	// Output URLs can point to a separate CDN. Keep the proxy transport, but
	// never copy the selected API key or configured private headers to it.
	var downloadProxy *platform.ProxyConfig
	if op.proxyConfig != nil {
		copy := *op.proxyConfig
		copy.CustomHeaders = nil
		downloadProxy = &copy
	}
	resp, err := sendUpstreamRequest(op.cfg, req, downloadProxy, op.firstByteTimeoutMs, false)
	if err != nil {
		return "", nil, err
	}
	body, err := readDirectMediaResponse(resp)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", replayDirectMediaResponse(resp, body, false), nil
	}
	if len(body) == 0 {
		return "", directMediaFailure(resp, nil, "empty_image", "downloaded ModelScope image is empty"), nil
	}
	normalized := proxy.NormalizeUpstreamBufferedBody(resp.Header, body)
	if !normalized.Readable {
		return "", nil, fmt.Errorf("image download encoding cannot be decoded")
	}
	return base64.StdEncoding.EncodeToString(normalized.Bytes), nil, nil
}
