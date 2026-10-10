// Package ollama translates the native Ollama Chat wire protocol to OpenAI Chat.
package ollama

import (
	"bytes"
	"encoding/base64"
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
	opts := object{}
	if raw, ok := in["options"]; ok {
		opts, err = decodeObject(raw)
		if err != nil {
			return nil, fmt.Errorf("options: %w", err)
		}
		// Native options stay byte-preserving; unlike unknown Chat fields, their
		// explicit namespace is already the target API's contract.
	}
	for _, key := range []string{"temperature", "top_p", "top_k", "seed", "frequency_penalty", "presence_penalty"} {
		if raw, ok := in[key]; ok {
			var number json.Number
			if string(raw) == "null" || json.Unmarshal(raw, &number) != nil || len(raw) == 0 || raw[0] == '"' {
				return nil, fmt.Errorf("%s must be a number", key)
			}
			if _, exists := opts[key]; exists {
				return nil, fmt.Errorf("%s conflicts with options", key)
			}
			opts[key] = raw
		}
	}
	for _, key := range []string{"max_tokens", "max_completion_tokens"} {
		if raw, ok := in[key]; ok {
			var count int64
			if json.Unmarshal(raw, &count) != nil || count <= 0 {
				return nil, fmt.Errorf("%s must be a positive integer", key)
			}
			if previous, ok := opts["num_predict"]; ok && !bytes.Equal(previous, raw) {
				return nil, fmt.Errorf("conflicting output token limits")
			}
			opts["num_predict"] = raw
		}
	}
	if raw, ok := in["stop"]; ok && string(raw) != "null" {
		var stops []string
		if err := json.Unmarshal(raw, &stops); err != nil {
			value, err := stringValue(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid stop")
			}
			stops = []string{value}
		}
		if _, ok := opts["stop"]; ok {
			return nil, fmt.Errorf("stop conflicts with options")
		}
		opts["stop"] = rawValue(stops)
	}
	if len(opts) > 0 {
		out["options"] = rawValue(opts)
	}
	if raw, ok := in["response_format"]; ok {
		format, err := decodeObject(raw)
		if err != nil {
			return nil, err
		}
		if err = known(format, "type", "json_schema"); err != nil {
			return nil, err
		}
		switch string(format["type"]) {
		case `"text"`:
			if len(format) != 1 {
				return nil, fmt.Errorf("text response_format cannot include a schema")
			}
		case `"json_object"`:
			if len(format) != 1 {
				return nil, fmt.Errorf("json_object cannot include a schema")
			}
			out["format"] = rawValue("json")
		case `"json_schema"`:
			schema, err := decodeObject(format["json_schema"])
			if err != nil {
				return nil, err
			}
			if err = known(schema, "name", "description", "schema", "strict"); err != nil {
				return nil, err
			}
			if _, err := decodeObject(schema["schema"]); err != nil {
				return nil, err
			}
			if raw, ok := schema["strict"]; ok && string(raw) != "false" {
				return nil, fmt.Errorf("Ollama cannot promise strict JSON schema semantics")
			}
			out["format"] = schema["schema"]
		default:
			return nil, fmt.Errorf("unsupported response_format")
		}
	}
	if raw, ok := in["think"]; ok {
		if string(raw) != "true" && string(raw) != "false" && string(raw) != `"low"` && string(raw) != `"medium"` && string(raw) != `"high"` {
			return nil, fmt.Errorf("invalid think option")
		}
		out["think"] = raw
	}
	if raw, ok := in["reasoning_effort"]; ok {
		if _, exists := out["think"]; exists {
			return nil, fmt.Errorf("reasoning_effort conflicts with think")
		}
		if string(raw) == `"none"` {
			out["think"] = rawValue(false)
		} else if string(raw) == `"low"` || string(raw) == `"medium"` || string(raw) == `"high"` {
			out["think"] = raw
		} else {
			return nil, fmt.Errorf("unsupported reasoning_effort")
		}
	}
	if raw, ok := in["keep_alive"]; ok {
		out["keep_alive"] = raw
	}
	return json.Marshal(out)
}

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
