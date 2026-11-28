// Package ollama translates the native Ollama Chat wire protocol to OpenAI Chat.
package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type object = map[string]json.RawMessage

func decodeObject(raw []byte) (object, error) {
	var obj object
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	return obj, nil
}

func known(obj object, keys ...string) error {
	for key := range obj {
		found := false
		for _, allowed := range keys {
			if key == allowed {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Ollama cannot represent field %q", key)
		}
	}
	return nil
}

func stringValue(raw json.RawMessage) (string, error) {
	var value string
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &value) != nil {
		return "", fmt.Errorf("expected a string")
	}
	return value, nil
}

func rawValue(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

// FromChatRequest converts a complete Chat request. It rejects fields whose
// meaning cannot be represented by native Ollama instead of silently dropping
// continuity, constrained tool selection, or unsupported modalities.
func FromChatRequest(body []byte) ([]byte, error) {
	in, err := decodeObject(body)
	if err != nil {
		return nil, err
	}
	if err = known(in, "model", "messages", "stream", "stream_options", "temperature", "top_p", "top_k", "max_tokens", "max_completion_tokens", "stop", "seed", "frequency_penalty", "presence_penalty", "tools", "tool_choice", "parallel_tool_calls", "n", "response_format", "reasoning_effort", "think", "keep_alive", "options", "store"); err != nil {
		return nil, err
	}
	if raw, ok := in["store"]; ok && string(raw) != "false" {
		return nil, fmt.Errorf("Ollama does not support stored response semantics")
	}
	model, err := stringValue(in["model"])
	if err != nil || strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("Ollama model is required")
	}
	out := object{"model": in["model"], "stream": rawValue(false)}
	if raw, ok := in["stream"]; ok {
		var stream bool
		if string(raw) == "null" || json.Unmarshal(raw, &stream) != nil {
			return nil, fmt.Errorf("stream must be boolean")
		}
		out["stream"] = raw
	}
	if raw, ok := in["n"]; ok && string(raw) != "1" {
		return nil, fmt.Errorf("Ollama supports n=1 only")
	}
	if raw, ok := in["stream_options"]; ok && string(raw) != "null" {
		opts, err := decodeObject(raw)
		if err != nil {
			return nil, err
		}
		if err = known(opts, "include_usage"); err != nil {
			return nil, err
		}
		if value, ok := opts["include_usage"]; ok && string(value) != "true" && string(value) != "false" {
			return nil, fmt.Errorf("include_usage must be boolean")
		}
	}
	if raw, ok := in["parallel_tool_calls"]; ok && string(raw) != "true" {
		return nil, fmt.Errorf("Ollama cannot enforce parallel_tool_calls=false")
	}
	var msgs []object
	if json.Unmarshal(in["messages"], &msgs) != nil || len(msgs) == 0 {
		return nil, fmt.Errorf("Ollama messages are required")
	}
	converted := make([]object, 0, len(msgs))
	calls := make(map[string]string)
	for i, msg := range msgs {
		value, err := convertMessage(msg, calls)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", i, err)
		}
		converted = append(converted, value)
	}
	out["messages"] = rawValue(converted)
	if raw, ok := in["tools"]; ok {
		var tools []object
		if json.Unmarshal(raw, &tools) != nil {
			return nil, fmt.Errorf("tools must be an array")
		}
		for _, tool := range tools {
			if err := known(tool, "type", "function"); err != nil {
				return nil, err
			}
			if string(tool["type"]) != `"function"` {
				return nil, fmt.Errorf("Ollama supports function tools only")
			}
			fn, err := decodeObject(tool["function"])
			if err != nil {
				return nil, err
			}
			if err = known(fn, "name", "description", "parameters", "strict"); err != nil {
				return nil, err
			}
			name, err := stringValue(fn["name"])
			if err != nil || name == "" {
				return nil, fmt.Errorf("tool function name is required")
			}
			if raw, ok := fn["description"]; ok {
				if _, err := stringValue(raw); err != nil {
					return nil, err
				}
			}
			if strict, ok := fn["strict"]; ok {
				if string(strict) != "false" {
					return nil, fmt.Errorf("Ollama cannot enforce strict tools")
				}
				delete(fn, "strict")
			}
			if raw, ok := fn["parameters"]; ok {
				if _, err := decodeObject(raw); err != nil {
					return nil, fmt.Errorf("tool parameters: %w", err)
				}
			} else {
				fn["parameters"] = rawValue(object{})
			}
			tool["function"] = rawValue(fn)
		}
		out["tools"] = rawValue(tools)
	}
	if raw, ok := in["tool_choice"]; ok {
		choice, err := stringValue(raw)
		if err != nil || (choice != "auto" && choice != "none") {
			return nil, fmt.Errorf("Ollama supports only auto or none tool_choice")
		}
		if choice == "none" {
			delete(out, "tools")
		}
	}
	if err := applyChatOptions(in, out); err != nil {
		return nil, err
	}
	return json.Marshal(out)
}
