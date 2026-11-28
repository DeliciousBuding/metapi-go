package service

import (
	"encoding/json"
	"strings"
)

// ParseExtraConfig parses an extraConfig field (JSON string) as a map.
func ParseExtraConfig(raw *string) map[string]any {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(*raw), &result); err != nil {
		return nil
	}
	return result
}

// MarshalExtraConfig marshals a map to a JSON string.
func MarshalExtraConfig(config map[string]any) *string {
	if len(config) == 0 {
		return nil
	}
	b, err := json.Marshal(config)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}
