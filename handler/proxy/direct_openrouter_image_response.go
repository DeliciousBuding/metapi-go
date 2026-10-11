package proxyhandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
)

func sendDirectOpenRouterImageRequest(cfg *UpstreamConfig, original *http.Request, proxyConfig *platform.ProxyConfig, firstByteTimeoutMs int64, responseFormat string) (*http.Response, error) {
	started := time.Now()
	resp, err := sendUpstreamRequest(cfg, original, proxyConfig, firstByteTimeoutMs, false)
	firstByte := time.Since(started).Milliseconds()
	if err != nil {
		return nil, err
	}
	body, err := readDirectMediaResponse(resp)
	if err != nil {
		return nil, err
	}
	resp.Request = original.WithContext(context.WithValue(original.Context(), directMediaTimingKey{}, &firstByte))
	failedHTTP := resp.StatusCode < 200 || resp.StatusCode >= 300
	payload, err := decodeDirectMediaResponse(resp, body)
	if err != nil {
		if failedHTTP {
			return replayDirectMediaResponse(resp, body, false), nil
		}
		return directMediaFailure(resp, nil, "invalid_response", err.Error()), nil
	}
	if err := normalizeOpenRouterImageUsage(payload); err != nil {
		payload["upstream_usage"] = payload["usage"]
		delete(payload, "usage")
		if !failedHTTP {
			return directMediaFailure(resp, payload, "invalid_usage", err.Error()), nil
		}
	}
	if failedHTTP {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		return replayDirectMediaResponse(resp, encoded, true), nil
	}
	if raw := payload["error"]; len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return directMediaFailure(resp, payload, "openrouter_image_error", "OpenRouter image generation failed"), nil
	}
	if err := normalizeDirectOpenRouterImage(payload, responseFormat); err != nil {
		return directMediaFailure(resp, payload, "invalid_response", err.Error()), nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if int64(len(encoded)) > proxy.MaxBufferedResponseBodyBytes() {
		// URL plus base64 can expand an otherwise bounded upstream response.
		// Keep verified consumption when discarding the oversized image body.
		failure := map[string]json.RawMessage{}
		if usage, ok := payload["usage"]; ok {
			failure["usage"] = usage
		}
		return directMediaFailure(resp, failure, "response_too_large", "OpenRouter image response exceeds buffered limit"), nil
	}
	return replayDirectMediaResponse(resp, encoded, true), nil
}

func normalizeDirectOpenRouterImage(payload map[string]json.RawMessage, responseFormat string) error {
	if responseFormat != "" && responseFormat != "url" && responseFormat != "b64_json" {
		return fmt.Errorf("unsupported image response_format")
	}
	if raw, ok := payload["created"]; ok {
		var created int64
		if json.Unmarshal(raw, &created) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("invalid OpenRouter image creation timestamp")
		}
	}
	var images []map[string]json.RawMessage
	if json.Unmarshal(payload["data"], &images) != nil || len(images) == 0 {
		return fmt.Errorf("OpenRouter returned no image data")
	}
	outputFormat := ""
	for _, entry := range images {
		var encoded, mediaType string
		if json.Unmarshal(entry["b64_json"], &encoded) != nil || encoded == "" {
			return fmt.Errorf("OpenRouter returned invalid base64 image data")
		}
		if raw, ok := entry["media_type"]; ok && json.Unmarshal(raw, &mediaType) != nil {
			return fmt.Errorf("invalid OpenRouter image media_type")
		}
		mediaType = strings.TrimSpace(mediaType)
		if strings.HasPrefix(encoded, "data:") {
			header, content, ok := strings.Cut(encoded, ",")
			if !ok || !strings.HasSuffix(header, ";base64") {
				return fmt.Errorf("invalid OpenRouter image data URL")
			}
			embedded := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
			if mediaType != "" && !strings.EqualFold(mediaType, embedded) {
				return fmt.Errorf("OpenRouter image media_type conflicts with data URL")
			}
			mediaType, encoded = embedded, content
		}
		hasMediaType := mediaType != ""
		if mediaType == "" {
			mediaType = "image/png"
		}
		parsed, params, err := mime.ParseMediaType(mediaType)
		if err != nil || !strings.HasPrefix(parsed, "image/") || len(params) != 0 {
			return fmt.Errorf("invalid OpenRouter image media_type")
		}
		mediaType = strings.ToLower(parsed)
		if len(encoded) > base64.StdEncoding.EncodedLen(int(defaultMaxMultipartFileBytes)) || strings.ContainsAny(encoded, "\r\n\t ") {
			return fmt.Errorf("OpenRouter output image exceeds size or encoding limits")
		}
		encoding := base64.StdEncoding.Strict()
		if len(encoded)%4 != 0 {
			encoding = base64.RawStdEncoding.Strict()
		}
		n, err := io.Copy(io.Discard, base64.NewDecoder(encoding, strings.NewReader(encoded)))
		if err != nil || n <= 0 || n > defaultMaxMultipartFileBytes {
			return fmt.Errorf("invalid OpenRouter output image encoding")
		}
		entry["media_type"], _ = json.Marshal(mediaType)
		if responseFormat != "url" {
			entry["b64_json"], _ = json.Marshal(encoded)
		} else {
			delete(entry, "b64_json")
		}
		if responseFormat != "b64_json" {
			entry["url"], _ = json.Marshal("data:" + mediaType + ";base64," + encoded)
		} else {
			delete(entry, "url")
		}
		if outputFormat == "" && hasMediaType {
			outputFormat = strings.TrimPrefix(mediaType, "image/")
			if outputFormat == "svg+xml" {
				outputFormat = "svg"
			}
		}
	}
	payload["data"], _ = json.Marshal(images)
	if outputFormat != "" {
		payload["output_format"], _ = json.Marshal(outputFormat)
	} else {
		delete(payload, "output_format")
	}
	return nil
}

func normalizeOpenRouterImageUsage(payload map[string]json.RawMessage) error {
	if raw, ok := payload["usage"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		var usage map[string]json.RawMessage
		if json.Unmarshal(raw, &usage) != nil || usage == nil {
			return fmt.Errorf("invalid OpenRouter image usage")
		}
		for _, pair := range [][2]string{{"prompt_tokens", "input_tokens"}, {"completion_tokens", "output_tokens"}, {"prompt_tokens_details", "input_tokens_details"}, {"completion_tokens_details", "output_tokens_details"}} {
			if value, exists := usage[pair[0]]; exists {
				if canonical, exists := usage[pair[1]]; exists && !bytes.Equal(canonical, value) {
					return fmt.Errorf("conflicting OpenRouter image usage fields")
				}
				usage[pair[1]] = value
				delete(usage, pair[0])
			}
		}
		for _, key := range []string{"input_tokens", "output_tokens", "total_tokens"} {
			if value, ok := usage[key]; ok {
				var n int64
				if json.Unmarshal(value, &n) != nil || n < 0 || bytes.Equal(value, []byte("null")) {
					return fmt.Errorf("invalid OpenRouter image usage count")
				}
			}
		}
		for _, key := range []string{"input_tokens_details", "output_tokens_details"} {
			if raw, ok := usage[key]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				var details map[string]json.RawMessage
				if json.Unmarshal(raw, &details) != nil || details == nil {
					return fmt.Errorf("invalid OpenRouter image usage details")
				}
				for _, counter := range []string{"text_tokens", "image_tokens", "cached_tokens", "reasoning_tokens"} {
					if value, ok := details[counter]; ok {
						var n int64
						if json.Unmarshal(value, &n) != nil || n < 0 || bytes.Equal(value, []byte("null")) {
							return fmt.Errorf("invalid OpenRouter image usage detail count")
						}
					}
				}
			}
		}
		payload["usage"], _ = json.Marshal(usage)
	}
	return nil
}
