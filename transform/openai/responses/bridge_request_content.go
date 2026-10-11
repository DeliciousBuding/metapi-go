package responses

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"
)

// Text-only content keeps the existing representation. Multimodal user arrays
// retain every part in order; images are never moved to a different role.
func bridgeRequestContent(value any, fromResponses bool, role string) (any, error) {
	parts, ok := value.([]any)
	if !ok {
		return bridgeContent(value, fromResponses)
	}
	converted := make([]any, 0, len(parts))
	hasImage := false
	for _, raw := range parts {
		part := bridgeMap(raw)
		typ := bridgeString(part["type"])
		if fromResponses && typ == "input_image" || !fromResponses && typ == "image_url" {
			if role != "user" {
				return nil, fmt.Errorf("Responses/Chat bridge: images require a user message; %s images cannot be represented", role)
			}
			image, err := bridgeRequestImage(part, fromResponses)
			if err != nil {
				return nil, err
			}
			converted = append(converted, image)
			hasImage = true
			continue
		}
		text, err := bridgeContent([]any{raw}, fromResponses)
		if err != nil {
			return nil, err
		}
		textType := "text"
		if !fromResponses {
			textType = "input_text"
		}
		converted = append(converted, bridgeObject{"type": textType, "text": text})
	}
	if hasImage {
		return converted, nil
	}
	return bridgeContent(value, fromResponses)
}

func bridgeRequestImage(part bridgeObject, fromResponses bool) (bridgeObject, error) {
	image := part
	if fromResponses {
		if err := bridgeFields(part, "type", "image_url", "detail"); err != nil {
			return nil, err
		}
	} else {
		if err := bridgeFields(part, "type", "image_url"); err != nil {
			return nil, err
		}
		image = bridgeMap(part["image_url"])
		if image == nil {
			return nil, fmt.Errorf("Responses/Chat bridge: image_url must be an object")
		}
		if err := bridgeFields(image, "url", "detail"); err != nil {
			return nil, err
		}
	}
	key := "url"
	if fromResponses {
		key = "image_url"
	}
	location, err := bridgeText(image[key], "image URL")
	if err != nil {
		return nil, err
	}
	if err := bridgeImageURL(location); err != nil {
		return nil, err
	}
	var detail string
	if image["detail"] != nil {
		detail, err = bridgeText(image["detail"], "image detail")
		if err != nil {
			return nil, err
		}
		switch detail {
		case "auto", "low", "high", "original":
		default:
			return nil, fmt.Errorf("Responses/Chat bridge: unsupported image detail")
		}
	}
	if fromResponses {
		image = bridgeObject{"url": location}
		if detail != "" {
			image["detail"] = detail
		}
		return bridgeObject{"type": "image_url", "image_url": image}, nil
	}
	image = bridgeObject{"type": "input_image", "image_url": location}
	if detail != "" {
		image["detail"] = detail
	}
	return image, nil
}

// Validate references only. Image fetching and credentials are never part of
// protocol conversion, and encoded pixels are not decoded/re-encoded as images.
func bridgeImageURL(location string) error {
	if strings.HasPrefix(location, "data:") {
		header, data, ok := strings.Cut(strings.TrimPrefix(location, "data:"), ",")
		media, encoded := strings.CutSuffix(header, ";base64")
		if !ok || !encoded || data == "" {
			return fmt.Errorf("Responses/Chat bridge: image data URL requires base64 data")
		}
		switch media {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
		default:
			return fmt.Errorf("Responses/Chat bridge: unsupported image media type")
		}
		if size, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(data))); err != nil || size == 0 {
			return fmt.Errorf("Responses/Chat bridge: invalid image base64")
		}
		return nil
	}
	parsed, err := url.Parse(location)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || strings.ContainsAny(location, "\\") || strings.IndexFunc(location, unicode.IsSpace) >= 0 {
		return fmt.Errorf("Responses/Chat bridge: image URL must be HTTP(S) without embedded credentials")
	}
	return nil
}
