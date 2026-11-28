package platform

import (
	"fmt"
	"strings"
)

func normalizeTokenKeyForCompare(key string) string {
	return stripBearerPrefix(key)
}

func parseTokenItemsFromMap(resp map[string]interface{}) []map[string]interface{} {
	// Try multiple response formats
	sources := [][]interface{}{}
	if data, ok := resp["data"].([]interface{}); ok {
		sources = append(sources, data)
	}
	if data, ok := getMap(resp, "data"); ok {
		if items, ok := data["items"].([]interface{}); ok {
			sources = append(sources, items)
		}
		if items, ok := data["data"].([]interface{}); ok {
			sources = append(sources, items)
		}
		if items, ok := data["list"].([]interface{}); ok {
			sources = append(sources, items)
		}
	}
	if items, ok := resp["items"].([]interface{}); ok {
		sources = append(sources, items)
	}
	if items, ok := resp["list"].([]interface{}); ok {
		sources = append(sources, items)
	}

	for _, src := range sources {
		items := make([]map[string]interface{}, 0, len(src))
		for _, v := range src {
			if m, ok := v.(map[string]interface{}); ok {
				items = append(items, m)
			}
		}
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

func normalizeTokenItems(items []map[string]interface{}) []ApiTokenInfo {
	result := make([]ApiTokenInfo, 0, len(items))
	for i, item := range items {
		key, _ := getString(item, "key")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		name, _ := getString(item, "name")
		name = strings.TrimSpace(name)
		if name == "" {
			if i == 0 {
				name = "default"
			} else {
				name = fmt.Sprintf("token-%d", i+1)
			}
		}

		// Extract group from multiple possible fields
		group := ""
		if g, ok := getString(item, "group"); ok {
			group = strings.TrimSpace(g)
		}
		if group == "" {
			if g, ok := getString(item, "group_name"); ok {
				group = strings.TrimSpace(g)
			}
		}
		if group == "" {
			if g, ok := getString(item, "token_group"); ok {
				group = strings.TrimSpace(g)
			}
		}

		enabled := true
		if status, ok := getFloat(item, "status"); ok {
			enabled = status == 1
		}

		info := ApiTokenInfo{
			Name:    name,
			Key:     key,
			Enabled: enabled,
		}
		if group != "" {
			info.TokenGroup = group
		}
		result = append(result, info)
	}
	return result
}

// Token list: find the first enabled token key, or the first token key.
func findFirstEnabledToken(tokens []ApiTokenInfo) *string {
	for _, t := range tokens {
		if t.Enabled {
			s := t.Key
			return &s
		}
	}
	if len(tokens) > 0 {
		s := tokens[0].Key
		return &s
	}
	return nil
}

// Pick token ID from token list by matching key.
func pickTokenID(items []map[string]interface{}, targetKey string) *int {
	for _, item := range items {
		key, _ := getString(item, "key")
		if normalizeTokenKeyForCompare(key) == targetKey {
			if id := getIntPtr(item, "id"); id != nil {
				return id
			}
		}
	}
	return nil
}
