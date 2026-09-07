package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// --- Token CRUD ---

// apiTokenListPath is the new-api/one-api token listing path with the shared
// page limit applied, so the six call sites across the two adapters cannot
// drift from UpstreamTokenListPageLimit (the service-layer convergence guard
// reads the same constant to decide whether a listing could be truncated).
func apiTokenListPath() string {
	return fmt.Sprintf("/api/token/?p=0&size=%d", UpstreamTokenListPageLimit)
}

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
	resp, err := fetchJSON(ctx, baseURL+apiTokenListPath(), "GET", nil, headers, proxy)
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

		resp, err := fetchJSON(ctx, baseURL+apiTokenListPath(), "GET", nil, headers, proxy)
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
	for _, item := range items {
		key, _ := getString(item, "key")
		if !strings.Contains(key, "*") {
			continue
		}
		id, ok := getFloat(item, "id")
		if !ok || id <= 0 {
			return fmt.Errorf("New API returned a masked token key without a usable token id")
		}
		maskedIDs = append(maskedIDs, int(id))
	}
	if len(maskedIDs) == 0 {
		return nil
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
	for _, item := range items {
		key, _ := getString(item, "key")
		if !strings.Contains(key, "*") {
			continue
		}
		id, _ := getFloat(item, "id")
		fullKey, _ := getString(keys, fmt.Sprintf("%d", int(id)))
		fullKey = strings.TrimSpace(fullKey)
		if fullKey == "" || strings.Contains(fullKey, "*") {
			return fmt.Errorf("fetch full New API token keys: token %d is missing", int(id))
		}
		item["key"] = fullKey
	}
	return nil
}

func (n *NewApiAdapter) listedTokenIDForDelete(ctx context.Context, baseURL string, response map[string]interface{}, headers map[string]string, targetKey string, proxy *ProxyConfig) (*int, error) {
	if success, present := getBool(response, "success"); present && !success {
		return nil, fmt.Errorf("upstream token listing was refused")
	}
	items := parseTokenItemsFromMap(response)
	if len(items) == 0 {
		data, _ := getMap(response, "data")
		var emptyList bool
		for _, value := range []interface{}{response["data"], response["items"], response["list"], data["items"], data["data"], data["list"]} {
			if entries, ok := value.([]interface{}); ok && len(entries) == 0 {
				emptyList = true
			}
		}
		if !emptyList {
			return nil, fmt.Errorf("upstream token listing contains no supported token collection")
		}
	}
	if err := n.hydrateListedTokenKeys(ctx, baseURL, items, headers, proxy); err != nil {
		return nil, err
	}
	// New API stores bare keys but presents them with an optional sk- prefix.
	// Compare the credential identity, not that display prefix.
	targetKey = strings.TrimPrefix(targetKey, "sk-")
	for _, item := range items {
		key, ok := getString(item, "key")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("upstream token listing contains an unresolved key")
		}
		if strings.TrimPrefix(normalizeTokenKeyForCompare(key), "sk-") == targetKey {
			id := getIntPtr(item, "id")
			if id == nil || *id <= 0 {
				return nil, fmt.Errorf("matching upstream token has no usable id")
			}
			return id, nil
		}
	}
	// A capped first page cannot prove that an unmatched token is absent.
	total, _ := getFloat(response, "total")
	if data, ok := getMap(response, "data"); ok {
		if nestedTotal, present := getFloat(data, "total"); present {
			total = nestedTotal
		}
	}
	if len(items) >= UpstreamTokenListPageLimit || total > float64(len(items)) {
		return nil, fmt.Errorf("upstream token listing may be truncated; cannot confirm token absence")
	}
	return nil, nil
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
	resp, err := fetchJSON(ctx, baseURL+apiTokenListPath(), "GET", nil, n.authHeaders(accessToken, resolvedUserID), proxy)
	if err != nil {
		reason = err.Error()
	} else {
		var resolveErr error
		tokenID, resolveErr = n.listedTokenIDForDelete(ctx, baseURL, resp, n.authHeaders(accessToken, resolvedUserID), targetKey, proxy)
		if resolveErr != nil {
			reason = resolveErr.Error()
		} else {
			listed = true
		}
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
			resp, err := fetchJSON(ctx, baseURL+apiTokenListPath(), "GET", nil, headers, proxy)
			if err != nil {
				reason = err.Error()
			} else {
				var resolveErr error
				tokenID, resolveErr = n.listedTokenIDForDelete(ctx, baseURL, resp, headers, targetKey, proxy)
				if resolveErr != nil {
					reason = resolveErr.Error()
				} else {
					listed = true
				}
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
