package messages

import (
	"encoding/json"
	"fmt"
)

// ToChatRequest converts a native Messages request, not an input-shaped alias.
// It rejects fields whose semantics this bridge cannot preserve.
func ToChatRequest(body []byte, options ...Options) ([]byte, error) {
	replay, err := requestOptionsValue(options)
	if err != nil {
		return nil, err
	}
	req, err := object(body, "request")
	if err != nil {
		return nil, err
	}
	if err := allowOnly(req, "request", "model", "messages", "system", "max_tokens", "stream", "tools", "tool_choice", "temperature", "top_p", "stop_sequences", "metadata", "cache_control", "thinking", "output_config", "context_management"); err != nil {
		return nil, err
	}
	model, err := identity(req["model"], "model")
	if err != nil {
		return nil, err
	}
	maxTokens, err := nonnegativeInt(req["max_tokens"], "max_tokens")
	if err != nil || maxTokens == 0 {
		return nil, invalid("max_tokens", "must be a positive integer")
	}
	out := wireObject{"model": model, "max_tokens": maxTokens}
	if err := requestOptions(req, out); err != nil {
		return nil, err
	}
	tools, names, err := requestTools(req["tools"])
	if err != nil {
		return nil, err
	}
	if len(tools) != 0 {
		out["tools"] = tools
	}
	if err := requestToolChoice(req["tool_choice"], names, out); err != nil {
		return nil, err
	}
	var transcript []wireObject
	if !absent(req["system"]) {
		content, err := textContent(req["system"], "system")
		if err != nil {
			return nil, err
		}
		transcript = append(transcript, wireObject{"role": "system", "content": content})
	}
	messages, err := array(req["messages"], "messages")
	if err != nil || len(messages) == 0 {
		return nil, invalid("messages", "must be a nonempty array")
	}
	pending := make(map[string]bool)
	usedIDs := make(map[string]bool)
	for i, raw := range messages {
		path := fmt.Sprintf("messages[%d]", i)
		msg, err := object(raw, path)
		if err != nil {
			return nil, err
		}
		if err := allowOnly(msg, path, "role", "content"); err != nil {
			return nil, err
		}
		role, err := text(msg["role"], path+".role")
		if err != nil {
			return nil, err
		}
		if role != "user" && role != "assistant" {
			return nil, unsupported(path + ".role")
		}
		if role == "assistant" && (len(pending) != 0 || i == len(messages)-1) {
			return nil, invalid(path, "cannot contain an assistant prefill or unanswered tool calls")
		}
		converted, err := requestMessage(msg["content"], role, path+".content", pending, usedIDs)
		if err != nil {
			return nil, err
		}
		if role == "assistant" {
			required := out["reasoning_effort"] != nil && out["reasoning_effort"] != "none"
			for _, message := range converted {
				if err := replay.replay(message, required); err != nil {
					return nil, err
				}
			}
		}
		transcript = append(transcript, converted...)
	}
	if len(pending) != 0 {
		return nil, invalid("messages", "must answer every tool_use before continuing")
	}
	out["messages"] = transcript
	return json.Marshal(out)
}

func requestTools(raw json.RawMessage) ([]wireObject, map[string]bool, error) {
	names := make(map[string]bool)
	var out []wireObject
	if absent(raw) {
		return out, names, nil
	}
	tools, err := array(raw, "tools")
	if err != nil {
		return nil, nil, err
	}
	for i, raw := range tools {
		path := fmt.Sprintf("tools[%d]", i)
		tool, err := object(raw, path)
		if err != nil {
			return nil, nil, err
		}
		if err := allowOnly(tool, path, "type", "name", "description", "input_schema", "cache_control"); err != nil {
			return nil, nil, err
		}
		if kind, ok := tool["type"]; ok {
			typ, err := text(kind, path+".type")
			if err != nil {
				return nil, nil, err
			}
			if typ != "custom" {
				return nil, nil, unsupported(path + ".type")
			}
		}
		name, err := identity(tool["name"], path+".name")
		if err != nil {
			return nil, nil, err
		}
		if names[name] {
			return nil, nil, invalid(path+".name", "duplicates another tool")
		}
		names[name] = true
		if _, err := object(tool["input_schema"], path+".input_schema"); err != nil {
			return nil, nil, err
		}
		function := wireObject{"name": name, "parameters": tool["input_schema"]}
		if description, ok := tool["description"]; ok {
			value, err := text(description, path+".description")
			if err != nil {
				return nil, nil, err
			}
			function["description"] = value
		}
		if err := hint(tool["cache_control"], path+".cache_control"); err != nil {
			return nil, nil, err
		}
		out = append(out, wireObject{"type": "function", "function": function})
	}
	return out, names, nil
}

func requestToolChoice(raw json.RawMessage, names map[string]bool, out wireObject) error {
	if absent(raw) {
		return nil
	}
	choice, err := object(raw, "tool_choice")
	if err != nil {
		return err
	}
	typ, err := text(choice["type"], "tool_choice.type")
	if err != nil {
		return err
	}
	fields := []string{"type"}
	if typ == "tool" {
		fields = append(fields, "name")
	}
	if typ != "none" {
		fields = append(fields, "disable_parallel_tool_use")
	}
	if err := allowOnly(choice, "tool_choice", fields...); err != nil {
		return err
	}
	switch typ {
	case "auto", "any", "tool":
		if len(names) == 0 {
			return invalid("tool_choice", "requires tools")
		}
		if typ == "tool" {
			name, err := identity(choice["name"], "tool_choice.name")
			if err != nil {
				return err
			}
			if !names[name] {
				return invalid("tool_choice.name", "must name a declared tool")
			}
			out["tool_choice"] = wireObject{"type": "function", "function": wireObject{"name": name}}
		} else if typ == "any" {
			out["tool_choice"] = "required"
		} else {
			out["tool_choice"] = "auto"
		}
	case "none":
		out["tool_choice"] = "none"
	default:
		return unsupported("tool_choice.type")
	}
	if raw, ok := choice["disable_parallel_tool_use"]; ok {
		disabled, err := boolean(raw, "tool_choice.disable_parallel_tool_use")
		if err != nil {
			return err
		}
		out["parallel_tool_calls"] = !disabled
	}
	return nil
}

// Text arrays stay arrays: do not inject separators or stringify their JSON.
func textContent(raw json.RawMessage, path string) (any, error) {
	if value, err := text(raw, path); err == nil {
		return value, nil
	}
	parts, err := array(raw, path)
	if err != nil {
		return nil, invalid(path, "must be a string or text block array")
	}
	out := make([]wireObject, 0, len(parts))
	for i, raw := range parts {
		partPath := fmt.Sprintf("%s[%d]", path, i)
		block, err := object(raw, partPath)
		if err != nil {
			return nil, err
		}
		part, err := textBlock(block, partPath)
		if err != nil {
			return nil, err
		}
		out = append(out, part)
	}
	return out, nil
}

func textBlock(block rawObject, path string) (wireObject, error) {
	if err := allowOnly(block, path, "type", "text", "cache_control"); err != nil {
		return nil, err
	}
	typ, err := text(block["type"], path+".type")
	if err != nil {
		return nil, err
	}
	if typ != "text" {
		return nil, unsupported(path + ".type")
	}
	value, err := text(block["text"], path+".text")
	if err != nil {
		return nil, err
	}
	if err := hint(block["cache_control"], path+".cache_control"); err != nil {
		return nil, err
	}
	return wireObject{"type": "text", "text": value}, nil
}

func requestMessage(raw json.RawMessage, role, path string, pending, usedIDs map[string]bool) ([]wireObject, error) {
	if value, err := text(raw, path); err == nil {
		if len(pending) != 0 {
			return nil, invalid(path, "must answer pending tool_use IDs before text")
		}
		return []wireObject{{"role": role, "content": value}}, nil
	}
	blocks, err := array(raw, path)
	if err != nil || len(blocks) == 0 {
		return nil, invalid(path, "must be a string or nonempty content block array")
	}
	var out, texts, calls []wireObject
	for i, raw := range blocks {
		blockPath := fmt.Sprintf("%s[%d]", path, i)
		block, err := object(raw, blockPath)
		if err != nil {
			return nil, err
		}
		typ, err := text(block["type"], blockPath+".type")
		if err != nil {
			return nil, err
		}
		switch typ {
		case "text":
			if len(calls) != 0 || (role == "user" && len(pending) != 0) {
				return nil, invalid(blockPath, "cannot reorder text across tool calls or unanswered results")
			}
			part, err := textBlock(block, blockPath)
			if err != nil {
				return nil, err
			}
			texts = append(texts, part)
		case "tool_use":
			if role != "assistant" {
				return nil, invalid(blockPath, "requires the assistant role")
			}
			if err := allowOnly(block, blockPath, "type", "id", "name", "input", "cache_control"); err != nil {
				return nil, err
			}
			id, err := identity(block["id"], blockPath+".id")
			if err != nil {
				return nil, err
			}
			name, err := identity(block["name"], blockPath+".name")
			if err != nil {
				return nil, err
			}
			if usedIDs[id] {
				return nil, invalid(blockPath+".id", "duplicates a tool_use ID")
			}
			if _, err := object(block["input"], blockPath+".input"); err != nil {
				return nil, err
			}
			if err := hint(block["cache_control"], blockPath+".cache_control"); err != nil {
				return nil, err
			}
			usedIDs[id], pending[id] = true, true
			calls = append(calls, wireObject{"id": id, "type": "function", "function": wireObject{"name": name, "arguments": string(block["input"])}})
		case "tool_result":
			if role != "user" || len(texts) != 0 {
				return nil, invalid(blockPath, "must precede user text and answer an assistant tool_use")
			}
			if err := allowOnly(block, blockPath, "type", "tool_use_id", "content", "is_error", "cache_control"); err != nil {
				return nil, err
			}
			id, err := identity(block["tool_use_id"], blockPath+".tool_use_id")
			if err != nil {
				return nil, err
			}
			if !pending[id] {
				return nil, invalid(blockPath+".tool_use_id", "must match an unanswered tool_use ID")
			}
			if raw, ok := block["is_error"]; ok {
				isError, err := boolean(raw, blockPath+".is_error")
				if err != nil {
					return nil, err
				}
				if isError {
					return nil, unsupported(blockPath + ".is_error=true")
				}
			}
			var content any = ""
			if raw, ok := block["content"]; ok {
				content, err = textContent(raw, blockPath+".content")
				if err != nil {
					return nil, err
				}
			}
			if err := hint(block["cache_control"], blockPath+".cache_control"); err != nil {
				return nil, err
			}
			delete(pending, id)
			out = append(out, wireObject{"role": "tool", "tool_call_id": id, "content": content})
		default:
			return nil, unsupported(blockPath + ".type")
		}
	}
	if len(texts) != 0 || len(calls) != 0 {
		msg := wireObject{"role": role, "content": nil}
		if len(texts) != 0 {
			msg["content"] = texts
		}
		if len(calls) != 0 {
			msg["tool_calls"] = calls
		}
		out = append(out, msg)
	}
	return out, nil
}
