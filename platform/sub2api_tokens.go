package platform

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

type sub2apiKeyItem struct {
	id         int
	key        string
	name       string
	enabled    bool
	tokenGroup string
}

// GetAPITokens: GET /api/v1/keys?page=1&page_size=100 + /api/v1/api-keys?page=1&page_size=100.
func (s *Sub2ApiAdapter) GetAPITokens(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	items, err := s.listAPIKeys(ctx, normalizeBaseURL(baseURL), accessToken, proxy)
	if err != nil {
		return nil, err
	}

	result := make([]ApiTokenInfo, 0, len(items))
	for _, item := range items {
		info := ApiTokenInfo{
			Name:    item.name,
			Key:     item.key,
			Enabled: item.enabled,
		}
		if item.tokenGroup != "" {
			info.TokenGroup = item.tokenGroup
		}
		result = append(result, info)
	}
	return result, nil
}

// GetAPIToken: returns first enabled token.
func (s *Sub2ApiAdapter) GetAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := s.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		// Propagate: GetAPITokens now distinguishes "the upstream answered with no
		// tokens" (nil, nil below) from "we could not ask". Returning nil, nil here
		// re-hid the failure one level up, which is why the empty answer survived.
		return nil, err
	}
	return pickKeyForGroup(tokens, ""), nil
}

// pickKeyForGroup selects the first enabled key whose TokenGroup matches the
// user's group. Falls back to first-enabled when the group is empty, no keys
// match, or all group-matched keys are disabled. Returns nil when there are
// no keys at all.
func pickKeyForGroup(tokens []ApiTokenInfo, group string) *string {
	if len(tokens) == 0 {
		return nil
	}
	if strings.TrimSpace(group) != "" {
		for _, t := range tokens {
			if !t.Enabled {
				continue
			}
			if strings.TrimSpace(t.TokenGroup) == strings.TrimSpace(group) {
				k := t.Key
				return &k
			}
		}
	}
	for _, t := range tokens {
		if t.Enabled {
			k := t.Key
			return &k
		}
	}
	k := tokens[0].Key
	return &k
}

// CreateAPIToken: POST /api/v1/keys + POST /api/v1/api-keys.
func (s *Sub2ApiAdapter) CreateAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, options *CreateAPITokenOptions, proxy *ProxyConfig) (bool, error) {
	normalized := normalizeBaseURL(baseURL)

	payload := map[string]interface{}{}
	if options != nil && strings.TrimSpace(options.Name) != "" {
		payload["name"] = strings.TrimSpace(options.Name)
	} else {
		payload["name"] = "metapi"
	}

	if options != nil {
		if groupID, err := parseGroupID(options.Group); err == nil && groupID > 0 {
			payload["group_id"] = groupID
		}
		if expiresInDays := s.resolveExpiresInDays(options.ExpiredTime); expiresInDays > 0 {
			payload["expires_in_days"] = expiresInDays
		}
		if !options.UnlimitedQuota && options.RemainQuota > 0 {
			payload["quota"] = math.Max(0, options.RemainQuota)
		}
	}

	headers := authBearerHeaders(accessToken)
	endpoints := []string{"/api/v1/keys", "/api/v1/api-keys"}
	// Two endpoints because sub2api versions expose different paths, so one of them
	// failing is expected — the same ladder listAPIKeys uses. When neither produced
	// a key the caller has to hear why: `false, nil` reads as "the upstream refused",
	// which is a different fact from "no request reached the upstream".
	lastReason := ""
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, normalized+endpoint, "POST", payload, headers, proxy)
		if err != nil {
			lastReason = err.Error()
			continue
		}
		if err := s.parseSub2ApiEnvelope(resp, endpoint); err != nil {
			lastReason = err.Error()
			continue
		}
		return true, nil
	}

	return false, fmt.Errorf("create upstream key: %s", lastReason)
}

// DeleteAPIToken: list -> find key -> DELETE /api/v1/keys/{id} + /api/v1/api-keys/{id}.
func (s *Sub2ApiAdapter) DeleteAPIToken(ctx context.Context, baseURL, accessToken, tokenKey string, platformUserId *int, proxy *ProxyConfig) error {
	targetKey := normalizeTokenKeyForCompare(tokenKey)
	if targetKey == "" {
		return nil
	}

	normalized := normalizeBaseURL(baseURL)
	items, err := s.listAPIKeys(ctx, normalized, accessToken, proxy)
	if err != nil {
		// Cannot know whether the key exists upstream, so cannot claim it is gone.
		// listAPIKeys already names the operation; wrapping it again stutters.
		return err
	}

	var tokenID *int
	for _, item := range items {
		if normalizeTokenKeyForCompare(item.key) == targetKey {
			id := item.id
			tokenID = &id
			break
		}
	}

	if tokenID == nil {
		return nil // already absent upstream: nothing to delete
	}

	headers := authBearerHeaders(accessToken)
	endpoints := []string{
		fmt.Sprintf("/api/v1/keys/%d", *tokenID),
		fmt.Sprintf("/api/v1/api-keys/%d", *tokenID),
	}
	// The caller deletes the local row only when this returns nil, so "neither
	// variant worked" has to be an error: otherwise the row disappears while the
	// upstream key stays live.
	lastReason := ""
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, normalized+endpoint, "DELETE", nil, headers, proxy)
		if err != nil {
			lastReason = err.Error()
			continue
		}
		if err := s.parseSub2ApiEnvelope(resp, endpoint); err != nil {
			lastReason = err.Error()
			continue
		}
		return nil
	}

	return fmt.Errorf("delete upstream key %d: %s", *tokenID, lastReason)
}

func (s *Sub2ApiAdapter) resolveExpiresInDays(expiredTime int64) int {
	if expiredTime <= 0 {
		return 0
	}

	var expiresAtMs int64
	if expiredTime > 10_000_000_000 {
		expiresAtMs = expiredTime
	} else {
		expiresAtMs = expiredTime * 1000
	}

	deltaMs := expiresAtMs - time.Now().UnixMilli()
	days := int(math.Max(1, math.Ceil(float64(deltaMs)/float64(24*60*60*1000))))
	if days > 3650 {
		days = 3650
	}
	return days
}

func (s *Sub2ApiAdapter) listAPIKeys(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) ([]sub2apiKeyItem, error) {
	endpoints := []string{
		fmt.Sprintf("/api/v1/keys?page=1&page_size=%d", UpstreamTokenListPageLimit),
		fmt.Sprintf("/api/v1/api-keys?page=1&page_size=%d", UpstreamTokenListPageLimit),
	}

	headers := authBearerHeaders(accessToken)
	// terminalError mirrors GetUserGroups: the two endpoints exist because sub2api
	// versions expose different paths, so one failing is expected. Both failing is
	// a fetch failure, and returning an empty list for it made "cannot reach the
	// upstream" indistinguishable from "this account has no keys".
	terminalError := ""
	answered := false
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
		if err != nil {
			terminalError = err.Error()
			continue
		}
		rawData, err := s.parseSub2ApiEnvelopeRaw(resp, endpoint)
		if err != nil {
			terminalError = err.Error()
			continue
		}
		answered = true
		items := s.parseTokenItems(rawData)
		if len(items) > 0 {
			return items, nil
		}
	}
	if !answered && terminalError != "" {
		return nil, fmt.Errorf("list upstream keys: %s", terminalError)
	}

	return nil, nil
}

func (s *Sub2ApiAdapter) parseTokenItems(raw interface{}) []sub2apiKeyItem {
	var rawItems []interface{}
	switch v := raw.(type) {
	case []interface{}:
		rawItems = v
	case map[string]interface{}:
		if items, ok := v["items"].([]interface{}); ok {
			rawItems = items
		} else if items, ok := v["list"].([]interface{}); ok {
			rawItems = items
		} else if items, ok := v["data"].([]interface{}); ok {
			rawItems = items
		}
	}

	items := make([]sub2apiKeyItem, 0, len(rawItems))
	for _, rawItem := range rawItems {
		m, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}

		key, _ := getString(m, "key")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		idFloat, ok := m["id"].(float64)
		if !ok || idFloat <= 0 {
			continue
		}

		name, _ := getString(m, "name")
		name = strings.TrimSpace(name)
		if name == "" {
			name = fmt.Sprintf("token-%d", int(idFloat))
		}

		enabled := true
		if status, ok := m["status"]; ok {
			switch v := status.(type) {
			case bool:
				enabled = v
			case float64:
				enabled = v == 1
			case string:
				lower := strings.ToLower(strings.TrimSpace(v))
				enabled = lower != "inactive" && lower != "disabled" && lower != "false" && lower != "0" && lower != "off"
			}
		}

		tokenGroup := ""
		if gid, ok := m["group_id"].(float64); ok && gid > 0 {
			tokenGroup = fmt.Sprintf("%d", int(gid))
		} else if g, ok := getString(m, "group_name"); ok {
			tokenGroup = g
		} else if g, ok := getString(m, "group"); ok {
			tokenGroup = g
		}

		items = append(items, sub2apiKeyItem{
			id:         int(idFloat),
			key:        key,
			name:       name,
			enabled:    enabled,
			tokenGroup: tokenGroup,
		})
	}
	return items
}

func parseGroupID(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	var id int
	_, err := fmt.Sscanf(raw, "%d", &id)
	return id, err
}
