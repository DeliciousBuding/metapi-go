package platform

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

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
			checkinHeaders := n.cookieHeaders(cookie, cookieUserID)
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

	if data, _ := n.userSelfDataByCookie(ctx, baseURL, accessToken, resolvedUserID, proxy); data != nil {
		b := parseOneApiStyleBalance(data, 500000, true)
		return &b, nil
	}

	if failureMessage == "" {
		failureMessage = "failed to fetch balance"
	}
	return nil, fmt.Errorf("%s", failureMessage)
}
