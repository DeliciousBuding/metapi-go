package oauth

import (
	"encoding/json"
	"strings"
)

// MergeAccountExtraConfig merges a patch into an existing extraConfig JSON string.
func MergeAccountExtraConfig(existing *string, patch map[string]interface{}) *string {
	base := make(map[string]interface{})
	if existing != nil && strings.TrimSpace(*existing) != "" {
		if err := json.Unmarshal([]byte(*existing), &base); err != nil {
			base = make(map[string]interface{})
		}
	}
	for k, v := range patch {
		if v == nil {
			delete(base, k)
		} else {
			base[k] = v
		}
	}
	if len(base) == 0 {
		return nil
	}
	b, err := json.Marshal(base)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

// GetProxyURLFromExtraConfig reads proxyUrl from extraConfig.
func GetProxyURLFromExtraConfig(extraConfig *string) string {
	parsed := parseRawExtraConfigGo(extraConfig)
	if parsed == nil {
		return ""
	}
	if v, ok := parsed["proxyUrl"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// GetUseSystemProxyFromExtraConfig reads useSystemProxy from extraConfig.
func GetUseSystemProxyFromExtraConfig(extraConfig *string) bool {
	parsed := parseRawExtraConfigGo(extraConfig)
	if parsed == nil {
		return false
	}
	if v, ok := parsed["useSystemProxy"].(bool); ok {
		return v
	}
	return false
}

func parseRawExtraConfigGo(extraConfig *string) map[string]interface{} {
	if extraConfig == nil || strings.TrimSpace(*extraConfig) == "" {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(*extraConfig), &result); err != nil {
		return nil
	}
	return result
}
