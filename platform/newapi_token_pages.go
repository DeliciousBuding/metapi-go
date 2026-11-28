package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	// Bound the complete-list fetch independently of the remote total. The
	// limits cover the normal 1000-token account without letting a hostile or
	// broken upstream keep returning new IDs forever.
	newAPITokenListMaxItems        = 1000
	newAPITokenListMaxPages        = 10
	newAPITokenListMaxDecodedBytes = 8 << 20
)

// apiTokenListPath is the legacy one-api token listing path. Keep its p=0/size
// spelling: one-api's older query contract is not the New API p/page_size
// contract below.
func apiTokenListPath() string {
	return fmt.Sprintf("/api/token/?p=0&size=%d", UpstreamTokenListPageLimit)
}

// newAPITokenListPath uses New API's current one-based page and page_size
// contract. page_size is capped at 100 upstream, so this is also the ownership
// batch size used by /api/token/batch/keys.
func newAPITokenListPath(page int) string {
	if page < 1 {
		page = 1
	}
	return fmt.Sprintf("/api/token/?p=%d&page_size=%d", page, UpstreamTokenListPageLimit)
}

func (n *NewApiAdapter) fetchNewAPITokenItems(ctx context.Context, baseURL string, headers map[string]string, proxy *ProxyConfig) ([]map[string]interface{}, bool, bool, error) {
	all := make([]map[string]interface{}, 0)
	seenIDs := make(map[int]struct{})
	total := -1
	started := false
	decodedBytes := 0

	for page := 1; ; page++ {
		if page > newAPITokenListMaxPages {
			return nil, started, false, fmt.Errorf("list upstream tokens: page limit %d exceeded", newAPITokenListMaxPages)
		}
		resp, err := fetchJSON(ctx, strings.TrimRight(baseURL, "/")+newAPITokenListPath(page), http.MethodGet, nil, headers, proxy)
		if err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: %w", page, err)
		}
		started = true

		if success, present := getBool(resp, "success"); present && !success {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: upstream refused the listing", page)
		}

		items, flat, present, err := newAPITokenPageItems(resp)
		if err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: %w", page, err)
		}
		if !present {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: response has no supported token collection", page)
		}

		pageTotal, hasTotal, err := newAPITokenPageTotal(resp)
		if err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: %w", page, err)
		}
		// A flat response to the current p/page_size query has no page marker
		// that proves it is the first page. Retry the known legacy query before
		// using any of it; never treat this current-query payload as complete.
		if page == 1 && !hasTotal && flat {
			return n.fetchLegacyNewAPITokenItems(ctx, baseURL, headers, proxy)
		}
		if page == 1 {
			if hasTotal {
				total = pageTotal
				if total > newAPITokenListMaxItems {
					return nil, started, false, fmt.Errorf("list upstream tokens: total %d exceeds limit %d", total, newAPITokenListMaxItems)
				}
			} else if !flat {
				return nil, started, false, fmt.Errorf("list upstream tokens page 1: response has no total")
			}
		} else if !hasTotal || pageTotal != total {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: total changed or missing", page)
		}

		if len(items) > newAPITokenListMaxItems-len(all) {
			return nil, started, false, fmt.Errorf("list upstream tokens: item count exceeds limit %d", newAPITokenListMaxItems)
		}
		pageBytes, err := json.Marshal(items)
		if err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: measure decoded items: %w", page, err)
		}
		if len(pageBytes) > newAPITokenListMaxDecodedBytes-decodedBytes {
			return nil, started, false, fmt.Errorf("list upstream tokens: decoded items exceed limit %d bytes", newAPITokenListMaxDecodedBytes)
		}
		decodedBytes += len(pageBytes)
		if err := appendNewAPITokenItems(&all, seenIDs, items); err != nil {
			return nil, started, false, fmt.Errorf("list upstream tokens page %d: %w", page, err)
		}

		if total >= 0 {
			if len(all) > total {
				return nil, started, false, fmt.Errorf("list upstream tokens: returned %d tokens beyond total %d", len(all), total)
			}
			if len(all) == total {
				return all, started, true, nil
			}
			if len(items) < UpstreamTokenListPageLimit {
				return nil, started, false, fmt.Errorf("list upstream tokens page %d: short page before total %d", page, total)
			}
			continue
		}

		return nil, started, false, fmt.Errorf("legacy token listing has no total")
	}
}

// fetchLegacyNewAPITokenItems retries the recognized One API/New API legacy
// first-page contract after a flat response to the current query. Without a
// total, the returned tokens are still usable for synchronization but are not
// proof that the listing is complete.
func (n *NewApiAdapter) fetchLegacyNewAPITokenItems(ctx context.Context, baseURL string, headers map[string]string, proxy *ProxyConfig) ([]map[string]interface{}, bool, bool, error) {
	resp, err := fetchJSON(ctx, strings.TrimRight(baseURL, "/")+apiTokenListPath(), http.MethodGet, nil, headers, proxy)
	if err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: %w", err)
	}
	if success, present := getBool(resp, "success"); present && !success {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: upstream refused the listing")
	}
	items, _, present, err := newAPITokenPageItems(resp)
	if err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: %w", err)
	}
	if !present {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: response has no supported token collection")
	}
	if len(items) > newAPITokenListMaxItems {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: item count exceeds limit %d", newAPITokenListMaxItems)
	}
	pageBytes, err := json.Marshal(items)
	if err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: measure decoded items: %w", err)
	}
	if len(pageBytes) > newAPITokenListMaxDecodedBytes {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: decoded items exceed limit %d bytes", newAPITokenListMaxDecodedBytes)
	}

	all := make([]map[string]interface{}, 0, len(items))
	if err := appendNewAPITokenItems(&all, make(map[int]struct{}), items); err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: %w", err)
	}

	total, hasTotal, err := newAPITokenPageTotal(resp)
	if err != nil {
		return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: %w", err)
	}
	if hasTotal {
		if total > newAPITokenListMaxItems {
			return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: total %d exceeds limit %d", total, newAPITokenListMaxItems)
		}
		if len(items) > total {
			return nil, true, false, fmt.Errorf("list upstream tokens with legacy contract: returned %d tokens beyond total %d", len(items), total)
		}
		if len(items) == total {
			return all, true, true, nil
		}
	}
	return all, true, false, ErrNewAPITokenListingCompletenessUnproven
}

func newAPITokenPageItems(resp map[string]interface{}) ([]map[string]interface{}, bool, bool, error) {
	if data, ok := resp["data"].([]interface{}); ok {
		items, err := tokenMapsFromList(data)
		return items, true, true, err
	}

	var candidates []interface{}
	if data, ok := getMap(resp, "data"); ok {
		candidates = append(candidates, data["items"], data["data"], data["list"])
	}
	candidates = append(candidates, resp["items"], resp["list"])
	for _, raw := range candidates {
		list, ok := raw.([]interface{})
		if !ok {
			continue
		}
		items, err := tokenMapsFromList(list)
		return items, false, true, err
	}
	return nil, false, false, nil
}

func tokenMapsFromList(list []interface{}) ([]map[string]interface{}, error) {
	items := make([]map[string]interface{}, 0, len(list))
	for i, raw := range list {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("token item %d is not an object", i+1)
		}
		items = append(items, item)
	}
	return items, nil
}

func newAPITokenPageTotal(resp map[string]interface{}) (int, bool, error) {
	total, ok := getFloat(resp, "total")
	if !ok {
		data, hasData := getMap(resp, "data")
		if hasData {
			total, ok = getFloat(data, "total")
		}
	}
	if !ok {
		return 0, false, nil
	}
	if total < 0 || total != float64(int(total)) {
		return 0, false, fmt.Errorf("invalid total %v", total)
	}
	return int(total), true, nil
}

func appendNewAPITokenItems(all *[]map[string]interface{}, seenIDs map[int]struct{}, items []map[string]interface{}) error {
	for i, item := range items {
		key, ok := getString(item, "key")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return fmt.Errorf("token item %d has no key", i+1)
		}

		if rawID, exists := item["id"]; exists {
			id, ok := rawID.(float64)
			if !ok || id <= 0 || id != float64(int(id)) {
				return fmt.Errorf("token item %d has an invalid id", i+1)
			}
			if _, exists := seenIDs[int(id)]; exists {
				return fmt.Errorf("duplicate token id %d", int(id))
			}
			seenIDs[int(id)] = struct{}{}
		}
		*all = append(*all, item)
	}
	return nil
}
