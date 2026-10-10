package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Cline's Chat adapter applies to both default and custom Chat endpoints, not
// to other API formats that happen to belong to the same provider.
func buildDirectClineHeaders(headers http.Header) {
	headers.Set("X-Client-Type", "cline-cli")
}

func prepareDirectClineRequest(body []byte) ([]byte, error) {
	obj, err := directClineObject(body)
	if err != nil {
		return nil, err
	}
	var messages []json.RawMessage
	if json.Unmarshal(obj["messages"], &messages) != nil || len(messages) == 0 {
		return nil, fmt.Errorf("Cline Chat requires messages")
	}
	for i, raw := range messages {
		message, err := directClineObject(raw)
		if err != nil {
			return nil, fmt.Errorf("Cline message %d: %w", i, err)
		}
		var role string
		if json.Unmarshal(message["role"], &role) != nil || role == "" {
			return nil, fmt.Errorf("Cline message %d requires a role", i)
		}
		switch role {
		case "user", "system", "developer":
			if directClineEmptyContent(message["content"]) {
				message["content"] = json.RawMessage(`[{"type":"text","text":""}]`)
			}
		}
		if raw := message["reasoning_content"]; directClinePresent(raw) {
			var reasoning string
			if json.Unmarshal(raw, &reasoning) != nil {
				return nil, fmt.Errorf("Cline message %d reasoning_content must be a string", i)
			}
			if other := message["reasoning"]; directClinePresent(other) {
				var existing string
				if json.Unmarshal(other, &existing) != nil || existing != reasoning {
					return nil, fmt.Errorf("Cline message %d has conflicting reasoning fields", i)
				}
			}
			message["reasoning"] = raw
			delete(message, "reasoning_content")
		}
		messages[i], err = json.Marshal(message)
		if err != nil {
			return nil, err
		}
	}
	obj["messages"], err = json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

func directClineEmptyContent(raw json.RawMessage) bool {
	if !directClinePresent(raw) {
		return true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text == ""
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return false
	}
	if len(parts) == 0 {
		return true
	}
	if len(parts) != 1 {
		return false
	}
	part, err := directClineObject(parts[0])
	var partType string
	if err != nil || json.Unmarshal(part["type"], &partType) != nil || partType != "text" {
		return false
	}
	return !directClinePresent(part["text"]) || json.Unmarshal(part["text"], &text) == nil && text == ""
}

func normalizeDirectClineJSON(body []byte, readable bool) ([]byte, error) {
	if !readable {
		return nil, fmt.Errorf("unsupported Cline response encoding")
	}
	converted, _, err := directClineChatJSON(body, false)
	return converted, err
}

// RawMessage retains unknown provider fields and number lexemes, including
// tool arguments, billing metadata and counters beyond float64 precision.
func directClineChatJSON(body []byte, stream bool) ([]byte, []directClineChoice, error) {
	obj, err := directClineObject(body)
	if err != nil {
		return nil, nil, err
	}
	if err := directClinePayloadError(obj); err != nil {
		return nil, nil, err
	}
	if _, exists := obj["success"]; exists {
		obj, err = directClineObject(obj["data"])
		if err != nil {
			return nil, nil, fmt.Errorf("Cline response requires data: %w", err)
		}
		if err := directClinePayloadError(obj); err != nil {
			return nil, nil, err
		}
	}
	var choices []json.RawMessage
	if !directClinePresent(obj["choices"]) || json.Unmarshal(obj["choices"], &choices) != nil {
		return nil, nil, fmt.Errorf("Cline response requires Chat choices")
	}
	if len(choices) == 0 && (!stream || !directClinePresent(obj["usage"])) {
		return nil, nil, fmt.Errorf("Cline response has no Chat choices or stream usage")
	}
	object := "chat.completion"
	if stream {
		object = "chat.completion.chunk"
	}
	if directClinePresent(obj["object"]) {
		var actual string
		if json.Unmarshal(obj["object"], &actual) != nil || actual != object {
			return nil, nil, fmt.Errorf("invalid Cline Chat object type")
		}
	} else {
		// Cline omits the discriminator on some usage-only stream envelopes.
		// The validated Chat payload and requested stream mode fix its type.
		obj["object"], _ = json.Marshal(object)
	}
	if directClinePresent(obj["usage"]) {
		if _, err := directClineObject(obj["usage"]); err != nil {
			return nil, nil, fmt.Errorf("Cline response has invalid usage")
		}
	}
	states := make([]directClineChoice, 0, len(choices))
	seen := make(map[int]bool, len(choices))
	for i, raw := range choices {
		choice, err := directClineObject(raw)
		if err != nil {
			return nil, nil, err
		}
		var index int
		if !directClinePresent(choice["index"]) || json.Unmarshal(choice["index"], &index) != nil || index < 0 || seen[index] {
			return nil, nil, fmt.Errorf("Cline choice requires a unique nonnegative index")
		}
		seen[index] = true
		finish := ""
		if directClinePresent(choice["finish_reason"]) && json.Unmarshal(choice["finish_reason"], &finish) != nil {
			return nil, nil, fmt.Errorf("Cline choice has invalid finish_reason")
		}
		field := "message"
		if stream {
			field = "delta"
		}
		message, err := directClineObject(choice[field])
		if err != nil {
			return nil, nil, fmt.Errorf("Cline choice requires %s: %w", field, err)
		}
		if err := normalizeDirectClineReasoning(message); err != nil {
			return nil, nil, err
		}
		choice[field], err = json.Marshal(message)
		if err != nil {
			return nil, nil, err
		}
		choices[i], err = json.Marshal(choice)
		if err != nil {
			return nil, nil, err
		}
		states = append(states, directClineChoice{index: index, finished: finish != ""})
	}
	obj["choices"], err = json.Marshal(choices)
	if err != nil {
		return nil, nil, err
	}
	out, err := json.Marshal(obj)
	return out, states, err
}

func normalizeDirectClineReasoning(message map[string]json.RawMessage) error {
	var details []struct {
		Text string `json:"text"`
	}
	if directClinePresent(message["reasoning_details"]) && json.Unmarshal(message["reasoning_details"], &details) != nil {
		return fmt.Errorf("Cline message has invalid reasoning_details")
	}
	if len(details) > 0 {
		var text strings.Builder
		for _, detail := range details {
			text.WriteString(detail.Text)
		}
		message["reasoning_content"], _ = json.Marshal(text.String())
	} else if raw := message["reasoning"]; directClinePresent(raw) {
		var text string
		if json.Unmarshal(raw, &text) != nil {
			return fmt.Errorf("Cline message reasoning must be a string")
		}
		message["reasoning_content"] = raw
	}
	return nil
}

// The native Chat response keeps provider fields. Other protocols consume the
// normalized reasoning_content text; remove only its known source aliases.
// Encrypted or unknown detail payloads cannot be represented as plain thinking.
func projectDirectClineReasoning(body []byte, stream bool) ([]byte, error) {
	obj, err := directClineObject(body)
	if err != nil {
		return nil, err
	}
	var choices []map[string]json.RawMessage
	if json.Unmarshal(obj["choices"], &choices) != nil {
		return nil, fmt.Errorf("invalid Cline Chat choices")
	}
	for _, choice := range choices {
		field := "message"
		if stream {
			field = "delta"
		}
		message, err := directClineObject(choice[field])
		if err != nil {
			return nil, err
		}
		var details []map[string]json.RawMessage
		if directClinePresent(message["reasoning_details"]) {
			if json.Unmarshal(message["reasoning_details"], &details) != nil {
				return nil, fmt.Errorf("invalid Cline reasoning details")
			}
			for _, detail := range details {
				for name := range detail {
					switch name {
					case "text", "type", "id", "index", "format":
					default:
						return nil, fmt.Errorf("Cline reasoning detail %s requires native Chat", name)
					}
				}
				var text, kind string
				if !directClinePresent(detail["text"]) || json.Unmarshal(detail["text"], &text) != nil {
					return nil, fmt.Errorf("non-text Cline reasoning requires native Chat")
				}
				if directClinePresent(detail["type"]) && (json.Unmarshal(detail["type"], &kind) != nil || kind != "reasoning.text" && kind != "reasoning.summary") {
					return nil, fmt.Errorf("unsupported Cline reasoning type requires native Chat")
				}
			}
		}
		delete(message, "reasoning")
		delete(message, "reasoning_details")
		choice[field], err = json.Marshal(message)
		if err != nil {
			return nil, err
		}
	}
	obj["choices"], err = json.Marshal(choices)
	if err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

func directClineObject(raw []byte) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return nil, fmt.Errorf("Cline requires a JSON object")
	}
	return obj, nil
}

func directClinePresent(raw json.RawMessage) bool {
	return len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func directClinePayloadError(obj map[string]json.RawMessage) error {
	for _, key := range []string{"error", "errors"} {
		if raw := obj[key]; directClinePresent(raw) {
			return fmt.Errorf("Cline API error: %s", directClineErrorMessage(raw))
		}
	}
	if raw, exists := obj["success"]; exists {
		var success bool
		if !directClinePresent(raw) || json.Unmarshal(raw, &success) != nil {
			return fmt.Errorf("Cline response success must be a boolean")
		}
		if !success {
			return fmt.Errorf("Cline request failed")
		}
	}
	var event string
	_ = json.Unmarshal(obj["event"], &event)
	if event == "error" {
		if nested, err := directClineObject(obj["data"]); err == nil {
			if err := directClinePayloadError(nested); err != nil {
				return err
			}
		}
		return fmt.Errorf("Cline stream error")
	}
	return nil
}

func directClineErrorMessage(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil && text != "" {
		return text
	}
	var detail struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &detail) == nil && detail.Message != "" {
		return detail.Message
	}
	var details []json.RawMessage
	if json.Unmarshal(raw, &details) == nil && len(details) > 0 {
		messages := make([]string, 0, len(details))
		for _, item := range details {
			messages = append(messages, directClineErrorMessage(item))
		}
		return strings.Join(messages, "; ")
	}
	return "upstream request failed"
}
