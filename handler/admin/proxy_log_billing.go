package admin

import (
	"encoding/json"
	"math"
)

// Billing is application-owned JSON. Legacy writers used snake_case inside
// these three sections; expose the same camelCase contract as current writers.
func decodeProxyLogBilling(raw any) any {
	parsed := raw
	if text, ok := raw.(string); ok && text != "" {
		if json.Unmarshal([]byte(text), &parsed) != nil {
			return raw
		}
	}
	if billing, ok := parsed.(map[string]any); ok {
		billing = mapKeysToCamel(billing)
		for _, section := range []string{"usage", "pricing", "breakdown"} {
			if fields, ok := billing[section].(map[string]any); ok {
				billing[section] = mapKeysToCamel(fields)
			}
		}
		return billing
	}
	return parsed
}

// Cache counts are a projection of already-stored billing, not another join or
// a second usage store. Missing or explicitly unknown usage stays unobserved.
func projectProxyLogCacheUsage(row map[string]any, billing any) {
	details, ok := billing.(map[string]any)
	if !ok {
		return
	}
	source, _ := details["usageSource"].(string)
	if source == "upstream" || source == "unknown" || source == "self-log" {
		row["usageSource"] = source
	}
	if source == "unknown" {
		if value, ok := row["estimatedCost"].(float64); ok && value == 0 {
			row["estimatedCost"] = nil
		}
		return
	}
	usage, ok := details["usage"].(map[string]any)
	if !ok {
		return
	}
	for _, field := range []string{"cacheReadTokens", "cacheCreationTokens"} {
		value, ok := usage[field].(float64)
		if ok && value >= 0 && value == math.Trunc(value) && value <= float64(1<<53-1) {
			row[field] = value
		}
	}
}
