package platform

import (
	"context"
	"fmt"
	"strings"
)

// Login POSTs /api/user/login and accepts either a legacy data.access_token
// or a captured session cookie. one-api v0.6.10 returns success:true with an
// EMPTY data.access_token and authenticates via a Set-Cookie session cookie,
// so the cookie header becomes the stored credential. Mirrors
// NewApiAdapter.Login (same fetchLoginResponse + hasUsableSessionCookie flow).
func (o *OneApiAdapter) Login(ctx context.Context, baseURL, username, password string, platformUserId *int, proxy *ProxyConfig) (*LoginResult, error) {
	answer, err := fetchLoginResponse(ctx, baseURL, username, password, proxy)
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
//
// A failed fetch is an error, mirroring NewApiAdapter.GetBalance below in this
// package: service/balance stores whatever struct comes back as the account's
// balance and drives runtime health, the token-expired alert and the auto-relogin
// retry from err != nil. Returning a zero BalanceInfo with a nil error therefore
// wrote balance=0 over the real one and skipped all three. #1244 fixed the same
// shape in veloera and done-hub; one-api and the one-hub adapter that embeds it
// were the last two holding it.
func (o *OneApiAdapter) GetBalance(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*BalanceInfo, error) {
	headers := authBearerHeaders(accessToken)
	var failureMessage string

	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, headers, proxy)
	if err != nil {
		failureMessage = err.Error()
	} else {
		if success, _ := getBool(resp, "success"); success {
			if data, ok := getMap(resp, "data"); ok {
				balance := parseOneApiStyleBalance(data, 500000, false)
				return &balance, nil
			}
		}
		failureMessage = extractResponseMessage(resp)
	}

	if isCookieCredential(accessToken) {
		cookieResp, cookieErr := o.fetchSelfByCookie(ctx, baseURL, accessToken, proxy)
		if cookieErr == nil && cookieResp != nil {
			if data, ok := getMap(cookieResp, "data"); ok {
				balance := parseOneApiStyleBalance(data, 500000, false)
				return &balance, nil
			}
		}
		if cookieErr != nil && failureMessage == "" {
			failureMessage = cookieErr.Error()
		}
	}

	if failureMessage == "" {
		failureMessage = "failed to fetch balance"
	}
	return nil, fmt.Errorf("%s", failureMessage)
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
