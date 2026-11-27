package relaykitbridge

import (
	"fmt"
	"strings"
)

func containsOpaque(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if item == nil {
				continue
			}
			switch key {
			case "signature", "thoughtSignature", "thought_signature", "encrypted_content":
				if s, ok := item.(string); !ok || s != "" {
					return true
				}
			}
			if key == "type" {
				if typ, ok := item.(string); ok && (typ == "redacted_thinking" || typ == "signature_delta") {
					return true
				}
			}
			// Function arguments and schemas are user data, not protocol markers.
			if key == "args" || key == "input" || key == "parameters" || key == "arguments" {
				continue
			}
			if containsOpaque(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if containsOpaque(item) {
				return true
			}
		}
	}
	return false
}

func safeResponse(format Format, raw []byte, stream bool) error {
	var value any
	if (numberCodec{}).Unmarshal(raw, &value) != nil {
		return ErrConversion
	}
	if containsOpaque(value) {
		return fmt.Errorf("opaque reasoning requires its native protocol: %w", ErrConversion)
	}
	var walk func(any) error
	walk = func(value any) error {
		switch v := value.(type) {
		case map[string]any:
			for key, item := range v {
				if item == nil {
					continue
				}
				switch key {
				case "executableCode", "codeExecutionResult":
					return fmt.Errorf("hosted code execution requires its native protocol: %w", ErrConversion)
				case "inlineData", "inline_data", "fileData", "file_data", "images", "image_url", "audio":
					return fmt.Errorf("multimodal response requires its specialized bridge: %w", ErrConversion)
				}
				if key == "type" {
					typ, _ := item.(string)
					if strings.Contains(typ, "image") || typ == "document" {
						return fmt.Errorf("multimodal response: %w", ErrConversion)
					}
				}
				if key != "args" && key != "input" && key != "arguments" {
					if err := walk(item); err != nil {
						return err
					}
				}
			}
		case []any:
			for _, item := range v {
				if err := walk(item); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(value); err != nil {
		return err
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return ErrConversion
	}
	if obj["error"] != nil || obj["type"] == "error" || obj["type"] == "response.failed" {
		return fmt.Errorf("upstream error event: %w", ErrConversion)
	}
	if !stream {
		return validateJSONTerminal(format, obj)
	}
	return nil
}
