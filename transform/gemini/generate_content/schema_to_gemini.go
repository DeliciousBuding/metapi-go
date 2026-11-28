package generate_content

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Gemini accepts a full JSON Schema document but does not enforce every keyword.
// Keep this check at schema nodes: enum/default/example are data, not schemas.
// oneOf is deliberately excluded because Gemini interprets it as anyOf.
// Contract: https://ai.google.dev/api/generate-content#GenerationConfig
func validateGeminiJSONSchema(value any) error {
	return validateGeminiSchemaKeywords(value, false)
}

func validateGeminiSchemaKeywords(value any, fromGemini bool) error {
	schema, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("Gemini JSON Schema requires schema objects")
	}
	for key, entry := range schema {
		if schema["$ref"] != nil && !strings.HasPrefix(key, "$") {
			return fmt.Errorf("Gemini JSON Schema cannot preserve $ref sibling %s", key)
		}
		switch key {
		case "$id", "$ref", "$anchor", "$schema", "type", "format", "title", "description", "enum", "minimum", "maximum", "minItems", "maxItems", "required", "propertyOrdering", "default", "examples", "example":
		case "properties", "$defs":
			children, ok := entry.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid JSON Schema %s", key)
			}
			for _, child := range children {
				if err := validateGeminiSchemaKeywords(child, fromGemini); err != nil {
					return err
				}
			}
		case "items", "additionalProperties":
			if _, ok := entry.(bool); ok && key == "additionalProperties" {
				continue
			}
			if err := validateGeminiSchemaKeywords(entry, fromGemini); err != nil {
				return err
			}
		case "anyOf", "prefixItems", "oneOf":
			if key == "oneOf" && !fromGemini {
				return fmt.Errorf("Gemini responseJsonSchema interprets oneOf as anyOf")
			}
			children, ok := entry.([]any)
			if !ok {
				return fmt.Errorf("invalid JSON Schema %s", key)
			}
			for _, child := range children {
				if err := validateGeminiSchemaKeywords(child, fromGemini); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("Gemini responseJsonSchema cannot preserve keyword %s", key)
		}
	}
	return nil
}

func jsonSchemaToGeminiSchema(value any) (map[string]any, error) {
	in, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Gemini Schema requires an object")
	}
	out := map[string]any{}
	for key, value := range in {
		switch key {
		case "title", "description", "format", "default", "example", "minimum", "maximum", "pattern", "required", "propertyOrdering":
			out[key] = value
		case "type":
			name, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("Gemini Schema requires a single type")
			}
			switch name {
			case "object", "array", "string", "number", "integer", "boolean", "null":
				out[key] = strings.ToUpper(name)
			default:
				return nil, fmt.Errorf("invalid JSON Schema type")
			}
		case "minItems", "maxItems", "minLength", "maxLength", "minProperties", "maxProperties":
			n, ok := value.(json.Number)
			if !ok {
				return nil, fmt.Errorf("invalid JSON Schema %s", key)
			}
			count, err := n.Int64()
			if err != nil || count < 0 {
				return nil, fmt.Errorf("invalid JSON Schema %s", key)
			}
			out[key] = strconv.FormatInt(count, 10)
		case "properties":
			children, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid JSON Schema properties")
			}
			properties := map[string]any{}
			for name, child := range children {
				converted, err := jsonSchemaToGeminiSchema(child)
				if err != nil {
					return nil, err
				}
				properties[name] = converted
			}
			out[key] = properties
		case "items":
			converted, err := jsonSchemaToGeminiSchema(value)
			if err != nil {
				return nil, err
			}
			out[key] = converted
		case "anyOf":
			children, ok := value.([]any)
			if !ok {
				return nil, fmt.Errorf("invalid JSON Schema anyOf")
			}
			converted := make([]any, len(children))
			for i, child := range children {
				v, err := jsonSchemaToGeminiSchema(child)
				if err != nil {
					return nil, err
				}
				converted[i] = v
			}
			out[key] = converted
		case "enum":
			items, ok := value.([]any)
			if !ok {
				return nil, fmt.Errorf("invalid JSON Schema enum")
			}
			for _, item := range items {
				if _, ok := item.(string); !ok {
					return nil, fmt.Errorf("Gemini Schema requires string enum values")
				}
			}
			out[key] = value
		default:
			return nil, fmt.Errorf("Gemini Schema cannot preserve keyword %s", key)
		}
	}
	return out, nil
}
