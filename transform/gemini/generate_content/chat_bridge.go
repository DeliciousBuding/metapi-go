package generate_content

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// ToChatRequest maps native generation input to the shared Chat representation.
// Provider-only resources and safety policies require a native endpoint.
func ToChatRequest(body []byte, model string) ([]byte, error) {
	in, err := bridgeObject(body)
	if err != nil {
		return nil, err
	}
	if err := bridgeKeys(in, "contents", "systemInstruction", "generationConfig", "tools", "toolConfig", "model", "stream"); err != nil {
		return nil, err
	}
	out := map[string]any{"model": model}
	if in["stream"] != nil {
		out["stream"] = in["stream"]
	}
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
	out["messages"] = messages
	if gc, ok := in["generationConfig"].(map[string]any); ok {
		if err := bridgeKeys(gc, "temperature", "topP", "maxOutputTokens", "stopSequences", "candidateCount", "responseMimeType", "responseSchema", "thinkingConfig"); err != nil {
			return nil, err
		}
		for from, to := range map[string]string{"temperature": "temperature", "topP": "top_p", "maxOutputTokens": "max_tokens", "stopSequences": "stop"} {
			if gc[from] != nil {
				out[to] = gc[from]
			}
		}
		if n, ok := gc["candidateCount"]; ok && bridgeNumber(n) != 1 {
			return nil, fmt.Errorf("Gemini candidateCount must be one for conversion")
		}
		if gc["thinkingConfig"] != nil {
			return nil, fmt.Errorf("Gemini thinkingConfig requires a native endpoint")
		}
		if mime, _ := gc["responseMimeType"].(string); mime != "" && mime != "text/plain" {
			if mime != "application/json" {
				return nil, fmt.Errorf("unsupported Gemini responseMimeType")
			}
			if gc["responseSchema"] != nil {
				return nil, fmt.Errorf("Gemini responseSchema requires a native endpoint")
			}
			out["response_format"] = map[string]any{"type": "json_object"}
		}
	}
	if raw, ok := in["tools"].([]any); ok {
		var tools []any
		for _, t := range raw {
			tool, ok := t.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Gemini tool")
			}
			if err := bridgeKeys(tool, "functionDeclarations"); err != nil {
				return nil, err
			}
			decls, ok := tool["functionDeclarations"].([]any)
			if !ok {
				return nil, fmt.Errorf("Gemini tool requires functionDeclarations")
			}
			for _, d := range decls {
				fn, ok := d.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini function declaration")
				}
				if err := bridgeKeys(fn, "name", "description", "parameters", "parametersJsonSchema"); err != nil {
					return nil, err
				}
				copy := map[string]any{}
				for k, v := range fn {
					copy[k] = v
				}
				if v := copy["parametersJsonSchema"]; v != nil {
					copy["parameters"] = v
					delete(copy, "parametersJsonSchema")
				} else if schema := copy["parameters"]; schema != nil {
					converted, err := geminiSchemaToJSON(schema)
					if err != nil {
						return nil, err
					}
					copy["parameters"] = converted
				}
				tools = append(tools, map[string]any{"type": "function", "function": copy})
			}
		}
		out["tools"] = tools
	}
	if tc, ok := in["toolConfig"].(map[string]any); ok {
		if err := bridgeKeys(tc, "functionCallingConfig"); err != nil {
			return nil, err
		}
		fc, ok := tc["functionCallingConfig"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Gemini functionCallingConfig")
		}
		if err := bridgeKeys(fc, "mode", "allowedFunctionNames"); err != nil {
			return nil, err
		}
		mode, _ := fc["mode"].(string)
		names, _ := fc["allowedFunctionNames"].([]any)
		if len(names) > 1 {
			return nil, fmt.Errorf("Gemini allowedFunctionNames subset requires native endpoint")
		}
		switch mode {
		case "", "AUTO":
			out["tool_choice"] = "auto"
		case "NONE":
			out["tool_choice"] = "none"
		case "ANY":
			out["tool_choice"] = "required"
		default:
			return nil, fmt.Errorf("unsupported Gemini function calling mode")
		}
		if len(names) == 1 {
			if mode != "ANY" {
				return nil, fmt.Errorf("Gemini allowedFunctionNames requires ANY mode")
			}
			out["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": names[0]}}
		}
	}
	return json.Marshal(out)
}

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
	if in["response_format"] != nil {
		return nil, fmt.Errorf("Chat response_format requires a native endpoint")
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
	arguments := map[string]map[string]any{}
	results := map[string]map[string]any{}
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
			results[id] = obj
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
				parsed, err := bridgeObject([]byte(args))
				if err != nil {
					return nil, fmt.Errorf("Chat tool arguments must be a JSON object")
				}
				id, _ := call["id"].(string)
				if id == "" || arguments[id] != nil {
					return nil, fmt.Errorf("Chat tool identity missing or duplicate")
				}
				arguments[id] = parsed
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
	// Preserve JSON integer arguments and object tool results across the legacy builder.
	if contents, ok := out["contents"].([]map[string]any); ok {
		for _, content := range contents {
			parts, _ := content["parts"].([]map[string]any)
			for _, part := range parts {
				if fc, ok := part["functionCall"].(map[string]any); ok {
					id, _ := fc["id"].(string)
					if args := arguments[id]; args != nil {
						fc["args"] = args
					}
				}
				if fr, ok := part["functionResponse"].(map[string]any); ok {
					id, _ := fr["id"].(string)
					if result := results[id]; result != nil {
						fr["response"] = result
					}
				}
			}
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

func bridgeObject(body []byte) (map[string]any, error) {
	var out map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&out) != nil || out == nil || !json.Valid(body) {
		return nil, fmt.Errorf("protocol body must be a JSON object")
	}
	return out, nil
}

func geminiSchemaToJSON(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, entry := range v {
			if key == "properties" {
				properties, ok := entry.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini schema properties")
				}
				converted := map[string]any{}
				for name, schema := range properties {
					value, err := geminiSchemaToJSON(schema)
					if err != nil {
						return nil, err
					}
					converted[name] = value
				}
				out[key] = converted
				continue
			}
			if key == "type" {
				name, ok := entry.(string)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini schema type")
				}
				out[key] = strings.ToLower(name)
				continue
			}
			if key == "nullable" {
				continue
			}
			converted, err := geminiSchemaToJSON(entry)
			if err != nil {
				return nil, err
			}
			out[key] = converted
		}
		if v["nullable"] == true {
			if typ, ok := out["type"].(string); ok {
				out["type"] = []any{typ, "null"}
			}
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, entry := range v {
			converted, err := geminiSchemaToJSON(entry)
			if err != nil {
				return nil, err
			}
			out[i] = converted
		}
		return out, nil
	default:
		return value, nil
	}
}
func bridgeKeys(obj map[string]any, allowed ...string) error {
	for key, value := range obj {
		if value == nil {
			continue
		}
		found := false
		for _, a := range allowed {
			if a == key {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("field %s requires a native protocol endpoint", key)
		}
	}
	return nil
}
func bridgeID(prefix string) string {
	var data [12]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(data[:])
}
