package responses

import (
	"encoding/json"
	"fmt"
)

// ToChatRequest converts stateless Responses text/image/function requests to Chat.
// Plain reasoning is retained; continuity IDs, encrypted reasoning, file IDs
// and built-in tools are rejected.
func ToChatRequest(body []byte) ([]byte, error) {
	req, err := bridgeDecode(body)
	if err != nil {
		return nil, err
	}
	if err = bridgeFields(req, "model", "input", "instructions", "stream", "max_output_tokens", "temperature", "top_p", "tools", "tool_choice", "parallel_tool_calls", "metadata", "user", "store", "reasoning", "text"); err != nil {
		return nil, err
	}
	out := bridgeObject{}
	if err = bridgeRequestOptions(req, out, true); err != nil {
		return nil, err
	}
	messages := []any{}
	if req["instructions"] != nil {
		s, err := bridgeText(req["instructions"], "instructions")
		if err != nil {
			return nil, err
		}
		messages = append(messages, bridgeObject{"role": "system", "content": s})
	}
	if text, ok := req["input"].(string); ok {
		messages = append(messages, bridgeObject{"role": "user", "content": text})
	} else {
		items, ok := req["input"].([]any)
		if !ok {
			return nil, fmt.Errorf("Responses/Chat bridge: input must be text or an array")
		}
		var reasoningMessage bridgeObject
		previousType := ""
		for _, raw := range items {
			item := bridgeMap(raw)
			if item == nil {
				return nil, fmt.Errorf("Responses/Chat bridge: invalid input item")
			}
			kind := bridgeString(item["type"])
			switch kind {
			case "reasoning":
				text, err := bridgeReasoningText(item)
				if err != nil {
					return nil, err
				}
				if previousType == "reasoning" {
					reasoningMessage["reasoning_content"] = bridgeString(reasoningMessage["reasoning_content"]) + text
				} else {
					reasoningMessage = bridgeObject{"role": "assistant", "content": nil, "reasoning_content": text}
					messages = append(messages, reasoningMessage)
				}
			case "", "message":
				if err = bridgeFields(item, "type", "role", "content", "id", "status"); err != nil {
					return nil, err
				}
				role := bridgeString(item["role"])
				if role != "user" && role != "assistant" && role != "system" && role != "developer" {
					return nil, fmt.Errorf("Responses/Chat bridge: unsupported message role")
				}
				content, err := bridgeRequestContent(item["content"], true, role)
				if err != nil {
					return nil, err
				}
				if role == "assistant" && reasoningMessage != nil {
					reasoningMessage["content"] = bridgeString(reasoningMessage["content"]) + bridgeString(content)
				} else {
					reasoningMessage = nil
					messages = append(messages, bridgeObject{"role": role, "content": content})
				}
			case "function_call":
				if err = bridgeFields(item, "type", "call_id", "name", "arguments", "id", "status", "reasoning_content"); err != nil {
					return nil, err
				}
				call, err := bridgeFunctionCall(item, false)
				if err != nil {
					return nil, err
				}
				// Adjacent calls belong to one assistant turn, including parallel calls.
				var msg bridgeObject
				if len(messages) > 0 {
					last := bridgeMap(messages[len(messages)-1])
					if last["role"] == "assistant" {
						msg = last
					}
				}
				if msg == nil {
					msg = bridgeObject{"role": "assistant", "content": nil}
					messages = append(messages, msg)
				}
				thought, err := bridgeReasoningParts(item["reasoning_content"], "reasoning_text")
				if err != nil {
					return nil, err
				}
				if thought != "" {
					msg["reasoning_content"] = bridgeString(msg["reasoning_content"]) + thought
				}
				calls, _ := msg["tool_calls"].([]any)
				msg["tool_calls"] = append(calls, call)
			case "function_call_output":
				reasoningMessage = nil
				if err = bridgeFields(item, "type", "call_id", "output", "id", "status"); err != nil {
					return nil, err
				}
				id, err := bridgeID(item["call_id"], "function_call_output.call_id")
				if err != nil {
					return nil, err
				}
				text, err := bridgeText(item["output"], "function_call_output.output")
				if err != nil {
					return nil, err
				}
				messages = append(messages, bridgeObject{"role": "tool", "tool_call_id": id, "content": text})
			default:
				return nil, fmt.Errorf("Responses/Chat bridge: unsupported input item type %q", item["type"])
			}
			previousType = kind
		}
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("Responses/Chat bridge: empty input")
	}
	out["messages"] = messages
	return json.Marshal(out)
}

// FromChatRequest converts one-choice Chat text/image/function requests to Responses.
func FromChatRequest(body []byte) ([]byte, error) {
	req, err := bridgeDecode(body)
	if err != nil {
		return nil, err
	}
	if err = bridgeFields(req, "model", "messages", "stream", "stream_options", "max_tokens", "max_completion_tokens", "temperature", "top_p", "tools", "tool_choice", "parallel_tool_calls", "metadata", "user", "store", "n", "reasoning_effort", "reasoning_summary", "reasoning_budget", "response_format", "verbosity"); err != nil {
		return nil, err
	}
	if n := req["n"]; n != nil && n != json.Number("1") {
		return nil, fmt.Errorf("Responses/Chat bridge: only n=1 is supported")
	}
	out := bridgeObject{}
	if err = bridgeRequestOptions(req, out, false); err != nil {
		return nil, err
	}
	messages, ok := req["messages"].([]any)
	if !ok || len(messages) == 0 {
		return nil, fmt.Errorf("Responses/Chat bridge: messages must be nonempty")
	}
	input := []any{}
	for _, raw := range messages {
		msg := bridgeMap(raw)
		if msg == nil {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid message")
		}
		if err = bridgeFields(msg, "role", "content", "tool_calls", "tool_call_id", "reasoning_content"); err != nil {
			return nil, err
		}
		role := bridgeString(msg["role"])
		reasoning, err := bridgeOptionalString(msg, "reasoning_content")
		if err != nil {
			return nil, err
		}
		if msg["reasoning_content"] != nil && role != "assistant" {
			return nil, fmt.Errorf("Responses/Chat bridge: reasoning_content requires assistant role")
		}
		if reasoning != "" {
			input = append(input, bridgeReasoningItem(reasoning, "", "completed"))
		}
		if role == "tool" {
			if msg["tool_calls"] != nil {
				return nil, fmt.Errorf("Responses/Chat bridge: tool message cannot call tools")
			}
			id, err := bridgeID(msg["tool_call_id"], "tool_call_id")
			if err != nil {
				return nil, err
			}
			content, err := bridgeRequestContent(msg["content"], false, role)
			if err != nil {
				return nil, err
			}
			input = append(input, bridgeObject{"type": "function_call_output", "call_id": id, "output": content})
			continue
		}
		if role != "user" && role != "assistant" && role != "system" && role != "developer" {
			return nil, fmt.Errorf("Responses/Chat bridge: unsupported role")
		}
		if msg["tool_call_id"] != nil || (msg["tool_calls"] != nil && role != "assistant") {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid tool fields for role")
		}
		if msg["content"] != nil {
			content, err := bridgeRequestContent(msg["content"], false, role)
			if err != nil {
				return nil, err
			}
			input = append(input, bridgeObject{"type": "message", "role": role, "content": content})
		} else if msg["tool_calls"] == nil && reasoning == "" {
			return nil, fmt.Errorf("Responses/Chat bridge: message lacks content")
		}
		if raw := msg["tool_calls"]; raw != nil {
			calls, ok := raw.([]any)
			if !ok || len(calls) == 0 {
				return nil, fmt.Errorf("Responses/Chat bridge: invalid tool_calls")
			}
			for _, raw := range calls {
				call, err := bridgeFunctionCall(bridgeMap(raw), true)
				if err != nil {
					return nil, err
				}
				input = append(input, call)
			}
		}
	}
	out["input"] = input
	return json.Marshal(out)
}

func bridgeFunctionCall(call bridgeObject, fromChat bool) (bridgeObject, error) {
	if call == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: invalid function call")
	}
	fn, idKey := call, "call_id"
	if fromChat {
		if err := bridgeFields(call, "id", "type", "function"); err != nil {
			return nil, err
		}
		if bridgeString(call["type"]) != "function" {
			return nil, fmt.Errorf("Responses/Chat bridge: unsupported tool type")
		}
		fn, idKey = bridgeMap(call["function"]), "id"
		if err := bridgeFields(fn, "name", "arguments"); err != nil {
			return nil, err
		}
	}
	id, err := bridgeID(call[idKey], idKey)
	if err != nil {
		return nil, err
	}
	name, err := bridgeID(fn["name"], "function.name")
	if err != nil {
		return nil, err
	}
	args, err := bridgeArguments(fn["arguments"])
	if err != nil {
		return nil, err
	}
	if fromChat {
		return bridgeObject{"type": "function_call", "call_id": id, "name": name, "arguments": args}, nil
	}
	return bridgeObject{"id": id, "type": "function", "function": bridgeObject{"name": name, "arguments": args}}, nil
}
