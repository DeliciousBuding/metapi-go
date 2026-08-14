package platform

import (
	"context"
	"encoding/json"
	"fmt"
)

// --- Token CRUD ---

func (n *NewApiAdapter) GetAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := n.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		return nil, nil
	}
	return findFirstEnabledToken(tokens), nil
}

func (n *NewApiAdapter) GetAPITokens(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	return n.getAPITokensWithUser(ctx, baseURL, accessToken, platformUserId, proxy)
}

func (n *NewApiAdapter) getAPITokenWithUser(ctx context.Context, baseURL, accessToken string, userID *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := n.getAPITokensWithUser(ctx, baseURL, accessToken, userID, proxy)
	if err != nil || len(tokens) == 0 {
		return nil, nil
	}
	return findFirstEnabledToken(tokens), nil
}

func (n *NewApiAdapter) getAPITokensWithUser(ctx context.Context, baseURL, accessToken string, userID *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	// Try Bearer auth
	resp, err := fetchJSON(ctx, baseURL+"/api/token/?p=0&size=100", "GET", nil, n.authHeaders(accessToken, userID), proxy)
	if err == nil {
		items := parseTokenItemsFromMap(resp)
		normalized := normalizeTokenItems(items)
		if len(normalized) > 0 {
			return normalized, nil
		}
	}

	// Cookie fallback
	cookieTokens := n.getAPITokensByCookie(ctx, baseURL, accessToken, userID, proxy)
	if len(cookieTokens) > 0 {
		return cookieTokens, nil
	}

	// Alternate userID cookie fallback
	altID := n.probeAlternateUserIDByCookie(ctx, baseURL, accessToken, userID, proxy)
	if altID != nil {
		fallbackTokens := n.getAPITokensByCookie(ctx, baseURL, accessToken, altID, proxy)
		if len(fallbackTokens) > 0 {
			return fallbackTokens, nil
		}
	}

	return []ApiTokenInfo{}, nil
}

func (n *NewApiAdapter) getAPITokensByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) []ApiTokenInfo {
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
		normalized := normalizeTokenItems(items)
		if len(normalized) > 0 {
			return normalized
		}
	}
	return nil
}

func (n *NewApiAdapter) CreateAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, options *CreateAPITokenOptions, proxy *ProxyConfig) (bool, error) {
	payload := buildDefaultTokenPayload(options)
	bodyBytes, _ := json.Marshal(payload)

	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	// Try Bearer auth
	resp, err := fetchJSON(ctx, baseURL+"/api/token/", "POST", json.RawMessage(bodyBytes), n.authHeaders(accessToken, resolvedUserID), proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			return true, nil
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

		resp, err := fetchJSON(ctx, baseURL+"/api/token/", "POST", json.RawMessage(bodyBytes), headers, proxy)
		if err == nil {
			if success, _ := getBool(resp, "success"); success {
				return true, nil
			}
		}
	}

	return false, nil
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

	// Try Bearer auth list
	resp, err := fetchJSON(ctx, baseURL+"/api/token/?p=0&size=100", "GET", nil, n.authHeaders(accessToken, resolvedUserID), proxy)
	if err == nil {
		items := parseTokenItemsFromMap(resp)
		tokenID = pickTokenID(items, targetKey)
	}

	if tokenID != nil {
		// Try Bearer DELETE
		delResp, err := fetchJSON(ctx, fmt.Sprintf("%s/api/token/%d", baseURL, *tokenID), "DELETE", nil, n.authHeaders(accessToken, resolvedUserID), proxy)
		if err == nil {
			if success, _ := getBool(delResp, "success"); success {
				return nil
			}
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
			if err == nil {
				items := parseTokenItemsFromMap(resp)
				tokenID = pickTokenID(items, targetKey)
			}
		}

		if tokenID == nil {
			continue
		}

		delResp, err := fetchJSON(ctx, fmt.Sprintf("%s/api/token/%d", baseURL, *tokenID), "DELETE", nil, headers, proxy)
		if err == nil {
			if success, _ := getBool(delResp, "success"); success {
				return nil
			}
		}
	}

	// Already absent = safe
	if tokenID == nil {
		return nil
	}
	return fmt.Errorf("failed to delete token")
}
