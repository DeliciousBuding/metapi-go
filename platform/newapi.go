package platform

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// NewApiAdapter handles NewAPI platforms with full cookie fallback, shield challenge,
// user-ID probing, and 7-header injection. Serves as the base for AnyRouterAdapter.
type NewApiAdapter struct {
	*BaseAdapter
}

// Detect probes GET /api/status and checks success===true and that
// data.system_name's value names a NewAPI fork. one-api v0.6.10 also ships
// system_name (default "One API"), so mere presence of the key misdetects
// one-api as new-api; the value-based check keeps the two apart.
// Known NewAPI fork aliases (vo-api/super-api/rix-api/neo-api) short-circuit
// via URL keyword so shield/WAF-fronted deployments whose /api/status probe is
// blocked still get detected.
// Operators who rename SYSTEM_NAME to something unrelated degrade to manual
// platform selection — acceptable for an auto-detect heuristic.
func (n *NewApiAdapter) Detect(ctx context.Context, url string) (bool, error) {
	lower := strings.ToLower(url)
	for _, kw := range []string{"vo-api", "super-api", "rix-api", "neo-api"} {
		if strings.Contains(lower, kw) {
			return true, nil
		}
	}

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
	systemName, hasSystemName := getString(data, "system_name")
	if !hasSystemName {
		return false, nil
	}
	folded := strings.ToLower(systemName)
	return strings.Contains(folded, "newapi") || strings.Contains(folded, "new api"), nil
}

// --- Login ---

func (n *NewApiAdapter) Login(ctx context.Context, baseURL, username, password string, platformUserId *int, proxy *ProxyConfig) (*LoginResult, error) {
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

	// v1 omits top-level success; presence of a usable credential implies success.
	// Explicit success:false is always a failure.
	if accessToken != "" && (!hasSuccess || success) {
		return &LoginResult{Success: true, AccessToken: accessToken, Username: username}, nil
	}

	if hasSuccess && success && hasUsableSessionCookie(cookieHeader) {
		return &LoginResult{Success: true, AccessToken: cookieHeader, Username: username}, nil
	}

	msg := extractResponseMessage(parsed)
	if msg == "" {
		msg = "login failed: no usable session credential, try Cookie/Token import"
	}
	return &LoginResult{Success: false, Message: msg}, nil
}

// --- GetUserInfo ---

func (n *NewApiAdapter) GetUserInfo(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*UserInfo, error) {
	// Try Bearer direct
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, authBearerHeaders(accessToken), proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			if data, ok := getMap(resp, "data"); ok {
				return parseUserInfo(data), nil
			}
		}
	}

	// Cookie fallback
	cookieResp, err := n.fetchUserSelfByCookie(ctx, baseURL, accessToken, platformUserId, proxy)
	if err == nil && cookieResp != nil {
		if data, ok := getMap(cookieResp, "data"); ok {
			return parseUserInfo(data), nil
		}
	}

	// Alternate userID cookie fallback
	altID := n.probeAlternateUserIDByCookie(ctx, baseURL, accessToken, platformUserId, proxy)
	if altID != nil {
		cookieResp2, err := n.fetchUserSelfByCookie(ctx, baseURL, accessToken, altID, proxy)
		if err == nil && cookieResp2 != nil {
			if data, ok := getMap(cookieResp2, "data"); ok {
				return parseUserInfo(data), nil
			}
		}
	}

	return nil, nil
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

// --- VerifyToken ---

func (n *NewApiAdapter) VerifyToken(ctx context.Context, baseURL, token string, platformUserId *int, proxy *ProxyConfig) (*TokenVerifyResult, error) {
	// Try API key path first (/v1/models)
	openAIModels := n.getOpenAIModels(ctx, baseURL, token, proxy)
	if len(openAIModels) > 0 {
		return &TokenVerifyResult{TokenType: "apikey", Models: openAIModels}, nil
	}

	// Try Bearer direct
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, authBearerHeaders(token), proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			if data, ok := getMap(resp, "data"); ok {
				userInfo := parseUserInfo(data)
				balance := parseOneApiStyleBalance(data, 500000, true)
				userID := getIntPtr(data, "id")
				apiToken, _ := n.getAPITokenWithUser(ctx, baseURL, token, userID, proxy)
				apiTokenStr := ""
				if apiToken != nil {
					apiTokenStr = *apiToken
				}
				return &TokenVerifyResult{
					TokenType: "session",
					UserInfo:  userInfo,
					Balance:   &balance,
					APIToken:  apiTokenStr,
				}, nil
			}
		}

		// Some sites return 200 with a message telling us to retry with the
		// New-Api-User header (e.g. "Unauthorized, New-Api-User header not
		// provided"). Others return HTTP 401 with the same body; fetchJSON
		// surfaces that as an error whose text contains "New-Api-User".
		if msg, _ := getString(resp, "message"); strings.Contains(msg, "New-Api-User") {
			if result := n.verifyWithUserIDHeader(ctx, baseURL, token, platformUserId, proxy); result != nil {
				return result, nil
			}
		}
	} else if strings.Contains(err.Error(), "New-Api-User") {
		// 401 + "New-Api-User header not provided": retry with the header.
		if result := n.verifyWithUserIDHeader(ctx, baseURL, token, platformUserId, proxy); result != nil {
			return result, nil
		}
	}

	// Cookie fallback
	cookieResp, err := n.fetchUserSelfByCookie(ctx, baseURL, token, platformUserId, proxy)
	if err == nil && cookieResp != nil {
		if data, ok := getMap(cookieResp, "data"); ok {
			userInfo := parseUserInfo(data)
			balance := parseOneApiStyleBalance(data, 500000, true)
			userID := getIntPtr(data, "id")
			apiToken, _ := n.getAPITokenWithUser(ctx, baseURL, token, userID, proxy)
			apiTokenStr := ""
			if apiToken != nil {
				apiTokenStr = *apiToken
			}
			return &TokenVerifyResult{
				TokenType: "session",
				UserInfo:  userInfo,
				Balance:   &balance,
				APIToken:  apiTokenStr,
			}, nil
		}
	}

	// Alternate userID cookie fallback
	altID := n.probeAlternateUserIDByCookie(ctx, baseURL, token, platformUserId, proxy)
	if altID != nil {
		cookieResp2, err := n.fetchUserSelfByCookie(ctx, baseURL, token, altID, proxy)
		if err == nil && cookieResp2 != nil {
			if data, ok := getMap(cookieResp2, "data"); ok {
				userInfo := parseUserInfo(data)
				balance := parseOneApiStyleBalance(data, 500000, true)
				apiToken, _ := n.getAPITokenWithUser(ctx, baseURL, token, altID, proxy)
				apiTokenStr := ""
				if apiToken != nil {
					apiTokenStr = *apiToken
				}
				return &TokenVerifyResult{
					TokenType: "session",
					UserInfo:  userInfo,
					Balance:   &balance,
					APIToken:  apiTokenStr,
				}, nil
			}
		}
	}

	return &TokenVerifyResult{TokenType: "unknown"}, nil
}

// verifyWithUserIDHeader retries /api/user/self with the New-Api-User header.
// Used when a site reports "New-Api-User header not provided" (as HTTP 200 with
// a message or as HTTP 401). Returns nil when the retry does not yield a
// verified session.
func (n *NewApiAdapter) verifyWithUserIDHeader(ctx context.Context, baseURL, token string, platformUserId *int, proxy *ProxyConfig) *TokenVerifyResult {
	var userID *int
	if platformUserId != nil {
		userID = platformUserId
	} else {
		userID = n.probeUserID(ctx, baseURL, token, proxy)
	}
	if userID == nil {
		return nil
	}
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, n.authHeaders(token, userID), proxy)
	if err != nil {
		return nil
	}
	success, _ := getBool(resp, "success")
	if !success {
		return nil
	}
	data, ok := getMap(resp, "data")
	if !ok {
		return nil
	}
	userInfo := parseUserInfo(data)
	balance := parseOneApiStyleBalance(data, 500000, true)
	apiToken, _ := n.getAPITokenWithUser(ctx, baseURL, token, userID, proxy)
	apiTokenStr := ""
	if apiToken != nil {
		apiTokenStr = *apiToken
	}
	return &TokenVerifyResult{
		TokenType: "session",
		UserInfo:  userInfo,
		Balance:   &balance,
		APIToken:  apiTokenStr,
	}
}

func (n *NewApiAdapter) probeUserID(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) *int {
	if err := ctx.Err(); err != nil {
		return nil
	}
	if jwtID := n.tryDecodeUserID(accessToken); jwtID != nil {
		idCopy := *jwtID
		if n.testUserID(ctx, baseURL, accessToken, idCopy, proxy) {
			return &idCopy
		}
	}

	for _, id := range n.buildUserIDProbeCandidates(accessToken) {
		if err := ctx.Err(); err != nil {
			return nil
		}
		if n.testUserID(ctx, baseURL, accessToken, id, proxy) {
			result := id
			return &result
		}
	}
	return nil
}

func (n *NewApiAdapter) testUserID(ctx context.Context, baseURL, accessToken string, userID int, proxy *ProxyConfig) bool {
	idCopy := userID
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, n.authHeaders(accessToken, &idCopy), proxy)
	if err != nil {
		return false
	}
	success, _ := getBool(resp, "success")
	return success
}

// --- Checkin ---

func (n *NewApiAdapter) Checkin(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*CheckinResult, error) {
	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	var firstFailureMessage string

	// Try Bearer auth
	headers := n.authHeaders(accessToken, resolvedUserID)
	resp, err := fetchJSON(ctx, baseURL+"/api/user/checkin", "POST", nil, headers, proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			return checkinResultFromResponse(resp, "checkin success", "checkin failed"), nil
		}
		firstFailureMessage = extractResponseMessage(resp)
	} else {
		firstFailureMessage = err.Error()
	}

	if firstFailureMessage != "" && !shouldFallbackToCookieCheckin(firstFailureMessage) {
		return &CheckinResult{Success: false, Message: firstFailureMessage}, nil
	}

	// Cookie checkin
	tryCookieCheckin := func(cookieUserID *int) *CheckinResult {
		for _, cookie := range buildCookieCandidates(accessToken) {
			// Try sign_in first
			signInHeaders := map[string]string{
				"Cookie":           cookie,
				"X-Requested-With": "XMLHttpRequest",
			}
			signInResp, _ := fetchJSON(ctx, baseURL+"/api/user/sign_in", "POST", map[string]interface{}{}, signInHeaders, proxy)
			if signInResp != nil {
				if success, _ := getBool(signInResp, "success"); success {
					return checkinResultFromResponse(signInResp, "checked in", "checked in failed")
				}
			}

			// Try cookie-based checkin
			checkinHeaders := map[string]string{"Cookie": cookie}
			for k, v := range n.userIDHeaders(cookieUserID) {
				checkinHeaders[k] = v
			}
			checkinResp, err := fetchJSON(ctx, baseURL+"/api/user/checkin", "POST", nil, checkinHeaders, proxy)
			if err == nil {
				if success, _ := getBool(checkinResp, "success"); success {
					return checkinResultFromResponse(checkinResp, "checkin success", "checkin failed")
				}
				fm := extractResponseMessage(checkinResp)
				if fm != "" && firstFailureMessage == "" {
					firstFailureMessage = fm
				}
			}
		}
		return nil
	}

	if result := tryCookieCheckin(resolvedUserID); result != nil {
		return result, nil
	}

	altCookieUserID := n.probeAlternateUserIDByCookie(ctx, baseURL, accessToken, resolvedUserID, proxy)
	if altCookieUserID != nil {
		if result := tryCookieCheckin(altCookieUserID); result != nil {
			return result, nil
		}
	}

	if isMissingCheckinEndpointMessage(firstFailureMessage) {
		cookieSessionMsg := n.detectCookieSessionFailure(ctx, baseURL, accessToken, []*int{resolvedUserID, altCookieUserID}, proxy)
		if cookieSessionMsg != "" {
			return &CheckinResult{Success: false, Message: cookieSessionMsg}, nil
		}
	}

	if firstFailureMessage == "" {
		firstFailureMessage = "checkin failed"
	}
	return &CheckinResult{Success: false, Message: firstFailureMessage}, nil
}

func (n *NewApiAdapter) detectCookieSessionFailure(ctx context.Context, baseURL, accessToken string, candidateUserIDs []*int, proxy *ProxyConfig) string {
	for _, userID := range candidateUserIDs {
		if userID == nil {
			continue
		}
		resp, err := n.fetchUserSelfByCookie(ctx, baseURL, accessToken, userID, proxy)
		if err != nil || resp == nil {
			continue
		}
		if msg := extractResponseMessage(resp); isCookieSessionFailureMessage(msg) {
			return msg
		}
	}
	return ""
}

func shouldFallbackToCookieCheckin(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "unexpected token") ||
		strings.Contains(lower, "not valid json") ||
		strings.Contains(lower, "<html") ||
		strings.Contains(lower, "new-api-user") ||
		strings.Contains(lower, "access token") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "not login") ||
		strings.Contains(lower, "not logged") ||
		strings.Contains(lower, "invalid url") ||
		(strings.Contains(lower, "http 404") && strings.Contains(lower, "/api/user/checkin")) ||
		strings.Contains(lower, "未登录") ||
		strings.Contains(lower, "未提供")
}

func isMissingCheckinEndpointMessage(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "invalid url (post /api/user/checkin)") ||
		(strings.Contains(lower, "http 404") && strings.Contains(lower, "/api/user/checkin")) ||
		strings.Contains(lower, "checkin endpoint not found") ||
		strings.Contains(lower, "check-in is not supported") ||
		strings.Contains(lower, "checkin is not supported") ||
		strings.Contains(lower, "does not support checkin") ||
		strings.Contains(lower, "not support checkin")
}

func isCookieSessionFailureMessage(msg string) bool {
	// Session/cookie retry heuristic only — never marks accounts.status.
	// Non-auth classes (billing/model/validation/transient) must not look like
	// cookie session failures just because they mention "expired" or "token".
	switch ClassifyUpstreamError(0, msg) {
	case ClassExpired, ClassAuth:
		return true
	case ClassBilling, ClassModel, ClassValidation, ClassTransient:
		return false
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "access token") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "new-api-user") ||
		strings.Contains(lower, "user id") ||
		strings.Contains(lower, "invalid token") ||
		strings.Contains(lower, "无权") ||
		strings.Contains(lower, "未登录") ||
		strings.Contains(lower, "未提供") ||
		strings.Contains(lower, "未授权") ||
		strings.Contains(lower, "not login") ||
		strings.Contains(lower, "not logged")
}

// --- GetBalance ---

func (n *NewApiAdapter) GetBalance(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*BalanceInfo, error) {
	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	var failureMessage string

	// Try Bearer auth
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, n.authHeaders(accessToken, resolvedUserID), proxy)
	if err == nil {
		if success, _ := getBool(resp, "success"); success {
			if data, ok := getMap(resp, "data"); ok {
				b := parseOneApiStyleBalance(data, 500000, true)
				return &b, nil
			}
		}
		msg := extractResponseMessage(resp)
		if msg != "" {
			failureMessage = msg
		}
	} else {
		failureMessage = err.Error()
	}

	// Cookie fallback
	cookieResp, err := n.fetchUserSelfByCookie(ctx, baseURL, accessToken, resolvedUserID, proxy)
	if err == nil && cookieResp != nil {
		if data, ok := getMap(cookieResp, "data"); ok {
			b := parseOneApiStyleBalance(data, 500000, true)
			return &b, nil
		}
	}

	// Alternate userID cookie fallback
	altID := n.probeAlternateUserIDByCookie(ctx, baseURL, accessToken, resolvedUserID, proxy)
	if altID != nil {
		cookieResp2, err := n.fetchUserSelfByCookie(ctx, baseURL, accessToken, altID, proxy)
		if err == nil && cookieResp2 != nil {
			if data, ok := getMap(cookieResp2, "data"); ok {
				b := parseOneApiStyleBalance(data, 500000, true)
				return &b, nil
			}
		}
	}

	if failureMessage == "" {
		failureMessage = "failed to fetch balance"
	}
	return nil, fmt.Errorf("%s", failureMessage)
}

// --- GetModels ---

func (n *NewApiAdapter) GetModels(ctx context.Context, baseURL, token string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	openAIModels := n.getOpenAIModels(ctx, baseURL, token, proxy)
	if len(openAIModels) > 0 {
		return openAIModels, nil
	}

	userID := platformUserId
	if userID == nil {
		userID = n.discoverUserID(ctx, baseURL, token, proxy)
	}

	if userID != nil {
		idCopy := *userID
		headers := n.authHeaders(token, &idCopy)
		resp, err := fetchJSON(ctx, baseURL+"/api/user/models", "GET", nil, headers, proxy)
		if err == nil {
			if data, ok := resp["data"].([]interface{}); ok {
				models := make([]string, 0, len(data))
				for _, item := range data {
					if s, ok := item.(string); ok && s != "" {
						models = append(models, s)
					}
				}
				if len(models) > 0 {
					return normalizeModelIDs(models), nil
				}
			}
			if data, ok := getMap(resp, "data"); ok {
				models := make([]string, 0, len(data))
				for k := range data {
					if k != "" {
						models = append(models, k)
					}
				}
				if len(models) > 0 {
					return normalizeModelIDs(models), nil
				}
			}
		}
	}

	// Cookie model fallback
	cookieModels := n.getSessionModelsByCookie(ctx, baseURL, token, userID, proxy)
	if len(cookieModels) > 0 {
		return cookieModels, nil
	}

	// Alternate userID cookie fallback
	altID := n.probeAlternateUserIDByCookie(ctx, baseURL, token, userID, proxy)
	if altID != nil {
		fallbackModels := n.getSessionModelsByCookie(ctx, baseURL, token, altID, proxy)
		if len(fallbackModels) > 0 {
			return fallbackModels, nil
		}
	}

	return []string{}, nil
}

func (n *NewApiAdapter) getOpenAIModels(ctx context.Context, baseURL, token string, proxy *ProxyConfig) []string {
	// Try /v1/models
	resp, err := fetchJSON(ctx, baseURL+"/v1/models", "GET", nil, authBearerHeaders(token), proxy)
	if err != nil {
		return nil
	}

	return extractModelIDsFromData(resp)
}

func (n *NewApiAdapter) getSessionModelsByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) []string {
	for _, cookie := range buildCookieCandidates(token) {
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(userID) {
			headers[k] = v
		}

		resp, err := fetchJSON(ctx, baseURL+"/api/user/models", "GET", nil, headers, proxy)
		if err != nil {
			continue
		}

		if data, ok := resp["data"].([]interface{}); ok && len(data) > 0 {
			models := make([]string, 0, len(data))
			for _, item := range data {
				if s, ok := item.(string); ok && s != "" {
					models = append(models, s)
				}
			}
			if len(models) > 0 {
				return models
			}
		}

		if data, ok := getMap(resp, "data"); ok {
			models := make([]string, 0, len(data))
			for k := range data {
				if k != "" {
					models = append(models, k)
				}
			}
			if len(models) > 0 {
				return models
			}
		}
	}
	return nil
}

// --- GetUserGroups ---

func (n *NewApiAdapter) GetUserGroups(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	resolvedUserID := platformUserId
	if resolvedUserID == nil {
		resolvedUserID = n.discoverUserID(ctx, baseURL, accessToken, proxy)
	}

	var terminalError string

	// Try /api/user/self/groups
	groups, err := n.tryGetGroupsEndpoint(ctx, baseURL, accessToken, resolvedUserID, "/api/user/self/groups", proxy)
	if err != nil {
		terminalError = err.Error()
	}
	if len(groups) > 0 {
		return dedupeStrings(groups), nil
	}

	// Try /api/user_group_map
	groups, err = n.tryGetGroupsEndpoint(ctx, baseURL, accessToken, resolvedUserID, "/api/user_group_map", proxy)
	if err != nil {
		if terminalError == "" {
			terminalError = err.Error()
		}
	}
	if len(groups) > 0 {
		return dedupeStrings(groups), nil
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

		for _, endpoint := range []string{"/api/user/self/groups", "/api/user_group_map"} {
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

func (n *NewApiAdapter) tryGetGroupsEndpoint(ctx context.Context, baseURL, accessToken string, userID *int, endpoint string, proxy *ProxyConfig) ([]string, error) {
	resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, n.authHeaders(accessToken, userID), proxy)
	if err != nil {
		return nil, err
	}
	if success, _ := getBool(resp, "success"); !success {
		msg := resolveGroupFetchErrorMessage(resp)
		return nil, fmt.Errorf("%s", msg)
	}
	return extractGroupKeys(resp), nil
}

// --- GetSiteAnnouncements ---

func (n *NewApiAdapter) GetSiteAnnouncements(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]SiteAnnouncement, error) {
	resp, err := fetchJSON(ctx, baseURL+"/api/notice", "GET", nil, nil, proxy)
	if err != nil {
		return nil, fmt.Errorf("fetch notice: %w", err)
	}

	success, hasSuccess := getBool(resp, "success")
	if !hasSuccess {
		return nil, fmt.Errorf("fetch notice: invalid response envelope: missing boolean success")
	}
	if !success {
		msg, _ := getString(resp, "message")
		msg = strings.TrimSpace(msg)
		if msg == "" {
			msg = "upstream reported failure"
		}
		return nil, fmt.Errorf("fetch notice: %s", msg)
	}

	rawData, hasData := resp["data"]
	if !hasData || rawData == nil {
		return []SiteAnnouncement{}, nil
	}
	dataStr, ok := rawData.(string)
	if !ok {
		return nil, fmt.Errorf("fetch notice: invalid response envelope: data must be a string")
	}
	content := strings.TrimSpace(dataStr)
	if content == "" {
		return []SiteAnnouncement{}, nil
	}

	return []SiteAnnouncement{{
		SourceKey: fmt.Sprintf("notice:%x", sha1.Sum([]byte(content))),
		Title:     "Site notice",
		Content:   content,
		Level:     "info",
		SourceURL: "/api/notice",
	}}, nil
}

// --- Login fetch ---

// fetchLoginResponse performs a single login POST and returns the parsed JSON
// body (nil for non-JSON responses) plus the accumulated Set-Cookie header.
// Shield-protected sites return an HTML challenge here; parsing fails and the
// caller reports "shield challenge blocked login" without retrying (the
// acw_sc__v2 challenge requires JS execution, which Go cannot provide).
func fetchLoginResponse(ctx context.Context, url string, body map[string]string, headers map[string]string, proxy *ProxyConfig) (map[string]interface{}, string, error) {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, "", fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json"
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := DoWithProxy(ctx, req, proxy)
	if err != nil {
		return nil, "", fmt.Errorf("request: %w", err)
	}
	cookieHeader := mergeSetCookie("", resp.Header["Set-Cookie"])

	respBody, err := readPlatformResponseBody(resp.Body, platformTextResponseBodyLimit)
	resp.Body.Close()
	if err != nil {
		return nil, cookieHeader, fmt.Errorf("read body: %w", err)
	}

	var parsed map[string]interface{}
	if json.Unmarshal(respBody, &parsed) != nil {
		return nil, cookieHeader, nil
	}
	return parsed, cookieHeader, nil
}
