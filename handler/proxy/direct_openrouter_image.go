package proxyhandler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// The selected endpoint is already the complete /images URL. This leaf only
// owns the image wire contract, not URL selection, authentication or retries.
func prepareDirectOpenRouterImage(path, contentType string, body []byte) (*directProviderWire, string, error) {
	if path != "/v1/images/generations" && path != "/v1/images/edits" {
		return nil, "", fmt.Errorf("OpenRouter image profile does not support this operation")
	}
	var payload map[string]json.RawMessage
	var err error
	multipart := isMultipartFormDataContentType(contentType)
	if multipart {
		payload, err = directImageMultipart(body, contentType)
	} else {
		err = json.Unmarshal(body, &payload)
	}
	if err != nil || payload == nil {
		return nil, "", fmt.Errorf("invalid OpenRouter image request: %v", err)
	}
	if multipart {
		// The shared multipart parser treats provider-specific fields as text.
		if raw, ok := payload["output_compression"]; ok {
			var n int64
			if json.Unmarshal(raw, &n) != nil {
				var value string
				if json.Unmarshal(raw, &value) != nil {
					return nil, "", fmt.Errorf("invalid output_compression")
				}
				n, err = strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, "", fmt.Errorf("invalid output_compression")
				}
			}
			payload["output_compression"] = json.RawMessage(strconv.FormatInt(n, 10))
		}
		if raw, ok := payload["images"]; ok {
			var value string
			if json.Unmarshal(raw, &value) == nil && strings.HasPrefix(strings.TrimSpace(value), "[") {
				payload["images"] = json.RawMessage(value)
			}
		}
	}
	allowed := map[string]bool{"model": true, "prompt": true, "n": true, "size": true, "quality": true, "background": true,
		"output_format": true, "output_compression": true, "seed": true, "image": true, "images": true, "response_format": true, "stream": true, "mask": true}
	for key := range payload {
		if !allowed[key] {
			return nil, "", fmt.Errorf("OpenRouter image field %q is unsupported", key)
		}
	}
	for _, key := range []string{"model", "prompt"} {
		var value string
		if json.Unmarshal(payload[key], &value) != nil || strings.TrimSpace(value) == "" {
			return nil, "", fmt.Errorf("image %s is required", key)
		}
	}
	if raw, ok := payload["stream"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("false")) {
		return nil, "", fmt.Errorf("this OpenRouter image adapter does not support streaming")
	}
	delete(payload, "stream")
	if raw, ok := payload["mask"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && !bytes.Equal(raw, []byte(`""`)) {
		return nil, "", fmt.Errorf("OpenRouter images do not support masks")
	}
	delete(payload, "mask")
	responseFormat := ""
	if raw, ok := payload["response_format"]; ok && json.Unmarshal(raw, &responseFormat) != nil {
		return nil, "", fmt.Errorf("invalid image response_format")
	}
	if responseFormat != "" && responseFormat != "b64_json" && responseFormat != "url" {
		return nil, "", fmt.Errorf("unsupported image response_format")
	}
	delete(payload, "response_format")
	for _, key := range []string{"size", "quality", "background", "output_format"} {
		if raw, ok := payload[key]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil || bytes.Equal(raw, []byte("null")) {
				return nil, "", fmt.Errorf("image %s must be a string", key)
			}
		}
	}
	for _, key := range []string{"n", "output_compression", "seed"} {
		if raw, ok := payload[key]; ok {
			var value int64
			if json.Unmarshal(raw, &value) != nil || bytes.Equal(raw, []byte("null")) || (key == "n" && value <= 0) || (key == "output_compression" && (value < 0 || value > 100)) {
				return nil, "", fmt.Errorf("invalid image %s", key)
			}
		}
	}
	var references []string
	if raw, ok := payload["image"]; ok {
		var single string
		if json.Unmarshal(raw, &single) == nil {
			references = []string{single}
		} else if json.Unmarshal(raw, &references) != nil {
			return nil, "", fmt.Errorf("image must be an image URL or array of image URLs")
		}
	}
	if len(references) > 0 && len(payload["images"]) > 0 {
		return nil, "", fmt.Errorf("use either image or images, not both")
	}
	if len(references) == 0 {
		if raw, ok := payload["images"]; ok {
			var entries []json.RawMessage
			if json.Unmarshal(raw, &entries) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return nil, "", fmt.Errorf("images must be an array")
			}
			if len(entries) > 16 {
				return nil, "", fmt.Errorf("OpenRouter accepts at most 16 input references")
			}
			for _, entry := range entries {
				var value string
				if json.Unmarshal(entry, &value) != nil {
					var object map[string]json.RawMessage
					if json.Unmarshal(entry, &object) != nil || len(object) != 1 || json.Unmarshal(object["image_url"], &value) != nil {
						return nil, "", fmt.Errorf("images entries must be image URLs or objects with image_url")
					}
				}
				references = append(references, value)
			}
		}
	}
	if len(references) > 16 {
		return nil, "", fmt.Errorf("OpenRouter accepts at most 16 input references")
	}
	if path == "/v1/images/edits" && len(references) == 0 {
		return nil, "", fmt.Errorf("image edits require at least one reference image")
	}
	parts := make([]map[string]any, 0, len(references))
	for _, reference := range references {
		if err := validateOpenRouterImageReference(reference); err != nil {
			return nil, "", err
		}
		parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]string{"url": reference}})
	}
	delete(payload, "image")
	delete(payload, "images")
	if len(parts) > 0 {
		payload["input_references"], _ = json.Marshal(parts)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	wire := &directProviderWire{Profile: "openrouter-image", Body: encoded, Headers: make(http.Header), Query: make(url.Values), ImageResponseFormat: responseFormat}
	wire.Headers.Set("Content-Type", "application/json")
	wire.Headers.Set("Accept", "application/json")
	return wire, "application/json", nil
}

func validateOpenRouterImageReference(reference string) error {
	if !strings.HasPrefix(reference, "data:") {
		u, err := url.Parse(reference)
		if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || (!strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https")) || strings.ContainsAny(reference, " \t\r\n") {
			return fmt.Errorf("invalid OpenRouter reference image URL")
		}
		// OpenRouter resolves HTTP(S) references itself. Preserve signed query
		// strings verbatim and never send the upstream credential to this URL.
		return nil
	}
	header, _, ok := strings.Cut(reference, ",")
	if !ok {
		return fmt.Errorf("OpenRouter reference images must be base64 data URLs")
	}
	media := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	switch media {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return fmt.Errorf("unsupported OpenRouter reference image media type")
	}
	encoded, err := directImageDataURL(reference)
	if err != nil {
		return err
	}
	decodedSize := base64.StdEncoding.DecodedLen(len(encoded)) - strings.Count(encoded, "=")
	if decodedSize <= 0 || int64(decodedSize) > defaultMaxMultipartFileBytes {
		return fmt.Errorf("OpenRouter reference image exceeds size limit")
	}
	return nil
}
