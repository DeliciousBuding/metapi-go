package generate_content

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

// Preserve exact JSON numbers and the compatibility builder's fallback for free-form arguments.
func parseToolArguments(raw string) any {
	if raw == "" {
		return map[string]any{}
	}
	var value any
	if shared.DecodeJSON(strings.NewReader(raw), &value) != nil {
		return map[string]any{"value": raw}
	}
	return value
}

func convertOpenAiToolsToGemini(tools any) any {
	arr, ok := tools.([]any)
	if !ok {
		return tools
	}
	var fds []map[string]any
	var otherTools []map[string]any
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t := strings.ToLower(shared.AsTrimmedString(m["type"]))
		if t == "function" {
			if fn, ok := m["function"].(map[string]any); ok {
				name := shared.AsTrimmedString(fn["name"])
				if name == "" {
					continue
				}
				fd := map[string]any{"name": name}
				if d := shared.AsTrimmedString(fn["description"]); d != "" {
					fd["description"] = d
				}
				if params, ok := fn["parameters"].(map[string]any); ok {
					fd["parametersJsonSchema"] = params
				}
				fds = append(fds, fd)
			}
		} else if t == "web_search" || t == "google_search" {
			otherTools = append(otherTools, map[string]any{"googleSearch": map[string]any{}})
		} else if t == "code_interpreter" {
			otherTools = append(otherTools, map[string]any{"codeExecution": map[string]any{}})
		}
	}
	var result []map[string]any
	if len(fds) > 0 {
		result = append(result, map[string]any{"functionDeclarations": fds})
	}
	result = append(result, otherTools...)
	if len(result) == 0 {
		return tools
	}
	return result
}

func convertOpenAiToolChoiceToGemini(tc any) any {
	var mode string
	if s, ok := tc.(string); ok {
		n := strings.ToUpper(strings.TrimSpace(s))
		switch n {
		case "NONE":
			mode = "NONE"
		case "REQUIRED", "ANY":
			mode = "ANY"
		case "AUTO":
			mode = "AUTO"
		default:
			mode = "AUTO"
		}
	} else if m, ok := tc.(map[string]any); ok {
		t := strings.ToLower(shared.AsTrimmedString(m["type"]))
		switch t {
		case "none":
			mode = "NONE"
		case "required", "any":
			mode = "ANY"
		case "function", "tool":
			mode = "ANY"
		default:
			mode = "AUTO"
		}
	} else {
		mode = "AUTO"
	}
	return map[string]any{
		"functionCallingConfig": map[string]any{"mode": mode},
	}
}
