package admin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/schedulespec"
)

func normalizeString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func hasAnyKey(body map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, ok := body[key]; ok {
			return true
		}
	}
	return false
}

func toBool(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		val = strings.ToLower(strings.TrimSpace(val))
		return val == "1" || val == "true" || val == "yes" || val == "on"
	case float64:
		return val != 0
	case int:
		return val != 0
	default:
		return false
	}
}

// toFloat64Strict converts numeric JSON values and rejects non-numeric types
// such as strings, booleans, or objects. This prevents silent zero-coercion
// that hides client-side type errors.
func toFloat64Strict(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case json.Number:
		n, err := val.Float64()
		if err != nil {
			return 0, fmt.Errorf("invalid numeric value: %s", val.String())
		}
		return n, nil
	default:
		return 0, fmt.Errorf("expected a number, got %T", v)
	}
}

// toBoolStrict converts boolean JSON values and rejects non-boolean types
// such as strings, numbers, or objects. This prevents silent false-coercion
// that hides client-side type errors.
func toBoolStrict(v any) (bool, error) {
	if b, ok := v.(bool); ok {
		return b, nil
	}
	return false, fmt.Errorf("expected a boolean (true/false), got %T", v)
}

// parseStringArraySetting validates an explicit JSON array setting.
// null / non-array values are rejected so accidental clients cannot wipe lists.
// Empty arrays are allowed and mean "clear / allow all" for the model whitelist.
func parseStringArraySetting(v any, field string) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("%s must be an array of strings (use [] to clear)", field)
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", field)
	}
	out := make([]string, 0, len(arr))
	seen := make(map[string]struct{}, len(arr))
	for _, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings", field)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

func stringSliceOrEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// decodeScheduleSpec converts a decoded JSON body value into a ScheduleSpec.
func decodeScheduleSpec(v any) (schedulespec.ScheduleSpec, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return schedulespec.ScheduleSpec{}, err
	}
	var spec schedulespec.ScheduleSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return schedulespec.ScheduleSpec{}, err
	}
	return spec, nil
}
