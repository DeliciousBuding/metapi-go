package generate_content

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
)

// Chat providers may return generated images alongside text. Preserve their
// order without fetching a remote image or guessing an inline image's encoding.
func chatResponseContentParts(content any) ([]any, error) {
	if text, ok := content.(string); ok {
		if text == "" {
			return nil, nil
		}
		return []any{map[string]any{"text": text}}, nil
	}
	blocks, ok := content.([]any)
	if !ok {
		return nil, fmt.Errorf("Chat response content must be text or image parts")
	}
	parts := make([]any, 0, len(blocks))
	for _, raw := range blocks {
		block, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Chat response content part")
		}
		switch block["type"] {
		case "text":
			if err := bridgeKeys(block, "type", "text"); err != nil {
				return nil, err
			}
			text, ok := block["text"].(string)
			if !ok {
				return nil, fmt.Errorf("invalid Chat text part")
			}
			parts = append(parts, map[string]any{"text": text})
		case "image_url":
			if err := bridgeKeys(block, "type", "image_url"); err != nil {
				return nil, err
			}
			image, ok := block["image_url"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Chat image part")
			}
			if err := bridgeKeys(image, "url", "detail"); err != nil {
				return nil, err
			}
			if detail := image["detail"]; detail != nil && detail != "auto" {
				return nil, fmt.Errorf("Chat output image detail requires a native endpoint")
			}
			address, ok := image["url"].(string)
			if !ok || address == "" {
				return nil, fmt.Errorf("Chat image URL is required")
			}
			if strings.HasPrefix(address, "data:") {
				header, data, ok := strings.Cut(address, ",")
				if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") || data == "" {
					return nil, fmt.Errorf("invalid Chat image data URL")
				}
				if _, err := base64.StdEncoding.Strict().DecodeString(data); err != nil {
					return nil, fmt.Errorf("invalid Chat base64 image")
				}
				mime := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
				if strings.ContainsAny(mime, "; \r\n\t") {
					return nil, fmt.Errorf("invalid Chat image MIME type")
				}
				parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": data}})
			} else {
				u, err := url.Parse(address)
				if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
					return nil, fmt.Errorf("invalid Chat image URL")
				}
				parts = append(parts, map[string]any{"fileData": map[string]any{"fileUri": address, "mimeType": inferMimeFromURL(u.Path)}})
			}
		default:
			return nil, fmt.Errorf("Chat response content part requires a native endpoint")
		}
	}
	return parts, nil
}

func geminiResponseImage(part map[string]any) (map[string]any, error) {
	if part["thoughtSignature"] != nil || part["thought"] == true {
		return nil, fmt.Errorf("signed or thought Gemini images require a native endpoint")
	}
	address := ""
	if raw := part["inlineData"]; raw != nil {
		data, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Gemini inlineData")
		}
		if err := bridgeKeys(data, "mimeType", "data"); err != nil {
			return nil, err
		}
		mime, _ := data["mimeType"].(string)
		encoded, _ := data["data"].(string)
		if !strings.HasPrefix(mime, "image/") || strings.ContainsAny(mime, "; \r\n\t") || encoded == "" {
			return nil, fmt.Errorf("Gemini response requires an inline image")
		}
		if _, err := base64.StdEncoding.Strict().DecodeString(encoded); err != nil {
			return nil, fmt.Errorf("invalid Gemini base64 image")
		}
		address = "data:" + mime + ";base64," + encoded
	} else {
		data, ok := part["fileData"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Gemini fileData")
		}
		if err := bridgeKeys(data, "mimeType", "fileUri"); err != nil {
			return nil, err
		}
		mime, _ := data["mimeType"].(string)
		address, _ = data["fileUri"].(string)
		u, err := url.Parse(address)
		if !strings.HasPrefix(mime, "image/") || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return nil, fmt.Errorf("Gemini provider file resources require a native endpoint")
		}
	}
	return map[string]any{"type": "image_url", "image_url": map[string]any{"url": address}}, nil
}
