package shared

import (
	"encoding/json"
	"strings"
)

// AsTrimmedString returns the trimmed string form of v, or "" for non-strings.
func AsTrimmedString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// ParseJSONLike parses raw as JSON, returning an empty map for empty input and
// wrapping unparseable input as {"value": raw}.
func ParseJSONLike(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return map[string]any{"value": raw}
	}
	return v
}
