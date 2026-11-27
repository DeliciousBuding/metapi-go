package proxyhandler

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildCodexWebsocketRequestBody wraps a responses body as type=response.create.
func BuildCodexWebsocketRequestBody(body map[string]any) map[string]any {
	next := make(map[string]any, len(body)+1)
	for k, v := range body {
		next[k] = v
	}
	next["type"] = "response.create"
	return next
}

func buildContinuationAwareRuntimeBody(sessionID string, body map[string]any) map[string]any {
	remembered := getCodexSessionResponseID(sessionID)
	if !shouldInferResponsesPreviousResponseID(body, remembered) {
		return body
	}
	return withResponsesPreviousResponseID(body, remembered)
}

func hasResponsesToolOutput(input any) bool {
	arr, ok := input.([]any)
	if !ok {
		return false
	}
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		t := strings.ToLower(strings.TrimSpace(asStringAny(m["type"])))
		if t != "function_call_output" && t != "custom_tool_call_output" {
			continue
		}
		id := strings.TrimSpace(asStringAny(m["call_id"]))
		if id == "" {
			id = strings.TrimSpace(asStringAny(m["id"]))
		}
		if id != "" {
			return true
		}
	}
	return false
}

func shouldInferResponsesPreviousResponseID(body map[string]any, candidate string) bool {
	if body == nil {
		return false
	}
	if strings.TrimSpace(asStringAny(body["previous_response_id"])) != "" {
		return false
	}
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	return hasResponsesToolOutput(body["input"])
}

func withResponsesPreviousResponseID(body map[string]any, previousResponseID string) map[string]any {
	next := make(map[string]any, len(body)+1)
	for k, v := range body {
		next[k] = v
	}
	next["previous_response_id"] = strings.TrimSpace(previousResponseID)
	return next
}

func stripResponsesPreviousResponseID(body map[string]any) (map[string]any, bool) {
	if body == nil {
		return body, false
	}
	if _, ok := body["previous_response_id"]; !ok {
		return body, false
	}
	next := make(map[string]any, len(body))
	for k, v := range body {
		if k == "previous_response_id" {
			continue
		}
		next[k] = v
	}
	return next, true
}

func isResponsesPreviousResponseNotFoundError(rawErrText string, payload any) bool {
	fragments := collectResponsesErrorFragments(payload)
	if s := strings.TrimSpace(rawErrText); s != "" {
		fragments = append(fragments, s)
	}
	combined := strings.ToLower(strings.Join(fragments, " "))
	if combined == "" {
		return false
	}
	// TS parity: previous_response_id not found / unknown response id style errors.
	return (strings.Contains(combined, "previous_response_id") || strings.Contains(combined, "previous response")) &&
		(strings.Contains(combined, "not found") || strings.Contains(combined, "unknown") || strings.Contains(combined, "invalid"))
}

func collectResponsesErrorFragments(value any) []string {
	if value == nil {
		return nil
	}
	if s, ok := value.(string); ok {
		if t := strings.TrimSpace(s); t != "" {
			return []string{t}
		}
		return nil
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	var out []string
	for _, key := range []string{"type", "code", "message", "reason"} {
		if t := strings.TrimSpace(asStringAny(m[key])); t != "" {
			out = append(out, t)
		}
	}
	if e, ok := m["error"]; ok {
		out = append(out, collectResponsesErrorFragments(e)...)
	}
	if r, ok := m["response"]; ok {
		out = append(out, collectResponsesErrorFragments(r)...)
	}
	if d, ok := m["incomplete_details"]; ok {
		out = append(out, collectResponsesErrorFragments(d)...)
	}
	return out
}

func asStringAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func isTerminalResponsesEvent(payload map[string]any) bool {
	t := strings.TrimSpace(asStringAny(payload["type"]))
	return isResponsesTerminalType(t) || t == "error"
}

func isRuntimeErrorEvent(payload map[string]any) bool {
	return strings.TrimSpace(asStringAny(payload["type"])) == "error"
}

func extractResponsesTerminalResponseID(payload any) string {
	m, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	// Prefer nested response.id on terminal events.
	if resp, ok := m["response"].(map[string]any); ok {
		if id := strings.TrimSpace(asStringAny(resp["id"])); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(asStringAny(m["id"])); id != "" && strings.HasPrefix(id, "resp_") {
		return id
	}
	return ""
}

func extractTerminalErrorMessage(payload map[string]any) string {
	t := strings.TrimSpace(asStringAny(payload["type"]))
	if t == "error" {
		if e, ok := payload["error"].(map[string]any); ok {
			if msg := strings.TrimSpace(asStringAny(e["message"])); msg != "" {
				return msg
			}
		}
		return "upstream websocket error"
	}
	if t == "response.failed" || t == "response.incomplete" {
		if resp, ok := payload["response"].(map[string]any); ok {
			if e, ok := resp["error"].(map[string]any); ok {
				if msg := strings.TrimSpace(asStringAny(e["message"])); msg != "" {
					return msg
				}
			}
			if d, ok := resp["incomplete_details"].(map[string]any); ok {
				if reason := strings.TrimSpace(asStringAny(d["reason"])); reason != "" {
					return reason
				}
			}
		}
		return "upstream " + t
	}
	if t == "" {
		return "upstream websocket error"
	}
	return "upstream " + t
}

func extractFailureTerminalStatus(payload map[string]any) int {
	candidates := []any{payload["status"], payload["statusCode"], payload["code"]}
	if e, ok := payload["error"].(map[string]any); ok {
		candidates = append(candidates, e["status"], e["statusCode"], e["code"])
	}
	if resp, ok := payload["response"].(map[string]any); ok {
		if e, ok := resp["error"].(map[string]any); ok {
			candidates = append(candidates, e["status"], e["statusCode"], e["code"])
		}
	}
	for _, c := range candidates {
		switch n := c.(type) {
		case float64:
			if n >= 400 && n < 600 {
				return int(n)
			}
		case int:
			if n >= 400 && n < 600 {
				return n
			}
		case json.Number:
			if v, err := n.Int64(); err == nil && v >= 400 && v < 600 {
				return int(v)
			}
		}
	}
	return 502
}
