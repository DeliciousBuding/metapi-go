package requestoverride

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BeforeProtocol replaces developer messages before the existing protocol
// bridge. Other options are serializer choices and run on the target protocol.
func BeforeProtocol(body []byte, protocol string, options TransformOptions) ([]byte, error) {
	if !options.ReplaceDeveloperRoleWithSystem || (protocol != "chat" && protocol != "responses") {
		return body, nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object == nil {
		return nil, fmt.Errorf("invalid request transform body")
	}
	field := "messages"
	if protocol == "responses" {
		field = "input"
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(object[field], &messages) != nil {
		// Responses string input has no developer role.
		if protocol == "responses" {
			return body, nil
		}
		return nil, fmt.Errorf("invalid request transform messages")
	}
	changed := false
	for _, message := range messages {
		var role string
		_ = json.Unmarshal(message["role"], &role)
		if strings.EqualFold(role, "developer") {
			message["role"] = json.RawMessage(`"system"`)
			changed = true
		}
	}
	if !changed {
		return body, nil
	}
	if protocol == "responses" {
		// Responses represents system messages as its instructions string.
		var instructions string
		if raw := object["instructions"]; len(raw) != 0 && string(raw) != "null" && json.Unmarshal(raw, &instructions) != nil {
			return nil, fmt.Errorf("unsupported instructions transform")
		}
		kept := make([]map[string]json.RawMessage, 0, len(messages))
		for _, message := range messages {
			var role string
			_ = json.Unmarshal(message["role"], &role)
			if role != "system" {
				kept = append(kept, message)
				continue
			}
			text, err := instructionText(message["content"])
			if err != nil {
				return nil, err
			}
			if instructions != "" && text != "" {
				instructions += "\n"
			}
			instructions += text
		}
		messages = kept
		object["instructions"], _ = json.Marshal(instructions)
	}
	object[field], _ = json.Marshal(messages)
	return json.Marshal(object)
}

func instructionText(raw json.RawMessage) (string, error) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return "", fmt.Errorf("unsupported instructions content")
	}
	var values []string
	for _, part := range parts {
		if part.Type != "text" && part.Type != "input_text" {
			return "", fmt.Errorf("unsupported instructions content")
		}
		values = append(values, part.Text)
	}
	return strings.Join(values, "\n"), nil
}

// AfterProtocol applies the source's serializer options. ArrayInstructions
// affects Messages.system; ArrayInputs affects Responses.input. Other protocol
// serializers ignore the corresponding option just as the source does.
func AfterProtocol(body []byte, protocol string, options TransformOptions, nativeChatMapping bool) ([]byte, error) {
	if !options.Active() || (protocol != "chat" && protocol != "messages" && protocol != "responses") {
		return body, nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object == nil {
		return nil, fmt.Errorf("invalid request transform body")
	}
	changed := false
	if protocol == "messages" && options.ForceArrayInstructions {
		var text string
		if json.Unmarshal(object["system"], &text) == nil {
			object["system"], _ = json.Marshal([]any{map[string]string{"type": "text", "text": text}})
			changed = true
		}
	}
	if protocol == "responses" && options.ForceArrayInputs {
		var text string
		if json.Unmarshal(object["input"], &text) == nil {
			object["input"], _ = json.Marshal([]any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": text}}}})
			changed = true
		}
	}
	if protocol == "chat" && nativeChatMapping && len(options.ReasoningEffortMapping) > 0 {
		var effort string
		if json.Unmarshal(object["reasoning_effort"], &effort) == nil && effort != "" {
			for _, mapping := range options.ReasoningEffortMapping {
				if mapping.From == effort {
					object["reasoning_effort"], _ = json.Marshal(mapping.To)
					changed = true
					break
				}
			}
		}
	}
	if !changed {
		return body, nil
	}
	return json.Marshal(object)
}

// ClaudeCodeMapping reproduces the fallback after provider serialization.
// An adapter that already chose a different value remains authoritative.
func ClaudeCodeMapping(body []byte, protocol string, options TransformOptions, ctx Context) ([]byte, error) {
	if len(options.ReasoningEffortMapping) == 0 || ctx.ReasoningEffort == "" ||
		!strings.HasPrefix(strings.TrimSpace(ctx.RequestHeader["User-Agent"]), "claude-cli/") ||
		(protocol != "chat" && protocol != "responses") {
		return body, nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil {
		return nil, fmt.Errorf("invalid request transform body")
	}
	field := "reasoning_effort"
	container := object
	if protocol == "responses" {
		var reasoning map[string]json.RawMessage
		if json.Unmarshal(object["reasoning"], &reasoning) != nil {
			return body, nil
		}
		field, container = "effort", reasoning
	}
	var final string
	if json.Unmarshal(container[field], &final) != nil || final != ctx.ReasoningEffort {
		return body, nil
	}
	for _, mapping := range options.ReasoningEffortMapping {
		if mapping.From != ctx.ReasoningEffort {
			continue
		}
		container[field], _ = json.Marshal(mapping.To)
		if protocol == "responses" {
			object["reasoning"], _ = json.Marshal(container)
		}
		return json.Marshal(object)
	}
	return body, nil
}
