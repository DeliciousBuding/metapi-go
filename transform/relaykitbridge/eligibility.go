package relaykitbridge

import (
	"encoding/json"
	"fmt"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

type object map[string]json.RawMessage

func readObject(raw []byte) (object, error) {
	var value object
	if (numberCodec{}).Unmarshal(raw, &value) != nil || value == nil {
		return nil, fmt.Errorf("invalid JSON object: %w", ErrConversion)
	}
	return value, nil
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
			return specialized(path + " field")
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
func eligibleRequest(format, target Format, req object) error {
	var err error
	if cache, ok := req["prompt_cache_key"]; ok {
		if !(format == Chat && target == Responses || format == Responses && target == Chat) || len(cache) == 0 || cache[0] != '"' {
			return specialized("prompt_cache_key")
		}
		delete(req, "prompt_cache_key") // classified above; the converter receives the original bytes
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
			if shared.RawString(last["role"]) == "assistant" {
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
					if shared.RawString(call["type"]) != "function" {
						return specialized("tool call type")
					}
					fn, e := readObject(call["function"])
					if e != nil {
						return e
					}
					if e = fields(fn, "function", "name", "arguments"); e != nil {
						return e
					}
					if shared.RawString(call["id"]) == "" || shared.RawString(fn["name"]) == "" {
						return ErrConversion
					}
					if _, e := readObject([]byte(shared.RawString(fn["arguments"]))); e != nil {
						return e
					}
				}
			}
		case Responses:
			switch shared.RawString(msg["type"]) {
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
					if shared.RawString(fn["name"]) == "" {
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
		switch shared.RawString(part["type"]) {
		case "text", "input_text", "output_text":
			if format != Responses && shared.RawString(part["type"]) != "text" {
				return specialized(path + ".type")
			}
			if format == Responses && shared.RawString(part["type"]) == "text" {
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
	role := shared.RawString(msg["role"])
	if format == Responses && shared.RawString(msg["type"]) != "" && shared.RawString(msg["type"]) != "message" {
		return nil
	}
	switch format {
	case Chat:
		if role == "tool" {
			if shared.RawString(msg["tool_call_id"]) == "" {
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
