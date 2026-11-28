package platform

import (
	"context"
	"fmt"
	"strings"
)

// GetUserGroups: GET /api/user_group_map, fallback /api/user/self/groups.
func (o *OneApiAdapter) GetUserGroups(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	headers := authBearerHeaders(accessToken)
	var terminalError string

	// Try /api/user_group_map first (OneApi order)
	groups, err := fetchUserGroups(ctx, baseURL+"/api/user_group_map", headers, proxy)
	if err != nil {
		terminalError = err.Error()
	}
	if len(groups) > 0 {
		return dedupeStrings(groups), nil
	}

	// Fallback: /api/user/self/groups
	groups, err = fetchUserGroups(ctx, baseURL+"/api/user/self/groups", headers, proxy)
	if err != nil {
		if terminalError == "" {
			terminalError = err.Error()
		}
	}
	if len(groups) > 0 {
		return dedupeStrings(groups), nil
	}

	if terminalError != "" {
		return nil, fmt.Errorf("%s", terminalError)
	}

	return []string{"default"}, nil
}

func fetchUserGroups(ctx context.Context, url string, headers map[string]string, proxy *ProxyConfig) ([]string, error) {
	resp, err := fetchJSON(ctx, url, "GET", nil, headers, proxy)
	if err != nil {
		return nil, err
	}

	if success, _ := getBool(resp, "success"); !success {
		msg := resolveGroupFetchErrorMessage(resp)
		return nil, fmt.Errorf("%s", msg)
	}

	return extractGroupKeys(resp), nil
}

func resolveGroupFetchErrorMessage(payload map[string]interface{}) string {
	msg, _ := getString(payload, "message")
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "failed to fetch groups"
	}
	// UX rewrite only — does not mark accounts.status. Prefer auth-related
	// classes; avoid rewriting pure billing/model/validation messages.
	if IsAuthRelatedUpstreamError(0, msg) || IsTokenExpiredError(0, msg) {
		return "账号会话可能已过期，请重新登录后再拉取分组"
	}
	// Keep a few historical session phrases that classify as auth.
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "未登录") ||
		strings.Contains(lower, "not login") ||
		strings.Contains(lower, "not logged") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "invalid token") ||
		strings.Contains(lower, "access token") {
		// Exclude non-auth classes already handled by classifier above; if we
		// still land here, rewrite only when not clearly billing/model/validation.
		switch ClassifyUpstreamError(0, msg) {
		case ClassBilling, ClassModel, ClassValidation, ClassTransient:
			return msg
		default:
			return "账号会话可能已过期，请重新登录后再拉取分组"
		}
	}
	return msg
}

func extractGroupKeys(payload map[string]interface{}) []string {
	source := payload
	if data, ok := getMap(payload, "data"); ok {
		source = data
	}

	if source == nil {
		return nil
	}

	excluded := map[string]bool{
		"success": true, "message": true, "code": true, "data": true, "error": true,
	}

	keys := make([]string, 0, len(source))
	for k := range source {
		if !excluded[strings.ToLower(k)] && strings.TrimSpace(k) != "" {
			keys = append(keys, strings.TrimSpace(k))
		}
	}
	return keys
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))
	for _, item := range items {
		t := strings.TrimSpace(item)
		if t != "" && !seen[t] {
			seen[t] = true
			result = append(result, t)
		}
	}
	return result
}
