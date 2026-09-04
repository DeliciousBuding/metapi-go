package platform

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"log/slog"
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

	answer, err := fetchLoginResponse(ctx, baseURL+"/api/user/login", body, headers, proxy)
	if err != nil {
		return &LoginResult{Success: false, Message: err.Error()}, nil
	}
	parsed, cookieHeader := answer.Parsed, answer.Cookie

	if parsed == nil {
		return &LoginResult{Success: false, Message: loginBlockedMessage(answer.Status, answer.ContentType)}, nil
	}

	data, _ := getMap(parsed, "data")
	accessToken := extractLoginToken(parsed, data)
	success, hasSuccess := getBool(parsed, "success")

	// v1 omits top-level success and returns a dashboard session JWT that expires
	// after minutes. Persisting it makes account binding look successful, then
	// model discovery/balance refresh die later; periodically logging in again
	// also leaks active upstream sessions until AUTH_SESSION_LIMIT. When this is
	// the v1 session shape, exchange the fresh JWT for New API's durable dashboard
	// personal access token and immediately revoke the transient login session.
	// Legacy New API responses have no session metadata and keep their original
	// token unchanged.
	if accessToken != "" && (!hasSuccess || success) {
		if isSession, sessionID := newAPIV1LoginSession(data); isSession {
			durableToken, promoteErr := n.promoteV1LoginCredential(
				ctx, baseURL, accessToken, cookieHeader, sessionID, platformUserId, proxy)
			if promoteErr != nil {
				return &LoginResult{Success: false, Message: promoteErr.Error()}, nil
			}
			accessToken = durableToken
		}
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

// newAPIV1LoginSession distinguishes New API v1's short-lived dashboard login
// JWT from legacy durable dashboard tokens. v1 includes session metadata and an
// access expiry; either marker is enough to require promotion. The session ID is
// only needed for best-effort logout.
func newAPIV1LoginSession(data map[string]interface{}) (bool, string) {
	if data == nil {
		return false, ""
	}
	_, hasExpiry := data["access_expires_at"]
	session, hasSession := getMap(data, "session")
	if !hasExpiry && !hasSession {
		return false, ""
	}
	sessionID, _ := getString(session, "sid")
	return true, strings.TrimSpace(sessionID)
}

// promoteV1LoginCredential turns a short-lived New API v1 session JWT into the
// durable dashboard PAT that Metapi can safely persist. There is deliberately
// no refresh-token subsystem here: New API already owns the durable credential,
// and using it removes a lifecycle rather than adding one.
//
// New API's GET /api/user/token rotates the user's one dashboard PAT. This path
// therefore runs only when Login itself is requested. In steady state the PAT
// does not expire, so check-in/balance auto-relogin never reaches this method;
// an explicit re-login or recovery after a revoked PAT intentionally rotates it.
func (n *NewApiAdapter) promoteV1LoginCredential(
	ctx context.Context,
	baseURL, sessionJWT, cookieHeader, sessionID string,
	platformUserID *int,
	proxy *ProxyConfig,
) (string, error) {
	resp, err := fetchJSON(
		ctx,
		strings.TrimRight(baseURL, "/")+"/api/user/token",
		http.MethodGet,
		nil,
		n.authHeaders(sessionJWT, platformUserID),
		proxy,
	)
	if err != nil {
		return "", fmt.Errorf("login succeeded, but New API could not issue a durable dashboard token: %w", err)
	}
	durableToken, _ := getString(resp, "data")
	durableToken = strings.TrimSpace(durableToken)
	if durableToken == "" {
		return "", fmt.Errorf("login succeeded, but New API returned no durable dashboard token")
	}

	// Minting the PAT required a browser login session. Revoke that transient
	// session immediately so automated relogin cannot consume New API's active
	// session quota. PAT issuance already succeeded, so logout is best-effort:
	// failing the whole bind here would discard a usable durable credential while
	// still leaving the upstream session behind.
	if strings.TrimSpace(sessionID) != "" {
		headers := map[string]string{
			"Authorization":  "Bearer " + sessionJWT,
			"X-Auth-Session": sessionID,
		}
		if strings.TrimSpace(cookieHeader) != "" {
			headers["Cookie"] = cookieHeader
		}
		logoutResp, logoutErr := fetchJSON(
			ctx,
			strings.TrimRight(baseURL, "/")+"/api/user/auth/logout",
			http.MethodPost,
			nil,
			headers,
			proxy,
		)
		logoutOK := false
		if logoutErr == nil {
			logoutOK, _ = getBool(logoutResp, "success")
		}
		if logoutErr != nil || !logoutOK {
			slog.Warn("new-api login: durable token issued but transient session logout failed",
				"session_id", sessionID,
				"error", logoutErr)
		}
	} else {
		slog.Warn("new-api login: durable token issued but upstream returned no session id; transient session could not be revoked")
	}

	return durableToken, nil
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
	// Try API key path first (/v1/models). nil ladder: here a rung that produces
	// nothing is a signal to try the next credential shape, not a failure to report.
	openAIModels := n.getOpenAIModels(ctx, baseURL, token, proxy, nil)
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
				apiToken, apiTokenErr := n.getAPITokenWithUser(ctx, baseURL, token, userID, proxy)
				if apiTokenErr != nil {
					return nil, apiTokenErr
				}
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
			result, verifyErr := n.verifyWithUserIDHeader(ctx, baseURL, token, platformUserId, proxy)
			if verifyErr != nil {
				return nil, verifyErr
			}
			if result != nil {
				return result, nil
			}
		}
	} else if strings.Contains(err.Error(), "New-Api-User") {
		// 401 + "New-Api-User header not provided": retry with the header.
		result, verifyErr := n.verifyWithUserIDHeader(ctx, baseURL, token, platformUserId, proxy)
		if verifyErr != nil {
			return nil, verifyErr
		}
		if result != nil {
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
			apiToken, apiTokenErr := n.getAPITokenWithUser(ctx, baseURL, token, userID, proxy)
			if apiTokenErr != nil {
				return nil, apiTokenErr
			}
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
				apiToken, apiTokenErr := n.getAPITokenWithUser(ctx, baseURL, token, altID, proxy)
				if apiTokenErr != nil {
					return nil, apiTokenErr
				}
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
func (n *NewApiAdapter) verifyWithUserIDHeader(ctx context.Context, baseURL, token string, platformUserId *int, proxy *ProxyConfig) (*TokenVerifyResult, error) {
	var userID *int
	if platformUserId != nil {
		userID = platformUserId
	} else {
		userID = n.probeUserID(ctx, baseURL, token, proxy)
	}
	if userID == nil {
		return nil, nil
	}
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, n.authHeaders(token, userID), proxy)
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
	userInfo := parseUserInfo(data)
	balance := parseOneApiStyleBalance(data, 500000, true)
	apiToken, apiTokenErr := n.getAPITokenWithUser(ctx, baseURL, token, userID, proxy)
	if apiTokenErr != nil {
		return nil, apiTokenErr
	}
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

	if userID != nil {
		idCopy := *userID
		headers := n.authHeaders(token, &idCopy)
		resp, err := fetchJSON(ctx, baseURL+"/api/user/models", "GET", nil, headers, proxy)
		if err != nil {
			lad.fail(err.Error())
		} else {
			lad.answer()
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
		lad.fail(err.Error())
		return nil
	}
	lad.answer()

	return extractModelIDsFromData(resp)
}

func (n *NewApiAdapter) getSessionModelsByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig, lad *modelFetchLadder) []string {
	for _, cookie := range buildCookieCandidates(token) {
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(userID) {
			headers[k] = v
		}

		resp, err := fetchJSON(ctx, baseURL+"/api/user/models", "GET", nil, headers, proxy)
		if err != nil {
			lad.fail(err.Error())
			continue
		}
		lad.answer()

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

// loginResponse is what an upstream login endpoint actually answered. The status
// and content type travel with the parsed body because "could not parse it" is
// not a diagnosis: a rate limit, a WAF challenge, an error page and a proxy in
// front of the site all arrive here looking the same, and only the status line
// tells them apart.
type loginResponse struct {
	Parsed      map[string]interface{} // nil when the body was not a JSON object
	Cookie      string                 // accumulated Set-Cookie header
	Status      int
	ContentType string
}

// fetchLoginResponse performs a single login POST and returns what arrived.
// Shield-protected sites return an HTML challenge here; parsing fails and the
// caller reports the observed status instead of retrying (the acw_sc__v2
// challenge requires JS execution, which Go cannot provide).
func fetchLoginResponse(ctx context.Context, url string, body map[string]string, headers map[string]string, proxy *ProxyConfig) (loginResponse, error) {
	reqBody, err := json.Marshal(body)
	if err != nil {
		return loginResponse{}, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(reqBody)))
	if err != nil {
		return loginResponse{}, fmt.Errorf("create request: %w", err)
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json"
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := DoWithProxy(ctx, req, proxy)
	if err != nil {
		return loginResponse{}, fmt.Errorf("request: %w", err)
	}
	out := loginResponse{
		Cookie:      mergeSetCookie("", resp.Header["Set-Cookie"]),
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
	}

	respBody, err := readPlatformResponseBody(resp.Body, platformTextResponseBodyLimit)
	resp.Body.Close()
	if err != nil {
		return out, fmt.Errorf("read body: %w", err)
	}

	if json.Unmarshal(respBody, &out.Parsed) != nil {
		out.Parsed = nil
	}
	return out, nil
}

// loginBlockedMessage renders the failure an operator actually has to act on when
// a login answer was not a JSON object. This used to be one sentence for every
// case — "shield challenge blocked login" — so a site answering HTTP 429 was
// reported as a WAF challenge, sending the operator hunting for anti-bot
// protection that did not exist. Observed live against a real new-api: its access
// log shows 429 on /api/user/login at exactly the moments Metapi reported a
// shield challenge, and the same request replayed two minutes later returned
// JSON 200. Re-binding an account a few times in a row is enough to trip a
// site's own login limiter, and re-binding is the recovery path the docs tell
// users to take when a credential ages out.
//
// Only the status and the content type are reported. The body is deliberately
// not echoed: a challenge or error page can carry markup, tokens or internal
// URLs, and none of that belongs in a message a downstream client can read.
func loginBlockedMessage(status int, contentType string) string {
	if status == http.StatusTooManyRequests {
		return "upstream rate-limited the login (HTTP 429): wait a minute and retry — repeated re-binds trip the site's own login limiter"
	}
	kind := strings.TrimSpace(contentType)
	if kind == "" {
		kind = "no content type"
	}
	return fmt.Sprintf("login blocked: upstream answered HTTP %d with %s instead of JSON — a WAF/shield challenge (which needs a real browser), an error or rate-limit page, or a proxy in front of the site", status, kind)
}
