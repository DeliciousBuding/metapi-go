// Package tokenestimate estimates request size for routing and TPM admission.
package tokenestimate

import (
	"encoding/json"
	"unicode/utf8"
)

// Request counts text runes, including property names in tool schemas, at the
// gateway's historical four-runes-per-token rate, then adds one output budget.
// A missing input signal returns zero so the caller can apply its own fallback.
func Request(body map[string]any, limit int64) int64 {
	var chars int64
	for _, key := range []string{"messages", "input", "system", "contents", "instructions", "prompt", "tools"} {
		chars += textRunes(body[key], key == "tools")
	}
	if chars == 0 {
		return 0
	}
	tokens := min(limit, max(int64(1), chars/4))
	return tokens + min(parsePositiveTokenCap(body), limit-tokens)
}

func textRunes(v any, countKeys bool) int64 {
	var total int64
	switch v := v.(type) {
	case string:
		return int64(utf8.RuneCountInString(v))
	case []any:
		for _, item := range v {
			total += textRunes(item, countKeys)
		}
	case map[string]any:
		for k, item := range v {
			if countKeys {
				total += int64(utf8.RuneCountInString(k))
			}
			total += textRunes(item, countKeys)
		}
	case []string:
		for _, s := range v {
			total += int64(utf8.RuneCountInString(s))
		}
	case []map[string]any:
		for _, item := range v {
			total += textRunes(item, countKeys)
		}
	case json.RawMessage:
		var item any
		if json.Unmarshal(v, &item) == nil {
			return textRunes(item, countKeys)
		}
	}
	return total
}
func parsePositiveTokenCap(body map[string]any) int64 {
	for _, key := range []string{"max_output_tokens", "max_completion_tokens", "max_tokens"} {
		if v, ok := body[key]; ok {
			if n, ok := anyToPositiveInt64(v); ok {
				return n
			}
		}
	}
	// Gemini nested generationConfig.maxOutputTokens
	if gen, ok := body["generationConfig"].(map[string]any); ok {
		for _, key := range []string{"maxOutputTokens", "max_output_tokens"} {
			if v, ok := gen[key]; ok {
				if n, ok := anyToPositiveInt64(v); ok {
					return n
				}
			}
		}
	}
	if req, ok := body["request"].(map[string]any); ok {
		return parsePositiveTokenCap(req)
	}
	return 0
}

func anyToPositiveInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		if n > 0 {
			return int64(n), true
		}
	case int32:
		if n > 0 {
			return int64(n), true
		}
	case int64:
		if n > 0 {
			return n, true
		}
	case float64:
		if n > 0 && n < float64(int64(^uint64(0)>>1)) {
			return int64(n), true
		}
	case json.Number:
		i, err := n.Int64()
		if err == nil && i > 0 {
			return i, true
		}
	case string:
		// leave unparsed — avoid inventing
	}
	return 0, false
}
