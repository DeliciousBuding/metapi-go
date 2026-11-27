package platform

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// Login: JWT-only, always unsupported.
func (s *Sub2ApiAdapter) Login(ctx context.Context, url, username, password string, platformUserId *int, proxy *ProxyConfig) (*LoginResult, error) {
	return &LoginResult{Success: false, Message: "Sub2API uses JWT authentication; login is not supported"}, nil
}

// GetUserInfo: GET /api/v1/auth/me.
func (s *Sub2ApiAdapter) GetUserInfo(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*UserInfo, error) {
	user, err := s.fetchAuthMe(ctx, normalizeBaseURL(baseURL), accessToken, proxy)
	if err != nil {
		return nil, nil
	}

	displayName := user.username
	if displayName == "" && user.email != "" {
		if idx := strings.Index(user.email, "@"); idx > 0 {
			displayName = user.email[:idx]
		} else {
			displayName = user.email
		}
	}

	result := &UserInfo{
		Username:    displayName,
		DisplayName: displayName,
	}
	if user.email != "" {
		result.Email = user.email
	}
	return result, nil
}

// Checkin: not supported.
func (s *Sub2ApiAdapter) Checkin(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) (*CheckinResult, error) {
	return &CheckinResult{Success: false, Message: "Check-in is not supported by Sub2API"}, nil
}

// GetBalance: USD balance from /api/v1/auth/me, converted to quota, plus subscription summary.
func (s *Sub2ApiAdapter) GetBalance(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*BalanceInfo, error) {
	normalized := normalizeBaseURL(baseURL)

	type result struct {
		user *sub2apiUser
		subs *SubscriptionSummary
		err  error
	}

	userCh := make(chan result, 1)
	subsCh := make(chan result, 1)

	go func() {
		u, err := s.fetchAuthMe(ctx, normalized, accessToken, proxy)
		userCh <- result{user: u, err: err}
	}()
	go func() {
		subs, _ := s.fetchSubscriptionSummary(ctx, normalized, accessToken, proxy)
		subsCh <- result{subs: subs}
	}()

	userRes := <-userCh
	subsRes := <-subsCh

	if userRes.err != nil {
		return nil, fmt.Errorf("sub2api /api/v1/auth/me: %w", userRes.err)
	}

	quotaValue := s.usdToQuota(userRes.user.balance)
	quotaUSD := quotaValue / 500000

	return &BalanceInfo{
		Balance:             quotaUSD,
		Used:                0,
		Quota:               quotaUSD,
		SubscriptionSummary: subsRes.subs,
	}, nil
}

type sub2apiUser struct {
	id       int
	username string
	email    string
	balance  float64
	// group holds the user's subscription group identifier (group_id or
	// group_name) from /api/v1/auth/me, used to filter API keys by group
	// during model enumeration fallback. Empty when the upstream omits it.
	group string
}

func (s *Sub2ApiAdapter) fetchAuthMe(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) (*sub2apiUser, error) {
	endpoint := "/api/v1/auth/me"
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
	if err != nil {
		return nil, err
	}

	rawData, err := s.parseSub2ApiEnvelopeRaw(resp, endpoint)
	if err != nil {
		return nil, err
	}
	data, ok := rawData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Invalid response from %s", endpoint)
	}

	idFloat, ok := data["id"].(float64)
	if !ok || idFloat <= 0 {
		return nil, fmt.Errorf("Invalid user ID in response from %s", endpoint)
	}

	username, _ := getString(data, "username")
	email, _ := getString(data, "email")
	balance, _ := getFloat(data, "balance")
	userGroup := parseSub2ApiUserGroup(data)

	return &sub2apiUser{
		id:       int(idFloat),
		username: username,
		email:    email,
		balance:  balance,
		group:    userGroup,
	}, nil
}

// parseSub2ApiUserGroup extracts the subscription group identifier from an
// /api/v1/auth/me payload. Sub2API exposes the group as either a numeric
// group_id, a string group_name/group, or a nested subscription object.
// Returns "" when the upstream omits the field.
func parseSub2ApiUserGroup(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	if gid, ok := data["group_id"].(float64); ok && gid > 0 {
		return fmt.Sprintf("%d", int(gid))
	}
	if g, ok := getString(data, "group_name"); ok && strings.TrimSpace(g) != "" {
		return strings.TrimSpace(g)
	}
	if g, ok := getString(data, "group"); ok && strings.TrimSpace(g) != "" {
		return strings.TrimSpace(g)
	}
	// Some deployments nest group under subscription/group.
	if sub, ok := getMap(data, "subscription"); ok {
		if gid, ok := sub["group_id"].(float64); ok && gid > 0 {
			return fmt.Sprintf("%d", int(gid))
		}
		if g, ok := getString(sub, "group_name"); ok && strings.TrimSpace(g) != "" {
			return strings.TrimSpace(g)
		}
		if g, ok := getString(sub, "group"); ok && strings.TrimSpace(g) != "" {
			return strings.TrimSpace(g)
		}
	}
	return ""
}

func (s *Sub2ApiAdapter) usdToQuota(balanceUsd float64) float64 {
	return math.Round(math.Max(0, balanceUsd) * 500000)
}
