package generate_content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

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
