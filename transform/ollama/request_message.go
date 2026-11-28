package ollama

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func convertMessage(msg object, calls map[string]string) (object, error) {
	if err := known(msg, "role", "content", "reasoning_content", "thinking", "tool_calls", "tool_call_id", "name"); err != nil {
		return nil, err
	}
	role, err := stringValue(msg["role"])
	if err != nil {
		return nil, err
	}
	if role == "developer" {
		role = "system"
	}
	if role != "system" && role != "user" && role != "assistant" && role != "tool" {
		return nil, fmt.Errorf("unsupported role %q", role)
	}
	content, images, err := convertContent(msg["content"])
	if err != nil {
		return nil, err
	}
	out := object{"role": rawValue(role), "content": rawValue(content)}
	if len(images) > 0 {
		if role != "user" {
			return nil, fmt.Errorf("images require user role")
		}
		out["images"] = rawValue(images)
	}
	for _, key := range []string{"reasoning_content", "thinking"} {
		if raw, ok := msg[key]; ok {
			if role != "assistant" {
				return nil, fmt.Errorf("thinking requires assistant role")
			}
			if _, err := stringValue(raw); err != nil {
				return nil, err
			}
			if prev, ok := out["thinking"]; ok && !bytes.Equal(prev, raw) {
				return nil, fmt.Errorf("conflicting thinking content")
			}
			out["thinking"] = raw
		}
	}
	if raw, ok := msg["tool_calls"]; ok {
		if role != "assistant" {
			return nil, fmt.Errorf("tool_calls require assistant role")
		}
		var list []object
		if json.Unmarshal(raw, &list) != nil {
			return nil, fmt.Errorf("invalid tool_calls")
		}
		for i, call := range list {
			if err := known(call, "id", "type", "function"); err != nil {
				return nil, err
			}
			id, err := stringValue(call["id"])
			if err != nil || id == "" {
				return nil, fmt.Errorf("tool call id required")
			}
			if _, exists := calls[id]; exists {
				return nil, fmt.Errorf("duplicate tool call id")
			}
			if string(call["type"]) != `"function"` {
				return nil, fmt.Errorf("unsupported tool call type")
			}
			fn, err := decodeObject(call["function"])
			if err != nil {
				return nil, err
			}
			if err = known(fn, "name", "arguments", "description"); err != nil {
				return nil, err
			}
			if raw, ok := fn["description"]; ok {
				if _, err := stringValue(raw); err != nil {
					return nil, err
				}
			}
			name, err := stringValue(fn["name"])
			if err != nil || name == "" {
				return nil, fmt.Errorf("function name required")
			}
			arguments, err := stringValue(fn["arguments"])
			if err != nil {
				return nil, fmt.Errorf("tool arguments must be JSON text")
			}
			if arguments == "" {
				arguments = "{}"
			}
			if _, err := decodeObject([]byte(arguments)); err != nil {
				return nil, fmt.Errorf("tool arguments must contain an object")
			}
			fn["arguments"] = json.RawMessage(arguments)
			list[i] = object{"function": rawValue(fn)}
			calls[id] = name
		}
		out["tool_calls"] = rawValue(list)
	}
	if role == "tool" {
		id, err := stringValue(msg["tool_call_id"])
		if err != nil || calls[id] == "" {
			return nil, fmt.Errorf("tool result has no matching tool call")
		}
		if raw, ok := msg["name"]; ok {
			if name, err := stringValue(raw); err != nil || name != calls[id] {
				return nil, fmt.Errorf("tool result name does not match call")
			}
		}
		out["tool_name"] = rawValue(calls[id])
		delete(calls, id)
	} else if _, ok := msg["tool_call_id"]; ok {
		return nil, fmt.Errorf("tool_call_id requires tool role")
	} else if _, ok := msg["name"]; ok {
		return nil, fmt.Errorf("Ollama cannot represent named chat participants")
	}
	return out, nil
}

func convertContent(raw json.RawMessage) (string, []string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil, nil
	}
	if value, err := stringValue(raw); err == nil {
		return value, nil, nil
	}
	var parts []object
	if json.Unmarshal(raw, &parts) != nil {
		return "", nil, fmt.Errorf("invalid content")
	}
	var content strings.Builder
	var images []string
	for _, part := range parts {
		switch string(part["type"]) {
		case `"text"`:
			if err := known(part, "type", "text"); err != nil {
				return "", nil, err
			}
			text, err := stringValue(part["text"])
			if err != nil {
				return "", nil, err
			}
			content.WriteString(text)
		case `"image_url"`:
			if err := known(part, "type", "image_url"); err != nil {
				return "", nil, err
			}
			image, err := decodeObject(part["image_url"])
			if err != nil {
				return "", nil, err
			}
			if err = known(image, "url", "detail"); err != nil {
				return "", nil, err
			}
			if detail, ok := image["detail"]; ok && string(detail) != `"auto"` {
				return "", nil, fmt.Errorf("Ollama cannot represent image detail")
			}
			url, err := stringValue(image["url"])
			if err != nil {
				return "", nil, err
			}
			prefix, data, ok := strings.Cut(url, ",")
			if !ok || !strings.HasPrefix(prefix, "data:image/") || !strings.HasSuffix(prefix, ";base64") || data == "" {
				return "", nil, fmt.Errorf("Ollama images require base64 image data URLs")
			}
			if _, err := base64.StdEncoding.Strict().DecodeString(data); err != nil {
				return "", nil, fmt.Errorf("invalid base64 image")
			}
			images = append(images, data)
		default:
			return "", nil, fmt.Errorf("unsupported content part type")
		}
	}
	return content.String(), images, nil
}
