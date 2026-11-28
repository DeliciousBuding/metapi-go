package generate_content

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func geminiSchemaToJSON(value any) (any, error) {
	return convertGeminiSchema(value, true)
}

func geminiJSONSchemaToChat(value any) (any, error) {
	if err := validateGeminiSchemaKeywords(value, true); err != nil {
		return nil, err
	}
	return convertGeminiSchema(value, false)
}

func convertGeminiSchema(value any, legacy bool) (any, error) {
	in, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Gemini schema must be an object")
	}
	out := make(map[string]any, len(in))
	for key, entry := range in {
		switch key {
		case "properties", "$defs", "definitions":
			properties, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Gemini schema %s", key)
			}
			converted := map[string]any{}
			for name, child := range properties {
				v, err := convertGeminiSchema(child, legacy)
				if err != nil {
					return nil, err
				}
				converted[name] = v
			}
			out[key] = converted
		case "items", "additionalProperties", "not":
			if _, ok := entry.(bool); ok && !legacy {
				out[key] = entry
				continue
			}
			v, err := convertGeminiSchema(entry, legacy)
			if err != nil {
				return nil, err
			}
			out[key] = v
		case "anyOf", "allOf", "oneOf", "prefixItems":
			if key == "oneOf" && !legacy {
				if in["anyOf"] != nil {
					return nil, fmt.Errorf("combined Gemini oneOf and anyOf require a native endpoint")
				}
				key = "anyOf"
			}
			children, ok := entry.([]any)
			if !ok {
				return nil, fmt.Errorf("invalid Gemini schema %s", key)
			}
			converted := make([]any, len(children))
			for i, child := range children {
				v, err := convertGeminiSchema(child, legacy)
				if err != nil {
					return nil, err
				}
				converted[i] = v
			}
			out[key] = converted
		case "type":
			if !legacy {
				out[key] = entry
				continue
			}
			name, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("invalid Gemini schema type")
			}
			name = strings.ToLower(name)
			switch name {
			case "object", "array", "string", "integer", "number", "boolean", "null":
				out[key] = name
			default:
				return nil, fmt.Errorf("invalid Gemini schema type")
			}
		case "nullable":
			if !legacy {
				out[key] = entry
			} else if _, ok := entry.(bool); !ok {
				return nil, fmt.Errorf("invalid Gemini schema nullable")
			}
		case "minItems", "maxItems", "minLength", "maxLength", "minProperties", "maxProperties":
			if text, ok := entry.(string); ok && legacy {
				n, err := strconv.ParseInt(text, 10, 64)
				if err != nil || n < 0 {
					return nil, fmt.Errorf("invalid Gemini schema %s", key)
				}
				out[key] = json.Number(strconv.FormatInt(n, 10))
			} else {
				out[key] = entry
			}
		case "propertyOrdering":
			// Applied below to the actual JSON properties object.
		default:
			out[key] = entry
		}
	}
	if ordering := in["propertyOrdering"]; ordering != nil {
		properties, ok := out["properties"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Gemini propertyOrdering requires properties")
		}
		ordered, err := orderedSchemaProperties(properties, ordering)
		if err != nil {
			return nil, err
		}
		out["properties"] = ordered
	}
	if legacy && in["nullable"] == true {
		// Union rather than only changing type also preserves nullable enums.
		return map[string]any{"anyOf": []any{out, map[string]any{"type": "null"}}}, nil
	}
	return out, nil
}
