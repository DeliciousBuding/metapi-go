package generate_content

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToChatResponse converts a completed Gemini candidate without dropping tools,
// signatures, reasoning or measured usage.
func ToChatResponse(body []byte, model string) ([]byte, error) {
	in, err := bridgeObject(body)
	if err != nil {
		return nil, err
	}
	delta, finish, err := geminiDelta(in, false)
	if err != nil {
		return nil, err
	}
	if finish == "" {
		return nil, fmt.Errorf("Gemini response has no finishReason")
	}
	delta["role"] = "assistant"
	id, _ := in["responseId"].(string)
	if id == "" {
		id = bridgeID("chatcmpl_")
	}
	if m, _ := in["modelVersion"].(string); m != "" {
		model = m
	}
	out := map[string]any{"id": id, "object": "chat.completion", "model": model, "choices": []any{map[string]any{"index": 0, "message": delta, "finish_reason": finish}}}
	if meta, ok := in["usageMetadata"].(map[string]any); ok {
		out["usage"], err = geminiUsage(meta)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

// FromChatResponse produces a native Gemini response from one Chat choice.
func FromChatResponse(body []byte) ([]byte, error) {
	in, err := bridgeObject(body)
	if err != nil {
		return nil, err
	}
	if in["error"] != nil {
		return nil, fmt.Errorf("upstream Chat error")
	}
	choices, ok := in["choices"].([]any)
	if !ok || len(choices) != 1 {
		return nil, fmt.Errorf("Chat response requires one choice")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid Chat choice")
	}
	msg, ok := choice["message"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid Chat message")
	}
	parts, err := chatParts(msg)
	if err != nil {
		return nil, err
	}
	finish, _ := choice["finish_reason"].(string)
	if finish == "tool_calls" {
		calls, _ := msg["tool_calls"].([]any)
		if len(calls) == 0 {
			return nil, fmt.Errorf("Chat tool_calls terminal without tools")
		}
	}
	reason, err := geminiFinish(finish)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"candidates": []any{map[string]any{"index": 0, "content": map[string]any{"role": "model", "parts": parts}, "finishReason": reason}}}
	if in["id"] != nil {
		out["responseId"] = in["id"]
	}
	if in["model"] != nil {
		out["modelVersion"] = in["model"]
	}
	if usage, ok := in["usage"].(map[string]any); ok {
		out["usageMetadata"], err = chatUsage(usage)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}

func geminiDelta(in map[string]any, stream bool) (map[string]any, string, error) {
	if in["error"] != nil {
		return nil, "", fmt.Errorf("upstream Gemini error")
	}
	if feedback, ok := in["promptFeedback"].(map[string]any); ok && feedback["blockReason"] != nil {
		return nil, "", fmt.Errorf("Gemini prompt was blocked")
	}
	choices, ok := in["candidates"].([]any)
	if !ok || len(choices) == 0 {
		if stream && in["usageMetadata"] != nil {
			return map[string]any{}, "", nil
		}
		return nil, "", fmt.Errorf("Gemini response has no candidate")
	}
	if len(choices) != 1 {
		return nil, "", fmt.Errorf("Gemini bridge requires one candidate")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("invalid Gemini candidate")
	}
	msg := map[string]any{}
	var text, thought strings.Builder
	var calls []any
	var blocks []any
	hasImages := false
	if content, ok := choice["content"].(map[string]any); ok {
		if role := content["role"]; role != nil && role != "model" {
			return nil, "", fmt.Errorf("Gemini response content requires model role")
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			return nil, "", fmt.Errorf("invalid Gemini response parts")
		}
		for _, raw := range parts {
			part, ok := raw.(map[string]any)
			if !ok {
				return nil, "", fmt.Errorf("invalid Gemini response part")
			}
			if err := bridgeKeys(part, "text", "thought", "thoughtSignature", "functionCall", "inlineData", "fileData"); err != nil {
				return nil, "", err
			}
			payloads := 0
			for _, key := range []string{"text", "functionCall", "inlineData", "fileData"} {
				if part[key] != nil {
					payloads++
				}
			}
			if payloads != 1 {
				return nil, "", fmt.Errorf("Gemini response part requires one payload")
			}
			if raw, exists := part["text"]; exists {
				t, ok := raw.(string)
				if !ok {
					return nil, "", fmt.Errorf("invalid Gemini text")
				}
				if part["thoughtSignature"] != nil {
					return nil, "", fmt.Errorf("signed Gemini text cannot be converted")
				}
				if part["thought"] == true {
					thought.WriteString(t)
				} else {
					text.WriteString(t)
					blocks = append(blocks, map[string]any{"type": "text", "text": t})
				}
			} else if fc, ok := part["functionCall"].(map[string]any); ok {
				if err := bridgeKeys(fc, "id", "name", "args"); err != nil {
					return nil, "", err
				}
				name, _ := fc["name"].(string)
				args, ok := fc["args"].(map[string]any)
				if name == "" || !ok {
					return nil, "", fmt.Errorf("invalid Gemini function call")
				}
				encoded, _ := json.Marshal(args)
				id, _ := fc["id"].(string)
				if id == "" {
					id = bridgeID("call_")
				}
				call := map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": string(encoded)}}
				if sig, _ := part["thoughtSignature"].(string); sig != "" {
					call["provider_specific_fields"] = map[string]any{"thought_signature": sig}
				}
				calls = append(calls, call)
			} else if part["inlineData"] != nil || part["fileData"] != nil {
				image, err := geminiResponseImage(part)
				if err != nil {
					return nil, "", err
				}
				hasImages = true
				blocks = append(blocks, image)
			} else {
				return nil, "", fmt.Errorf("unsupported Gemini response part")
			}
		}
	}
	if hasImages {
		msg["content"] = blocks
	} else if text.Len() > 0 {
		msg["content"] = text.String()
	}
	if thought.Len() > 0 {
		msg["reasoning_content"] = thought.String()
	}
	if len(calls) > 0 {
		msg["tool_calls"] = calls
	}
	finish := ""
	if reason, _ := choice["finishReason"].(string); reason != "" {
		switch reason {
		case "STOP":
			finish = "stop"
			if len(calls) > 0 {
				finish = "tool_calls"
			}
		case "MAX_TOKENS":
			finish = "length"
		case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
			finish = "content_filter"
		default:
			return nil, "", fmt.Errorf("unsupported Gemini finishReason %q", reason)
		}
	}
	return msg, finish, nil
}

func chatParts(msg map[string]any) ([]any, error) {
	if err := bridgeKeys(msg, "role", "content", "reasoning_content", "tool_calls", "refusal"); err != nil {
		return nil, err
	}
	if msg["refusal"] != nil && msg["refusal"] != "" {
		return nil, fmt.Errorf("Chat refusal requires a native endpoint")
	}
	var parts []any
	for _, key := range []string{"reasoning_content", "content"} {
		if raw, ok := msg[key]; ok && raw != nil {
			if key == "content" {
				content, err := chatResponseContentParts(raw)
				if err != nil {
					return nil, err
				}
				parts = append(parts, content...)
				continue
			}
			t, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("Chat response content must be text")
			}
			if t != "" {
				part := map[string]any{"text": t}
				if key == "reasoning_content" {
					part["thought"] = true
				}
				parts = append(parts, part)
			}
		}
	}
	if raw, ok := msg["tool_calls"]; ok && raw != nil {
		calls, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("invalid Chat tool_calls")
		}
		for _, raw := range calls {
			call, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Chat tool call")
			}
			if err := bridgeKeys(call, "id", "type", "function", "provider_specific_fields", "index"); err != nil {
				return nil, err
			}
			if typ, _ := call["type"].(string); typ != "" && typ != "function" {
				return nil, fmt.Errorf("unsupported Chat tool call type")
			}
			fn, ok := call["function"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Chat function")
			}
			name, _ := fn["name"].(string)
			rawArgs, _ := fn["arguments"].(string)
			args, err := bridgeObject([]byte(rawArgs))
			if name == "" || err != nil {
				return nil, fmt.Errorf("invalid Chat function name or arguments")
			}
			fc := map[string]any{"name": name, "args": args}
			if id, _ := call["id"].(string); id != "" {
				fc["id"] = id
			}
			part := map[string]any{"functionCall": fc}
			if fields, ok := call["provider_specific_fields"].(map[string]any); ok {
				if err := bridgeKeys(fields, "thought_signature"); err != nil {
					return nil, err
				}
				if sig, _ := fields["thought_signature"].(string); sig != "" {
					part["thoughtSignature"] = sig
				}
			}
			parts = append(parts, part)
		}
	}
	return parts, nil
}

func geminiFinish(reason string) (string, error) {
	switch reason {
	case "stop", "tool_calls":
		return "STOP", nil
	case "length":
		return "MAX_TOKENS", nil
	case "content_filter":
		return "SAFETY", nil
	default:
		return "", fmt.Errorf("Chat response has unsupported or missing finish_reason %q", reason)
	}
}
func bridgeNumber(value any) float64 {
	if n, ok := value.(json.Number); ok {
		v, _ := n.Float64()
		return v
	}
	n, _ := value.(float64)
	return n
}
