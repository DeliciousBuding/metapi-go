package messages

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// ToChatResponse converts a completed native Messages response to Chat. Thinking,
// signatures, hosted tool state and citations fail explicitly rather than being
// flattened into ordinary assistant text.
func ToChatResponse(body []byte) ([]byte, error) {
	value, err := object(body, "Messages response")
	if err != nil {
		return nil, err
	}
	id, model, err := checkMessagesEnvelope(value, false)
	if err != nil {
		return nil, err
	}
	blocks, err := array(value["content"], "Messages content")
	if err != nil {
		return nil, err
	}
	var content strings.Builder
	calls := make([]wireObject, 0)
	ids := make(map[string]bool)
	for i, raw := range blocks {
		path := fmt.Sprintf("Messages content[%d]", i)
		block, err := object(raw, path)
		if err != nil {
			return nil, err
		}
		typ, err := checkMessagesBlock(block, path)
		if err != nil {
			return nil, err
		}
		switch typ {
		case "text":
			if len(calls) != 0 {
				return nil, invalid(path, "cannot reorder text after tool calls through Chat")
			}
			text, err := text(block["text"], path+".text")
			if err != nil {
				return nil, err
			}
			content.WriteString(text)
		case "tool_use":
			id, name, err := messagesToolIdentity(block, path)
			if err != nil {
				return nil, err
			}
			if ids[id] {
				return nil, invalid(path+".id", "duplicates another tool call")
			}
			ids[id] = true
			if _, err := object(block["input"], path+".input"); err != nil {
				return nil, err
			}
			calls = append(calls, wireObject{"id": id, "type": "function", "function": wireObject{"name": name, "arguments": string(block["input"])}})
		}
	}
	reason, err := chatStopReason(value, len(calls))
	if err != nil {
		return nil, err
	}
	message := wireObject{"role": "assistant", "content": content.String()}
	if len(calls) != 0 {
		message["tool_calls"] = calls
		if content.Len() == 0 {
			message["content"] = nil
		}
	}
	choice := wireObject{"index": 0, "message": message, "finish_reason": reason}
	if !absent(value["stop_sequence"]) {
		choice["stop_sequence"] = value["stop_sequence"]
	}
	out := wireObject{"id": id, "model": model, "object": "chat.completion", "created": time.Now().Unix(), "choices": []wireObject{choice}}
	usage, err := messagesUsage(value["usage"])
	if err != nil {
		return nil, err
	}
	if usage != nil {
		out["usage"] = usage
	}
	return json.Marshal(out)
}

func checkMessagesEnvelope(value rawObject, stream bool) (string, string, error) {
	if err := rejectUpstreamError(value, "Messages response"); err != nil {
		return "", "", err
	}
	if err := allowOnly(value, "Messages response", "id", "type", "role", "model", "content", "stop_reason", "stop_sequence", "usage", "container", "context_management"); err != nil {
		return "", "", err
	}
	for _, field := range []string{"container", "context_management"} {
		if !absent(value[field]) {
			return "", "", unsupported("Messages response." + field)
		}
	}
	if string(value["type"]) != `"message"` || string(value["role"]) != `"assistant"` {
		return "", "", invalid("Messages response", "must be an assistant message")
	}
	id, err := identity(value["id"], "Messages response.id")
	if err != nil {
		return "", "", err
	}
	model, err := optionalText(value["model"], "Messages response.model")
	if err != nil || !stream && strings.TrimSpace(model) == "" {
		return "", "", invalid("Messages response.model", "must be nonempty")
	}
	return id, model, nil
}

func checkMessagesBlock(block rawObject, path string) (string, error) {
	typ, err := text(block["type"], path+".type")
	if err != nil {
		return "", err
	}
	switch typ {
	case "text":
		if err := allowOnly(block, path, "type", "text", "citations"); err != nil {
			return "", err
		}
		if !absent(block["citations"]) {
			citations, err := array(block["citations"], path+".citations")
			if err != nil || len(citations) != 0 {
				return "", unsupported(path + ".citations")
			}
		}
	case "tool_use":
		if err := allowOnly(block, path, "type", "id", "name", "input"); err != nil {
			return "", err
		}
	default:
		return "", unsupported(path + ".type")
	}
	return typ, nil
}

func messagesToolIdentity(block rawObject, path string) (string, string, error) {
	id, err := identity(block["id"], path+".id")
	if err != nil {
		return "", "", err
	}
	name, err := identity(block["name"], path+".name")
	return id, name, err
}

func chatStopReason(value rawObject, tools int) (string, error) {
	reason, err := text(value["stop_reason"], "Messages stop_reason")
	if err != nil {
		return "", err
	}
	if !absent(value["stop_sequence"]) {
		if _, err := text(value["stop_sequence"], "Messages stop_sequence"); err != nil {
			return "", err
		}
		if reason != "stop_sequence" {
			return "", invalid("Messages stop_sequence", "requires the stop_sequence stop reason")
		}
	}
	switch reason {
	case "end_turn", "stop_sequence":
		if reason == "stop_sequence" && absent(value["stop_sequence"]) {
			return "", invalid("Messages stop_sequence", "must identify the matched stop sequence")
		}
		if tools != 0 {
			return "", invalid("Messages stop_reason", "does not declare the returned tool calls")
		}
		return "stop", nil
	case "max_tokens":
		return "length", nil
	case "tool_use":
		if tools == 0 {
			return "", invalid("Messages stop_reason", "declares tool use without tools")
		}
		return "tool_calls", nil
	case "refusal":
		return "content_filter", nil
	default:
		return "", unsupported("Messages stop_reason")
	}
}

// Anthropic input_tokens excludes both cache reads and writes. Chat prompt
// tokens includes them. Preserve reported cache write counts/TTL details under
// prompt_tokens_details without introducing Anthropic top-level keys (which
// consumers would add to the prompt total a second time).
func messagesUsage(raw json.RawMessage) (wireObject, error) {
	if absent(raw) {
		return nil, nil
	}
	usage, err := object(raw, "Messages usage")
	if err != nil {
		return nil, err
	}
	out, details := wireObject{}, wireObject{}
	var input, output int64
	inputKnown, outputKnown := false, false
	for _, field := range []string{"input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens"} {
		if absent(usage[field]) {
			continue
		}
		count, err := nonnegativeInt(usage[field], "Messages usage."+field)
		if err != nil {
			return nil, err
		}
		if field == "output_tokens" {
			output, outputKnown = count, true
			continue
		}
		if input > math.MaxInt64-count {
			return nil, invalid("Messages usage", "token total overflows")
		}
		input += count
		if field == "input_tokens" {
			inputKnown = true
		} else if field == "cache_read_input_tokens" {
			details["cached_tokens"] = count
		} else {
			details["cache_creation_tokens"] = count
		}
	}
	if inputKnown {
		out["prompt_tokens"] = input
	}
	if outputKnown {
		out["completion_tokens"] = output
	}
	if inputKnown && outputKnown {
		if input > math.MaxInt64-output {
			return nil, invalid("Messages usage", "token total overflows")
		}
		out["total_tokens"] = input + output
	}
	if !absent(usage["cache_creation"]) {
		cache, err := object(usage["cache_creation"], "Messages usage.cache_creation")
		if err != nil {
			return nil, err
		}
		for field, raw := range cache {
			if _, err := nonnegativeInt(raw, "Messages usage.cache_creation."+field); err != nil {
				return nil, err
			}
		}
		details["cache_creation"] = usage["cache_creation"]
	}
	if len(details) != 0 {
		out["prompt_tokens_details"] = details
	}
	return out, nil
}
