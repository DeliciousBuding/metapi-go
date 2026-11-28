package proxy

import (
	"strings"
)

// hasCompletionContentFromPayload checks if a parsed JSON payload has upstream output.
// This mirrors the TS hasCompletionContentFromPayload function.
func hasCompletionContentFromPayload(payload any) bool {
	if payload == nil {
		return false
	}

	obj, ok := payload.(map[string]any)
	if !ok {
		return false
	}

	if hasMediaOutput(obj) {
		return true
	}
	if candidates, ok := obj["candidates"].([]any); ok {
		for _, candidate := range candidates {
			item, _ := candidate.(map[string]any)
			content, _ := item["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, part := range parts {
				if partHasContent(part) {
					return true
				}
			}
		}
	}

	// Check choices
	if choices, ok := obj["choices"].([]any); ok {
		for _, choice := range choices {
			if hasCompletionContentFromChoice(choice) {
				return true
			}
		}
	}

	// Direct output_text
	if s, ok := obj["output_text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := obj["outputText"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}

	// Check output array
	if output, ok := obj["output"].([]any); ok {
		for _, item := range output {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			itemType := strings.ToLower(stringValue(itemMap["type"]))
			if itemType == "compaction" || itemType == "compaction_summary" {
				if content, ok := itemMap["encrypted_content"].(string); ok && content != "" {
					return true
				}
			}
			if strings.Contains(itemType, "function_call") || strings.Contains(itemType, "tool_call") {
				return true
			}
			if s, ok := itemMap["text"].(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
			if s, ok := itemMap["output_text"].(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
			if content, ok := itemMap["content"].([]any); ok {
				for _, part := range content {
					if partHasContent(part) {
						return true
					}
				}
			}
			if hasToolCallLikeMap(itemMap) {
				return true
			}
		}
	}

	// Check content parts
	if content, ok := obj["content"].([]any); ok {
		for _, part := range content {
			if partHasContent(part) {
				return true
			}
		}
	}

	// Direct text/delta
	if s, ok := obj["delta"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := obj["text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if hasToolCallLikeMap(obj) {
		return true
	}

	return false
}

func hasCompletionContentFromChoice(choice any) bool {
	cm, ok := choice.(map[string]any)
	if !ok {
		return false
	}
	if s, ok := cm["text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := cm["completion"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := cm["output_text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}

	message, _ := cm["message"].(map[string]any)
	if message != nil {
		if partsHaveContent(message["images"]) {
			return true
		}
		if s, ok := message["content"].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
		if contentArr, ok := message["content"].([]any); ok {
			for _, part := range contentArr {
				if partHasContent(part) {
					return true
				}
			}
		}
		if s, ok := message["refusal"].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
		if hasToolCallLikeMap(message) {
			return true
		}
	}

	// Direct tool calls on choice
	if hasToolCallLikeMap(cm) {
		return true
	}

	// Delta
	delta, _ := cm["delta"].(map[string]any)
	if delta != nil {
		if partsHaveContent(delta["content"]) || partsHaveContent(delta["images"]) {
			return true
		}
		if s, ok := delta["content"].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
		if s, ok := delta["refusal"].(string); ok && strings.TrimSpace(s) != "" {
			return true
		}
		if hasToolCallLikeMap(delta) {
			return true
		}
	}

	return false
}

func partHasContent(part any) bool {
	pm, ok := part.(map[string]any)
	if !ok {
		return false
	}
	if s, ok := pm["text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := pm["output_text"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	if s, ok := pm["content"].(string); ok && strings.TrimSpace(s) != "" {
		return true
	}
	partType := strings.ToLower(stringValue(pm["type"]))
	if partType == "image_url" {
		image, _ := pm["image_url"].(map[string]any)
		if strings.TrimSpace(stringValue(image["url"])) != "" {
			return true
		}
	}
	for key, field := range map[string]string{"inlineData": "data", "fileData": "fileUri", "functionCall": "name"} {
		data, _ := pm[key].(map[string]any)
		if strings.TrimSpace(stringValue(data[field])) != "" {
			return true
		}
	}
	if strings.Contains(partType, "function_call") || strings.Contains(partType, "tool_call") {
		return true
	}
	return false
}

func partsHaveContent(value any) bool {
	parts, _ := value.([]any)
	for _, part := range parts {
		if partHasContent(part) {
			return true
		}
	}
	return false
}

func hasToolCallLikeMap(m map[string]any) bool {
	for _, key := range []string{"tool_calls", "toolCalls", "function_call", "functionCall"} {
		if v, ok := m[key]; ok {
			return hasToolCallLike(v)
		}
	}
	return false
}

func hasToolCallLike(v any) bool {
	if v == nil {
		return false
	}
	if arr, ok := v.([]any); ok {
		return len(arr) > 0
	}
	if m, ok := v.(map[string]any); ok {
		return len(m) > 0
	}
	return false
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}
