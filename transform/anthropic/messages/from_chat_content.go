package messages

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"
)

func fromChatContent(raw json.RawMessage, path string, images bool) ([]wireObject, error) {
	if absent(raw) {
		return []wireObject{}, nil
	}
	if value, err := text(raw, path); err == nil {
		return []wireObject{{"type": "text", "text": value}}, nil
	}
	parts, err := array(raw, path)
	if err != nil {
		return nil, err
	}
	out := make([]wireObject, 0, len(parts))
	for i, raw := range parts {
		path := fmt.Sprintf("%s[%d]", path, i)
		part, err := object(raw, path)
		if err != nil {
			return nil, err
		}
		typ, err := text(part["type"], path+".type")
		if err != nil {
			return nil, err
		}
		var block wireObject
		switch typ {
		case "text":
			block, err = textBlock(part, path)
		case "image_url":
			if !images {
				return nil, invalid(path, "images require a user or tool role")
			}
			if err := allowOnly(part, path, "type", "image_url", "cache_control"); err != nil {
				return nil, err
			}
			block, err = fromChatImage(part["image_url"], path+".image_url")
		default:
			return nil, unsupported(path + ".type")
		}
		if err != nil {
			return nil, err
		}
		if err := copyCacheControl(part, block, path); err != nil {
			return nil, err
		}
		out = append(out, block)
	}
	return out, nil
}

func fromChatImage(raw json.RawMessage, path string) (wireObject, error) {
	image, err := object(raw, path)
	if err != nil {
		return nil, err
	}
	if err := allowOnly(image, path, "url", "detail"); err != nil {
		return nil, err
	}
	if !absent(image["detail"]) {
		detail, err := text(image["detail"], path+".detail")
		if err != nil || detail != "auto" {
			return nil, invalid(path+".detail", "cannot preserve an explicit Chat image detail level")
		}
	}
	location, err := identity(image["url"], path+".url")
	if err != nil {
		return nil, err
	}
	source, err := imageSourceForURL(location, path+".url")
	if err != nil {
		return nil, err
	}
	return wireObject{"type": "image", "source": source}, nil
}

func imageSourceForURL(location, path string) (wireObject, error) {
	var source wireObject
	if strings.HasPrefix(location, "data:") {
		header, data, ok := strings.Cut(strings.TrimPrefix(location, "data:"), ",")
		media, encoded := strings.CutSuffix(header, ";base64")
		if !ok || !encoded || data == "" {
			return nil, invalid(path, "must contain base64 image data")
		}
		switch media {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
		default:
			return nil, unsupported(path + " media type")
		}
		if size, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(data))); err != nil || size == 0 {
			return nil, invalid(path, "contains invalid base64")
		}
		source = wireObject{"type": "base64", "media_type": media, "data": data}
	} else {
		parsed, err := url.Parse(location)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || strings.Contains(location, "\\") || strings.IndexFunc(location, unicode.IsSpace) >= 0 {
			return nil, invalid(path, "must be an HTTP(S) image URL without embedded credentials or base64 data URL")
		}
		source = wireObject{"type": "url", "url": location}
	}
	return source, nil
}
