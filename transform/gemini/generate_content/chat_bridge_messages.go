package generate_content

import (
	"encoding/json"
	"fmt"
	"strings"
)

func geminiChatMessages(in map[string]any) ([]any, error) {
	var messages []any
	if system, ok := in["systemInstruction"].(map[string]any); ok {
		parts, _ := system["parts"].([]any)
		var text strings.Builder
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok || len(part) != 1 {
				return nil, fmt.Errorf("Gemini systemInstruction requires text parts")
			}
			t, ok := part["text"].(string)
			if !ok {
				return nil, fmt.Errorf("Gemini systemInstruction requires text parts")
			}
			text.WriteString(t)
		}
		messages = append(messages, map[string]any{"role": "system", "content": text.String()})
	}
	contents, ok := in["contents"].([]any)
	if !ok || len(contents) == 0 {
		return nil, fmt.Errorf("Gemini contents must be nonempty")
	}
	callIDs := map[string][]string{}
	for _, c := range contents {
		content, ok := c.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Gemini content")
		}
		if err := bridgeKeys(content, "role", "parts"); err != nil {
			return nil, err
		}
		role, _ := content["role"].(string)
		if role == "model" {
			role = "assistant"
		}
		if role == "" {
			role = "user"
		}
		if role != "assistant" && role != "user" {
			return nil, fmt.Errorf("unsupported Gemini role %q", role)
		}
		msg := map[string]any{"role": role}
		var blocks, calls []any
		var reasoning strings.Builder
		parts, ok := content["parts"].([]any)
		if !ok {
			return nil, fmt.Errorf("Gemini parts must be an array")
		}
		for _, raw := range parts {
			part, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Gemini part")
			}
			if err := bridgeKeys(part, "text", "thought", "thoughtSignature", "functionCall", "functionResponse", "inlineData", "fileData"); err != nil {
				return nil, err
			}
			switch {
			case part["text"] != nil:
				t, ok := part["text"].(string)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini text")
				}
				if part["thoughtSignature"] != nil {
					return nil, fmt.Errorf("signed Gemini text requires a native endpoint")
				}
				if part["thought"] == true {
					reasoning.WriteString(t)
				} else {
					blocks = append(blocks, map[string]any{"type": "text", "text": t})
				}
			case part["functionCall"] != nil:
				if role != "assistant" {
					return nil, fmt.Errorf("Gemini functionCall requires model role")
				}
				fc, ok := part["functionCall"].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini functionCall")
				}
				if err := bridgeKeys(fc, "id", "name", "args"); err != nil {
					return nil, err
				}
				name, _ := fc["name"].(string)
				if name == "" {
					return nil, fmt.Errorf("Gemini functionCall name is required")
				}
				args, ok := fc["args"].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("Gemini functionCall args must be an object")
				}
				id, _ := fc["id"].(string)
				if id == "" {
					id = bridgeID("call_")
				}
				callIDs[name] = append(callIDs[name], id)
				encoded, _ := json.Marshal(args)
				call := map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}
				if sig, ok := part["thoughtSignature"].(string); ok {
					call["provider_specific_fields"] = map[string]any{"thought_signature": sig}
				}
				calls = append(calls, call)
			case part["functionResponse"] != nil:
				if role != "user" || len(blocks) > 0 || len(calls) > 0 {
					return nil, fmt.Errorf("Gemini functionResponse requires separate user content")
				}
				fr, ok := part["functionResponse"].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini functionResponse")
				}
				if err := bridgeKeys(fr, "id", "name", "response"); err != nil {
					return nil, err
				}
				name, _ := fr["name"].(string)
				id, _ := fr["id"].(string)
				if id == "" && len(callIDs[name]) > 0 {
					id = callIDs[name][0]
					callIDs[name] = callIDs[name][1:]
				}
				if id == "" {
					return nil, fmt.Errorf("Gemini functionResponse has no matching call")
				}
				encoded, err := json.Marshal(fr["response"])
				if err != nil {
					return nil, err
				}
				messages = append(messages, map[string]any{"role": "tool", "tool_call_id": id, "name": name, "content": string(encoded)})
			case part["inlineData"] != nil:
				data, ok := part["inlineData"].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini inlineData")
				}
				mime, _ := data["mimeType"].(string)
				encoded, _ := data["data"].(string)
				if !strings.HasPrefix(mime, "image/") || encoded == "" {
					return nil, fmt.Errorf("only Gemini inline images can be converted")
				}
				blocks = append(blocks, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + mime + ";base64," + encoded}})
			case part["fileData"] != nil:
				data, ok := part["fileData"].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini fileData")
				}
				mime, _ := data["mimeType"].(string)
				uri, _ := data["fileUri"].(string)
				if !strings.HasPrefix(mime, "image/") || (!strings.HasPrefix(uri, "https://") && !strings.HasPrefix(uri, "http://")) {
					return nil, fmt.Errorf("Gemini provider file resources require a native endpoint")
				}
				blocks = append(blocks, map[string]any{"type": "image_url", "image_url": map[string]any{"url": uri}})
			default:
				return nil, fmt.Errorf("unsupported empty Gemini part")
			}
		}
		if len(blocks) > 0 {
			msg["content"] = blocks
		}
		if len(calls) > 0 {
			msg["tool_calls"] = calls
		}
		if reasoning.Len() > 0 {
			msg["reasoning_content"] = reasoning.String()
		}
		if len(msg) > 1 {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}
