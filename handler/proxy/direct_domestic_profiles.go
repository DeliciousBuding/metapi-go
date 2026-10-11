package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// prepareDirectDomesticProfileRequest runs after conversion to Chat and request
// overrides. These are the default Moonshot/LongCat Chat adapters; a provider's
// custom generic Chat endpoint or native Messages endpoint does not imply one.
func prepareDirectDomesticProfileRequest(body []byte, profile string) ([]byte, error) {
	if profile != "moonshot" && profile != "longcat" {
		return nil, fmt.Errorf("unsupported domestic Chat profile")
	}
	doc, err := directDomesticProfileObject(body, "request")
	if err != nil {
		return nil, err
	}
	changed := false
	if profile == "moonshot" {
		changed, err = prepareDirectMoonshotFormat(doc)
		if err != nil {
			return nil, err
		}
	}
	var messages []json.RawMessage
	if json.Unmarshal(doc["messages"], &messages) != nil || len(messages) == 0 {
		return nil, fmt.Errorf("domestic Chat messages must be a nonempty array")
	}
	messagesChanged := false
	for i, raw := range messages {
		message, err := directDomesticProfileObject(raw, fmt.Sprintf("messages[%d]", i))
		if err != nil {
			return nil, err
		}
		messageChanged, err := prepareDirectDomesticReasoning(message)
		if err != nil {
			return nil, fmt.Errorf("messages[%d]: %w", i, err)
		}
		if profile == "longcat" {
			content, rewritten, err := directLongcatContent(message["content"])
			if err != nil {
				return nil, fmt.Errorf("messages[%d]: %w", i, err)
			}
			if rewritten {
				message["content"] = content
				messageChanged = true
			}
		}
		if messageChanged {
			messages[i], _ = json.Marshal(message)
			messagesChanged = true
		}
	}
	if messagesChanged {
		doc["messages"], _ = json.Marshal(messages)
	}
	if !changed && !messagesChanged {
		return body, nil
	}
	return json.Marshal(doc)
}

func prepareDirectMoonshotFormat(doc map[string]json.RawMessage) (bool, error) {
	raw := doc["response_format"]
	if directDomesticProfileNull(raw) {
		return false, nil
	}
	format, err := directDomesticProfileObject(raw, "Moonshot response_format")
	if err != nil {
		return false, err
	}
	var kind string
	if json.Unmarshal(format["type"], &kind) != nil || kind == "" {
		return false, fmt.Errorf("Moonshot response_format.type must be a nonempty string")
	}
	switch kind {
	case "text", "json_object":
		if !directDomesticProfileNull(format["json_schema"]) {
			return false, fmt.Errorf("Moonshot response_format.json_schema requires type json_schema")
		}
		return false, nil
	case "json_schema":
		if _, err := directDomesticProfileObject(format["json_schema"], "Moonshot response_format.json_schema"); err != nil {
			return false, err
		}
		// Moonshot's source adapter explicitly degrades structured output to
		// JSON Object mode. The response schema and its strictness are not sent upstream.
		format["type"] = json.RawMessage(`"json_object"`)
		delete(format, "json_schema")
		doc["response_format"], _ = json.Marshal(format)
		return true, nil
	default:
		return false, fmt.Errorf("unsupported Moonshot response_format.type")
	}
}

func prepareDirectDomesticReasoning(message map[string]json.RawMessage) (bool, error) {
	var values [2]string
	for i, key := range []string{"reasoning_content", "reasoning"} {
		if raw := message[key]; !directDomesticProfileNull(raw) && json.Unmarshal(raw, &values[i]) != nil {
			return false, fmt.Errorf("%s must be a string or null", key)
		}
	}
	reasoning, exists := message["reasoning"]
	if !exists {
		return false, nil
	}
	if !directDomesticProfileNull(reasoning) {
		if directDomesticProfileNull(message["reasoning_content"]) {
			message["reasoning_content"] = reasoning
		} else if values[0] != values[1] {
			// Both source adapters serialize one reasoning_content string. A
			// conflict cannot be represented without discarding one history value.
			return false, fmt.Errorf("conflicting reasoning and reasoning_content")
		}
	}
	delete(message, "reasoning")
	return true, nil
}

func directLongcatContent(raw json.RawMessage) (json.RawMessage, bool, error) {
	text := json.RawMessage(`""`)
	if !directDomesticProfileNull(raw) {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			text = raw
		} else {
			var parts []json.RawMessage
			if json.Unmarshal(raw, &parts) != nil {
				return nil, false, fmt.Errorf("LongCat content must be a string, array or null")
			}
			for _, part := range parts {
				if _, err := directDomesticProfileObject(part, "LongCat content part"); err != nil {
					return nil, false, err
				}
			}
			if len(parts) > 0 {
				return raw, false, nil
			}
		}
	}
	content, err := json.Marshal([]map[string]json.RawMessage{{"type": json.RawMessage(`"text"`), "text": text}})
	return content, true, err
}

func directDomesticProfileObject(raw []byte, path string) (map[string]json.RawMessage, error) {
	var doc map[string]json.RawMessage
	if !utf8.Valid(raw) || json.Unmarshal(raw, &doc) != nil || doc == nil {
		return nil, fmt.Errorf("%s must be a JSON object", path)
	}
	return doc, nil
}

func directDomesticProfileNull(raw []byte) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
