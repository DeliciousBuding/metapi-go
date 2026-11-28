package generate_content

import (
	"encoding/json"
	"fmt"
	"strings"
)

func geminiResponseFormat(gc map[string]any, body []byte) (map[string]any, error) {
	mime := ""
	if gc["responseMimeType"] != nil {
		var ok bool
		mime, ok = gc["responseMimeType"].(string)
		if !ok {
			return nil, fmt.Errorf("Gemini responseMimeType must be a string")
		}
	}
	legacy, modern := gc["responseSchema"], gc["responseJsonSchema"]
	if legacy != nil && modern != nil {
		return nil, fmt.Errorf("Gemini responseSchema and responseJsonSchema are mutually exclusive")
	}
	if legacy != nil || modern != nil {
		if mime != "application/json" {
			return nil, fmt.Errorf("Gemini response schema requires application/json responseMimeType")
		}
		var schema any
		var err error
		if legacy != nil {
			schema, err = geminiSchemaToJSON(legacy)
		} else {
			// Preserve the original JSON property order, numbers and references.
			// Explicit Gemini propertyOrdering takes precedence over wire order.
			schema, err = geminiJSONSchemaToChat(modern)
			if err == nil && !schemaNeedsRewrite(modern) {
				schema = rawSchema(body, "generationConfig", "responseJsonSchema")
			}
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "json_schema", "json_schema": map[string]any{
			"name": "gemini_response", "schema": schema, "strict": true,
		}}, nil
	}
	switch mime {
	case "":
		return nil, nil
	case "text/plain":
		return map[string]any{"type": "text"}, nil
	case "application/json":
		return map[string]any{"type": "json_object"}, nil
	default:
		return nil, fmt.Errorf("unsupported Gemini responseMimeType %q", mime)
	}
}

func chatResponseFormat(value any, body []byte) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	format, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Chat response_format must be an object")
	}
	gc := map[string]any{}
	switch format["type"] {
	case "text", "json_object":
		if err := bridgeKeys(format, "type"); err != nil {
			return nil, err
		}
		gc["responseMimeType"] = "text/plain"
		if format["type"] == "json_object" {
			gc["responseMimeType"] = "application/json"
		}
	case "json_schema":
		if err := bridgeKeys(format, "type", "json_schema"); err != nil {
			return nil, err
		}
		definition, ok := format["json_schema"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Chat json_schema must be an object")
		}
		if err := bridgeKeys(definition, "name", "description", "schema", "strict"); err != nil {
			return nil, err
		}
		name, ok := definition["name"].(string)
		if !ok || len(name) == 0 || len(name) > 64 || strings.IndexFunc(name, func(r rune) bool {
			return r != '_' && r != '-' && !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9')
		}) >= 0 {
			return nil, fmt.Errorf("Chat JSON Schema requires a valid name")
		}
		if definition["strict"] != nil {
			if _, ok := definition["strict"].(bool); !ok {
				return nil, fmt.Errorf("Chat JSON Schema strict must be boolean")
			}
		}
		schema, ok := definition["schema"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Chat JSON Schema must be an object")
		}
		if err := validateGeminiJSONSchema(schema); err != nil {
			// The older Schema dialect supports string/property constraints
			// absent from responseJsonSchema. Use it only for a lossless schema.
			legacy, legacyErr := jsonSchemaToGeminiSchema(schema)
			if legacyErr != nil {
				return nil, err
			}
			gc["responseSchema"] = legacy
		}
		raw := rawSchema(body, "response_format", "json_schema", "schema")
		if description := definition["description"]; description != nil {
			text, ok := description.(string)
			if !ok {
				return nil, fmt.Errorf("Chat JSON Schema description must be a string")
			}
			if text != "" {
				// The format label is metadata; its description is a model hint.
				// Keep both descriptions without rewriting nested schema objects.
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(raw, &fields)
				if existing, ok := schema["description"].(string); ok && existing != "" && existing != text {
					text += "\n\n" + existing
				}
				fields["description"], _ = json.Marshal(text)
				raw, _ = json.Marshal(fields)
			}
		}
		// Gemini's schema mode supplies constrained output for its supported
		// subset; there is no separate strict flag. Never strip constraints.
		gc["responseMimeType"] = "application/json"
		if legacy, ok := gc["responseSchema"].(map[string]any); ok {
			preserveLegacyPropertyOrder(legacy, raw)
			if description := definition["description"]; description != nil {
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(raw, &fields)
				legacy["description"] = fields["description"]
			}
		} else {
			gc["responseJsonSchema"] = raw
		}
	default:
		return nil, fmt.Errorf("unsupported Chat response_format type")
	}
	return gc, nil
}

func rawSchema(body []byte, path ...string) json.RawMessage {
	raw := json.RawMessage(body)
	for _, key := range path {
		var object map[string]json.RawMessage
		_ = json.Unmarshal(raw, &object)
		raw = object[key]
	}
	return raw
}
