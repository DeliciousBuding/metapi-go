package responses

import (
	"encoding/json"
	"fmt"
	"strings"
)

func bridgeResponsesOutput(response bridgeObject) (bridgeObject, int, error) {
	items, ok := response["output"].([]any)
	if ok && len(items) == 0 && response["status"] == "incomplete" {
		return bridgeObject{"role": "assistant", "content": nil}, 0, nil
	}
	if !ok || len(items) == 0 {
		return nil, 0, fmt.Errorf("Responses/Chat bridge: output must be a nonempty array")
	}
	msg := bridgeObject{"role": "assistant", "content": nil}
	var text, refusal, reasoning strings.Builder
	textSeen, refusalSeen := false, false
	calls := []any{}
	seen := map[string]bool{}
	for _, raw := range items {
		item := bridgeMap(raw)
		if item == nil {
			return nil, 0, fmt.Errorf("Responses/Chat bridge: invalid output item")
		}
		switch bridgeString(item["type"]) {
		case "reasoning":
			value, err := bridgeReasoningText(item)
			if err != nil {
				return nil, 0, err
			}
			reasoning.WriteString(value)
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
			if err := bridgeFields(item, "id", "type", "status", "call_id", "name", "arguments", "reasoning_content"); err != nil {
				return nil, 0, err
			}
			thought, err := bridgeReasoningParts(item["reasoning_content"], "reasoning_text")
			if err != nil {
				return nil, 0, err
			}
			reasoning.WriteString(thought)
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
	if reasoning.Len() > 0 {
		msg["reasoning_content"] = reasoning.String()
	}
	if !textSeen && !refusalSeen && len(calls) == 0 && reasoning.Len() == 0 {
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
	msg, tools, err := bridgeResponsesOutput(src)
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
		out["created"], err = bridgeCreatedAt(v)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(out)
}
