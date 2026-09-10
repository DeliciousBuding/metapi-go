package platform

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
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

const newAPIPATVerificationScope = "access_token.generate"
const newAPIManualPATImport = "complete verification in New API and import a durable dashboard PAT manually"

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
	if required, _ := getBool(data, "require_verification"); required {
		return &LoginResult{Success: false, Message: "New API login requires MFA/interactive verification; " + newAPIManualPATImport}, nil
	}
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
				ctx, baseURL, accessToken, password, cookieHeader, sessionID, platformUserId, proxy)
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
	if newAPIV1InvalidParametersMessage(msg) {
		if encrypted, probeErr := newAPIV1PasswordEncryptionRequired(ctx, baseURL, proxy); probeErr == nil && encrypted {
			return &LoginResult{Success: false, Message: "New API login requires encrypted-password verification; " + newAPIManualPATImport}, nil
		}
	}
	if msg == "" {
		msg = "login failed: no usable session credential, try Cookie/Token import"
	}
	return &LoginResult{Success: false, Message: msg}, nil
}

// newAPIV1PasswordEncryptionRequired distinguishes New API's browser-only
// encrypted-password login from a plaintext-capable legacy server. It is only
// consulted after an "invalid parameters" login response, so healthy logins do
// not pay an extra request. When the upstream advertises encryption, Login
// reports the unsupported variant and directs the operator to the durable PAT
// path instead of presenting a generic parameter error. A missing endpoint or
// malformed policy keeps the original failure message.
func newAPIV1PasswordEncryptionRequired(ctx context.Context, baseURL string, proxy *ProxyConfig) (bool, error) {
	headers := map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"User-Agent":       DefaultBrowserUserAgent,
	}
	answer, err := fetchLoginSessionResponse(ctx, baseURL+"/api/user/login/encryption-key", http.MethodGet, nil, headers, proxy)
	if err != nil {
		return false, err
	}
	if answer.Status == http.StatusNotFound || answer.Status == http.StatusMethodNotAllowed || answer.Parsed == nil {
		return false, nil
	}
	if answer.Status < 200 || answer.Status >= 300 {
		return false, nil
	}
	success, _ := getBool(answer.Parsed, "success")
	if !success {
		return false, nil
	}
	data, _ := getMap(answer.Parsed, "data")
	enabled, ok := getBool(data, "enabled")
	if !ok {
		return false, nil
	}
	return enabled, nil
}

func newAPIV1InvalidParametersMessage(message string) bool {
	switch strings.ToLower(strings.TrimSpace(message)) {
	case "invalid parameters", "invalid params", "参数错误":
		return true
	default:
		return false
	}
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
	baseURL, sessionJWT, password, cookieHeader, sessionID string,
	platformUserID *int,
	proxy *ProxyConfig,
) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	sessionID = strings.TrimSpace(sessionID)
	headers := n.authHeaders(sessionJWT, platformUserID)
	if sessionID != "" {
		headers["X-Auth-Session"] = sessionID
		// Refresh cookies are scoped to /api/user/auth, not /api/verify or
		// /api/user/token. Keep them for logout; the JWT authenticates the
		// proof ceremony. Revoke failed promotions as well, so rejected
		// step-up attempts do not exhaust the upstream login-session quota.
		logoutHeaders := n.authHeaders(sessionJWT, platformUserID)
		logoutHeaders["X-Auth-Session"] = sessionID
		if strings.TrimSpace(cookieHeader) != "" {
			logoutHeaders["Cookie"] = cookieHeader
		}
		if origin, err := url.Parse(baseURL); err == nil {
			logoutHeaders["Origin"] = origin.Scheme + "://" + origin.Host
		}
		defer func() {
			_, err := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/auth/logout", http.MethodPost, nil, logoutHeaders, proxy)
			if err != nil {
				// Only the fixed diagnostic/status is safe to log. An
				// upstream error body can echo passwords, proofs or tokens.
				slog.Warn("new-api login: transient session logout failed", "error", err)
			}
		}()
	} else {
		slog.Warn("new-api login: upstream returned no session id; transient session could not be revoked")
	}

	answer, err := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/token", http.MethodGet, nil, headers, proxy)
	var proof string
	if err != nil {
		code, _ := getString(answer.Parsed, "code")
		success, hasSuccess := getBool(answer.Parsed, "success")
		// A translated message or an arbitrary 403 is not a step-up demand.
		// Only the upstream's explicit machine contract permits re-auth.
		if answer.Status != http.StatusForbidden || code != "SECURITY_PROOF_REQUIRED" || !hasSuccess || success {
			return "", fmt.Errorf("login succeeded, but New API could not issue a durable dashboard token: %w; %s", err, newAPIManualPATImport)
		}
		proof, err = newAPIV1PasswordProof(ctx, baseURL, password, headers, proxy)
		if err != nil {
			return "", fmt.Errorf("login succeeded, but New API security verification failed: %w; %s", err, newAPIManualPATImport)
		}
		// The proof is single-use and bound to this session and operation.
		// Retry PAT issuance exactly once, never verify/replay in a loop.
		headers["X-Security-Proof"] = proof
		answer, err = fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/token", http.MethodGet, nil, headers, proxy)
		delete(headers, "X-Security-Proof")
		if err != nil {
			return "", fmt.Errorf("login succeeded, but New API could not issue a durable dashboard token after security verification: %w; %s", err, newAPIManualPATImport)
		}
	}
	durableToken, _ := getString(answer.Parsed, "data")
	durableToken = strings.TrimSpace(durableToken)
	if durableToken == "" || durableToken == sessionJWT || durableToken == proof {
		return "", fmt.Errorf("login succeeded, but New API returned no durable dashboard PAT; %s", newAPIManualPATImport)
	}
	// Logout remains best-effort: never discard a PAT already issued because
	// session revocation failed. The deferred cleanup uses the session JWT,
	// never the PAT or the consumed security proof.
	return durableToken, nil
}

// newAPIV1PasswordProof implements New API's advertised password ceremony for
// access_token.generate. MFA and encrypted-password flows are not implemented
// here; require manual PAT import rather than downgrading to plaintext.
func newAPIV1PasswordProof(ctx context.Context, baseURL, password string, headers map[string]string, proxy *ProxyConfig) (string, error) {
	answer, err := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/verify/methods?scope="+newAPIPATVerificationScope, http.MethodGet, nil, headers, proxy)
	if err != nil {
		return "", fmt.Errorf("could not obtain password verification methods: %w", err)
	}
	data, _ := getMap(answer.Parsed, "data")
	scope, _ := getString(data, "scope")
	if scope != newAPIPATVerificationScope {
		return "", fmt.Errorf("upstream returned unsupported verification requirements")
	}
	methods, _ := data["methods"].([]interface{})
	passwordAllowed := false
	for _, raw := range methods {
		option, _ := raw.(map[string]interface{})
		method, _ := getString(option, "method")
		available, _ := getBool(option, "available")
		if method == "password" && available {
			passwordAllowed = true
		}
	}
	if !passwordAllowed {
		return "", fmt.Errorf("upstream requires MFA/interactive verification or password verification is unavailable")
	}
	encrypted, hasEncryptionPolicy := getBool(data, "password_encryption_enabled")
	if !hasEncryptionPolicy || encrypted {
		return "", fmt.Errorf("upstream password encryption requirements are not supported by this login flow")
	}
	if password == "" {
		return "", fmt.Errorf("no password was supplied for verification")
	}
	body := map[string]string{"method": "password", "scope": newAPIPATVerificationScope, "password": password}
	answer, err = fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/verify", http.MethodPost, body, headers, proxy)
	if err != nil {
		return "", fmt.Errorf("password verification was rejected: %w", err)
	}
	data, _ = getMap(answer.Parsed, "data")
	proof, _ := getString(data, "proof_token")
	method, _ := getString(data, "method")
	scope, _ = getString(data, "scope")
	expires, _ := getFloat(data, "expires_at")
	if strings.TrimSpace(proof) == "" || method != "password" || scope != newAPIPATVerificationScope || expires <= float64(time.Now().Unix()) {
		return "", fmt.Errorf("upstream returned no valid password verification proof")
	}
	return proof, nil
}

// fetchNewAPIV1SessionJSON retains the status and structured error code for
// step-up detection, but never exposes upstream bodies or transport errors
// (which may contain credentials/URLs) to a login result or a log entry.
func fetchNewAPIV1SessionJSON(ctx context.Context, url, method string, body map[string]string, headers map[string]string, proxy *ProxyConfig) (loginResponse, error) {
	answer, err := fetchLoginSessionResponse(ctx, url, method, body, headers, proxy)
	if err != nil {
		return answer, fmt.Errorf("request failed")
	}
	if answer.Status < 200 || answer.Status >= 300 {
		return answer, fmt.Errorf("HTTP %d", answer.Status)
	}
	if answer.Parsed == nil {
		return answer, fmt.Errorf("invalid JSON response (HTTP %d)", answer.Status)
	}
	if success, _ := getBool(answer.Parsed, "success"); !success {
		return answer, fmt.Errorf("unsuccessful response (HTTP %d)", answer.Status)
	}
	return answer, nil
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
			return newAPICheckinResultFromResponse(resp, "checkin success", "checkin failed"), nil
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
					return newAPICheckinResultFromResponse(signInResp, "checked in", "checked in failed")
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
					return newAPICheckinResultFromResponse(checkinResp, "checkin success", "checkin failed")
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

	if IsCheckinEndpointAbsent(firstFailureMessage) {
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

// newAPICheckinResultFromResponse normalizes the native New API award into
// the same monetary units as GetBalance. quota_awarded is the integer quota
// actually credited by this check-in, not the account's remaining quota.
func newAPICheckinResultFromResponse(resp map[string]interface{}, successMsg, failureMsg string) *CheckinResult {
	result := checkinResultFromResponse(resp, successMsg, failureMsg)
	if result.Success {
		if data, ok := getMap(resp, "data"); ok {
			if quota, ok := getFloat(data, "quota_awarded"); ok && quota >= 0 && !math.IsInf(quota, 0) && math.Trunc(quota) == quota {
				// Keep explicit zero distinct from an absent reward. Decimal
				// formatting also prevents 1 quota from becoming "2e-06",
				// which a decorated-reward text parser could read as 2.
				result.Reward = strconv.FormatFloat(quota/500000, 'f', -1, 64)
			}
		}
	}
	return result
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

	// New API's /api/user/models route is UserAuth, which authenticates the
	// Authorization bearer directly and does not require New-Api-User. Try it
	// even when user-ID discovery did not answer; older forks that do require
	// the header still get it when platformUserId/discovery supplied one.
	userModels := n.getUserModels(ctx, baseURL, token, userID, proxy, lad)
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

func (n *NewApiAdapter) getUserModels(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig, lad *modelFetchLadder) []string {
	resp, err := fetchJSON(ctx, baseURL+"/api/user/models", "GET", nil, n.authHeaders(token, userID), proxy)
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
		headers := map[string]string{"Cookie": cookie}
		for k, v := range n.userIDHeaders(userID) {
			headers[k] = v
		}

		resp, err := fetchJSON(ctx, baseURL+"/api/user/models", "GET", nil, headers, proxy)
		if err != nil {
			lad.fail(safeModelDiscoveryReason(err.Error()))
			continue
		}

		models, answered, reason := parseUserModelsResponse(resp)
		if !answered {
			lad.fail(reason)
			continue
		}
		lad.answer()
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

	// SourceURL stays empty on purpose. The notice is scraped from the machine
	// endpoint /api/notice, which serves raw JSON rather than a page a person
	// can read; emitting it as the source made the web UI's "open upstream"
	// link land on a JSON payload (#1297). An empty source URL resolves to the
	// trusted site home, matching the other adapters (e.g. sub2api).
	return []SiteAnnouncement{{
		SourceKey: fmt.Sprintf("notice:%x", sha1.Sum([]byte(content))),
		Title:     "Site notice",
		Content:   content,
		Level:     "info",
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
	return fetchLoginSessionResponse(ctx, url, http.MethodPost, body, headers, proxy)
}

// fetchLoginSessionResponse shares the login transport with its authenticated
// verification/PAT/logout requests without discarding non-2xx JSON envelopes.
func fetchLoginSessionResponse(ctx context.Context, url, method string, body map[string]string, headers map[string]string, proxy *ProxyConfig) (loginResponse, error) {
	var bodyReader io.Reader
	if body != nil {
		reqBody, err := json.Marshal(body)
		if err != nil {
			return loginResponse{}, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = strings.NewReader(string(reqBody))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return loginResponse{}, fmt.Errorf("create request: %w", err)
	}
	if headers == nil {
		headers = make(map[string]string)
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json"
	}
	if _, ok := headers["User-Agent"]; !ok {
		headers["User-Agent"] = DefaultBrowserUserAgent
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	ApplySiteIdentity(req, proxy)

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
