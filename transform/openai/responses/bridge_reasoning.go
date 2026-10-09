package responses

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The summary setting is a provider preference, not an instruction to summarize
// locally. reasoning_summary/reasoning_budget are Chat provider extensions, as
// in AxonHub's Responses bridge. Returned upstream text is always copied as-is.
func bridgeReasoningOptions(req, out bridgeObject, toChat bool) error {
	config := bridgeMap(req["reasoning"])
	if toChat {
		if req["reasoning"] != nil && config == nil {
			return fmt.Errorf("Responses/Chat bridge: reasoning must be an object")
		}
		if err := bridgeFields(config, "effort", "summary", "generate_summary", "max_tokens"); err != nil {
			return err
		}
	} else {
		config = bridgeObject{"effort": req["reasoning_effort"], "summary": req["reasoning_summary"], "max_tokens": req["reasoning_budget"]}
	}
	converted := bridgeObject{}
	for _, key := range []string{"effort", "summary", "generate_summary"} {
		if config[key] == nil {
			continue
		}
		value, err := bridgeText(config[key], "reasoning."+key)
		if err != nil {
			return err
		}
		if value == "" {
			continue
		}
		allowed := value == "auto" || value == "concise" || value == "detailed"
		if key == "effort" {
			allowed = value == "none" || value == "minimal" || value == "low" || value == "medium" || value == "high" || value == "xhigh"
		}
		if !allowed {
			return fmt.Errorf("Responses/Chat bridge: unsupported reasoning.%s", key)
		}
		converted[key] = value
	}
	if converted["summary"] == nil {
		converted["summary"] = converted["generate_summary"]
	}
	delete(converted, "generate_summary")
	if converted["summary"] == nil {
		delete(converted, "summary")
	}
	if config["max_tokens"] != nil {
		n, ok := config["max_tokens"].(json.Number)
		if !ok {
			return fmt.Errorf("Responses/Chat bridge: reasoning budget must be a positive integer")
		}
		budget, err := n.Int64()
		if err != nil || budget <= 0 {
			return fmt.Errorf("Responses/Chat bridge: reasoning budget must be a positive integer")
		}
		converted["max_tokens"] = n
	}
	if toChat {
		for source, target := range map[string]string{"effort": "reasoning_effort", "summary": "reasoning_summary", "max_tokens": "reasoning_budget"} {
			if value := converted[source]; value != nil {
				out[target] = value
			}
		}
	} else if len(converted) != 0 {
		out["reasoning"] = converted
	}
	return nil
}

func bridgeReasoningItem(text, id, status string) bridgeObject {
	item := bridgeObject{"type": "reasoning", "summary": []any{bridgeObject{"type": "summary_text", "text": text}}, "status": status}
	if id != "" {
		item["id"] = id
	}
	return item
}

// A sanitizing caller may have copied summary into content. Accept that exact
// duplicate once. Distinct summary/content, opaque state and signed reasoning
// cannot be represented by one Chat reasoning_content string and fail closed.
func bridgeReasoningText(item bridgeObject) (string, error) {
	if err := bridgeFields(item, "id", "type", "status", "summary", "content"); err != nil {
		return "", err
	}
	summary, err := bridgeReasoningParts(item["summary"], "summary_text")
	if err != nil {
		return "", err
	}
	content, err := bridgeReasoningParts(item["content"], "reasoning_text")
	if err != nil {
		return "", err
	}
	if summary != "" && content != "" && summary != content {
		return "", fmt.Errorf("Responses/Chat bridge: distinct reasoning summary and content cannot be represented")
	}
	if content != "" {
		return content, nil
	}
	if summary == "" {
		return "", fmt.Errorf("Responses/Chat bridge: reasoning item has no plain text")
	}
	return summary, nil
}

func bridgeReasoningParts(raw any, kind string) (string, error) {
	if raw == nil {
		return "", nil
	}
	if text, ok := raw.(string); ok {
		return text, nil
	}
	parts, ok := raw.([]any)
	if !ok {
		return "", fmt.Errorf("Responses/Chat bridge: reasoning text must be a string or typed text array")
	}
	var result strings.Builder
	for _, raw := range parts {
		part := bridgeMap(raw)
		if part == nil || part["type"] != kind {
			return "", fmt.Errorf("Responses/Chat bridge: unsupported reasoning text part")
		}
		if err := bridgeFields(part, "type", "text"); err != nil {
			return "", err
		}
		text, err := bridgeText(part["text"], "reasoning text")
		if err != nil {
			return "", err
		}
		result.WriteString(text)
	}
	return result.String(), nil
}
