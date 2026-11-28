package responses

import (
	"encoding/json"
	"fmt"
)

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
	if err := bridgeFields(msg, "role", "content", "tool_calls", "refusal", "reasoning_content"); err != nil {
		return nil, err
	}
	output := []any{}
	reasoning, err := bridgeOptionalString(msg, "reasoning_content")
	if err != nil {
		return nil, err
	}
	if reasoning != "" {
		output = append(output, bridgeReasoningItem(reasoning, "rs_"+id, "completed"))
	}
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

// FromChatResponse converts a complete Chat result, retaining plain reasoning,
// text, function calls, refusal, usage and explicit incomplete token-limit results.
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
