package generate_content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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
				var fields map[string]any
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

func geminiSchemaToJSON(value any) (any, error) {
	return convertGeminiSchema(value, true)
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

func orderedSchemaProperties(properties map[string]any, value any) (json.RawMessage, error) {
	order, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid Gemini propertyOrdering")
	}
	names := make([]string, 0, len(properties))
	seen := map[string]bool{}
	for _, raw := range order {
		name, ok := raw.(string)
		if !ok || seen[name] || properties[name] == nil {
			return nil, fmt.Errorf("Gemini propertyOrdering must name distinct properties")
		}
		seen[name] = true
		names = append(names, name)
	}
	var remaining []string
	for name := range properties {
		if !seen[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)
	names = append(names, remaining...)
	var out bytes.Buffer
	out.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			out.WriteByte(',')
		}
		key, _ := json.Marshal(name)
		value, err := json.Marshal(properties[name])
		if err != nil {
			return nil, err
		}
		out.Write(key)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

func schemaNeedsRewrite(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if v["propertyOrdering"] != nil || v["oneOf"] != nil {
			return true
		}
		for _, child := range v {
			if schemaNeedsRewrite(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if schemaNeedsRewrite(child) {
				return true
			}
		}
	}
	return false
}

func preserveLegacyPropertyOrder(schema map[string]any, raw json.RawMessage) {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if properties, ok := schema["properties"].(map[string]any); ok {
		decoder := json.NewDecoder(bytes.NewReader(fields["properties"]))
		_, _ = decoder.Token()
		var order []string
		for decoder.More() {
			key, _ := decoder.Token()
			name, _ := key.(string)
			var child json.RawMessage
			_ = decoder.Decode(&child)
			order = append(order, name)
			if converted, ok := properties[name].(map[string]any); ok {
				preserveLegacyPropertyOrder(converted, child)
			}
		}
		if len(order) > 0 && schema["propertyOrdering"] == nil {
			schema["propertyOrdering"] = order
		}
	}
	if items, ok := schema["items"].(map[string]any); ok {
		preserveLegacyPropertyOrder(items, fields["items"])
	}
	if alternatives, ok := schema["anyOf"].([]any); ok {
		var children []json.RawMessage
		_ = json.Unmarshal(fields["anyOf"], &children)
		for i, child := range alternatives {
			if converted, ok := child.(map[string]any); ok && i < len(children) {
				preserveLegacyPropertyOrder(converted, children[i])
			}
		}
	}
}
