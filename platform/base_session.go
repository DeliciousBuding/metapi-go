package platform

import (
	"context"
	"net/http"
	"strings"
)

// Login provides a default login implementation: POST /api/user/login.
func (b *BaseAdapter) Login(ctx context.Context, url, username, password string, platformUserId *int, proxy *ProxyConfig) (*LoginResult, error) {
	body := map[string]string{"username": username, "password": password}
	resp, err := fetchJSON(ctx, url+"/api/user/login", http.MethodPost, body, nil, proxy)
	if err != nil {
		return &LoginResult{Success: false, Message: err.Error()}, nil
	}

	// Extract token from various possible fields
	data, _ := getMap(resp, "data")
	token := extractLoginToken(resp, data)
	success, hasSuccess := getBool(resp, "success")

	// v1 omits top-level success; presence of a usable credential implies success.
	// Explicit success:false is always a failure.
	if token == "" && !(hasSuccess && success) {
		msg, _ := getString(resp, "message")
		if msg == "" {
			msg = "login failed"
		}
		return &LoginResult{Success: false, Message: msg}, nil
	}

	return &LoginResult{
		Success:     true,
		AccessToken: token,
		Username:    username,
	}, nil
}

// GetUserInfo provides a default implementation: GET /api/user/self with Bearer auth.
func (b *BaseAdapter) GetUserInfo(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) (*UserInfo, error) {
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, url+"/api/user/self", http.MethodGet, nil, headers, proxy)
	if err != nil {
		return nil, nil
	}

	success, _ := getBool(resp, "success")
	if !success {
		return nil, nil
	}

	data, ok := getMap(resp, "data")
	if !ok {
		return nil, nil
	}

	return parseUserInfo(data), nil
}

func parseUserInfo(data map[string]interface{}) *UserInfo {
	username, _ := getString(data, "username")
	displayName, _ := getString(data, "display_name")
	if username == "" {
		username = displayName
	}

	email, _ := getString(data, "email")

	return &UserInfo{
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Role:        getIntPtr(data, "role"),
	}
}

// VerifyToken provides default token verification (session + apikey paths).
func (b *BaseAdapter) VerifyToken(ctx context.Context, url, token string, platformUserId *int, proxy *ProxyConfig) (*TokenVerifyResult, error) {
	// 1. Try as session/access token
	userInfo, err := b.GetUserInfo(ctx, url, token, platformUserId, proxy)
	if err == nil && userInfo != nil {
		var balance *BalanceInfo
		balanceInfo, err := b.GetBalance(ctx, url, token, platformUserId, proxy)
		if err == nil {
			balance = balanceInfo
		}

		var apiToken *string
		apiT, err := b.GetAPIToken(ctx, url, token, platformUserId, proxy)
		if err == nil && apiT != nil {
			apiToken = apiT
		}

		apiTokenStr := ""
		if apiToken != nil {
			apiTokenStr = *apiToken
		}
		return &TokenVerifyResult{
			TokenType: "session",
			UserInfo:  userInfo,
			Balance:   balance,
			APIToken:  apiTokenStr,
		}, nil
	}

	// 2. Try as API key (via /v1/models)
	models, err := b.GetModels(ctx, url, token, platformUserId, proxy)
	if err == nil && len(models) > 0 {
		return &TokenVerifyResult{
			TokenType: "apikey",
			Models:    models,
		}, nil
	}

	return &TokenVerifyResult{TokenType: "unknown"}, nil
}

func authBearerHeaders(token string) map[string]string {
	t := strings.TrimPrefix(strings.TrimSpace(token), "Bearer ")
	return map[string]string{"Authorization": "Bearer " + t}
}

func stripBearerPrefix(token string) string {
	t := strings.TrimSpace(token)
	t = strings.TrimPrefix(t, "Bearer ")
	return strings.TrimSpace(t)
}

func buildCookieCandidates(token string) []string {
	t := strings.TrimSpace(token)
	t = strings.TrimPrefix(t, "Bearer ")
	t = strings.TrimSpace(t)
	if t == "" {
		return nil
	}

	candidates := []string{}
	seen := map[string]bool{}

	if strings.Contains(t, "=") {
		candidates = append(candidates, t)
		seen[t] = true
	}

	s := "session=" + t
	if !seen[s] {
		candidates = append(candidates, s)
		seen[s] = true
	}

	tt := "token=" + t
	if !seen[tt] {
		candidates = append(candidates, tt)
	}
	return candidates
}

func mergeSetCookie(existing string, setCookies []string) string {
	merged := existing
	for _, raw := range setCookies {
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, ";", 2)
		pair := strings.TrimSpace(parts[0])
		eqIdx := strings.Index(pair, "=")
		if eqIdx <= 0 {
			continue
		}
		name := strings.TrimSpace(pair[:eqIdx])
		value := pair[eqIdx+1:]
		merged = upsertCookie(merged, name, value)
	}
	return merged
}

func upsertCookie(cookieHeader, name, value string) string {
	parts := strings.Split(cookieHeader, ";")
	filtered := make([]string, 0, len(parts))
	replaced := false
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eqIdx := strings.Index(part, "=")
		if eqIdx < 0 {
			filtered = append(filtered, part)
			continue
		}
		key := strings.TrimSpace(part[:eqIdx])
		if key == name {
			replaced = true
			filtered = append(filtered, name+"="+value)
		} else {
			filtered = append(filtered, part)
		}
	}
	if !replaced {
		filtered = append(filtered, name+"="+value)
	}
	return strings.Join(filtered, "; ")
}

func hasUsableSessionCookie(cookieHeader string) bool {
	if cookieHeader == "" {
		return false
	}
	ignored := map[string]bool{"acw_tc": true, "acw_sc__v2": true, "cdn_sec_tc": true}
	parts := strings.Split(cookieHeader, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		eqIdx := strings.Index(part, "=")
		if eqIdx <= 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(part[:eqIdx]))
		if name == "" || ignored[name] {
			continue
		}
		lower := strings.ToLower(name)
		if lower == "session" || lower == "token" || lower == "auth_token" ||
			lower == "access_token" || lower == "jwt" || lower == "jwt_token" ||
			strings.Contains(lower, "session") || strings.Contains(lower, "token") ||
			strings.Contains(lower, "auth") {
			return true
		}
	}
	return false
}

func extractLoginToken(resp, data map[string]interface{}) string {
	candidates := []interface{}{
		data,
		resp["token"],
		resp["accessToken"],
		resp["access_token"],
	}
	if data != nil {
		candidates = append(candidates, data["token"], data["accessToken"], data["access_token"])
	}
	for _, c := range candidates {
		if s, ok := c.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}
