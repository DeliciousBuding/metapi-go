package responses

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type bridgeObject map[string]any

func bridgeDecode(raw []byte) (bridgeObject, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj bridgeObject
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: expected a JSON object")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("Responses/Chat bridge: trailing JSON data")
	}
	return obj, nil
}

func bridgeMap(v any) bridgeObject {
	if obj, ok := v.(bridgeObject); ok {
		return obj
	}
	obj, _ := v.(map[string]any)
	return bridgeObject(obj)
}

func bridgeString(v any) string { s, _ := v.(string); return s }

func bridgeFields(obj bridgeObject, fields ...string) error {
	for key, value := range obj {
		if value == nil {
			continue
		}
		found := false
		for _, field := range fields {
			if key == field {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Responses/Chat bridge: unsupported field %q", key)
		}
	}
	return nil
}

func bridgeText(v any, path string) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("Responses/Chat bridge: %s must be a string", path)
	}
	return s, nil
}

func bridgeID(v any, path string) (string, error) {
	s, err := bridgeText(v, path)
	if err != nil || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("Responses/Chat bridge: %s must be nonempty", path)
	}
	return s, nil
}

func bridgeOptionalString(obj bridgeObject, key string) (string, error) {
	if obj[key] == nil {
		return "", nil
	}
	return bridgeText(obj[key], key)
}

func bridgeContent(v any, responses bool) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	parts, ok := v.([]any)
	if !ok {
		return "", fmt.Errorf("Responses/Chat bridge: content must be text or a text part array")
	}
	var out strings.Builder
	for _, raw := range parts {
		part := bridgeMap(raw)
		if err := bridgeFields(part, "type", "text", "annotations"); err != nil {
			return "", err
		}
		typ := bridgeString(part["type"])
		if (responses && typ != "input_text" && typ != "output_text") || (!responses && typ != "text") {
			return "", fmt.Errorf("Responses/Chat bridge: unsupported content type %q", typ)
		}
		if a, ok := part["annotations"].([]any); part["annotations"] != nil && (!ok || len(a) != 0) {
			return "", fmt.Errorf("Responses/Chat bridge: annotations cannot be represented")
		}
		s, err := bridgeText(part["text"], "content.text")
		if err != nil {
			return "", err
		}
		out.WriteString(s)
	}
	return out.String(), nil
}

func bridgeArguments(v any) (string, error) {
	s, err := bridgeText(v, "function arguments")
	if err != nil {
		return "", err
	}
	if _, err = bridgeDecode([]byte(s)); err != nil {
		return "", fmt.Errorf("Responses/Chat bridge: function arguments must contain a complete JSON object")
	}
	return s, nil
}

func bridgeRequestOptions(req, out bridgeObject, toChat bool) error {
	for _, key := range []string{"model", "stream", "temperature", "top_p", "parallel_tool_calls", "metadata", "user"} {
		if v, ok := req[key]; ok {
			out[key] = v
		}
	}
	if _, err := bridgeID(out["model"], "model"); err != nil {
		return err
	}
	for _, key := range []string{"stream", "parallel_tool_calls"} {
		if v := out[key]; v != nil {
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("Responses/Chat bridge: %s must be boolean", key)
			}
		}
	}
	for _, key := range []string{"temperature", "top_p"} {
		if v := out[key]; v != nil {
			if _, ok := v.(json.Number); !ok {
				return fmt.Errorf("Responses/Chat bridge: %s must be numeric", key)
			}
		}
	}
	from, to := "max_output_tokens", "max_completion_tokens"
	if !toChat {
		from, to = "max_completion_tokens", "max_output_tokens"
		if req[from] == nil {
			from = "max_tokens"
		}
	}
	if v := req[from]; v != nil {
		n, ok := v.(json.Number)
		if !ok {
			return fmt.Errorf("Responses/Chat bridge: %s must be a positive integer", from)
		}
		i, err := n.Int64()
		if err != nil || i <= 0 {
			return fmt.Errorf("Responses/Chat bridge: %s must be a positive integer", from)
		}
		out[to] = v
	}
	if !toChat && req["max_tokens"] != nil && req["max_completion_tokens"] != nil {
		return fmt.Errorf("Responses/Chat bridge: conflicting token limits")
	}
	if v := req["store"]; v != nil && v != false {
		return fmt.Errorf("Responses/Chat bridge: stored response semantics are unsupported")
	}
	out["store"] = false
	if toChat && req["stream"] == true {
		out["stream_options"] = bridgeObject{"include_usage": true}
	}
	if v := req["stream_options"]; v != nil {
		opts := bridgeMap(v)
		if opts == nil {
			return fmt.Errorf("Responses/Chat bridge: invalid stream_options")
		}
		if err := bridgeFields(opts, "include_usage"); err != nil {
			return err
		}
		if value := opts["include_usage"]; value != nil {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("Responses/Chat bridge: invalid include_usage")
			}
		}
	}
	if err := bridgeReasoningOptions(req, out, toChat); err != nil {
		return err
	}
	return bridgeRequestTools(req, out, toChat)
}

func bridgeRequestTools(req, out bridgeObject, toChat bool) error {
	if raw := req["tools"]; raw != nil {
		tools, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("Responses/Chat bridge: tools must be an array")
		}
		converted := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool := bridgeMap(raw)
			if bridgeString(tool["type"]) != "function" {
				return fmt.Errorf("Responses/Chat bridge: only function tools are supported")
			}
			fn := tool
			if !toChat {
				if err := bridgeFields(tool, "type", "function"); err != nil {
					return err
				}
				fn = bridgeMap(tool["function"])
			}
			if err := bridgeFields(fn, "type", "name", "description", "parameters", "strict"); err != nil {
				return err
			}
			if _, err := bridgeID(fn["name"], "function.name"); err != nil {
				return err
			}
			if fn["parameters"] != nil && bridgeMap(fn["parameters"]) == nil {
				return fmt.Errorf("Responses/Chat bridge: function parameters must be an object")
			}
			if fn["description"] != nil {
				if _, err := bridgeText(fn["description"], "function.description"); err != nil {
					return err
				}
			}
			if fn["strict"] != nil {
				if _, ok := fn["strict"].(bool); !ok {
					return fmt.Errorf("Responses/Chat bridge: function strict must be boolean")
				}
			}
			copy := bridgeObject{}
			for _, key := range []string{"name", "description", "parameters", "strict"} {
				if v, ok := fn[key]; ok {
					copy[key] = v
				}
			}
			// Chat function tools default to non-strict schemas. Responses may
			// normalize an omitted strict flag, so make the source default explicit.
			if !toChat && copy["strict"] == nil {
				copy["strict"] = false
			}
			if toChat {
				converted = append(converted, bridgeObject{"type": "function", "function": copy})
			} else {
				copy["type"] = "function"
				converted = append(converted, copy)
			}
		}
		out["tools"] = converted
	}
	if choice := req["tool_choice"]; choice != nil {
		if text, ok := choice.(string); ok {
			if text != "auto" && text != "none" && text != "required" {
				return fmt.Errorf("Responses/Chat bridge: unsupported tool_choice")
			}
			out["tool_choice"] = text
		} else {
			obj := bridgeMap(choice)
			if bridgeString(obj["type"]) != "function" {
				return fmt.Errorf("Responses/Chat bridge: only function tool_choice is supported")
			}
			var name string
			var err error
			if toChat {
				err = bridgeFields(obj, "type", "name")
				name = bridgeString(obj["name"])
			} else {
				err = bridgeFields(obj, "type", "function")
				fn := bridgeMap(obj["function"])
				if err == nil {
					err = bridgeFields(fn, "name")
				}
				name = bridgeString(fn["name"])
			}
			if err != nil {
				return err
			}
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("Responses/Chat bridge: tool_choice needs a function name")
			}
			if toChat {
				out["tool_choice"] = bridgeObject{"type": "function", "function": bridgeObject{"name": name}}
			} else {
				out["tool_choice"] = bridgeObject{"type": "function", "name": name}
			}
		}
	}
	return nil
}

// ToChatRequest converts stateless Responses text/function requests to Chat.
// Plain reasoning is retained; continuity IDs, encrypted reasoning, multimodal
// input and built-in tools are rejected.
func ToChatRequest(body []byte) ([]byte, error) {
	req, err := bridgeDecode(body)
	if err != nil {
		return nil, err
	}
	if err = bridgeFields(req, "model", "input", "instructions", "stream", "max_output_tokens", "temperature", "top_p", "tools", "tool_choice", "parallel_tool_calls", "metadata", "user", "store", "reasoning"); err != nil {
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
				content, err := bridgeContent(item["content"], true)
				if err != nil {
					return nil, err
				}
				if role == "assistant" && reasoningMessage != nil {
					reasoningMessage["content"] = bridgeString(reasoningMessage["content"]) + content
				} else {
					reasoningMessage = nil
					messages = append(messages, bridgeObject{"role": role, "content": content})
				}
			case "function_call":
				if err = bridgeFields(item, "type", "call_id", "name", "arguments", "id", "status"); err != nil {
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

// FromChatRequest converts one-choice Chat text/function requests to Responses.
func FromChatRequest(body []byte) ([]byte, error) {
	req, err := bridgeDecode(body)
	if err != nil {
		return nil, err
	}
	if err = bridgeFields(req, "model", "messages", "stream", "stream_options", "max_tokens", "max_completion_tokens", "temperature", "top_p", "tools", "tool_choice", "parallel_tool_calls", "metadata", "user", "store", "n", "reasoning_effort", "reasoning_summary", "reasoning_budget"); err != nil {
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
			content, err := bridgeContent(msg["content"], false)
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
			content, err := bridgeContent(msg["content"], false)
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
