package platform

import (
	"context"
	"fmt"
	"strings"
)

// OneApiAdapter handles OneAPI platforms (detect: /api/status by version +
// system_name value discriminator against new-api).
// Serves as the base for OneHubAdapter and DoneHubAdapter.
type OneApiAdapter struct {
	*BaseAdapter
}

// Detect probes GET /api/status and checks success===true, data.version is
// present, and system_name (when present) names one-api. one-api v0.5.x has no
// system_name (legacy shape); v0.6.10+ ships system_name (default "One API").
// The value check keeps new-api (default "New API") and one-api apart.
// Requiring version keeps unrelated /api/status endpoints from being labeled
// one-api. Operators who rename SYSTEM_NAME to something unrelated degrade to
// manual platform selection — acceptable for an auto-detect heuristic.
func (o *OneApiAdapter) Detect(ctx context.Context, url string) (bool, error) {
	ctx, cancel := withProbeTimeout(ctx)
	defer cancel()
	resp, err := fetchJSON(ctx, url+"/api/status", "GET", nil, nil, nil)
	if err != nil {
		return false, nil
	}
	success, _ := getBool(resp, "success")
	if !success {
		return false, nil
	}
	data, ok := getMap(resp, "data")
	if !ok {
		return false, nil
	}
	if _, hasVersion := data["version"]; !hasVersion {
		return false, nil
	}
	systemName, hasSystemName := getString(data, "system_name")
	if !hasSystemName {
		// Legacy v0.5 shape (or a non-string system_name): nothing to
		// discriminate on, treat as absent.
		return true, nil
	}
	folded := strings.ToLower(systemName)
	return strings.Contains(folded, "oneapi") || strings.Contains(folded, "one api"), nil
}

// Login POSTs /api/user/login and accepts either a legacy data.access_token
// or a captured session cookie. one-api v0.6.10 returns success:true with an
// EMPTY data.access_token and authenticates via a Set-Cookie session cookie,
// so the cookie header becomes the stored credential. Mirrors
// NewApiAdapter.Login (same fetchLoginResponse + hasUsableSessionCookie flow).
func (o *OneApiAdapter) Login(ctx context.Context, baseURL, username, password string, platformUserId *int, proxy *ProxyConfig) (*LoginResult, error) {
	body := map[string]string{"username": username, "password": password}
	headers := map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"User-Agent":       DefaultBrowserUserAgent,
	}

	parsed, cookieHeader, err := fetchLoginResponse(ctx, baseURL+"/api/user/login", body, headers, proxy)
	if err != nil {
		return &LoginResult{Success: false, Message: err.Error()}, nil
	}

	if parsed == nil {
		return &LoginResult{Success: false, Message: "shield challenge blocked login"}, nil
	}

	data, _ := getMap(parsed, "data")
	accessToken := extractLoginToken(parsed, data)
	success, hasSuccess := getBool(parsed, "success")

	// Legacy one-api (v0.5) returns a real data.access_token; tolerate a
	// missing top-level success the same way NewApiAdapter does.
	if accessToken != "" && (!hasSuccess || success) {
		return &LoginResult{Success: true, AccessToken: accessToken, Username: username}, nil
	}

	// one-api v0.6.10: success:true but empty data.access_token; the real
	// credential is the session cookie captured from Set-Cookie.
	if hasSuccess && success && hasUsableSessionCookie(cookieHeader) {
		return &LoginResult{Success: true, AccessToken: cookieHeader, Username: username}, nil
	}

	msg := extractResponseMessage(parsed)
	if msg == "" {
		msg = "login failed: no usable session credential, try Cookie/Token import"
	}
	return &LoginResult{Success: false, Message: msg}, nil
}

// GetUserInfo: GET /api/user/self with Bearer auth first; session-cookie
// credentials fall back to a Cookie header (one-api v0.6.10 login).
func (o *OneApiAdapter) GetUserInfo(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*UserInfo, error) {
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, authBearerHeaders(accessToken), proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			if data, ok := getMap(resp, "data"); ok {
				return parseUserInfo(data), nil
			}
		}
	}

	if isCookieCredential(accessToken) {
		cookieResp, cookieErr := o.fetchSelfByCookie(ctx, baseURL, accessToken, proxy)
		if cookieErr == nil && cookieResp != nil {
			if data, ok := getMap(cookieResp, "data"); ok {
				return parseUserInfo(data), nil
			}
		}
	}

	return nil, nil
}

// VerifyToken mirrors BaseAdapter.VerifyToken but dispatches to the
// cookie-aware overrides above. BaseAdapter.VerifyToken calls its own
// Bearer-only GetUserInfo/GetBalance (Go static dispatch on the embedded
// receiver), which would reject session-cookie credentials stored by the
// v0.6.10 login; this override routes through o.GetUserInfo/o.GetBalance
// instead.
func (o *OneApiAdapter) VerifyToken(ctx context.Context, baseURL, token string, platformUserId *int, proxy *ProxyConfig) (*TokenVerifyResult, error) {
	// 1. Try as session/access token
	userInfo, err := o.GetUserInfo(ctx, baseURL, token, platformUserId, proxy)
	if err == nil && userInfo != nil {
		var balance *BalanceInfo
		balanceInfo, err := o.GetBalance(ctx, baseURL, token, platformUserId, proxy)
		if err == nil {
			balance = balanceInfo
		}

		var apiToken *string
		apiT, err := o.GetAPIToken(ctx, baseURL, token, platformUserId, proxy)
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
	models, err := o.GetModels(ctx, baseURL, token, platformUserId, proxy)
	if err == nil && len(models) > 0 {
		return &TokenVerifyResult{
			TokenType: "apikey",
			Models:    models,
		}, nil
	}

	return &TokenVerifyResult{TokenType: "unknown"}, nil
}

// Checkin: POST /api/user/checkin. Bearer auth first; session-cookie
// credentials (one-api v0.6.10 login) fall back to a Cookie header.
func (o *OneApiAdapter) Checkin(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*CheckinResult, error) {
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, baseURL+"/api/user/checkin", "POST", nil, headers, proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			return checkinResultFromResponse(resp, "Check-in successful", "Check-in failed"), nil
		}
		if !isCookieCredential(accessToken) {
			// Report the upstream business message (e.g. "already checked in").
			return checkinResultFromResponse(resp, "Check-in successful", "Check-in failed"), nil
		}
	} else if !isCookieCredential(accessToken) {
		return &CheckinResult{Success: false, Message: err.Error()}, nil
	}

	// The Bearer attempt is always rejected for cookie-shaped credentials;
	// retry with the session cookie as the Cookie header.
	cookieResp, cookieErr := fetchJSON(ctx, baseURL+"/api/user/checkin", "POST", nil, map[string]string{"Cookie": strings.TrimSpace(accessToken)}, proxy)
	if cookieErr == nil {
		return checkinResultFromResponse(cookieResp, "Check-in successful", "Check-in failed"), nil
	}
	if err != nil {
		return &CheckinResult{Success: false, Message: err.Error()}, nil
	}
	return &CheckinResult{Success: false, Message: cookieErr.Error()}, nil
}

// GetBalance: quota=total, balance=quota-used, divisor=500000. Bearer auth
// first; session-cookie credentials fall back to a Cookie header.
func (o *OneApiAdapter) GetBalance(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*BalanceInfo, error) {
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, headers, proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			if data, ok := getMap(resp, "data"); ok {
				balance := parseOneApiStyleBalance(data, 500000, false)
				return &balance, nil
			}
		}
	}

	if isCookieCredential(accessToken) {
		cookieResp, cookieErr := o.fetchSelfByCookie(ctx, baseURL, accessToken, proxy)
		if cookieErr == nil && cookieResp != nil {
			if data, ok := getMap(cookieResp, "data"); ok {
				balance := parseOneApiStyleBalance(data, 500000, false)
				return &balance, nil
			}
		}
	}

	return &BalanceInfo{}, nil
}

// fetchSelfByCookie fetches /api/user/self with the credential sent as a
// Cookie header. Minimal mirror of NewApiAdapter.fetchUserSelfByCookie without
// the New-Api-User header injection (one-api does not use that header).
func (o *OneApiAdapter) fetchSelfByCookie(ctx context.Context, baseURL, credential string, proxy *ProxyConfig) (map[string]interface{}, error) {
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, map[string]string{"Cookie": strings.TrimSpace(credential)}, proxy)
	if err != nil {
		return nil, err
	}
	if success, _ := getBool(resp, "success"); !success {
		return nil, fmt.Errorf("cookie session rejected")
	}
	if _, ok := getMap(resp, "data"); !ok {
		return nil, fmt.Errorf("cookie session response missing data")
	}
	return resp, nil
}

// isCookieCredential reports whether a stored credential is a session-cookie
// header (e.g. "session=abc123") rather than a Bearer token (sk- key, JWT).
// A cookie header contains at least one name=value pair with no whitespace
// before '='; the name must not contain '.' so JWT segments (which only carry
// '=' as base64 padding) do not classify as cookies.
func isCookieCredential(credential string) bool {
	value := strings.TrimSpace(stripBearerPrefix(credential))
	if value == "" {
		return false
	}
	for _, pair := range strings.Split(value, ";") {
		trimmed := strings.TrimSpace(pair)
		eq := strings.Index(trimmed, "=")
		if eq <= 0 || eq == len(trimmed)-1 {
			continue
		}
		if strings.ContainsAny(trimmed[:eq], " \t.") {
			continue
		}
		return true
	}
	return false
}

// GetModels: GET /v1/models (Bearer auth).
func (o *OneApiAdapter) GetModels(ctx context.Context, baseURL string, apiToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	headers := authBearerHeaders(apiToken)
	resp, err := fetchJSON(ctx, baseURL+"/v1/models", "GET", nil, headers, proxy)
	if err != nil {
		return nil, err
	}

	return extractModelIDsFromData(resp), nil
}

// GetAPITokens: GET /api/token/?p=0&size=100 (Bearer auth).
func (o *OneApiAdapter) GetAPITokens(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, baseURL+"/api/token/?p=0&size=100", "GET", nil, headers, proxy)
	if err != nil {
		return []ApiTokenInfo{}, nil
	}

	items := parseTokenItemsFromMap(resp)
	if len(items) == 0 {
		return []ApiTokenInfo{}, nil
	}
	return normalizeTokenItems(items), nil
}

// GetAPIToken returns the first enabled token.
func (o *OneApiAdapter) GetAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := o.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		return nil, nil
	}
	return findFirstEnabledToken(tokens), nil
}

// GetUserGroups: GET /api/user_group_map, fallback /api/user/self/groups.
func (o *OneApiAdapter) GetUserGroups(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	headers := authBearerHeaders(accessToken)
	var terminalError string

	// Try /api/user_group_map first (OneApi order)
	groups, err := o.tryGetGroups(ctx, baseURL+"/api/user_group_map", headers, proxy)
	if err != nil {
		terminalError = err.Error()
	}
	if len(groups) > 0 {
		return dedupeStrings(groups), nil
	}

	// Fallback: /api/user/self/groups
	groups, err = o.tryGetGroups(ctx, baseURL+"/api/user/self/groups", headers, proxy)
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

func (o *OneApiAdapter) tryGetGroups(ctx context.Context, url string, headers map[string]string, proxy *ProxyConfig) ([]string, error) {
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

// CreateAPIToken: POST /api/token/ (Bearer auth).
func (o *OneApiAdapter) CreateAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, options *CreateAPITokenOptions, proxy *ProxyConfig) (bool, error) {
	payload := buildDefaultTokenPayload(options)
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, baseURL+"/api/token/", "POST", payload, headers, proxy)
	if err != nil {
		return false, nil
	}
	success, _ := getBool(resp, "success")
	return success, nil
}

// DeleteAPIToken: list -> find key -> DELETE /api/token/{id} (with trailing-slash fallback).
func (o *OneApiAdapter) DeleteAPIToken(ctx context.Context, baseURL, accessToken, tokenKey string, platformUserId *int, proxy *ProxyConfig) error {
	targetKey := normalizeTokenKeyForCompare(tokenKey)
	if targetKey == "" {
		return nil
	}

	headers := authBearerHeaders(accessToken)

	// List tokens
	resp, err := fetchJSON(ctx, baseURL+"/api/token/?p=0&size=100", "GET", nil, headers, proxy)
	if err != nil {
		return nil
	}

	items := parseTokenItemsFromMap(resp)
	tokenID := pickTokenID(items, targetKey)
	if tokenID == nil {
		return nil // Already absent, safe
	}

	// Try DELETE without trailing slash
	delResp, err := fetchJSON(ctx, fmt.Sprintf("%s/api/token/%d", baseURL, *tokenID), "DELETE", nil, headers, proxy)
	if err == nil {
		if success, _ := getBool(delResp, "success"); success {
			return nil
		}
	}

	// Double-DELETE: trailing slash fallback (OneApi-specific)
	delResp2, err := fetchJSON(ctx, fmt.Sprintf("%s/api/token/%d/", baseURL, *tokenID), "DELETE", nil, headers, proxy)
	if err == nil {
		success, _ := getBool(delResp2, "success")
		if success {
			return nil
		}
		_ = success
	}
	return nil
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

func buildDefaultTokenPayload(options *CreateAPITokenOptions) map[string]interface{} {
	name := "metapi"
	if options != nil && strings.TrimSpace(options.Name) != "" {
		name = strings.TrimSpace(options.Name)
	}

	unlimitedQuota := true
	if options != nil {
		unlimitedQuota = options.UnlimitedQuota
	}

	remainQuota := 0.0
	if options != nil && options.RemainQuota > 0 {
		remainQuota = options.RemainQuota
	}

	expiredTime := int64(-1)
	if options != nil && options.ExpiredTime != 0 {
		expiredTime = options.ExpiredTime
	}

	allowIPs := ""
	modelLimits := ""
	group := ""
	modelLimitsEnabled := false
	if options != nil {
		allowIPs = options.AllowIPs
		modelLimits = options.ModelLimits
		group = options.Group
		modelLimitsEnabled = options.ModelLimitsEnabled
	}

	return map[string]interface{}{
		"name":                 name,
		"unlimited_quota":      unlimitedQuota,
		"expired_time":         expiredTime,
		"remain_quota":         remainQuota,
		"allow_ips":            allowIPs,
		"model_limits_enabled": modelLimitsEnabled,
		"model_limits":         modelLimits,
		"group":                group,
	}
}
