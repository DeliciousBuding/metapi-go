package messages

import (
	"encoding/json"
	"fmt"
)

// FromChatResponse converts one completed assistant choice. It never repairs
// malformed arguments into an empty object or turns an upstream error into text.
func FromChatResponse(body []byte, options ...Options) ([]byte, error) {
	replay, err := requestOptionsValue(options)
	if err != nil {
		return nil, err
	}
	envelope, err := object(body, "Chat response")
	if err != nil {
		return nil, err
	}
	if err := rejectUpstreamError(envelope, "Chat response"); err != nil {
		return nil, err
	}
	if err := checkChatObject(envelope, "chat.completion"); err != nil {
		return nil, err
	}
	id, err := identity(envelope["id"], "Chat response.id")
	if err != nil {
		return nil, err
	}
	model, err := identity(envelope["model"], "Chat response.model")
	if err != nil {
		return nil, err
	}
	choice, err := singleChoice(envelope["choices"])
	if err != nil {
		return nil, err
	}
	message, err := object(choice["message"], "Chat message")
	if err != nil {
		return nil, err
	}
	if err := checkChatContent(message, false); err != nil {
		return nil, err
	}
	content := make([]wireObject, 0)
	hasText := !absent(message["content"])
	if hasText {
		value, err := text(message["content"], "Chat message.content")
		if err != nil {
			return nil, err
		}
		if value != "" {
			content = append(content, wireObject{"type": "text", "text": value})
		}
	}
	toolCount := 0
	var toolIDs []string
	if !absent(message["tool_calls"]) {
		tools, err := array(message["tool_calls"], "Chat message.tool_calls")
		if err != nil {
			return nil, err
		}
		ids := make(map[string]bool)
		outputIDs := make(map[string]bool)
		for i, raw := range tools {
			path := fmt.Sprintf("Chat message.tool_calls[%d]", i)
			tool, err := completeTool(raw, path)
			if err != nil {
				return nil, err
			}
			id := tool["id"].(string)
			if ids[id] {
				return nil, invalid(path+".id", "duplicates another tool call")
			}
			ids[id] = true
			outputID, err := replay.toolUseID(id)
			if err != nil {
				return nil, err
			}
			if outputIDs[outputID] {
				return nil, invalid("tool_use.id", "allocator returned duplicate identities")
			}
			outputIDs[outputID] = true
			tool["id"] = outputID
			toolIDs = append(toolIDs, outputID)
			content = append(content, tool)
		}
		toolCount = len(tools)
	}
	if len(content) == 0 {
		if !hasText {
			return nil, invalid("Chat message", "contains no supported assistant content")
		}
		content = append(content, wireObject{"type": "text", "text": ""})
	}
	reason, err := text(choice["finish_reason"], "Chat finish_reason")
	if err != nil {
		return nil, err
	}
	stopReason, err := nativeStopReason(reason, toolCount)
	if err != nil {
		return nil, err
	}
	usage, err := readUsage(envelope["usage"])
	if err != nil {
		return nil, err
	}
	counts, err := usage.native()
	if err != nil {
		return nil, err
	}
	out := nativeMessage(id, model, counts)
	out["content"], out["stop_reason"] = content, stopReason
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	reasoning, err := optionalText(message["reasoning_content"], "Chat reasoning_content")
	if err != nil {
		return nil, err
	}
	if err := replay.remember(toolIDs, reasoning); err != nil {
		return nil, err
	}
	return encoded, nil
}

func nativeMessage(id, model string, usage map[string]int64) wireObject {
	return wireObject{
		"id": id, "type": "message", "role": "assistant", "model": model,
		"content": []wireObject{}, "stop_reason": nil, "stop_sequence": nil, "usage": usage,
	}
}

func rejectUpstreamError(value rawObject, path string) error {
	if !absent(value["error"]) || string(value["type"]) == `"error"` {
		// Do not include upstream message/body contents: they can contain prompts
		// or credentials. HTTP error classification belongs to the caller.
		return invalid(path, "contains an upstream error")
	}
	return nil
}

func checkChatObject(value rawObject, want string) error {
	if raw, ok := value["object"]; ok {
		got, err := text(raw, "Chat object")
		if err != nil {
			return err
		}
		if got != want {
			return invalid("Chat object", "does not match the expected completion format")
		}
	}
	return nil
}

func singleChoice(raw json.RawMessage) (rawObject, error) {
	choices, err := array(raw, "Chat choices")
	if err != nil || len(choices) != 1 {
		return nil, invalid("Chat choices", "must contain exactly one choice")
	}
	choice, err := object(choices[0], "Chat choice")
	if err != nil {
		return nil, err
	}
	if err := rejectUpstreamError(choice, "Chat choice"); err != nil {
		return nil, err
	}
	if raw, ok := choice["index"]; ok {
		index, err := nonnegativeInt(raw, "Chat choice.index")
		if err != nil || index != 0 {
			return nil, invalid("Chat choice.index", "must be zero")
		}
	}
	return choice, nil
}

func checkChatContent(value rawObject, stream bool) error {
	if err := rejectUpstreamError(value, "Chat content"); err != nil {
		return err
	}
	if err := allowOnly(value, "Chat content", "role", "content", "tool_calls", "reasoning_content", "refusal", "annotations", "audio", "function_call"); err != nil {
		return err
	}
	role, err := optionalText(value["role"], "Chat content.role")
	if err != nil {
		return err
	}
	if role != "assistant" && (!stream || role != "") {
		return invalid("Chat content.role", "must be assistant")
	}
	if _, err := optionalText(value["reasoning_content"], "Chat reasoning_content"); err != nil {
		return err
	}
	refusal, err := optionalText(value["refusal"], "Chat refusal")
	if err != nil {
		return err
	}
	if refusal != "" {
		return unsupported("Chat refusal")
	}
	for _, field := range []string{"audio", "function_call"} {
		if !absent(value[field]) {
			return unsupported("Chat " + field)
		}
	}
	if !absent(value["annotations"]) {
		annotations, err := array(value["annotations"], "Chat annotations")
		if err != nil {
			return err
		}
		if len(annotations) != 0 {
			return unsupported("Chat annotations")
		}
	}
	return nil
}

func completeTool(raw json.RawMessage, path string) (wireObject, error) {
	tool, err := object(raw, path)
	if err != nil {
		return nil, err
	}
	if err := allowOnly(tool, path, "id", "type", "function"); err != nil {
		return nil, err
	}
	typ, err := text(tool["type"], path+".type")
	if err != nil {
		return nil, err
	}
	if typ != "function" {
		return nil, unsupported(path + ".type")
	}
	id, err := identity(tool["id"], path+".id")
	if err != nil {
		return nil, err
	}
	function, err := object(tool["function"], path+".function")
	if err != nil {
		return nil, err
	}
	if err := allowOnly(function, path+".function", "name", "arguments"); err != nil {
		return nil, err
	}
	name, err := identity(function["name"], path+".function.name")
	if err != nil {
		return nil, err
	}
	arguments, err := text(function["arguments"], path+".function.arguments")
	if err != nil {
		return nil, err
	}
	if _, err := object([]byte(arguments), path+".function.arguments"); err != nil {
		return nil, err
	}
	return wireObject{"type": "tool_use", "id": id, "name": name, "input": json.RawMessage(arguments)}, nil
}

func nativeStopReason(reason string, tools int) (string, error) {
	switch reason {
	case "stop":
		if tools != 0 {
			return "tool_use", nil
		}
		return "end_turn", nil
	case "tool_calls":
		if tools == 0 {
			return "", invalid("Chat finish_reason", "declares tool_calls without any tools")
		}
		return "tool_use", nil
	case "length":
		return "max_tokens", nil
	default:
		return "", unsupported("Chat finish_reason")
	}
}

type chatUsage struct {
	prompt, completion, cached *int64
}

func readUsage(raw json.RawMessage) (chatUsage, error) {
	var out chatUsage
	if absent(raw) {
		return out, nil
	}
	value, err := object(raw, "Chat usage")
	if err != nil {
		return out, err
	}
	for field, target := range map[string]**int64{"prompt_tokens": &out.prompt, "completion_tokens": &out.completion} {
		if !absent(value[field]) {
			count, err := nonnegativeInt(value[field], "Chat usage."+field)
			if err != nil {
				return out, err
			}
			*target = &count
		}
	}
	if !absent(value["prompt_tokens_details"]) {
		details, err := object(value["prompt_tokens_details"], "Chat usage.prompt_tokens_details")
		if err != nil {
			return out, err
		}
		if !absent(details["cached_tokens"]) {
			count, err := nonnegativeInt(details["cached_tokens"], "Chat usage.prompt_tokens_details.cached_tokens")
			if err != nil {
				return out, err
			}
			out.cached = &count
		}
	}
	return out, nil
}

func (usage *chatUsage) merge(next chatUsage) {
	if next.prompt != nil {
		usage.prompt = next.prompt
	}
	if next.completion != nil {
		usage.completion = next.completion
	}
	if next.cached != nil {
		usage.cached = next.cached
	}
}

func (usage chatUsage) native() (map[string]int64, error) {
	out := make(map[string]int64)
	if usage.prompt != nil {
		input := *usage.prompt
		if usage.cached != nil {
			if *usage.cached > input {
				return nil, invalid("Chat usage", "reports more cached tokens than prompt tokens")
			}
			input -= *usage.cached
		}
		out["input_tokens"] = input
	}
	if usage.completion != nil {
		out["output_tokens"] = *usage.completion
	}
	if usage.cached != nil {
		out["cache_read_input_tokens"] = *usage.cached
	}
	return out, nil
}
