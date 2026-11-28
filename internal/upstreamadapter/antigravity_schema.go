package upstreamadapter

import (
	"fmt"
	"slices"
	"strings"
)

func antigravityConvertSchema(object map[string]any, modern, legacy string) error {
	value, present := object[modern]
	if !present || value == nil {
		value, present = object[legacy]
	}
	if !present || value == nil {
		return nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid Antigravity %s", legacy)
	}
	object[legacy] = antigravitySchema(schema)
	delete(object, modern)
	return nil
}

// Antigravity uses a constrained protobuf schema. Match the source gateway's
// explicit hint conversion; schema keywords become descriptions rather than
// silently vanishing. Only schema nodes are traversed: property names and enum
// values remain user data. JSON numbers stay json.Number throughout the adapter.
func antigravitySchema(schema map[string]any) map[string]any {
	// The order matters: hints from allOf/union children must exist before
	// those children are merged into their parent schema.
	for _, phase := range []func(map[string]any) map[string]any{
		antigravitySchemaReferences, antigravitySchemaConsts, antigravitySchemaEnums,
		antigravitySchemaAdditional, antigravitySchemaConstraints, antigravitySchemaAllOf,
		antigravitySchemaUnions,
	} {
		schema = visitAntigravitySchema(schema, phase)
	}
	schema = visitAntigravitySchema(schema, antigravitySchemaTypes)
	antigravitySchemaNullableRoot(schema)
	for _, phase := range []func(map[string]any) map[string]any{
		antigravitySchemaCleanup, antigravitySchemaRequired, antigravitySchemaEmpty,
		antigravitySchemaUppercase,
	} {
		schema = visitAntigravitySchema(schema, phase)
	}
	return schema
}

func visitAntigravitySchema(schema map[string]any, phase func(map[string]any) map[string]any) map[string]any {
	schema = phase(schema)
	for _, name := range []string{"properties", "$defs", "definitions"} {
		if children, ok := schema[name].(map[string]any); ok {
			for key, value := range children {
				if child, ok := value.(map[string]any); ok {
					children[key] = visitAntigravitySchema(child, phase)
				}
			}
		}
	}
	for _, name := range []string{"items", "additionalProperties", "propertyNames", "not"} {
		if child, ok := schema[name].(map[string]any); ok {
			schema[name] = visitAntigravitySchema(child, phase)
		}
	}
	for _, name := range []string{"anyOf", "oneOf", "allOf", "prefixItems"} {
		if children, ok := schema[name].([]any); ok {
			for i, child := range children {
				if object, ok := child.(map[string]any); ok {
					children[i] = visitAntigravitySchema(object, phase)
				}
			}
		}
	}
	return schema
}

func antigravitySchemaReferences(schema map[string]any) map[string]any {
	if ref, ok := schema["$ref"].(string); ok {
		name := ref[strings.LastIndex(ref, "/")+1:]
		replacement := map[string]any{"type": "object"}
		if description, ok := schema["description"].(string); ok {
			replacement["description"] = description
		}
		antigravitySchemaHint(replacement, "See: "+name)
		return replacement
	}
	return schema
}

func antigravitySchemaConsts(schema map[string]any) map[string]any {
	if value, exists := schema["const"]; exists {
		if _, exists := schema["enum"]; !exists {
			schema["enum"] = []any{value}
		}
	}
	return schema
}

func antigravitySchemaEnums(schema map[string]any) map[string]any {
	if values, ok := schema["enum"].([]any); ok && len(values) > 1 && len(values) <= 10 {
		text := make([]string, len(values))
		for i, value := range values {
			text[i] = fmt.Sprint(value)
		}
		antigravitySchemaHint(schema, "Allowed: "+strings.Join(text, ", "))
	}
	return schema
}

func antigravitySchemaAdditional(schema map[string]any) map[string]any {
	if value, ok := schema["additionalProperties"].(bool); ok && !value {
		antigravitySchemaHint(schema, "No extra properties allowed")
	}
	return schema
}

var antigravitySchemaConstraintNames = []string{"minLength", "maxLength", "exclusiveMinimum", "exclusiveMaximum", "pattern", "minItems", "maxItems", "format", "default", "examples"}

func antigravitySchemaConstraints(schema map[string]any) map[string]any {
	for _, name := range antigravitySchemaConstraintNames {
		if value, exists := schema[name]; exists {
			switch value.(type) {
			case map[string]any, []any:
			default:
				antigravitySchemaHint(schema, name+": "+fmt.Sprint(value))
			}
		}
	}
	return schema
}

func antigravitySchemaAllOf(schema map[string]any) map[string]any {
	all, ok := schema["allOf"].([]any)
	if !ok {
		return schema
	}
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
	}
	required, _ := schema["required"].([]any)
	for _, raw := range all {
		child, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if props, ok := child["properties"].(map[string]any); ok {
			for key, value := range props {
				properties[key] = value
			}
		}
		if names, ok := child["required"].([]any); ok {
			for _, name := range names {
				if text, ok := name.(string); ok && !slices.ContainsFunc(required, func(value any) bool { return value == text }) {
					required = append(required, text)
				}
			}
		}
		for key, value := range child {
			if key != "properties" && key != "required" {
				if _, exists := schema[key]; !exists {
					schema[key] = value
				}
			}
		}
	}
	if len(properties) > 0 {
		schema["properties"] = properties
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	delete(schema, "allOf")
	return schema
}

func antigravitySchemaUnions(schema map[string]any) map[string]any {
	for _, union := range []string{"anyOf", "oneOf"} {
		options, ok := schema[union].([]any)
		if !ok || len(options) == 0 {
			continue
		}
		if values := antigravityUnionEnums(options); len(values) > 0 {
			schema["type"], schema["enum"] = "string", values
			delete(schema, union)
			continue
		}
		best, score := -1, -1
		var types []string
		for i, option := range options {
			if child, ok := option.(map[string]any); ok {
				kind, _ := child["type"].(string)
				points := 0
				switch {
				case kind == "object" || child["properties"] != nil:
					points, kind = 3, "object"
				case kind == "array" || child["items"] != nil:
					points, kind = 2, "array"
				case kind != "" && kind != "null":
					points = 1
				case kind == "":
					kind = "null"
				}
				if points > score {
					best, score = i, points
				}
				if !slices.Contains(types, kind) {
					types = append(types, kind)
				}
			}
		}
		if best >= 0 {
			selected := antigravitySchemaUnions(options[best].(map[string]any))
			parentDescription, _ := schema["description"].(string)
			for key, value := range selected {
				schema[key] = value
			}
			if parentDescription != "" {
				childDescription, _ := selected["description"].(string)
				schema["description"] = parentDescription
				if childDescription != "" && childDescription != parentDescription {
					antigravitySchemaHint(schema, childDescription)
				}
			}
			if len(types) > 1 {
				antigravitySchemaHint(schema, "Accepts: "+strings.Join(types, " | "))
			}
			delete(schema, union)
		}
	}
	return schema
}

func antigravitySchemaTypes(schema map[string]any) map[string]any {
	if values, ok := schema["type"].([]any); ok {
		var types []string
		nullable := false
		for _, value := range values {
			if name, ok := value.(string); ok {
				if name == "null" {
					nullable = true
				} else {
					types = append(types, name)
				}
			}
		}
		schema["type"] = "string"
		if len(types) > 0 {
			schema["type"] = types[0]
		} else if nullable {
			schema["type"] = "null"
		}
		if len(types) > 1 {
			antigravitySchemaHint(schema, "Accepts: "+strings.Join(types, " | "))
		}
		if nullable {
			antigravitySchemaHint(schema, "nullable")
		}
	}
	return schema
}

// The source removes nullable properties from required only at the root.
func antigravitySchemaNullableRoot(schema map[string]any) {
	names, ok := schema["required"].([]any)
	if !ok {
		return
	}
	properties, _ := schema["properties"].(map[string]any)
	filtered := make([]any, 0, len(names))
	for _, name := range names {
		key, ok := name.(string)
		if !ok {
			continue
		}
		child, _ := properties[key].(map[string]any)
		description, _ := child["description"].(string)
		if !strings.Contains(description, "nullable") {
			filtered = append(filtered, key)
		}
	}
	if len(filtered) == 0 {
		delete(schema, "required")
	} else {
		schema["required"] = filtered
	}
}

func antigravitySchemaCleanup(schema map[string]any) map[string]any {
	for _, name := range antigravitySchemaConstraintNames {
		delete(schema, name)
	}
	for _, name := range []string{"$schema", "$defs", "definitions", "const", "$ref", "additionalProperties", "propertyNames", "title", "$id", "$comment"} {
		delete(schema, name)
	}
	return schema
}

func antigravitySchemaRequired(schema map[string]any) map[string]any {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return schema
	}
	if required, ok := schema["required"].([]any); ok {
		valid := make([]any, 0, len(required))
		for _, raw := range required {
			if name, ok := raw.(string); ok {
				if _, exists := properties[name]; exists {
					valid = append(valid, name)
				}
			}
		}
		if len(valid) > 0 {
			schema["required"] = valid
		} else {
			delete(schema, "required")
		}
	}
	return schema
}

func antigravitySchemaEmpty(schema map[string]any) map[string]any {
	if schema["type"] == "object" {
		properties, _ := schema["properties"].(map[string]any)
		if len(properties) == 0 {
			schema["properties"] = map[string]any{"_placeholder": map[string]any{"type": "boolean", "description": "Placeholder. Always pass true."}}
			schema["required"] = []any{"_placeholder"}
		}
	}
	return schema
}

func antigravitySchemaUppercase(schema map[string]any) map[string]any {
	if kind, ok := schema["type"].(string); ok {
		schema["type"] = strings.ToUpper(kind)
	}
	return schema
}

func antigravitySchemaHint(schema map[string]any, hint string) {
	existing, _ := schema["description"].(string)
	if existing == "" {
		schema["description"] = hint
	} else {
		schema["description"] = existing + " (" + hint + ")"
	}
}

func antigravityUnionEnums(options []any) []any {
	var values []any
	for _, value := range options {
		child, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		if constant, exists := child["const"]; exists {
			values = append(values, constant)
		} else if enumeration, ok := child["enum"].([]any); ok && len(enumeration) > 0 {
			values = append(values, enumeration...)
		} else if child["properties"] != nil || child["items"] != nil || child["anyOf"] != nil || child["oneOf"] != nil || child["allOf"] != nil || child["type"] != nil {
			return nil
		}
	}
	return values
}
