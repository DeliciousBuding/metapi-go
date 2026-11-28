package routecondition

import (
	"bytes"
	"encoding/json"
	"strings"
)

func normalizeGeminiSchema(raw json.RawMessage, pointer bool) (string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if pointer {
			return "null", nil
		}
		// json.Unmarshal(null, *bool) succeeds with false in the source's
		// Schema.UnmarshalJSON first branch.
		return "false", nil
	}
	var schema map[string]json.RawMessage
	if json.Unmarshal(raw, &schema) != nil || schema == nil {
		// The source preserves an unparseable schema instead of normalizing it.
		return string(raw), nil
	}
	if value, ok := schema["type"]; ok {
		var kind string
		if len(value) > 0 && value[0] == '"' && json.Unmarshal(value, &kind) == nil {
			if kind == "" {
				delete(schema, "type")
			} else {
				schema["type"], _ = json.Marshal(strings.ToLower(kind))
			}
		} else if len(value) == 0 || value[0] != '[' {
			return string(raw), nil
		}
	}
	for _, key := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"} {
		var children map[string]json.RawMessage
		if value, ok := schema[key]; ok {
			if json.Unmarshal(value, &children) != nil {
				return string(raw), nil
			}
			for name, child := range children {
				normalized, err := normalizeGeminiSchema(child, true)
				if err != nil {
					return "", err
				}
				children[name] = json.RawMessage(normalized)
			}
			schema[key], _ = json.Marshal(children)
		}
	}
	if value, ok := schema["dependencies"]; ok {
		var children map[string]json.RawMessage
		if json.Unmarshal(value, &children) != nil {
			return string(raw), nil
		}
		for name, child := range children {
			if len(child) != 0 && child[0] != '[' {
				normalized, err := normalizeGeminiSchema(child, false)
				if err != nil {
					return "", err
				}
				children[name] = json.RawMessage(normalized)
			}
		}
		schema["dependencies"], _ = json.Marshal(children)
	}
	for _, key := range []string{"items", "additionalItems", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema", "additionalProperties"} {
		if value, ok := schema[key]; ok && len(value) > 0 && value[0] == '{' {
			normalized, err := normalizeGeminiSchema(value, key != "items")
			if err != nil {
				return "", err
			}
			schema[key] = json.RawMessage(normalized)
		}
	}
	for _, key := range []string{"prefixItems", "allOf", "anyOf", "oneOf", "items"} {
		var children []json.RawMessage
		if value, ok := schema[key]; ok {
			if key == "items" && (len(value) == 0 || value[0] != '[') {
				continue
			}
			if json.Unmarshal(value, &children) != nil {
				return string(raw), nil
			}
			for i, child := range children {
				normalized, err := normalizeGeminiSchema(child, true)
				if err != nil {
					return "", err
				}
				children[i] = json.RawMessage(normalized)
			}
			schema[key], _ = json.Marshal(children)
		}
	}
	// The source schema representation omits these false/empty metadata values.
	for _, key := range []string{"title", "description", "$id", "$schema", "$ref", "$comment", "$anchor", "$dynamicAnchor", "$dynamicRef", "pattern", "format", "contentEncoding", "contentMediaType"} {
		if value, ok := schema[key]; ok && bytes.Equal(value, []byte(`""`)) {
			delete(schema, key)
		}
	}
	for _, key := range []string{"deprecated", "readOnly", "writeOnly", "uniqueItems"} {
		if value, ok := schema[key]; ok && bytes.Equal(value, []byte("false")) {
			delete(schema, key)
		}
	}
	// The source's typed Schema uses omitempty for every slice/map keyword.
	for _, key := range []string{"$defs", "definitions", "dependencies", "$vocabulary",
		"examples", "enum", "prefixItems", "required", "dependentRequired", "properties",
		"patternProperties", "allOf", "anyOf", "oneOf", "dependentSchemas"} {
		collectionRaw, ok := schema[key]
		if !ok {
			continue
		}
		var collection any
		if json.Unmarshal(collectionRaw, &collection) != nil {
			return string(raw), nil
		}
		switch value := collection.(type) {
		case nil:
			delete(schema, key)
		case []any:
			if len(value) == 0 {
				delete(schema, key)
			}
		case map[string]any:
			if len(value) == 0 {
				delete(schema, key)
			}
		}
	}
	for _, key := range []string{"multipleOf", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
		"minLength", "maxLength", "minItems", "maxItems", "minContains", "maxContains", "minProperties", "maxProperties",
		"enum", "const", "examples"} {
		if raw, ok := schema[key]; ok && !bytes.Equal(raw, []byte("null")) {
			if key == "enum" || key == "const" || key == "examples" {
				normalized, err := compactJSON(raw, true)
				if err != nil {
					return "", err
				}
				schema[key] = json.RawMessage(normalized)
				continue
			}
			var number float64
			if json.Unmarshal(raw, &number) != nil {
				return string(raw), nil
			}
			schema[key], _ = json.Marshal(number)
		}
	}
	for key, value := range schema {
		if !bytes.Equal(value, []byte("null")) || !geminiKnownSchemaField(key) || key == "default" || key == "const" {
			continue
		}
		if key == "items" {
			schema[key] = json.RawMessage("false")
		} else {
			delete(schema, key)
		}
	}
	// Unknown keywords are stored in map[string]any by the source schema
	// library, while default is RawMessage and must keep its number spelling.
	for key, value := range schema {
		if geminiKnownSchemaField(key) {
			continue
		}
		normalized, err := compactJSON(value, true)
		if err != nil {
			return "", err
		}
		schema[key] = json.RawMessage(normalized)
	}
	if len(schema) == 0 {
		return "true", nil
	}
	if len(schema) == 1 && bytes.Equal(schema["not"], []byte("true")) {
		return "false", nil
	}
	encoded, err := json.Marshal(schema)
	return string(encoded), err
}

func geminiKnownSchemaField(key string) bool {
	switch key {
	case "$id", "$schema", "$ref", "$comment", "$defs", "definitions", "dependencies",
		"$anchor", "$dynamicAnchor", "$dynamicRef", "$vocabulary", "title", "description",
		"default", "deprecated", "readOnly", "writeOnly", "examples", "type", "enum", "const",
		"multipleOf", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "minLength",
		"maxLength", "pattern", "prefixItems", "items", "minItems", "maxItems", "additionalItems",
		"uniqueItems", "contains", "minContains", "maxContains", "unevaluatedItems", "minProperties",
		"maxProperties", "required", "dependentRequired", "properties", "patternProperties",
		"additionalProperties", "propertyNames", "unevaluatedProperties", "allOf", "anyOf",
		"oneOf", "not", "if", "then", "else", "dependentSchemas", "contentEncoding",
		"contentMediaType", "contentSchema", "format":
		return true
	}
	return false
}
