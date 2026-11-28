package routing

import (
	"strconv"
	"strings"
)

// ParseModelMappingRecord parses a model mapping JSON string into a map.
func ParseModelMappingRecord(raw *string) map[string]string {
	if raw == nil {
		return nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" || s[0] != '{' {
		return nil
	}
	// Simple JSON parse for string-to-string mapping.
	result := make(map[string]string)
	// Strip outer braces
	inner := s[1 : len(s)-1]
	if inner == "" {
		return nil
	}
	parts := splitJSONPairs(inner)
	for _, part := range parts {
		kv := splitJSONPair(part)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// splitJSONPairs splits a JSON object body into top-level key:value pairs.
// Simple implementation for model mapping JSON.
func splitJSONPairs(s string) []string {
	var result []string
	depth := 0
	start := 0
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
		}
		if inString {
			continue
		}
		if c == '{' || c == '[' {
			depth++
		} else if c == '}' || c == ']' {
			depth--
		} else if c == ',' && depth == 0 {
			part := strings.TrimSpace(s[start:i])
			if part != "" {
				result = append(result, part)
			}
			start = i + 1
		}
	}
	part := strings.TrimSpace(s[start:])
	if part != "" {
		result = append(result, part)
	}
	return result
}

// splitJSONPair splits a single "key":"value" pair.
func splitJSONPair(s string) []string {
	// Find the first colon outside strings
	inString := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
		}
		if !inString && c == ':' {
			key := unquoteJSON(strings.TrimSpace(s[:i]))
			val := unquoteJSON(strings.TrimSpace(s[i+1:]))
			return []string{key, val}
		}
	}
	return nil
}

// unquoteJSON removes surrounding double quotes and unescapes. Delegates to
// strconv.Unquote so all JSON string escapes decode faithfully — including
// \uXXXX (with surrogate pairs), \b, \f, \r — and escape handling is
// left-to-right correct (a hand-rolled ReplaceAll cascade is order-dependent
// and mis-tokenizes even runs of backslashes). On any parse error the original
// token is returned unchanged.
func unquoteJSON(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if unquoted, err := strconv.Unquote(s); err == nil {
			return unquoted
		}
	}
	return s
}

// ResolveMappedModel resolves the target model name from a model mapping.
func ResolveMappedModel(requestedModel string, modelMapping *string) string {
	parsed := ParseModelMappingRecord(modelMapping)
	if parsed == nil {
		return requestedModel
	}

	// Exact key match first
	if target, ok := parsed[requestedModel]; ok {
		return strings.TrimSpace(target)
	}

	// Pattern match fallback
	for pattern, target := range parsed {
		if MatchesModelPattern(requestedModel, pattern) {
			return strings.TrimSpace(target)
		}
	}

	return requestedModel
}
