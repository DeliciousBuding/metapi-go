package platform

import (
	"context"
	"fmt"
	"strings"
)

// validateNewAPITokenKeys runs after masked display values have been hydrated.
// IDs are the identity while listing; real keys are the identity once the
// ownership-checked hydration has replaced every mask.
func validateNewAPITokenKeys(items []map[string]interface{}) error {
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		key, ok := getString(item, "key")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("token item %d has no key", i+1)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate token entry")
		}
		seen[key] = struct{}{}
	}
	return nil
}

// normalizeListedTokens resolves New API v1's masked display keys before
// returning routing credentials. The same hydration owns deletion identity.
func (n *NewApiAdapter) normalizeListedTokens(
	ctx context.Context,
	baseURL string,
	items []map[string]interface{},
	headers map[string]string,
	proxy *ProxyConfig,
) ([]ApiTokenInfo, error) {
	if err := n.hydrateListedTokenKeys(ctx, baseURL, items, headers, proxy); err != nil {
		return nil, err
	}
	if err := validateNewAPITokenKeys(items); err != nil {
		return nil, err
	}
	return normalizeTokenItems(items), nil
}

// hydrateListedTokenKeys replaces masked keys in the owned list using the
// ownership-checked batch endpoint. An unresolved key is not proof of absence.
func (n *NewApiAdapter) hydrateListedTokenKeys(
	ctx context.Context,
	baseURL string,
	items []map[string]interface{},
	headers map[string]string,
	proxy *ProxyConfig,
) error {
	maskedIDs := make([]int, 0)
	seen := make(map[int]struct{})
	for _, item := range items {
		key, _ := getString(item, "key")
		if !strings.Contains(key, "*") {
			continue
		}
		id, ok := getFloat(item, "id")
		if !ok || id <= 0 || id != float64(int(id)) {
			return fmt.Errorf("New API returned a masked token key without a usable token id")
		}
		if _, exists := seen[int(id)]; exists {
			return fmt.Errorf("New API returned a duplicate masked token id %d", int(id))
		}
		seen[int(id)] = struct{}{}
		maskedIDs = append(maskedIDs, int(id))
	}
	if len(maskedIDs) == 0 {
		return nil
	}

	for start := 0; start < len(maskedIDs); start += UpstreamTokenListPageLimit {
		end := start + UpstreamTokenListPageLimit
		if end > len(maskedIDs) {
			end = len(maskedIDs)
		}
		batchIDs := maskedIDs[start:end]
		resp, err := fetchJSON(
			ctx,
			strings.TrimRight(baseURL, "/")+"/api/token/batch/keys",
			"POST",
			map[string]interface{}{"ids": batchIDs},
			headers,
			proxy,
		)
		if err != nil {
			return fmt.Errorf("fetch full New API token keys: %w", err)
		}
		if success, present := getBool(resp, "success"); present && !success {
			return fmt.Errorf("fetch full New API token keys: upstream refused the lookup")
		}
		data, ok := getMap(resp, "data")
		if !ok {
			return fmt.Errorf("fetch full New API token keys: response has no data")
		}
		keys, ok := getMap(data, "keys")
		if !ok {
			return fmt.Errorf("fetch full New API token keys: response has no keys")
		}
		for _, id := range batchIDs {
			fullKey, _ := getString(keys, fmt.Sprintf("%d", id))
			fullKey = strings.TrimSpace(fullKey)
			if fullKey == "" || strings.Contains(fullKey, "*") {
				return fmt.Errorf("fetch full New API token keys: token %d is missing", id)
			}
			for _, item := range items {
				itemID, _ := getFloat(item, "id")
				if int(itemID) == id {
					item["key"] = fullKey
					break
				}
			}
		}
	}
	return nil
}
