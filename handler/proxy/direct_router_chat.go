package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Only default OpenRouter/Cerebras Chat endpoints use these adapters in the
// source contract. Imported custom Chat endpoints retain generic OpenAI rules.
func prepareDirectRouterChatRequest(body []byte, profile string) ([]byte, error) {
	if profile != "openrouter" && profile != "cerebras" {
		return nil, fmt.Errorf("unsupported router Chat profile")
	}
	obj, err := routerChatObject(body)
	if err != nil {
		return nil, err
	}
	var model string
	if json.Unmarshal(obj["model"], &model) != nil || model == "" {
		return nil, fmt.Errorf("router Chat requires a model")
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(obj["messages"], &messages) != nil || len(messages) == 0 {
		return nil, fmt.Errorf("router Chat requires messages")
	}
	for _, message := range messages {
		if message == nil {
			return nil, fmt.Errorf("router Chat requires message objects")
		}
		if raw := message["reasoning_content"]; routerChatPresent(raw) {
			var reasoning string
			if json.Unmarshal(raw, &reasoning) != nil {
				return nil, fmt.Errorf("router Chat reasoning_content must be a string")
			}
			if other := message["reasoning"]; routerChatPresent(other) {
				var existing string
				if json.Unmarshal(other, &existing) != nil || existing != reasoning {
					return nil, fmt.Errorf("router Chat has conflicting reasoning fields")
				}
			}
			message["reasoning"] = raw
			delete(message, "reasoning_content")
		}
	}
	obj["messages"], err = json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	if profile == "cerebras" {
		delete(obj, "store")
	}
	return json.Marshal(obj)
}

func normalizeDirectRouterChatJSON(body []byte, readable bool) ([]byte, error) {
	if !readable {
		return nil, fmt.Errorf("unsupported router Chat response encoding")
	}
	converted, _, err := routerChatJSON(body, false, normalizeRouterChatMessage)
	return converted, err
}

// The native result preserves provider extensions. A cross-protocol projection
// removes only aliases already represented canonically; opaque reasoning and
// unsupported content remain explicit errors instead of invented plain text.
func projectDirectRouterChatResponse(body []byte, stream bool) ([]byte, error) {
	out, _, err := routerChatJSON(body, stream, func(message map[string]json.RawMessage) error {
		var details []map[string]json.RawMessage
		if routerChatPresent(message["reasoning_details"]) && json.Unmarshal(message["reasoning_details"], &details) != nil {
			return fmt.Errorf("router Chat reasoning_details must be an array")
		}
		for _, detail := range details {
			if _, plain, err := routerChatReasoningDetail(detail); err != nil || !plain {
				return fmt.Errorf("opaque router Chat reasoning requires native Chat")
			}
			for key, value := range detail {
				switch key {
				case "type", "text", "summary", "id", "index", "format":
				default:
					if routerChatPresent(value) {
						return fmt.Errorf("router Chat reasoning detail %s requires native Chat", key)
					}
				}
			}
		}
		delete(message, "reasoning")
		delete(message, "reasoning_details")
		// normalizeRouterChatMessage has merged every image into content,
		// preserving source order and unknown content fields for target checks.
		delete(message, "images")
		return nil
	})
	return out, err
}

type routerChatChoice struct {
	index    int
	finished bool
}

// RawMessage avoids rounding billing fields or rewriting opaque tool JSON.
// Message transforms are separate from validation so projections never merge
// images twice or duplicate reasoning aliases in the downstream converter.
func routerChatJSON(body []byte, stream bool, transform func(map[string]json.RawMessage) error) ([]byte, []routerChatChoice, error) {
	obj, err := routerChatObject(body)
	if err != nil {
		return nil, nil, err
	}
	if routerChatPresent(obj["error"]) || routerChatPresent(obj["errors"]) {
		return nil, nil, fmt.Errorf("router Chat upstream error")
	}
	var success bool
	if raw, exists := obj["success"]; exists && (json.Unmarshal(raw, &success) != nil || !success) {
		return nil, nil, fmt.Errorf("router Chat upstream reported failure")
	}
	var choices []map[string]json.RawMessage
	if !routerChatPresent(obj["choices"]) || json.Unmarshal(obj["choices"], &choices) != nil {
		return nil, nil, fmt.Errorf("router Chat response requires choices")
	}
	if len(choices) == 0 && (!stream || !routerChatPresent(obj["usage"])) {
		return nil, nil, fmt.Errorf("router Chat response has no choices or stream usage")
	}
	if routerChatPresent(obj["usage"]) {
		if _, err := routerChatObject(obj["usage"]); err != nil {
			return nil, nil, fmt.Errorf("router Chat response has invalid usage")
		}
	}
	object := "chat.completion"
	field := "message"
	if stream {
		object, field = "chat.completion.chunk", "delta"
	}
	if routerChatPresent(obj["object"]) {
		var actual string
		if json.Unmarshal(obj["object"], &actual) != nil || actual != object {
			return nil, nil, fmt.Errorf("router Chat response has invalid object")
		}
	} else {
		obj["object"], _ = json.Marshal(object)
	}
	states := make([]routerChatChoice, 0, len(choices))
	seen := make(map[int]bool, len(choices))
	for _, choice := range choices {
		var index int
		if !routerChatPresent(choice["index"]) || json.Unmarshal(choice["index"], &index) != nil || index < 0 || seen[index] {
			return nil, nil, fmt.Errorf("router Chat choice requires a unique nonnegative index")
		}
		seen[index] = true
		var finish string
		if routerChatPresent(choice["finish_reason"]) && json.Unmarshal(choice["finish_reason"], &finish) != nil {
			return nil, nil, fmt.Errorf("router Chat choice has invalid finish_reason")
		}
		if routerChatPresent(choice["error"]) || finish == "error" {
			return nil, nil, fmt.Errorf("router Chat choice reported an error")
		}
		if !stream && finish == "" {
			return nil, nil, fmt.Errorf("router Chat completion requires finish_reason")
		}
		message, err := routerChatObject(choice[field])
		if err != nil {
			return nil, nil, fmt.Errorf("router Chat choice requires %s", field)
		}
		if err := transform(message); err != nil {
			return nil, nil, err
		}
		choice[field], err = json.Marshal(message)
		if err != nil {
			return nil, nil, err
		}
		states = append(states, routerChatChoice{index: index, finished: finish != ""})
	}
	obj["choices"], err = json.Marshal(choices)
	if err != nil {
		return nil, nil, err
	}
	out, err := json.Marshal(obj)
	return out, states, err
}

func normalizeRouterChatMessage(message map[string]json.RawMessage) error {
	var details []map[string]json.RawMessage
	if routerChatPresent(message["reasoning_details"]) && json.Unmarshal(message["reasoning_details"], &details) != nil {
		return fmt.Errorf("router Chat reasoning_details must be an array")
	}
	if len(details) > 0 {
		var text strings.Builder
		plainSeen := false
		for _, detail := range details {
			part, plain, err := routerChatReasoningDetail(detail)
			if err != nil {
				return err
			}
			if plain {
				text.WriteString(part)
				plainSeen = true
			}
		}
		if plainSeen {
			message["reasoning_content"], _ = json.Marshal(text.String())
		}
	} else if raw := message["reasoning"]; routerChatPresent(raw) {
		var reasoning string
		if json.Unmarshal(raw, &reasoning) != nil {
			return fmt.Errorf("router Chat reasoning must be a string")
		}
		message["reasoning_content"] = raw
	}
	if !routerChatPresent(message["images"]) {
		return nil
	}
	var images []map[string]json.RawMessage
	if json.Unmarshal(message["images"], &images) != nil {
		return fmt.Errorf("router Chat images must be an array")
	}
	if len(images) == 0 {
		return nil
	}
	var content []json.RawMessage
	if raw := message["content"]; routerChatPresent(raw) {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			if text != "" {
				part, _ := json.Marshal(map[string]string{"type": "text", "text": text})
				content = append(content, part)
			}
		} else if json.Unmarshal(raw, &content) != nil {
			return fmt.Errorf("router Chat content must be text or parts")
		}
	}
	for _, image := range images {
		var kind, imageURL string
		imageObject, err := routerChatObject(image["image_url"])
		if err != nil || json.Unmarshal(image["type"], &kind) != nil || kind != "image_url" || json.Unmarshal(imageObject["url"], &imageURL) != nil || imageURL == "" {
			return fmt.Errorf("router Chat image requires an image_url part")
		}
		// The separate images array retains its native index. The equivalent
		// canonical content part is ordered by its array position, not an index.
		delete(image, "index")
		part, err := json.Marshal(image)
		if err != nil {
			return err
		}
		content = append(content, part)
	}
	var err error
	message["content"], err = json.Marshal(content)
	return err
}

// Only explicitly identified textual details contribute to canonical text.
// Encrypted/unknown variants survive native Chat verbatim, including any text
// field they contain. Their text is not evidence of a plaintext variant.
func routerChatReasoningDetail(detail map[string]json.RawMessage) (string, bool, error) {
	if detail == nil {
		return "", false, fmt.Errorf("router Chat reasoning detail must be an object")
	}
	var kind string
	if routerChatPresent(detail["type"]) && json.Unmarshal(detail["type"], &kind) != nil {
		return "", false, fmt.Errorf("router Chat reasoning detail type must be a string")
	}
	if kind != "" && kind != "reasoning.text" && kind != "reasoning.summary" {
		return "", false, nil
	}
	var text, summary string
	if routerChatPresent(detail["text"]) && json.Unmarshal(detail["text"], &text) != nil {
		return "", false, fmt.Errorf("router Chat reasoning text must be a string")
	}
	if routerChatPresent(detail["summary"]) && json.Unmarshal(detail["summary"], &summary) != nil {
		return "", false, fmt.Errorf("router Chat reasoning summary must be a string")
	}
	if kind == "reasoning.summary" && routerChatPresent(detail["summary"]) {
		if routerChatPresent(detail["text"]) && text != summary {
			return "", false, fmt.Errorf("router Chat has conflicting reasoning summary fields")
		}
		text = summary
	}
	if kind == "" && !routerChatPresent(detail["text"]) {
		return "", false, nil
	}
	return text, true, nil
}

func routerChatObject(raw []byte) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return nil, fmt.Errorf("router Chat requires a JSON object")
	}
	return obj, nil
}

func routerChatPresent(raw json.RawMessage) bool {
	return len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
