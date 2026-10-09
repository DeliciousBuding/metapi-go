package responses

import (
	"encoding/json"
	"fmt"
	"strings"
)

func bridgeUsage(raw any, fromChat bool) (bridgeObject, error) {
	if raw == nil {
		return nil, nil
	}
	usage := bridgeMap(raw)
	if usage == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: invalid usage")
	}
	in, out := "input_tokens", "output_tokens"
	if fromChat {
		in, out = "prompt_tokens", "completion_tokens"
	}
	result := bridgeObject{}
	for _, pair := range [][2]string{{in, "input_tokens"}, {out, "output_tokens"}, {"total_tokens", "total_tokens"}} {
		v, exists := usage[pair[0]]
		if !exists {
			continue
		}
		n, ok := v.(json.Number)
		if !ok {
			return nil, fmt.Errorf("Responses/Chat bridge: usage count must be numeric")
		}
		count, err := n.Int64()
		if err != nil || count < 0 {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid usage count")
		}
		result[pair[1]] = n
	}
	for _, pair := range [][2]string{{in + "_details", "input_tokens_details"}, {out + "_details", "output_tokens_details"}} {
		if v := usage[pair[0]]; v != nil {
			details := bridgeMap(v)
			if details == nil {
				return nil, fmt.Errorf("Responses/Chat bridge: invalid usage details")
			}
			for _, count := range details {
				n, ok := count.(json.Number)
				if !ok {
					return nil, fmt.Errorf("Responses/Chat bridge: invalid usage detail count")
				}
				i, err := n.Int64()
				if err != nil || i < 0 {
					return nil, fmt.Errorf("Responses/Chat bridge: invalid usage detail count")
				}
			}
			result[pair[1]] = v
		}
	}
	if !fromChat {
		for _, pair := range [][2]string{{"input_tokens", "prompt_tokens"}, {"output_tokens", "completion_tokens"}, {"input_tokens_details", "prompt_tokens_details"}, {"output_tokens_details", "completion_tokens_details"}} {
			if v, exists := result[pair[0]]; exists {
				result[pair[1]] = v
				delete(result, pair[0])
			}
		}
	}
	return result, nil
}

func bridgeRejectError(obj bridgeObject) error {
	if obj["error"] != nil {
		return fmt.Errorf("Responses/Chat bridge: upstream returned an error")
	}
	return nil
}

func bridgeChoice(obj bridgeObject) (bridgeObject, error) {
	choices, ok := obj["choices"].([]any)
	if !ok || len(choices) != 1 {
		return nil, fmt.Errorf("Responses/Chat bridge: exactly one choice is required")
	}
	choice := bridgeMap(choices[0])
	if choice == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: invalid choice")
	}
	if index, ok := choice["index"]; ok && index != json.Number("0") {
		return nil, fmt.Errorf("Responses/Chat bridge: only choice index 0 is supported")
	}
	if err := bridgeRejectError(choice); err != nil {
		return nil, err
	}
	if choice["logprobs"] != nil {
		return nil, fmt.Errorf("Responses/Chat bridge: logprobs are unsupported")
	}
	return choice, nil
}

func bridgeStatus(reason string, tools int) (string, any, error) {
	switch reason {
	case "stop":
		return "completed", nil, nil
	case "tool_calls":
		if tools == 0 {
			return "", nil, fmt.Errorf("Responses/Chat bridge: tool_calls terminal without tools")
		}
		return "completed", nil, nil
	case "length":
		return "incomplete", bridgeObject{"reason": "max_output_tokens"}, nil
	case "content_filter":
		return "incomplete", bridgeObject{"reason": "content_filter"}, nil
	default:
		return "", nil, fmt.Errorf("Responses/Chat bridge: unsupported or missing finish_reason %q", reason)
	}
}

func bridgeChatMessage(msg bridgeObject, id string) ([]any, error) {
	if msg == nil || bridgeString(msg["role"]) != "assistant" {
		return nil, fmt.Errorf("Responses/Chat bridge: expected assistant message")
	}
	if err := bridgeFields(msg, "role", "content", "tool_calls", "refusal"); err != nil {
		return nil, err
	}
	output := []any{}
	content := []any{}
	if msg["content"] != nil {
		text, err := bridgeText(msg["content"], "assistant.content")
		if err != nil {
			return nil, err
		}
		content = append(content, bridgeObject{"type": "output_text", "text": text, "annotations": []any{}})
	}
	if msg["refusal"] != nil {
		text, err := bridgeText(msg["refusal"], "assistant.refusal")
		if err != nil {
			return nil, err
		}
		content = append(content, bridgeObject{"type": "refusal", "refusal": text})
	}
	if len(content) > 0 {
		output = append(output, bridgeObject{"id": "msg_" + id, "type": "message", "role": "assistant", "status": "completed", "content": content})
	}
	if raw := msg["tool_calls"]; raw != nil {
		calls, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid tool_calls")
		}
		seen := map[string]bool{}
		for _, raw := range calls {
			item, err := bridgeFunctionCall(bridgeMap(raw), true)
			if err != nil {
				return nil, err
			}
			callID := bridgeString(item["call_id"])
			if seen[callID] {
				return nil, fmt.Errorf("Responses/Chat bridge: duplicate tool call ID")
			}
			seen[callID] = true
			item["id"], item["status"] = "fc_"+callID, "completed"
			output = append(output, item)
		}
	}
	if len(output) == 0 {
		return nil, fmt.Errorf("Responses/Chat bridge: assistant message has no supported content")
	}
	return output, nil
}

// FromChatResponse converts a complete Chat result, retaining text, function
// calls, refusal, token usage and an explicit incomplete result on token limits.
func FromChatResponse(body []byte) ([]byte, error) {
	src, err := bridgeDecode(body)
	if err != nil {
		return nil, err
	}
	if err = bridgeRejectError(src); err != nil {
		return nil, err
	}
	if bridgeString(src["object"]) != "chat.completion" {
		return nil, fmt.Errorf("Responses/Chat bridge: expected chat.completion")
	}
	id, err := bridgeID(src["id"], "response.id")
	if err != nil {
		return nil, err
	}
	model, err := bridgeID(src["model"], "response.model")
	if err != nil {
		return nil, err
	}
	choice, err := bridgeChoice(src)
	if err != nil {
		return nil, err
	}
	output, err := bridgeChatMessage(bridgeMap(choice["message"]), id)
	if err != nil {
		return nil, err
	}
	tools := 0
	for _, raw := range output {
		if bridgeMap(raw)["type"] == "function_call" {
			tools++
		}
	}
	status, details, err := bridgeStatus(bridgeString(choice["finish_reason"]), tools)
	if err != nil {
		return nil, err
	}
	if status == "incomplete" {
		for _, raw := range output {
			bridgeMap(raw)["status"] = "incomplete"
		}
	}
	usage, err := bridgeUsage(src["usage"], true)
	if err != nil {
		return nil, err
	}
	out := bridgeObject{"id": id, "object": "response", "model": model, "status": status, "output": output, "error": nil, "incomplete_details": details, "usage": usage, "store": false}
	if v := src["created"]; v != nil {
		out["created_at"] = v
	}
	return json.Marshal(out)
}

func bridgeResponsesOutput(output any) (bridgeObject, int, error) {
	items, ok := output.([]any)
	if !ok || len(items) == 0 {
		return nil, 0, fmt.Errorf("Responses/Chat bridge: output must be a nonempty array")
	}
	msg := bridgeObject{"role": "assistant", "content": nil}
	var text, refusal strings.Builder
	textSeen, refusalSeen := false, false
	calls := []any{}
	seen := map[string]bool{}
	for _, raw := range items {
		item := bridgeMap(raw)
		if item == nil {
			return nil, 0, fmt.Errorf("Responses/Chat bridge: invalid output item")
		}
		switch bridgeString(item["type"]) {
		case "message":
			if err := bridgeFields(item, "id", "type", "role", "status", "content"); err != nil {
				return nil, 0, err
			}
			if bridgeString(item["role"]) != "assistant" {
				return nil, 0, fmt.Errorf("Responses/Chat bridge: non-assistant output")
			}
			parts, ok := item["content"].([]any)
			if !ok {
				return nil, 0, fmt.Errorf("Responses/Chat bridge: invalid output content")
			}
			for _, raw := range parts {
				part := bridgeMap(raw)
				switch bridgeString(part["type"]) {
				case "output_text":
					s, err := bridgeContent([]any{part}, true)
					if err != nil {
						return nil, 0, err
					}
					text.WriteString(s)
					textSeen = true
				case "refusal":
					if err := bridgeFields(part, "type", "refusal"); err != nil {
						return nil, 0, err
					}
					s, err := bridgeText(part["refusal"], "refusal")
					if err != nil {
						return nil, 0, err
					}
					refusal.WriteString(s)
					refusalSeen = true
				default:
					return nil, 0, fmt.Errorf("Responses/Chat bridge: unsupported output content")
				}
			}
		case "function_call":
			if err := bridgeFields(item, "id", "type", "status", "call_id", "name", "arguments"); err != nil {
				return nil, 0, err
			}
			call, err := bridgeFunctionCall(item, false)
			if err != nil {
				return nil, 0, err
			}
			id := bridgeString(call["id"])
			if seen[id] {
				return nil, 0, fmt.Errorf("Responses/Chat bridge: duplicate tool call ID")
			}
			seen[id] = true
			calls = append(calls, call)
		default:
			return nil, 0, fmt.Errorf("Responses/Chat bridge: unsupported output type %q", item["type"])
		}
	}
	if textSeen {
		msg["content"] = text.String()
	}
	if refusalSeen {
		msg["refusal"] = refusal.String()
	}
	if len(calls) > 0 {
		msg["tool_calls"] = calls
	}
	if !textSeen && !refusalSeen && len(calls) == 0 {
		return nil, 0, fmt.Errorf("Responses/Chat bridge: response has no supported output")
	}
	return msg, len(calls), nil
}

func bridgeFinishReason(src bridgeObject, tools int) (string, error) {
	if err := bridgeRejectError(src); err != nil {
		return "", err
	}
	switch bridgeString(src["status"]) {
	case "completed":
		if tools > 0 {
			return "tool_calls", nil
		}
		return "stop", nil
	case "incomplete":
		switch bridgeString(bridgeMap(src["incomplete_details"])["reason"]) {
		case "max_output_tokens":
			return "length", nil
		case "content_filter":
			return "content_filter", nil
		}
	}
	return "", fmt.Errorf("Responses/Chat bridge: unsupported or unsuccessful response status")
}

// ToChatResponse converts a completed or explicitly incomplete Responses result.
func ToChatResponse(body []byte) ([]byte, error) {
	src, err := bridgeDecode(body)
	if err != nil {
		return nil, err
	}
	if err = bridgeRejectError(src); err != nil {
		return nil, err
	}
	if bridgeString(src["object"]) != "response" {
		return nil, fmt.Errorf("Responses/Chat bridge: expected response object")
	}
	id, err := bridgeID(src["id"], "response.id")
	if err != nil {
		return nil, err
	}
	model, err := bridgeID(src["model"], "response.model")
	if err != nil {
		return nil, err
	}
	msg, tools, err := bridgeResponsesOutput(src["output"])
	if err != nil {
		return nil, err
	}
	reason, err := bridgeFinishReason(src, tools)
	if err != nil {
		return nil, err
	}
	usage, err := bridgeUsage(src["usage"], false)
	if err != nil {
		return nil, err
	}
	out := bridgeObject{"id": id, "object": "chat.completion", "model": model, "choices": []any{bridgeObject{"index": 0, "message": msg, "finish_reason": reason}}, "usage": usage}
	if v := src["created_at"]; v != nil {
		out["created"] = v
	}
	return json.Marshal(out)
}
