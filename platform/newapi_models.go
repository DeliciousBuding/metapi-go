package platform

import (
	"context"
	"fmt"
	"strings"
)

func (n *NewApiAdapter) GetModels(ctx context.Context, baseURL, token string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	// Four rungs, and every one of them used to swallow its failure into the next,
	// ending in `[]string{}, nil`. That made "the site turned /v1/models off",
	// "the credential is dead", "this credential has no dashboard" and "this
	// account really has no models" the same answer, and the caller can only
	// classify the last one (#1232 is a report about exactly that ambiguity).
	lad := &modelFetchLadder{}

	openAIModels := n.getOpenAIModels(ctx, baseURL, token, proxy, lad)
	if len(openAIModels) > 0 {
		return openAIModels, nil
	}

	userID := platformUserId
	if userID == nil {
		userID = n.discoverUserID(ctx, baseURL, token, proxy)
	}

	// New API's /api/user/models route is UserAuth, which authenticates the
	// Authorization bearer directly and does not require New-Api-User. Try it
	// even when user-ID discovery did not answer; older forks that do require
	// the header still get it when platformUserId/discovery supplied one.
	userModels := fetchUserModels(ctx, baseURL, n.authHeaders(token, userID), proxy, lad)
	if len(userModels) > 0 {
		return userModels, nil
	}

	// Cookie model fallback
	cookieModels := n.getSessionModelsByCookie(ctx, baseURL, token, userID, proxy, lad)
	if len(cookieModels) > 0 {
		return cookieModels, nil
	}

	// Alternate userID cookie fallback
	altID := n.probeAlternateUserIDByCookie(ctx, baseURL, token, userID, proxy)
	if altID != nil {
		fallbackModels := n.getSessionModelsByCookie(ctx, baseURL, token, altID, proxy, lad)
		if len(fallbackModels) > 0 {
			return fallbackModels, nil
		}
	}

	return lad.result()
}

func (n *NewApiAdapter) getOpenAIModels(ctx context.Context, baseURL, token string, proxy *ProxyConfig, lad *modelFetchLadder) []string {
	// Try /v1/models
	resp, err := fetchJSON(ctx, baseURL+"/v1/models", "GET", nil, authBearerHeaders(token), proxy)
	if err != nil {
		lad.fail(safeModelDiscoveryReason(err.Error()))
		return nil
	}

	models, answered, reason := parseOpenAIModelsResponse(resp)
	if !answered {
		lad.fail(reason)
		return nil
	}
	lad.answer()
	return models
}

func fetchUserModels(ctx context.Context, baseURL string, headers map[string]string, proxy *ProxyConfig, lad *modelFetchLadder) []string {
	resp, err := fetchJSON(ctx, baseURL+"/api/user/models", "GET", nil, headers, proxy)
	if err != nil {
		lad.fail(safeModelDiscoveryReason(err.Error()))
		return nil
	}

	models, answered, reason := parseUserModelsResponse(resp)
	if !answered {
		lad.fail(reason)
		return nil
	}
	lad.answer()
	return models
}

func (n *NewApiAdapter) getSessionModelsByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig, lad *modelFetchLadder) []string {
	for _, cookie := range buildCookieCandidates(token) {
		models := fetchUserModels(ctx, baseURL, n.cookieHeaders(cookie, userID), proxy, lad)
		if len(models) > 0 {
			return models
		}
	}
	return nil
}

func parseOpenAIModelsResponse(resp map[string]interface{}) ([]string, bool, string) {
	if reason := explicitModelListFailure(resp); reason != "" {
		return nil, false, reason
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		return nil, false, "upstream returned an invalid model-list response"
	}
	models := make([]string, 0, len(data))
	for _, item := range data {
		model, ok := item.(map[string]interface{})
		if !ok {
			return nil, false, "upstream returned an invalid model-list response"
		}
		id, ok := model["id"].(string)
		if !ok {
			return nil, false, "upstream returned an invalid model-list response"
		}
		if strings.TrimSpace(id) != "" {
			models = append(models, id)
		}
	}
	return normalizeModelIDs(models), true, ""
}

func parseUserModelsResponse(resp map[string]interface{}) ([]string, bool, string) {
	if reason := explicitModelListFailure(resp); reason != "" {
		return nil, false, reason
	}

	if data, ok := resp["data"].([]interface{}); ok {
		models := make([]string, 0, len(data))
		for _, item := range data {
			model, ok := item.(string)
			if !ok {
				return nil, false, "upstream returned an invalid model-list response"
			}
			if strings.TrimSpace(model) != "" {
				models = append(models, model)
			}
		}
		return normalizeModelIDs(models), true, ""
	}

	if data, ok := getMap(resp, "data"); ok {
		models := make([]string, 0, len(data))
		for model := range data {
			if strings.TrimSpace(model) != "" {
				models = append(models, model)
			}
		}
		return normalizeModelIDs(models), true, ""
	}

	return nil, false, "upstream returned an invalid model-list response"
}

func explicitModelListFailure(resp map[string]interface{}) string {
	if success, ok := getBool(resp, "success"); ok && !success {
		return safeModelDiscoveryReason(extractResponseMessage(resp))
	}
	if raw, ok := resp["error"]; ok && raw != nil {
		return safeModelDiscoveryReason(extractResponseMessage(resp))
	}
	return ""
}

func safeModelDiscoveryReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "upstream rejected the model-list request"
	}
	if explained := ExplainUpstreamFailure(0, reason); explained != "" {
		return explained
	}
	return "upstream rejected the model-list request"
}

// --- GetUserGroups ---

func (n *NewApiAdapter) GetUserGroups(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	var terminalError string

	endpoints := []string{"/api/user/self/groups", "/api/user_group_map"}
	for _, endpoint := range endpoints {
		groups, err := fetchUserGroups(ctx, baseURL+endpoint, n.authHeaders(accessToken, resolvedUserID), proxy)
		if err != nil && terminalError == "" {
			terminalError = err.Error()
		}
		if len(groups) > 0 {
			return dedupeStrings(groups), nil
		}
	}

	// Cookie fallback
	cookieUserID := resolvedUserID
	if cookieUserID == nil {
		cookieUserID = n.probeUserIDByCookie(ctx, baseURL, accessToken, proxy)
	}

	for _, cookie := range buildCookieCandidates(accessToken) {
		headers := n.cookieHeaders(cookie, cookieUserID)

		for _, endpoint := range endpoints {
			resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
			if err != nil {
				continue
			}
			if success, _ := getBool(resp, "success"); !success {
				msg := resolveGroupFetchErrorMessage(resp)
				if terminalError == "" {
					terminalError = msg
				}
			}
			parsed := extractGroupKeys(resp)
			if len(parsed) > 0 {
				return dedupeStrings(parsed), nil
			}
		}
	}

	if terminalError != "" {
		return nil, fmt.Errorf("%s", terminalError)
	}

	return []string{"default"}, nil
}
