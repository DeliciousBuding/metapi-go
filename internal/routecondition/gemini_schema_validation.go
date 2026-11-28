package routecondition

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
)

func geminiSchema(raw json.RawMessage) (string, error) {
	if !geminiSchemaValid(raw) {
		return string(raw), nil
	}
	return normalizeGeminiSchema(raw, false)
}

// Validate only the schema decoding contract used before the source heuristic.
// When its typed schema decoder rejects a document, the source counts the
// original JSON instead of a partially normalized tree.
func geminiSchemaValid(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false")) || bytes.Equal(raw, []byte("null")) {
		return true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	if isNonNull(fields["$defs"]) && isNonNull(fields["definitions"]) {
		return false
	}
	for key, value := range fields {
		if bytes.Equal(value, []byte("null")) {
			if key == "type" {
				return false
			}
			continue
		}
		switch key {
		case "type":
			var one string
			var many []string
			if json.Unmarshal(value, &one) != nil && json.Unmarshal(value, &many) != nil {
				return false
			}
		case "$id", "$schema", "$ref", "$comment", "$anchor", "$dynamicAnchor", "$dynamicRef",
			"title", "description", "pattern", "format", "contentEncoding", "contentMediaType":
			var text string
			if json.Unmarshal(value, &text) != nil {
				return false
			}
		case "deprecated", "readOnly", "writeOnly", "uniqueItems":
			var flag bool
			if json.Unmarshal(value, &flag) != nil {
				return false
			}
		case "minLength", "maxLength", "minItems", "maxItems", "minContains", "maxContains", "minProperties", "maxProperties":
			var number float64
			if json.Unmarshal(value, &number) != nil || number != math.Trunc(number) || number < math.MinInt32 || number > math.MaxInt32 {
				return false
			}
			if !bytes.ContainsRune(value, '.') {
				if _, err := strconv.ParseInt(string(value), 10, 64); err != nil {
					return false
				}
			}
		case "multipleOf", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum":
			var number float64
			if json.Unmarshal(value, &number) != nil {
				return false
			}
		case "enum", "examples":
			var array []any
			if json.Unmarshal(value, &array) != nil {
				return false
			}
		case "required":
			var array []string
			if json.Unmarshal(value, &array) != nil {
				return false
			}
		case "dependentRequired":
			var entries map[string][]string
			if json.Unmarshal(value, &entries) != nil {
				return false
			}
		case "$vocabulary":
			var entries map[string]bool
			if json.Unmarshal(value, &entries) != nil {
				return false
			}
		case "$defs", "definitions", "properties", "patternProperties", "dependentSchemas", "dependencies":
			var children map[string]json.RawMessage
			if json.Unmarshal(value, &children) != nil {
				return false
			}
			for _, child := range children {
				if key == "dependencies" && len(child) > 0 && child[0] == '[' {
					var names []string
					if json.Unmarshal(child, &names) != nil {
						return false
					}
				} else if !geminiSchemaValid(child) {
					return false
				}
			}
		case "prefixItems", "allOf", "anyOf", "oneOf":
			var children []json.RawMessage
			if json.Unmarshal(value, &children) != nil {
				return false
			}
			for _, child := range children {
				if !geminiSchemaValid(child) {
					return false
				}
			}
		case "items":
			if len(value) > 0 && value[0] == '[' {
				var children []json.RawMessage
				if json.Unmarshal(value, &children) != nil {
					return false
				}
				for _, child := range children {
					if !geminiSchemaValid(child) {
						return false
					}
				}
			} else if !geminiSchemaValid(value) {
				return false
			}
		case "additionalItems", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema", "additionalProperties":
			if !geminiSchemaValid(value) {
				return false
			}
		}
	}
	return true
}
