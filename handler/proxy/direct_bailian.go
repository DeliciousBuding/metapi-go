package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// prepareDirectBailianRequest applies only to Bailian's Chat endpoint, after
// protocol conversion and parameter overrides. Native Responses and Messages
// endpoints do not share these request or stream quirks.
func prepareDirectBailianRequest(body []byte) ([]byte, error) {
	doc, err := directBailianObject(body)
	if err != nil {
		return nil, fmt.Errorf("Bailian Chat request: %w", err)
	}
	changed := false
	if directBailianString(doc["reasoning_effort"]) == "none" {
		delete(doc, "reasoning_effort")
		doc["enable_thinking"] = json.RawMessage(`false`)
		changed = true
	}
	if raw, ok := doc["messages"]; ok {
		var messages []json.RawMessage
		if json.Unmarshal(raw, &messages) != nil || messages == nil {
			return nil, fmt.Errorf("Bailian Chat messages must be an array")
		}
		merged := make([]json.RawMessage, 0, len(messages))
		var pending map[string]json.RawMessage
		var calls []json.RawMessage
		pendingChanged := false
		flush := func() {
			if pendingChanged {
				pending["tool_calls"], _ = json.Marshal(calls)
				merged[len(merged)-1], _ = json.Marshal(pending)
			}
		}
		for _, message := range messages {
			current, currentCalls := directBailianMergeableMessage(message)
			if pending != nil && current != nil {
				calls = append(calls, currentCalls...)
				pendingChanged = true
				changed = true
				continue
			}
			flush()
			merged = append(merged, message)
			pending, calls = current, currentCalls
			pendingChanged = false
		}
		flush()
		if len(merged) != len(messages) {
			doc["messages"], _ = json.Marshal(merged)
		}
	}
	if !changed {
		return body, nil
	}
	return json.Marshal(doc)
}

func directBailianMergeableMessage(raw []byte) (map[string]json.RawMessage, []json.RawMessage) {
	doc, err := directBailianObject(raw)
	if err != nil || !strings.EqualFold(directBailianString(doc["role"]), "assistant") {
		return nil, nil
	}
	// Even an unfamiliar extension may carry message identity, reasoning,
	// signatures or caching semantics. Preserve its boundary rather than merge
	// two messages and silently choose one message's metadata.
	for key := range doc {
		if key != "role" && key != "content" && key != "tool_calls" {
			return nil, nil
		}
	}
	content := bytes.TrimSpace(doc["content"])
	if len(content) > 0 && !bytes.Equal(content, []byte("null")) {
		var text string
		if json.Unmarshal(content, &text) == nil {
			if text != "" {
				return nil, nil
			}
		} else {
			var parts []json.RawMessage
			if json.Unmarshal(content, &parts) != nil || len(parts) != 0 {
				return nil, nil
			}
		}
	}
	var calls []json.RawMessage
	if json.Unmarshal(doc["tool_calls"], &calls) != nil || len(calls) == 0 {
		return nil, nil
	}
	for _, call := range calls {
		if _, err := directBailianObject(call); err != nil {
			return nil, nil
		}
	}
	return doc, calls
}

func directBailianObject(raw []byte) (map[string]json.RawMessage, error) {
	var doc map[string]json.RawMessage
	if !utf8.Valid(raw) || json.Unmarshal(raw, &doc) != nil || doc == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	return doc, nil
}

func directBailianString(raw []byte) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func directBailianPresent(raw []byte) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
