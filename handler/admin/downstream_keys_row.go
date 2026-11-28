package admin

import (
	"encoding/json"
	"strings"
)

func redactDownstreamKeySecret(row map[string]any) {
	if row == nil {
		return
	}
	if key, ok := row["key"].(string); ok {
		row["keyMasked"] = maskSecret(key)
	}
	delete(row, "key")
}

func rowValue(row map[string]any, key string) (any, bool) {
	if v, ok := row[key]; ok {
		return v, true
	}
	camelKey := snakeToCamel(key)
	if camelKey != key {
		v, ok := row[camelKey]
		return v, ok
	}
	return nil, false
}

func existingString(row map[string]any, key string) string {
	if v, ok := rowValue(row, key); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

func existingStringPtr(row map[string]any, key string) *string {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return nil
	}
	s, ok2 := v.(string)
	if !ok2 || s == "" {
		return nil
	}
	return &s
}

func existingBool(row map[string]any, key string) bool {
	v, ok := rowValue(row, key)
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val == "1" || strings.EqualFold(val, "true")
	}
	return false
}

func existingFloat64Ptr(row map[string]any, key string) *float64 {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		return &val
	case int64:
		f := float64(val)
		return &f
	case json.Number:
		f, err := val.Float64()
		if err != nil {
			return nil
		}
		return &f
	}
	return nil
}

func existingInt64Ptr(row map[string]any, key string) *int64 {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case int64:
		return &val
	case float64:
		i := int64(val)
		return &i
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return nil
		}
		return &i
	}
	return nil
}

// parseJsonField unmarshals a DB column value (string or already-parsed) into target.
func parseJsonField(row map[string]any, key string, target any) {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return
		}
		json.Unmarshal([]byte(val), target)
	case []byte:
		if len(val) == 0 {
			return
		}
		json.Unmarshal(val, target)
	default:
		// Already decoded (e.g. a PostgreSQL JSON column).
		data, _ := json.Marshal(val)
		json.Unmarshal(data, target)
	}
}

func parseStringArrayFromDB(row map[string]any, key string) []string {
	var arr []string
	parseJsonField(row, key, &arr)
	return arr
}

func parseIntArrayFromDB(row map[string]any, key string) []int64 {
	var arr []int64
	parseJsonField(row, key, &arr)
	return arr
}

func parseMapFromDB(row map[string]any, key string) map[string]float64 {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val == "" || val == "{}" {
			return nil
		}
		var raw map[string]float64
		if err := json.Unmarshal([]byte(val), &raw); err != nil {
			return nil
		}
		return raw
	case []byte:
		if len(val) == 0 || string(val) == "{}" {
			return nil
		}
		var raw map[string]float64
		if err := json.Unmarshal(val, &raw); err != nil {
			return nil
		}
		return raw
	default:
		data, _ := json.Marshal(val)
		var raw map[string]float64
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil
		}
		return raw
	}
}

func parseAnyArrayFromDB(row map[string]any, key string) []any {
	var arr []any
	parseJsonField(row, key, &arr)
	return arr
}

func toPersistenceJSON(v any) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []string:
		if len(val) == 0 {
			return nil
		}
	case []int64:
		if len(val) == 0 {
			return nil
		}
	case []any:
		if len(val) == 0 {
			return nil
		}
	case map[string]float64:
		if len(val) == 0 {
			return nil
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(data)
}
