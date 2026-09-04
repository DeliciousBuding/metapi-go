package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// --- Token CRUD ---

func (n *NewApiAdapter) GetAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := n.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		return nil, err
	}
	return findFirstEnabledToken(tokens), nil
}

func (n *NewApiAdapter) GetAPITokens(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	return n.getAPITokensWithUser(ctx, baseURL, accessToken, platformUserId, proxy)
}

func (n *NewApiAdapter) getAPITokenWithUser(ctx context.Context, baseURL, accessToken string, userID *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := n.getAPITokensWithUser(ctx, baseURL, accessToken, userID, proxy)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	return findFirstEnabledToken(tokens), nil
}

func (n *NewApiAdapter) getAPITokensWithUser(ctx context.Context, baseURL, accessToken string, userID *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	// Try Bearer auth
	headers := n.authHeaders(accessToken, userID)
	resp, err := fetchJSON(ctx, baseURL+"/api/token/?p=0&size=100", "GET", nil, headers, proxy)
	if err == nil {
		items := parseTokenItemsFromMap(resp)
		normalized, normalizeErr := n.normalizeListedTokens(ctx, baseURL, items, headers, proxy)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if len(normalized) > 0 {
			return normalized, nil
		}
	}

	// Cookie fallback
	cookieTokens, cookieErr := n.getAPITokensByCookie(ctx, baseURL, accessToken, userID, proxy)
	if cookieErr != nil {
		return nil, cookieErr
	}
	if len(cookieTokens) > 0 {
		return cookieTokens, nil
	}

	// Alternate userID cookie fallback
	altID := n.probeAlternateUserIDByCookie(ctx, baseURL, accessToken, userID, proxy)
	if altID != nil {
		fallbackTokens, fallbackErr := n.getAPITokensByCookie(ctx, baseURL, accessToken, altID, proxy)
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		if len(fallbackTokens) > 0 {
			return fallbackTokens, nil
		}
	}

	return []ApiTokenInfo{}, nil
}

func (n *NewApiAdapter) getAPITokensByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	for _, cookie := range buildCookieCandidates(token) {
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(userID) {
			headers[k] = v
		}

		resp, err := fetchJSON(ctx, baseURL+"/api/token/?p=0&size=100", "GET", nil, headers, proxy)
		if err != nil {
			continue
		}

		items := parseTokenItemsFromMap(resp)
		normalized, normalizeErr := n.normalizeListedTokens(ctx, baseURL, items, headers, proxy)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if len(normalized) > 0 {
			return normalized, nil
		}
	}
	return nil, nil
}

// normalizeListedTokens resolves New API v1's masked list keys through its
// ownership-checked batch key endpoint before returning them to account-token
// sync. A masked display value is never a usable routing credential.
func (n *NewApiAdapter) normalizeListedTokens(
	ctx context.Context,
	baseURL string,
	items []map[string]interface{},
	headers map[string]string,
	proxy *ProxyConfig,
) ([]ApiTokenInfo, error) {
	maskedIDs := make([]int, 0)
	for _, item := range items {
		key, _ := getString(item, "key")
		if !strings.Contains(key, "*") {
			continue
		}
		id, ok := getFloat(item, "id")
		if !ok || id <= 0 {
			return nil, fmt.Errorf("New API returned a masked token key without a usable token id")
		}
		maskedIDs = append(maskedIDs, int(id))
	}
	if len(maskedIDs) == 0 {
		return normalizeTokenItems(items), nil
	}

	resp, err := fetchJSON(
		ctx,
		strings.TrimRight(baseURL, "/")+"/api/token/batch/keys",
		"POST",
		map[string]interface{}{"ids": maskedIDs},
		headers,
		proxy,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch full New API token keys: %w", err)
	}
	data, ok := getMap(resp, "data")
	if !ok {
		return nil, fmt.Errorf("fetch full New API token keys: response has no data")
	}
	keys, ok := getMap(data, "keys")
	if !ok {
		return nil, fmt.Errorf("fetch full New API token keys: response has no keys")
	}
	for _, item := range items {
		key, _ := getString(item, "key")
		if !strings.Contains(key, "*") {
			continue
		}
		id, _ := getFloat(item, "id")
		fullKey, _ := getString(keys, fmt.Sprintf("%d", int(id)))
		fullKey = strings.TrimSpace(fullKey)
		if fullKey == "" || strings.Contains(fullKey, "*") {
			return nil, fmt.Errorf("fetch full New API token keys: token %d is missing", int(id))
		}
		item["key"] = fullKey
	}
	return normalizeTokenItems(items), nil
}

func (n *NewApiAdapter) CreateAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, options *CreateAPITokenOptions, proxy *ProxyConfig) (bool, error) {
	payload := buildDefaultTokenPayload(options)
	bodyBytes, _ := json.Marshal(payload)

	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	// `answered` separates "the upstream refused" from "no attempt reached the
	// upstream". Both end as a 502 in the caller, but only the second one is a
	// failure to ask, and reporting it as a refusal left the operator with no
	// reason and no WARN log.
	answered := false
	reason := ""

	// Try Bearer auth
	resp, err := fetchJSON(ctx, baseURL+"/api/token/", "POST", json.RawMessage(bodyBytes), n.authHeaders(accessToken, resolvedUserID), proxy)
	if err != nil {
		reason = err.Error()
	} else {
		if success, _ := getBool(resp, "success"); success {
			return true, nil
		}
		answered = true
		reason = newApiRefusalReason(resp)
	}

	// Cookie fallback
	cookieUserID := resolvedUserID
	if cookieUserID == nil {
		cookieUserID = n.probeUserIDByCookie(ctx, baseURL, accessToken, proxy)
	}
	for _, cookie := range buildCookieCandidates(accessToken) {
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(cookieUserID) {
			headers[k] = v
		}

		resp, err := fetchJSON(ctx, baseURL+"/api/token/", "POST", json.RawMessage(bodyBytes), headers, proxy)
		if err != nil {
			reason = err.Error()
			continue
		}
		if success, _ := getBool(resp, "success"); success {
			return true, nil
		}
		answered = true
		reason = newApiRefusalReason(resp)
	}

	if answered {
		return false, nil
	}
	return false, fmt.Errorf("create upstream token: %s", reason)
}

// newApiRefusalReason is the upstream's own verdict on a write it answered.
func newApiRefusalReason(resp map[string]interface{}) string {
	msg, _ := getString(resp, "message")
	if strings.TrimSpace(msg) == "" {
		return "upstream reported failure"
	}
	return msg
}

func (n *NewApiAdapter) DeleteAPIToken(ctx context.Context, baseURL, accessToken, tokenKey string, platformUserId *int, proxy *ProxyConfig) error {
	targetKey := normalizeTokenKeyForCompare(tokenKey)
	if targetKey == "" {
		return nil
	}

	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	var tokenID *int
	// listed separates "we read the listing and this key is not in it" (nothing to
	// delete) from "we never read a listing" (cannot know). Both used to arrive at
	// the same `tokenID == nil`, and the caller deletes the local row on nil.
	listed := false
	reason := ""

	// Try Bearer auth list
	resp, err := fetchJSON(ctx, baseURL+"/api/token/?p=0&size=100", "GET", nil, n.authHeaders(accessToken, resolvedUserID), proxy)
	if err != nil {
		reason = err.Error()
	} else {
		listed = true
		items := parseTokenItemsFromMap(resp)
		tokenID = pickTokenID(items, targetKey)
	}

	if tokenID != nil {
		// Try Bearer DELETE
		delResp, err := fetchJSON(ctx, fmt.Sprintf("%s/api/token/%d", baseURL, *tokenID), "DELETE", nil, n.authHeaders(accessToken, resolvedUserID), proxy)
		if err != nil {
			reason = err.Error()
		} else if success, _ := getBool(delResp, "success"); success {
			return nil
		} else {
			reason = newApiRefusalReason(delResp)
		}
	}

	// Cookie fallback
	cookieUserID := resolvedUserID
	if cookieUserID == nil {
		cookieUserID = n.probeUserIDByCookie(ctx, baseURL, accessToken, proxy)
	}

	for _, cookie := range buildCookieCandidates(accessToken) {
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(cookieUserID) {
			headers[k] = v
		}

		// List if not already found
		if tokenID == nil {
			resp, err := fetchJSON(ctx, baseURL+"/api/token/?p=0&size=100", "GET", nil, headers, proxy)
			if err != nil {
				reason = err.Error()
			} else {
				listed = true
				items := parseTokenItemsFromMap(resp)
				tokenID = pickTokenID(items, targetKey)
			}
		}

		if tokenID == nil {
			continue
		}

		delResp, err := fetchJSON(ctx, fmt.Sprintf("%s/api/token/%d", baseURL, *tokenID), "DELETE", nil, headers, proxy)
		if err != nil {
			reason = err.Error()
			continue
		}
		if success, _ := getBool(delResp, "success"); success {
			return nil
		}
		reason = newApiRefusalReason(delResp)
	}

	// Already absent upstream, and we know it because a listing answered.
	if tokenID == nil && listed {
		return nil
	}
	if tokenID == nil {
		return fmt.Errorf("list upstream tokens: %s", reason)
	}
	return fmt.Errorf("delete upstream token %d: %s", *tokenID, reason)
}
