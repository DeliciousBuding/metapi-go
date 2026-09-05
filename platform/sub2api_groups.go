package platform

import (
	"context"
	"fmt"
	"strings"
)

// listGroups asks the group endpoints. The five endpoints are version-skew
// alternatives of the same question, so — exactly as listAPIKeys documents for
// its two — the reason kept is the LAST one: any of them is representative, and
// a 404 on an older path must not become the verdict when a newer path replies.
//
// The returned error is nil whenever at least one endpoint actually answered,
// including when the answer was "there are no groups". An answered empty is a
// true statement about the upstream; turning it into a failure would be the same
// defect wearing the other face.
func (s *Sub2ApiAdapter) listGroups(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) ([]string, error) {
	endpoints := []string{
		"/api/v1/groups/available",
		"/api/v1/groups?page=1&page_size=100",
		"/api/v1/groups",
		"/api/v1/group?page=1&page_size=100",
		"/api/v1/group",
	}

	headers := authBearerHeaders(accessToken)
	answered := false
	var lastReason string
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
		if err != nil {
			lastReason = err.Error()
			continue
		}

		if reason, refused := s.groupEnvelopeRefusal(resp); refused {
			lastReason = reason
			continue
		}

		answered = true
		parsed := s.tryParseEnvelope(resp)
		groups := s.parseGroupItems(parsed)
		if len(groups) > 0 {
			return groups, nil
		}
	}
	if !answered && lastReason != "" {
		return nil, fmt.Errorf("fetch groups: %s", lastReason)
	}
	return nil, nil
}

// groupEnvelopeRefusal separates "the upstream answered" from "the upstream
// refused". sub2api's success code is numeric 0; anything else carrying a `code`
// — a non-zero number, or a string code such as "UNAUTHORIZED" — is a refusal
// whose `message` the family already knows how to render. A payload with no
// `code` key at all is one of the bare array/list shapes parseGroupItems accepts
// and counts as an answer.
func (s *Sub2ApiAdapter) groupEnvelopeRefusal(resp map[string]interface{}) (string, bool) {
	code, ok := resp["code"]
	if !ok {
		return "", false
	}
	if f, isFloat := code.(float64); isFloat && f == 0 {
		return "", false
	}
	return resolveGroupFetchErrorMessage(resp), true
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

// inferGroupsFromKeys derives the group list from the keys that already exist,
// for upstreams that expose no group endpoint. Same answered/refused split as
// listGroups, and the same last-reason choice for the same reason: the two
// endpoints are version-skew alternatives.
func (s *Sub2ApiAdapter) inferGroupsFromKeys(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) ([]string, error) {
	endpoints := []string{
		"/api/v1/keys?page=1&page_size=100",
		"/api/v1/api-keys?page=1&page_size=100",
	}

	headers := authBearerHeaders(accessToken)
	answered := false
	var lastReason string
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
		if err != nil {
			lastReason = err.Error()
			continue
		}

		if reason, refused := s.groupEnvelopeRefusal(resp); refused {
			lastReason = reason
			continue
		}

		answered = true
		parsed := s.tryParseEnvelope(resp)
		groupIDs := s.parseGroupIDsFromTokenPayload(parsed)
		if len(groupIDs) > 0 {
			return groupIDs, nil
		}
	}
	if !answered && lastReason != "" {
		return nil, fmt.Errorf("fetch groups from keys: %s", lastReason)
	}
	return nil, nil
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
