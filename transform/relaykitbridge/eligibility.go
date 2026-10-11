package relaykitbridge

import (
	"encoding/json"
	"fmt"
	"strings"
)

type object map[string]json.RawMessage

func readObject(raw []byte) (object, error) {
	var value object
	if (numberCodec{}).Unmarshal(raw, &value) != nil || value == nil {
		return nil, fmt.Errorf("invalid JSON object: %w", ErrConversion)
	}
	return value, nil
}
func text(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}
func present(raw json.RawMessage) bool { return len(raw) > 0 && string(raw) != "null" }
func specialized(path string) error    { return fmt.Errorf("%w: %s", ErrSpecialized, path) }
func fields(value object, path string, allowed ...string) error {
	for key, raw := range value {
		if !present(raw) {
			continue
		}
		found := false
		for _, candidate := range allowed {
			if key == candidate {
				found = true
				break
			}
		}
		if !found {
			return specialized(path + "." + key)
		}
	}
	return nil
}
func array(raw json.RawMessage) ([]json.RawMessage, error) {
	var out []json.RawMessage
	if json.Unmarshal(raw, &out) != nil || out == nil {
		return nil, ErrConversion
	}
	return out, nil
}

// This is a feature classifier, not a trial conversion. It deliberately routes
// protocol-owned state and multimodal/structured output to their existing owner.
func eligibleRequest(format, target Format, raw []byte) error {
	req, err := readObject(raw)
	if err != nil {
		return err
	}
	var messages []json.RawMessage
	switch format {
	case Chat:
		err = fields(req, "request", "model", "messages", "stream", "stream_options", "max_tokens", "max_completion_tokens", "temperature", "top_p", "tools", "tool_choice", "parallel_tool_calls", "n", "store")
		if err != nil {
			return err
		}
		if present(req["n"]) && string(req["n"]) != "1" {
			return specialized("n")
		}
		if present(req["stream_options"]) {
			opts, e := readObject(req["stream_options"])
			if e != nil {
				return e
			}
			if e = fields(opts, "stream_options", "include_usage"); e != nil {
				return e
			}
		}
		messages, err = array(req["messages"])
	case Responses:
		err = fields(req, "request", "model", "input", "instructions", "stream", "max_output_tokens", "temperature", "top_p", "tools", "tool_choice", "parallel_tool_calls", "store")
		if err != nil {
			return err
		}
		if len(req["input"]) > 0 && req["input"][0] == '"' {
			var input string
			if json.Unmarshal(req["input"], &input) != nil {
				return ErrConversion
			}
		} else {
			messages, err = array(req["input"])
		}
	case Messages:
		err = fields(req, "request", "model", "messages", "system", "max_tokens", "stream", "temperature", "top_p", "tools", "tool_choice")
		if err != nil {
			return err
		}
		if present(req["system"]) {
			if err = textContent(req["system"], "system", format); err != nil {
				return err
			}
		}
		messages, err = array(req["messages"])
		if err == nil && len(messages) > 0 {
			last, _ := readObject(messages[len(messages)-1])
			if text(last["role"]) == "assistant" {
				return specialized("assistant prefill")
			}
		}
	case Gemini:
		err = fields(req, "request", "contents", "systemInstruction", "generationConfig", "tools", "toolConfig")
		if err != nil {
			return err
		}
		if present(req["generationConfig"]) {
			cfg, e := readObject(req["generationConfig"])
			if e != nil {
				return e
			}
			if e = fields(cfg, "generationConfig", "temperature", "topP", "maxOutputTokens", "candidateCount"); e != nil {
				return e
			}
			if present(cfg["candidateCount"]) && string(cfg["candidateCount"]) != "1" {
				return specialized("candidateCount")
			}
		}
		if present(req["systemInstruction"]) {
			instruction, e := readObject(req["systemInstruction"])
			if e != nil {
				return e
			}
			if e = fields(instruction, "systemInstruction", "role", "parts"); e != nil {
				return e
			}
			if e = textContent(instruction["parts"], "systemInstruction.parts", Gemini); e != nil {
				return e
			}
		}
		messages, err = array(req["contents"])
	}
	if err != nil {
		return err
	}
	if present(req["store"]) && string(req["store"]) != "false" {
		return specialized("store")
	}
	if target == Gemini && present(req["parallel_tool_calls"]) {
		return specialized("parallel_tool_calls")
	}
	if format == Messages && target != Responses && present(req["tool_choice"]) {
		return specialized("Messages tool_choice")
	}
	// The pinned Gemini request converter does not read ToolConfig.
	if format == Gemini && present(req["toolConfig"]) {
		return specialized("Gemini toolConfig")
	}
	for _, raw := range messages {
		msg, e := readObject(raw)
		if e != nil {
			return e
		}
		if e := requestRole(format, msg); e != nil {
			return e
		}
		switch format {
		case Chat:
			if e = fields(msg, "message", "role", "content", "tool_calls", "tool_call_id"); e != nil {
				return e
			}
			if present(msg["content"]) {
				if e = textContent(msg["content"], "content", format); e != nil {
					return e
				}
			}
			if present(msg["tool_calls"]) {
				calls, e := array(msg["tool_calls"])
				if e != nil {
					return e
				}
				for _, raw := range calls {
					call, e := readObject(raw)
					if e != nil {
						return e
					}
					if e = fields(call, "tool_call", "id", "type", "function"); e != nil {
						return e
					}
					if text(call["type"]) != "function" {
						return specialized("tool call type")
					}
					fn, e := readObject(call["function"])
					if e != nil {
						return e
					}
					if e = fields(fn, "function", "name", "arguments"); e != nil {
						return e
					}
					if text(call["id"]) == "" || text(fn["name"]) == "" {
						return ErrConversion
					}
					if _, e := readObject([]byte(text(fn["arguments"]))); e != nil {
						return e
					}
				}
			}
		case Responses:
			switch text(msg["type"]) {
			case "", "message":
				if e = fields(msg, "message", "type", "role", "content", "id", "status"); e != nil {
					return e
				}
				if e = textContent(msg["content"], "content", format); e != nil {
					return e
				}
			case "function_call":
				if e = fields(msg, "function_call", "type", "call_id", "name", "arguments", "id", "status"); e != nil {
					return e
				}
			case "function_call_output":
				if e = fields(msg, "function_call_output", "type", "call_id", "output", "id", "status"); e != nil {
					return e
				}
				var value string
				if json.Unmarshal(msg["output"], &value) != nil {
					return specialized("function output content")
				}
			default:
				return specialized("Responses input item")
			}
		case Messages:
			if e = fields(msg, "message", "role", "content"); e != nil {
				return e
			}
			if e = textContent(msg["content"], "content", format); e != nil {
				return e
			}
		case Gemini:
			if e = fields(msg, "content", "role", "parts"); e != nil {
				return e
			}
			if e = textContent(msg["parts"], "parts", format); e != nil {
				return e
			}
		}
	}
	return eligibleTools(format, target, req)
}

func textContent(raw json.RawMessage, path string, format Format) error {
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return nil
	}
	parts, err := array(raw)
	if err != nil {
		return specialized(path)
	}
	for _, raw := range parts {
		part, e := readObject(raw)
		if e != nil {
			return e
		}
		if format == Gemini {
			if e = fields(part, path, "text", "functionCall", "functionResponse"); e != nil {
				return e
			}
			count := 0
			for _, key := range []string{"text", "functionCall", "functionResponse"} {
				if present(part[key]) {
					count++
				}
			}
			if count != 1 {
				return ErrConversion
			}
			for _, key := range []string{"functionCall", "functionResponse"} {
				if present(part[key]) {
					fn, e := readObject(part[key])
					if e != nil {
						return e
					}
					if e = fields(fn, key, "id", "name", "args", "response"); e != nil {
						return e
					}
					if text(fn["name"]) == "" {
						return ErrConversion
					}
					payload := "args"
					if key == "functionResponse" {
						payload = "response"
					}
					if _, e := readObject(fn[payload]); e != nil {
						return e
					}
				}
			}
			continue
		}
		switch text(part["type"]) {
		case "text", "input_text", "output_text":
			if format != Responses && text(part["type"]) != "text" {
				return specialized(path + ".type")
			}
			if format == Responses && text(part["type"]) == "text" {
				return specialized(path + ".type")
			}
			if e = fields(part, path, "type", "text"); e != nil {
				return e
			}
		case "tool_use":
			if format != Messages {
				return specialized(path)
			}
			if e = fields(part, path, "type", "id", "name", "input"); e != nil {
				return e
			}
		case "tool_result":
			if format != Messages {
				return specialized(path)
			}
			if e = fields(part, path, "type", "tool_use_id", "content", "is_error"); e != nil {
				return e
			}
			if present(part["is_error"]) && string(part["is_error"]) != "false" {
				return specialized(path + ".is_error")
			}
			if present(part["content"]) {
				if e = textContent(part["content"], path+".content", Chat); e != nil {
					return e
				}
			}
		default:
			return specialized(path + ".type")
		}
	}
	return nil
}

func requestRole(format Format, msg object) error {
	role := text(msg["role"])
	if format == Responses && text(msg["type"]) != "" && text(msg["type"]) != "message" {
		return nil
	}
	switch format {
	case Chat:
		if role == "tool" {
			if text(msg["tool_call_id"]) == "" {
				return ErrConversion
			}
			return nil
		}
		if role == "system" || role == "developer" || role == "user" || role == "assistant" {
			return nil
		}
	case Responses:
		if role == "system" || role == "developer" || role == "user" || role == "assistant" {
			return nil
		}
	case Messages:
		if role == "user" || role == "assistant" {
			return nil
		}
	case Gemini:
		if role == "" || role == "user" || role == "model" {
			return nil
		}
	}
	return ErrConversion
}

func eligibleTools(format, target Format, req object) error {
	if present(req["tools"]) {
		tools, err := array(req["tools"])
		if err != nil {
			return err
		}
		for _, raw := range tools {
			tool, err := readObject(raw)
			if err != nil {
				return err
			}
			switch format {
			case Chat:
				if err = fields(tool, "tool", "type", "function"); err != nil {
					return err
				}
				if text(tool["type"]) != "function" {
					return specialized("tool.type")
				}
				tool, err = readObject(tool["function"])
				if err != nil {
					return err
				}
				if err = fields(tool, "function", "name", "description", "parameters", "strict"); err != nil {
					return err
				}
			case Responses:
				if text(tool["type"]) != "function" {
					return specialized("tool.type")
				}
				if err = fields(tool, "tool", "type", "name", "description", "parameters", "strict"); err != nil {
					return err
				}
			case Messages:
				if present(tool["type"]) && text(tool["type"]) != "custom" {
					return specialized("tool.type")
				}
				if err = fields(tool, "tool", "type", "name", "description", "input_schema"); err != nil {
					return err
				}
			case Gemini:
				if err = fields(tool, "tool", "functionDeclarations"); err != nil {
					return err
				}
				declarations, e := array(tool["functionDeclarations"])
				if e != nil {
					return e
				}
				for _, raw := range declarations {
					fn, e := readObject(raw)
					if e != nil {
						return e
					}
					if e = fields(fn, "function", "name", "description", "parameters"); e != nil {
						return e
					}
					if e = simpleToolSchema(fn["parameters"], format, target, 0); e != nil {
						return e
					}
					if text(fn["name"]) == "" {
						return ErrConversion
					}
				}
			}
			if format != Gemini {
				if text(tool["name"]) == "" {
					return ErrConversion
				}
				schema := tool["parameters"]
				if format == Messages {
					schema = tool["input_schema"]
				}
				if err := simpleToolSchema(schema, format, target, 0); err != nil {
					return err
				}
			}
			if present(tool["strict"]) && string(tool["strict"]) != "false" {
				return specialized("tool.strict")
			}
		}
	}
	if present(req["tool_choice"]) {
		var value string
		if json.Unmarshal(req["tool_choice"], &value) == nil {
			if value != "auto" && value != "none" && value != "required" {
				return specialized("tool_choice")
			}
		} else {
			choice, err := readObject(req["tool_choice"])
			if err != nil {
				return err
			}
			allowed := []string{"type", "name"}
			if format == Chat {
				allowed = []string{"type", "function"}
			}
			if format == Messages {
				allowed = []string{"type", "name", "disable_parallel_tool_use"}
			}
			if err = fields(choice, "tool_choice", allowed...); err != nil {
				return err
			}
			kind := text(choice["type"])
			if kind != "function" && kind != "tool" && kind != "auto" && kind != "any" && kind != "none" {
				return specialized("tool_choice.type")
			}
			if format == Chat {
				if kind != "function" {
					return specialized("tool_choice.type")
				}
				fn, err := readObject(choice["function"])
				if err != nil {
					return err
				}
				if err := fields(fn, "tool_choice.function", "name"); err != nil {
					return err
				}
				if text(fn["name"]) == "" {
					return ErrConversion
				}
			} else if kind == "function" || kind == "tool" {
				if text(choice["name"]) == "" {
					return ErrConversion
				}
			}
		}
	}
	// Tool schemas remain opaque JSON objects; numberCodec retains numeric
	// bounds. Response schema/format features are excluded at the top level.
	return nil
}

// Gemini's library bridge rewrites a restricted OpenAPI schema. Do not let
// that normalizer silently delete JSON Schema constraints or truncate unions.
func simpleToolSchema(raw json.RawMessage, source, target Format, depth int) error {
	if !present(raw) {
		return nil
	}
	value, err := readObject(raw)
	if err != nil {
		return err
	}
	if depth >= 32 {
		return specialized("tool schema depth")
	}
	if source != Gemini && target != Gemini {
		return nil
	}
	if err := fields(value, "tool schema", "type", "properties", "required", "description", "enum", "items", "minimum", "maximum", "minItems", "maxItems", "minLength", "maxLength", "pattern", "format", "title", "default"); err != nil {
		return err
	}
	if present(value["type"]) {
		typ := text(value["type"])
		if typ != "object" && typ != "array" && typ != "string" && typ != "integer" && typ != "number" && typ != "boolean" {
			return specialized("tool schema type")
		}
	}
	if present(value["properties"]) {
		properties, err := readObject(value["properties"])
		if err != nil {
			return err
		}
		for _, property := range properties {
			if err := simpleToolSchema(property, source, target, depth+1); err != nil {
				return err
			}
		}
	}
	if present(value["items"]) {
		if err := simpleToolSchema(value["items"], source, target, depth+1); err != nil {
			return err
		}
	}
	return nil
}

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
