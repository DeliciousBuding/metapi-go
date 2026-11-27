package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (n *NewApiAdapter) Login(ctx context.Context, baseURL, username, password string, platformUserId *int, proxy *ProxyConfig) (*LoginResult, error) {
	return n.LoginWithExistingCredential(ctx, baseURL, username, password, platformUserId, proxy, "")
}

// LoginWithExistingCredential accepts the stored PAT only as a reuse candidate.
// The fresh browser login must succeed first; the upstream token listing then
// proves ownership, validity and sufficient grant before the PAT is reused.
func (n *NewApiAdapter) LoginWithExistingCredential(ctx context.Context, baseURL, username, password string, platformUserId *int, proxy *ProxyConfig, existing string) (*LoginResult, error) {
	answer, err := fetchLoginResponse(ctx, baseURL, username, password, proxy)
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
				ctx, baseURL, accessToken, password, cookieHeader, sessionID, existing, platformUserId, proxy)
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
func fetchLoginResponse(ctx context.Context, baseURL, username, password string, proxy *ProxyConfig) (loginResponse, error) {
	body := map[string]string{"username": username, "password": password}
	headers := map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"User-Agent":       DefaultBrowserUserAgent,
	}
	return fetchLoginSessionResponse(ctx, baseURL+"/api/user/login", http.MethodPost, body, headers, proxy)
}

// fetchLoginSessionResponse shares the login transport with its authenticated
// verification/PAT/logout requests without discarding non-2xx JSON envelopes.
func fetchLoginSessionResponse(ctx context.Context, url, method string, body interface{}, headers map[string]string, proxy *ProxyConfig) (loginResponse, error) {
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
