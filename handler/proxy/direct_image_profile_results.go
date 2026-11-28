package proxyhandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
)

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

func (op directMediaOperation) downloadModelScopeImage(ctx context.Context, imageURL string) (string, *http.Response, error) {
	if strings.HasPrefix(imageURL, "data:") {
		encoded, err := directImageDataURL(imageURL)
		return encoded, nil, err
	}
	parsed, err := url.Parse(imageURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", nil, fmt.Errorf("invalid ModelScope output image URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Accept", "image/*")
	// Output URLs can point to a separate CDN. Pin the resolved public IP and
	// isolate credentials, cookies, UA, custom headers, and TLS verification so
	// an upstream-supplied URL cannot DNS-rebind to private/metadata networks or
	// receive channel-private cookies/TLS overrides.
	var resp *http.Response
	if mediaTargetAllowPrivate {
		// Test-only escape hatch for loopback fake CDNs; production never sets it.
		var clean *platform.ProxyConfig
		if op.proxyConfig != nil {
			copy := *op.proxyConfig
			copy.CustomHeaders = nil
			copy.ClearanceCookie = ""
			copy.BrowserUA = ""
			copy.InsecureSkipTLS = false
			clean = &copy
		}
		resp, err = sendUpstreamRequest(op.cfg, req, clean, op.firstByteTimeoutMs, false)
	} else {
		var host, serverName string
		req.URL, host, serverName, err = pinnedPublicMediaTarget(ctx, imageURL, nil)
		if err != nil {
			return "", nil, fmt.Errorf("invalid ModelScope output image URL")
		}
		req.Host = host
		resp, err = platform.DoPinnedDownloadStreamBudget(ctx, req, op.proxyConfig, serverName, op.firstByteTimeoutMs)
	}
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
