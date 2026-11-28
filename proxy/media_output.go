package proxy

import (
	"strings"
)

// Media APIs return structured results rather than completion text. Only
// recognized result shapes count; an ID or an arbitrary non-empty array does
// not turn an empty/error response into successful output.
func hasMediaOutput(obj map[string]any) bool {
	if data, ok := obj["data"].([]any); ok {
		for _, value := range data {
			item, _ := value.(map[string]any)
			for _, key := range []string{"b64_json", "url", "embedding"} {
				if s, ok := item[key].(string); ok && strings.TrimSpace(s) != "" {
					return true
				}
			}
			if hasNumericVector(item["embedding"]) {
				return true
			}
		}
	}
	if embedding, ok := obj["embedding"].(map[string]any); ok && hasNumericVector(embedding["values"]) {
		return true
	}
	if embeddings, ok := obj["embeddings"].([]any); ok {
		for _, value := range embeddings {
			if embedding, ok := value.(map[string]any); ok && hasNumericVector(embedding["values"]) {
				return true
			}
		}
	}
	if results, ok := obj["results"].([]any); ok {
		for _, value := range results {
			item, _ := value.(map[string]any)
			if _, ok := item["relevance_score"].(float64); ok {
				if index, ok := item["index"].(float64); ok && index >= 0 {
					return true
				}
			}
			if _, ok := item["flagged"].(bool); ok {
				if categories, ok := item["categories"].(map[string]any); ok && len(categories) > 0 {
					return true
				}
			}
		}
	}
	if obj["object"] == "video" && strings.TrimSpace(stringValue(obj["id"])) != "" {
		switch obj["status"] {
		case "queued", "in_progress", "completed", "failed":
			// A failed asynchronous task is still a populated native resource.
			// Its status/error must reach the client, not become an empty-body 502.
			return true
		}
	}
	// Native video deletion acknowledges a completed operation without content.
	if obj["object"] == "video.deleted" && obj["deleted"] == true && strings.TrimSpace(stringValue(obj["id"])) != "" {
		return true
	}
	return false
}

func hasNumericVector(value any) bool {
	values, ok := value.([]any)
	if !ok || len(values) == 0 {
		return false
	}
	for _, item := range values {
		if _, ok := item.(float64); !ok {
			return false
		}
	}
	return true
}
