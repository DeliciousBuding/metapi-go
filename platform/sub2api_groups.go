package platform

import (
	"context"
	"fmt"
	"strings"
)

func (s *Sub2ApiAdapter) listGroups(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) []string {
	endpoints := []string{
		"/api/v1/groups/available",
		"/api/v1/groups?page=1&page_size=100",
		"/api/v1/groups",
		"/api/v1/group?page=1&page_size=100",
		"/api/v1/group",
	}

	headers := authBearerHeaders(accessToken)
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
		if err != nil {
			continue
		}

		parsed := s.tryParseEnvelope(resp)
		groups := s.parseGroupItems(parsed)
		if len(groups) > 0 {
			return groups
		}
	}
	return nil
}

func (s *Sub2ApiAdapter) tryParseEnvelope(resp map[string]interface{}) map[string]interface{} {
	if code, ok := resp["code"].(float64); ok && code == 0 {
		if data, ok := getMap(resp, "data"); ok {
			return data
		}
	}
	return resp
}

func (s *Sub2ApiAdapter) parseGroupItems(payload map[string]interface{}) []string {
	var rawItems []interface{}
	switch v := payload["data"].(type) {
	case []interface{}:
		rawItems = v
	}
	if rawItems == nil {
		if rawItems, _ = payload["data"].([]interface{}); rawItems == nil {
			rawItems, _ = payload["items"].([]interface{})
		}
	}
	if rawItems == nil {
		rawItems, _ = payload["list"].([]interface{})
	}
	if rawItems == nil {
		rawItems, _ = payload["groups"].([]interface{})
	}
	if rawItems == nil {
		// Try payload itself as array
		return nil
	}

	seen := make(map[string]bool)
	var groups []string
	for _, item := range rawItems {
		switch v := item.(type) {
		case float64:
			if v > 0 {
				s := fmt.Sprintf("%d", int(v))
				if !seen[s] {
					seen[s] = true
					groups = append(groups, s)
				}
			}
		case string:
			t := strings.TrimSpace(v)
			if t != "" && !seen[t] {
				seen[t] = true
				groups = append(groups, t)
			}
		case map[string]interface{}:
			// Try group_id, id, name
			if gid, ok := v["group_id"].(float64); ok && gid > 0 {
				s := fmt.Sprintf("%d", int(gid))
				if !seen[s] {
					seen[s] = true
					groups = append(groups, s)
				}
				continue
			}
			if id, ok := v["id"].(float64); ok && id > 0 {
				s := fmt.Sprintf("%d", int(id))
				if !seen[s] {
					seen[s] = true
					groups = append(groups, s)
				}
				continue
			}
			if name, ok := getString(v, "name"); ok && name != "" && !seen[name] {
				seen[name] = true
				groups = append(groups, name)
				continue
			}
			if name, ok := getString(v, "group_name"); ok && name != "" && !seen[name] {
				seen[name] = true
				groups = append(groups, name)
			}
		}
	}
	return groups
}

func (s *Sub2ApiAdapter) inferGroupsFromKeys(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) []string {
	endpoints := []string{
		"/api/v1/keys?page=1&page_size=100",
		"/api/v1/api-keys?page=1&page_size=100",
	}

	headers := authBearerHeaders(accessToken)
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
		if err != nil {
			continue
		}

		parsed := s.tryParseEnvelope(resp)
		groupIDs := s.parseGroupIDsFromTokenPayload(parsed)
		if len(groupIDs) > 0 {
			return groupIDs
		}
	}
	return nil
}

func (s *Sub2ApiAdapter) parseGroupIDsFromTokenPayload(payload map[string]interface{}) []string {
	var rawItems []interface{}
	if data, ok := payload["data"].([]interface{}); ok {
		rawItems = data
	} else if items, ok := payload["items"].([]interface{}); ok {
		rawItems = items
	} else if list, ok := payload["list"].([]interface{}); ok {
		rawItems = list
	}

	seen := make(map[string]bool)
	var groups []string
	for _, item := range rawItems {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if gid, ok := m["group_id"].(float64); ok && gid > 0 {
			s := fmt.Sprintf("%d", int(gid))
			if !seen[s] {
				seen[s] = true
				groups = append(groups, s)
			}
		}
	}
	return groups
}
