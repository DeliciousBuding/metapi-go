package messages

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"unicode"
)

// FromChatRequest converts Chat text, images and client function tools to a
// Messages request. Messages requires an output limit; absent Chat limits use
// 4096 tokens. Signed thinking and continuation state have no portable Chat
// representation, so this direction does not enable thinking implicitly.
func FromChatRequest(body []byte) ([]byte, error) {
	req, err := object(body, "Chat request")
	if err != nil {
		return nil, err
	}
	if raw, exists := req["store"]; exists {
		value, err := boolean(raw, "Chat store")
		if err != nil || value {
			return nil, invalid("Chat store", "stored responses require a native endpoint")
		}
		delete(req, "store")
	}
	if err := allowOnly(req, "Chat request", "model", "messages", "max_tokens", "max_completion_tokens", "stream", "stream_options", "temperature", "top_p", "stop", "tools", "tool_choice", "parallel_tool_calls", "n", "user", "reasoning_effort", "cache_control"); err != nil {
		return nil, err
	}
	model, err := identity(req["model"], "Chat model")
	if err != nil {
		return nil, err
	}
	out := wireObject{"model": model, "max_tokens": int64(4096)}
	if err := fromChatOptions(req, out); err != nil {
		return nil, err
	}
	if err := fromChatTools(req, out); err != nil {
		return nil, err
	}
	input, err := array(req["messages"], "Chat messages")
	if err != nil || len(input) == 0 {
		return nil, invalid("Chat messages", "must be a nonempty array")
	}
	var transcript, system []wireObject
	pending, usedIDs := make(map[string]bool), make(map[string]bool)
	for i, raw := range input {
		path := fmt.Sprintf("Chat messages[%d]", i)
		msg, err := object(raw, path)
		if err != nil {
			return nil, err
		}
		if err := allowOnly(msg, path, "role", "content", "tool_calls", "tool_call_id"); err != nil {
			return nil, err
		}
		role, err := text(msg["role"], path+".role")
		if err != nil {
			return nil, err
		}
		if role != "assistant" && !absent(msg["tool_calls"]) || role != "tool" && !absent(msg["tool_call_id"]) {
			return nil, invalid(path, "has tool fields on the wrong role")
		}
		if role != "tool" && len(pending) != 0 {
			return nil, invalid(path, "must answer pending tool calls before continuing")
		}
		parts, err := fromChatContent(msg["content"], path+".content", role == "user" || role == "tool")
		if err != nil {
			return nil, err
		}
		switch role {
		case "system", "developer":
			if len(transcript) != 0 {
				return nil, invalid(path, "cannot move system instructions across conversation turns")
			}
			system = append(system, parts...)
			continue
		case "user":
		case "assistant":
			if i == len(input)-1 {
				return nil, invalid(path, "cannot preserve assistant prefill semantics")
			}
			if !absent(msg["tool_calls"]) {
				calls, err := array(msg["tool_calls"], path+".tool_calls")
				if err != nil {
					return nil, err
				}
				for j, raw := range calls {
					call, err := completeTool(raw, fmt.Sprintf("%s.tool_calls[%d]", path, j))
					if err != nil {
						return nil, err
					}
					id := call["id"].(string)
					if usedIDs[id] {
						return nil, invalid(path+".tool_calls", "duplicates a tool call ID")
					}
					usedIDs[id], pending[id] = true, true
					parts = append(parts, call)
				}
			}
		case "tool":
			id, err := identity(msg["tool_call_id"], path+".tool_call_id")
			if err != nil {
				return nil, err
			}
			if !pending[id] {
				return nil, invalid(path+".tool_call_id", "must answer an unanswered tool call")
			}
			delete(pending, id)
			parts = []wireObject{{"type": "tool_result", "tool_use_id": id, "content": parts}}
			role = "user"
		default:
			return nil, unsupported(path + ".role")
		}
		if len(parts) == 0 {
			return nil, invalid(path+".content", "must contain text, images or tool calls")
		}
		// Messages represents consecutive tool results and the next user text in
		// one user turn, with tool_result blocks before ordinary content.
		if len(transcript) > 0 && transcript[len(transcript)-1]["role"] == role {
			last := transcript[len(transcript)-1]
			last["content"] = append(last["content"].([]wireObject), parts...)
		} else {
			transcript = append(transcript, wireObject{"role": role, "content": parts})
		}
	}
	if len(pending) != 0 || len(transcript) == 0 || transcript[0]["role"] != "user" {
		return nil, invalid("Chat messages", "must start with a user turn and answer every tool call")
	}
	if len(system) != 0 {
		out["system"] = system
	}
	out["messages"] = transcript
	return json.Marshal(out)
}

func fromChatOptions(req rawObject, out wireObject) error {
	var limit int64
	for _, field := range []string{"max_tokens", "max_completion_tokens"} {
		if absent(req[field]) {
			continue
		}
		n, err := nonnegativeInt(req[field], "Chat "+field)
		if err != nil || n == 0 || limit != 0 && limit != n {
			return invalid("Chat "+field, "must be positive and agree with other output limits")
		}
		limit, out["max_tokens"] = n, n
	}
	for _, field := range []string{"temperature", "top_p"} {
		if absent(req[field]) {
			continue
		}
		var n float64
		if json.Unmarshal(req[field], &n) != nil || n < 0 || n > 1 {
			return invalid("Chat "+field, "must be between 0 and 1 for Messages")
		}
		out[field] = req[field]
	}
	if !absent(req["n"]) {
		n, err := nonnegativeInt(req["n"], "Chat n")
		if err != nil || n != 1 {
			return invalid("Chat n", "must be one for Messages")
		}
	}
	if !absent(req["stream"]) {
		value, err := boolean(req["stream"], "Chat stream")
		if err != nil {
			return err
		}
		out["stream"] = value
	}
	if !absent(req["stream_options"]) {
		options, err := object(req["stream_options"], "Chat stream_options")
		if err != nil {
			return err
		}
		if err := allowOnly(options, "Chat stream_options", "include_usage"); err != nil {
			return err
		}
		if _, ok := options["include_usage"]; ok {
			if _, err := boolean(options["include_usage"], "Chat stream_options.include_usage"); err != nil {
				return err
			}
		}
	}
	if !absent(req["stop"]) {
		stops := []string{}
		if value, err := text(req["stop"], "Chat stop"); err == nil {
			stops = append(stops, value)
		} else {
			values, err := array(req["stop"], "Chat stop")
			if err != nil {
				return err
			}
			for _, raw := range values {
				value, err := text(raw, "Chat stop[]")
				if err != nil {
					return err
				}
				stops = append(stops, value)
			}
		}
		for _, stop := range stops {
			if stop == "" {
				return invalid("Chat stop", "must not contain empty stop sequences")
			}
		}
		out["stop_sequences"] = stops
	}
	if !absent(req["reasoning_effort"]) {
		effort, err := text(req["reasoning_effort"], "Chat reasoning_effort")
		if err != nil || effort != "none" {
			return invalid("Chat reasoning_effort", "cannot preserve signed Messages thinking through Chat")
		}
		out["thinking"] = wireObject{"type": "disabled"}
	}
	if !absent(req["user"]) {
		user, err := text(req["user"], "Chat user")
		if err != nil {
			return err
		}
		out["metadata"] = wireObject{"user_id": user}
	}
	return copyCacheControl(req, out, "Chat request")
}

func copyCacheControl(from rawObject, to wireObject, path string) error {
	if !absent(from["cache_control"]) {
		if err := hint(from["cache_control"], path+".cache_control"); err != nil {
			return err
		}
		to["cache_control"] = from["cache_control"]
	}
	return nil
}

func fromChatContent(raw json.RawMessage, path string, images bool) ([]wireObject, error) {
	if absent(raw) {
		return []wireObject{}, nil
	}
	if value, err := text(raw, path); err == nil {
		return []wireObject{{"type": "text", "text": value}}, nil
	}
	parts, err := array(raw, path)
	if err != nil {
		return nil, err
	}
	out := make([]wireObject, 0, len(parts))
	for i, raw := range parts {
		path := fmt.Sprintf("%s[%d]", path, i)
		part, err := object(raw, path)
		if err != nil {
			return nil, err
		}
		typ, err := text(part["type"], path+".type")
		if err != nil {
			return nil, err
		}
		var block wireObject
		switch typ {
		case "text":
			block, err = textBlock(part, path)
		case "image_url":
			if !images {
				return nil, invalid(path, "images require a user or tool role")
			}
			if err := allowOnly(part, path, "type", "image_url", "cache_control"); err != nil {
				return nil, err
			}
			block, err = fromChatImage(part["image_url"], path+".image_url")
		default:
			return nil, unsupported(path + ".type")
		}
		if err != nil {
			return nil, err
		}
		if err := copyCacheControl(part, block, path); err != nil {
			return nil, err
		}
		out = append(out, block)
	}
	return out, nil
}

func fromChatImage(raw json.RawMessage, path string) (wireObject, error) {
	image, err := object(raw, path)
	if err != nil {
		return nil, err
	}
	if err := allowOnly(image, path, "url", "detail"); err != nil {
		return nil, err
	}
	if !absent(image["detail"]) {
		detail, err := text(image["detail"], path+".detail")
		if err != nil || detail != "auto" {
			return nil, invalid(path+".detail", "cannot preserve an explicit Chat image detail level")
		}
	}
	location, err := identity(image["url"], path+".url")
	if err != nil {
		return nil, err
	}
	source, err := imageSourceForURL(location, path+".url")
	if err != nil {
		return nil, err
	}
	return wireObject{"type": "image", "source": source}, nil
}

func imageSourceForURL(location, path string) (wireObject, error) {
	var source wireObject
	if strings.HasPrefix(location, "data:") {
		header, data, ok := strings.Cut(strings.TrimPrefix(location, "data:"), ",")
		media, encoded := strings.CutSuffix(header, ";base64")
		if !ok || !encoded || data == "" {
			return nil, invalid(path, "must contain base64 image data")
		}
		switch media {
		case "image/jpeg", "image/png", "image/gif", "image/webp":
		default:
			return nil, unsupported(path + " media type")
		}
		if size, err := io.Copy(io.Discard, base64.NewDecoder(base64.StdEncoding.Strict(), strings.NewReader(data))); err != nil || size == 0 {
			return nil, invalid(path, "contains invalid base64")
		}
		source = wireObject{"type": "base64", "media_type": media, "data": data}
	} else {
		parsed, err := url.Parse(location)
		if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || strings.Contains(location, "\\") || strings.IndexFunc(location, unicode.IsSpace) >= 0 {
			return nil, invalid(path, "must be an HTTP(S) image URL without embedded credentials or base64 data URL")
		}
		source = wireObject{"type": "url", "url": location}
	}
	return source, nil
}

func fromChatTools(req rawObject, out wireObject) error {
	names := make(map[string]bool)
	if !absent(req["tools"]) {
		tools, err := array(req["tools"], "Chat tools")
		if err != nil {
			return err
		}
		converted := make([]wireObject, 0, len(tools))
		for i, raw := range tools {
			path := fmt.Sprintf("Chat tools[%d]", i)
			tool, err := object(raw, path)
			if err != nil {
				return err
			}
			if err := allowOnly(tool, path, "type", "function", "cache_control"); err != nil {
				return err
			}
			if string(tool["type"]) != `"function"` {
				return unsupported(path + ".type")
			}
			function, err := object(tool["function"], path+".function")
			if err != nil {
				return err
			}
			if err := allowOnly(function, path+".function", "name", "description", "parameters", "strict"); err != nil {
				return err
			}
			name, err := identity(function["name"], path+".function.name")
			if err != nil || names[name] {
				return invalid(path+".function.name", "must be nonempty and unique")
			}
			names[name] = true
			entry := wireObject{"name": name, "input_schema": wireObject{"type": "object"}}
			if !absent(function["parameters"]) {
				if _, err := object(function["parameters"], path+".function.parameters"); err != nil {
					return err
				}
				entry["input_schema"] = function["parameters"]
			}
			if !absent(function["description"]) {
				value, err := text(function["description"], path+".function.description")
				if err != nil {
					return err
				}
				entry["description"] = value
			}
			if !absent(function["strict"]) {
				strict, err := boolean(function["strict"], path+".function.strict")
				if err != nil || strict {
					return invalid(path+".function.strict", "cannot guarantee strict schema enforcement")
				}
			}
			if err := copyCacheControl(tool, entry, path); err != nil {
				return err
			}
			converted = append(converted, entry)
		}
		out["tools"] = converted
	}
	choice := wireObject{"type": "auto"}
	if !absent(req["tool_choice"]) {
		if value, err := text(req["tool_choice"], "Chat tool_choice"); err == nil {
			switch value {
			case "auto", "none":
				choice["type"] = value
			case "required":
				choice["type"] = "any"
			default:
				return unsupported("Chat tool_choice")
			}
		} else {
			value, err := object(req["tool_choice"], "Chat tool_choice")
			if err != nil {
				return err
			}
			if err := allowOnly(value, "Chat tool_choice", "type", "function"); err != nil {
				return err
			}
			function, err := object(value["function"], "Chat tool_choice.function")
			if err != nil {
				return err
			}
			if err := allowOnly(function, "Chat tool_choice.function", "name"); err != nil {
				return err
			}
			name, err := identity(function["name"], "Chat tool_choice.function.name")
			if err != nil || string(value["type"]) != `"function"` || !names[name] {
				return invalid("Chat tool_choice", "must name a declared function tool")
			}
			choice = wireObject{"type": "tool", "name": name}
		}
	}
	if !absent(req["parallel_tool_calls"]) {
		parallel, err := boolean(req["parallel_tool_calls"], "Chat parallel_tool_calls")
		if err != nil {
			return err
		}
		if choice["type"] != "none" {
			choice["disable_parallel_tool_use"] = !parallel
		}
	}
	if len(names) != 0 {
		out["tool_choice"] = choice
	} else if choice["type"] == "any" || choice["type"] == "tool" {
		return invalid("Chat tool_choice", "requires tools")
	}
	return nil
}
