package generate_content

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FromChatRequest validates the portable subset before using the existing Gemini
// request builder. Unknown fields are errors rather than silently discarded.
func FromChatRequest(body []byte, model string) ([]byte, error) {
	in, err := bridgeObject(body)
	if err != nil {
		return nil, err
	}
	if value, exists := in["store"]; exists {
		if value != false {
			return nil, fmt.Errorf("stored Chat responses require a native endpoint")
		}
		delete(in, "store")
	}
	if err := bridgeKeys(in, "model", "messages", "stream", "stream_options", "tools", "tool_choice", "temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "reasoning_effort", "reasoning_budget", "response_format", "parallel_tool_calls"); err != nil {
		return nil, err
	}
	formatConfig, err := chatResponseFormat(in["response_format"], body)
	if err != nil {
		return nil, err
	}
	if in["parallel_tool_calls"] == false {
		return nil, fmt.Errorf("Gemini cannot preserve disabled parallel tool calls")
	}
	msgs, ok := in["messages"].([]any)
	if !ok || len(msgs) == 0 {
		return nil, fmt.Errorf("Chat messages must be nonempty")
	}
	var system []string
	var normalized []any
	callIDs := map[string]bool{}
	for _, raw := range msgs {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Chat message")
		}
		if err := bridgeKeys(m, "role", "content", "tool_calls", "tool_call_id", "name", "reasoning_content"); err != nil {
			return nil, err
		}
		role, _ := m["role"].(string)
		switch role {
		case "user", "assistant", "tool":
		case "system", "developer":
			system = append(system, extractOpenAIText(m["content"]))
		default:
			return nil, fmt.Errorf("unsupported Chat role %q", role)
		}
		if role != "system" && role != "developer" {
			normalized = append(normalized, m)
		}
		if role == "tool" {
			id, _ := m["tool_call_id"].(string)
			text, ok := m["content"].(string)
			if id == "" || !ok {
				return nil, fmt.Errorf("Chat tool result requires identity and text")
			}
			obj, err := bridgeObject([]byte(text))
			if err != nil {
				obj = map[string]any{"result": text}
			}
			m["content"] = obj
		}
		if blocks, ok := m["content"].([]any); ok {
			for _, raw := range blocks {
				b, ok := raw.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Chat content block")
				}
				typ, _ := b["type"].(string)
				if typ != "text" && typ != "image_url" {
					return nil, fmt.Errorf("unsupported Chat content type %q", typ)
				}
				if err := bridgeKeys(b, "type", "text", "image_url"); err != nil {
					return nil, err
				}
			}
		}
		if calls, ok := m["tool_calls"].([]any); ok {
			for _, raw := range calls {
				call, ok := raw.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Chat tool call")
				}
				if err := bridgeKeys(call, "id", "type", "function", "provider_specific_fields"); err != nil {
					return nil, err
				}
				if call["type"] != "function" {
					return nil, fmt.Errorf("unsupported Chat tool type")
				}
				fn, ok := call["function"].(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Chat function")
				}
				args, _ := fn["arguments"].(string)
				if !strings.HasPrefix(strings.TrimSpace(args), "{") || !json.Valid([]byte(args)) {
					return nil, fmt.Errorf("Chat tool arguments must be a JSON object")
				}
				id, _ := call["id"].(string)
				if id == "" || callIDs[id] {
					return nil, fmt.Errorf("Chat tool identity missing or duplicate")
				}
				callIDs[id] = true
			}
		}
	}
	if len(system) > 0 {
		normalized = append([]any{map[string]any{"role": "system", "content": strings.Join(system, "\n\n")}}, normalized...)
	}
	in["messages"] = normalized
	if raw, exists := in["tools"]; exists {
		tools, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("Chat tools must be an array")
		}
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok || tool["type"] != "function" {
				return nil, fmt.Errorf("only Chat function tools can be converted")
			}
			if err := bridgeKeys(tool, "type", "function"); err != nil {
				return nil, err
			}
			fn, ok := tool["function"].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Chat function tool")
			}
			if err := bridgeKeys(fn, "name", "description", "parameters", "strict"); err != nil {
				return nil, err
			}
			if fn["strict"] != nil && fn["strict"] != false {
				return nil, fmt.Errorf("strict Chat tools require a native endpoint")
			}
		}
	}
	if in["max_tokens"] == nil && in["max_completion_tokens"] != nil {
		in["max_tokens"] = in["max_completion_tokens"]
	}
	for _, field := range []string{"temperature", "top_p", "max_tokens"} {
		if n, ok := in[field].(json.Number); ok {
			v, err := n.Float64()
			if err != nil {
				return nil, err
			}
			in[field] = v
		}
	}
	out := BuildGeminiGenerateContentRequestFromOpenAi(in, model)
	if len(formatConfig) > 0 {
		gc, _ := out["generationConfig"].(map[string]any)
		if gc == nil {
			gc = map[string]any{}
			out["generationConfig"] = gc
		}
		for key, value := range formatConfig {
			gc[key] = value
		}
	}
	if choice, ok := in["tool_choice"].(map[string]any); ok {
		fn, ok := choice["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if !ok || choice["type"] != "function" || name == "" {
			return nil, fmt.Errorf("invalid Chat tool_choice")
		}
		out["toolConfig"] = map[string]any{"functionCallingConfig": map[string]any{"mode": "ANY", "allowedFunctionNames": []string{name}}}
	}
	if stop := in["stop"]; stop != nil {
		gc, _ := out["generationConfig"].(map[string]any)
		if gc == nil {
			gc = map[string]any{}
			out["generationConfig"] = gc
		}
		if text, ok := stop.(string); ok {
			stop = []any{text}
		}
		gc["stopSequences"] = stop
	}
	return json.Marshal(out)
}
