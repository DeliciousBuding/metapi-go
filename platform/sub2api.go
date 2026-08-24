package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Sub2ApiAdapter handles Sub2API platforms (JWT auth, {code, message, data} envelope).
// Directly extends BaseAdapter. Does NOT support login or checkin.
type Sub2ApiAdapter struct {
	*BaseAdapter
}

// Detect uses a multi-path approach: URL keyword, /api/v1/auth/me probe, /v1/models probe, root title.
func (s *Sub2ApiAdapter) Detect(ctx context.Context, url string) (bool, error) {
	lower := strings.ToLower(url)
	if strings.Contains(lower, "sub2api") {
		return true, nil
	}

	base := normalizeBaseURL(url)
	if base == "" {
		return false, nil
	}

	// Probe /api/v1/auth/me
	if s.matchSub2ApiErrorEnvelope(ctx, base+"/api/v1/auth/me") {
		return true, nil
	}

	// Probe /v1/models
	if s.matchSub2ApiErrorEnvelope(ctx, base+"/v1/models") {
		return true, nil
	}

	// Fallback: check root HTML title (bounded so half-open hosts fail fast).
	titleCtx, cancel := withProbeTimeout(ctx)
	defer cancel()
	body, ct, err := fetchText(titleCtx, base+"/", nil)
	if err != nil {
		return false, nil
	}
	if !strings.Contains(strings.ToLower(ct), "text/html") {
		return false, nil
	}

	return regexp.MustCompile(`(?i)<title>\s*sub2api\b`).MatchString(body), nil
}

func (s *Sub2ApiAdapter) matchSub2ApiErrorEnvelope(ctx context.Context, url string) bool {
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx2, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := DoWithProxy(ctx2, req, nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.Contains(ct, "application/json") {
		return false
	}

	respBody, err := readPlatformResponseBody(resp.Body, platformJSONResponseBodyLimit)
	if err != nil {
		return false
	}

	var body map[string]interface{}
	if err := json.Unmarshal(respBody, &body); err != nil {
		return false
	}

	code := body["code"]
	switch c := code.(type) {
	case string:
		upper := strings.ToUpper(strings.TrimSpace(c))
		if upper == "UNAUTHORIZED" || upper == "API_KEY_REQUIRED" {
			return true
		}
	case float64:
		if c == 0 {
			_, hasData := body["data"]
			return hasData
		}
	}

	msg, _ := getString(body, "message")
	msgLower := strings.ToLower(msg)
	if strings.Contains(msgLower, "authorization header is required") ||
		strings.Contains(msgLower, "api key is required") {
		return true
	}

	return false
}

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

// GetModels: standard OpenAI-compat endpoints, with API key discovery fallback.
func (s *Sub2ApiAdapter) GetModels(ctx context.Context, baseURL, apiToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	normalized := normalizeBaseURL(baseURL)
	managementBase := s.resolveManagementBaseURL(normalized)

	// Surface a terminal fetch error so callers can classify it (unauthorized
	// vs timeout); otherwise return an explicit empty list for "no models".
	finish := func(err error) ([]string, error) {
		if err != nil {
			return nil, err
		}
		return []string{}, nil
	}

	directModels, directErr := s.fetchModelsByToken(ctx, normalized, apiToken, proxy)
	if len(directModels) > 0 {
		return directModels, nil
	}

	// Session JWT cannot access /v1/models directly; discover a user key first.
	// Filter keys by the user's subscription group (from /api/v1/auth/me) so
	// we pick a key from the SAME group as the user's subscription rather than
	// blindly taking the first enabled key, which may belong to a different
	// group and surface the wrong model set (#675).
	userGroup := ""
	if user, err := s.fetchAuthMe(ctx, normalized, apiToken, proxy); err == nil {
		userGroup = user.group
	}
	discoveredToken, _ := s.getAPITokenForGroup(ctx, managementBase, apiToken, userGroup, platformUserId, proxy)
	if discoveredToken == nil {
		return finish(directErr)
	}
	if normalizeTokenKeyForCompare(*discoveredToken) == normalizeTokenKeyForCompare(apiToken) {
		return finish(directErr)
	}

	fallbackModels, fallbackErr := s.fetchModelsByToken(ctx, normalized, *discoveredToken, proxy)
	if len(fallbackModels) > 0 {
		return fallbackModels, nil
	}
	if fallbackErr != nil {
		return finish(fallbackErr)
	}
	return finish(directErr)
}

// GetAPITokens: GET /api/v1/keys?page=1&page_size=100 + /api/v1/api-keys?page=1&page_size=100.
func (s *Sub2ApiAdapter) GetAPITokens(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	items, err := s.listAPIKeys(ctx, normalizeBaseURL(baseURL), accessToken, proxy)
	if err != nil {
		return []ApiTokenInfo{}, nil
	}

	result := make([]ApiTokenInfo, 0, len(items))
	for _, item := range items {
		info := ApiTokenInfo{
			Name:    item.name,
			Key:     item.key,
			Enabled: item.enabled,
		}
		if item.tokenGroup != "" {
			info.TokenGroup = item.tokenGroup
		}
		result = append(result, info)
	}
	return result, nil
}

// GetAPIToken: returns first enabled token.
func (s *Sub2ApiAdapter) GetAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := s.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		return nil, nil
	}
	for _, t := range tokens {
		if t.Enabled {
			k := t.Key
			return &k, nil
		}
	}
	if len(tokens) > 0 {
		k := tokens[0].Key
		return &k, nil
	}
	return nil, nil
}

// getAPITokenForGroup returns an API key for the user's subscription group.
// When group is non-empty, only keys whose TokenGroup matches are considered;
// if no key matches the group, it falls back to first-enabled so a missing
// group tag never blocks model enumeration entirely (#675).
func (s *Sub2ApiAdapter) getAPITokenForGroup(ctx context.Context, baseURL, accessToken, group string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := s.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		return nil, nil
	}
	return pickKeyForGroup(tokens, group), nil
}

// pickKeyForGroup selects the first enabled key whose TokenGroup matches the
// user's group. Falls back to first-enabled when the group is empty, no keys
// match, or all group-matched keys are disabled. Returns nil when there are
// no keys at all.
func pickKeyForGroup(tokens []ApiTokenInfo, group string) *string {
	if len(tokens) == 0 {
		return nil
	}
	if strings.TrimSpace(group) != "" {
		for _, t := range tokens {
			if !t.Enabled {
				continue
			}
			if strings.TrimSpace(t.TokenGroup) == strings.TrimSpace(group) {
				k := t.Key
				return &k
			}
		}
	}
	for _, t := range tokens {
		if t.Enabled {
			k := t.Key
			return &k
		}
	}
	k := tokens[0].Key
	return &k
}

// GetUserGroups: 5 endpoint fallback + key-based inference.
func (s *Sub2ApiAdapter) GetUserGroups(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	normalized := normalizeBaseURL(baseURL)

	directGroups := s.listGroups(ctx, normalized, accessToken, proxy)
	if len(directGroups) > 0 {
		return directGroups, nil
	}

	inferredFromKeys := s.inferGroupsFromKeys(ctx, normalized, accessToken, proxy)
	if len(inferredFromKeys) > 0 {
		return inferredFromKeys, nil
	}

	return []string{"default"}, nil
}

// CreateAPIToken: POST /api/v1/keys + POST /api/v1/api-keys.
func (s *Sub2ApiAdapter) CreateAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, options *CreateAPITokenOptions, proxy *ProxyConfig) (bool, error) {
	normalized := normalizeBaseURL(baseURL)

	payload := map[string]interface{}{}
	if options != nil && strings.TrimSpace(options.Name) != "" {
		payload["name"] = strings.TrimSpace(options.Name)
	} else {
		payload["name"] = "metapi"
	}

	if options != nil {
		if groupID, err := parseGroupID(options.Group); err == nil && groupID > 0 {
			payload["group_id"] = groupID
		}
		if expiresInDays := s.resolveExpiresInDays(options.ExpiredTime); expiresInDays > 0 {
			payload["expires_in_days"] = expiresInDays
		}
		if !options.UnlimitedQuota && options.RemainQuota > 0 {
			payload["quota"] = math.Max(0, options.RemainQuota)
		}
	}

	headers := authBearerHeaders(accessToken)
	endpoints := []string{"/api/v1/keys", "/api/v1/api-keys"}
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, normalized+endpoint, "POST", payload, headers, proxy)
		if err != nil {
			continue
		}
		if err := s.parseSub2ApiEnvelope(resp, endpoint); err == nil {
			return true, nil
		}
	}

	return false, nil
}

// DeleteAPIToken: list -> find key -> DELETE /api/v1/keys/{id} + /api/v1/api-keys/{id}.
func (s *Sub2ApiAdapter) DeleteAPIToken(ctx context.Context, baseURL, accessToken, tokenKey string, platformUserId *int, proxy *ProxyConfig) error {
	targetKey := normalizeTokenKeyForCompare(tokenKey)
	if targetKey == "" {
		return nil
	}

	normalized := normalizeBaseURL(baseURL)
	items, err := s.listAPIKeys(ctx, normalized, accessToken, proxy)
	if err != nil {
		return nil
	}

	var tokenID *int
	for _, item := range items {
		if normalizeTokenKeyForCompare(item.key) == targetKey {
			id := item.id
			tokenID = &id
			break
		}
	}

	if tokenID == nil {
		return nil // Already absent, safe
	}

	headers := authBearerHeaders(accessToken)
	endpoints := []string{
		fmt.Sprintf("/api/v1/keys/%d", *tokenID),
		fmt.Sprintf("/api/v1/api-keys/%d", *tokenID),
	}
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, normalized+endpoint, "DELETE", nil, headers, proxy)
		if err != nil {
			continue
		}
		if err := s.parseSub2ApiEnvelope(resp, endpoint); err == nil {
			return nil
		}
	}

	return nil
}

// GetSiteAnnouncements: GET /api/v1/announcements?page=1&page_size=100.
func (s *Sub2ApiAdapter) GetSiteAnnouncements(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]SiteAnnouncement, error) {
	endpoint := "/api/v1/announcements?page=1&page_size=100"
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, normalizeBaseURL(baseURL)+endpoint, "GET", nil, headers, proxy)
	if err != nil {
		return nil, fmt.Errorf("fetch announcements: %w", err)
	}

	data, err := s.parseSub2ApiEnvelopeRaw(resp, endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse announcements envelope: %w", err)
	}

	var rawItems []interface{}
	switch v := data.(type) {
	case []interface{}:
		rawItems = v
	case map[string]interface{}:
		if items, ok := v["items"].([]interface{}); ok {
			rawItems = items
		}
	}

	rows := make([]SiteAnnouncement, 0, len(rawItems))
	for _, item := range rawItems {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		idFloat, ok := m["id"].(float64)
		if !ok || int(idFloat) <= 0 {
			continue
		}
		id := int(idFloat)

		title, _ := getString(m, "title")
		content, _ := getString(m, "content")
		if title == "" && content == "" {
			continue
		}
		if title == "" {
			title = fmt.Sprintf("Announcement %d", id)
		}
		if content == "" {
			content = title
		}

		ann := SiteAnnouncement{
			SourceKey: fmt.Sprintf("announcement:%d", id),
			Title:     title,
			Content:   content,
			Level:     "info",
		}
		if v, ok := getString(m, "starts_at"); ok {
			ann.StartsAt = v
		}
		if v, ok := getString(m, "ends_at"); ok {
			ann.EndsAt = v
		}
		if v, ok := getString(m, "created_at"); ok {
			ann.UpstreamCreatedAt = v
		}
		if v, ok := getString(m, "updated_at"); ok {
			ann.UpstreamUpdatedAt = v
		}
		rows = append(rows, ann)
	}
	return rows, nil
}

// --- Internal types and helpers ---

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

type sub2apiKeyItem struct {
	id         int
	key        string
	name       string
	enabled    bool
	tokenGroup string
}

func (s *Sub2ApiAdapter) parseSub2ApiEnvelope(body map[string]interface{}, endpoint string) error {
	code, ok := body["code"]
	if !ok {
		return fmt.Errorf("Invalid response format from %s", endpoint)
	}

	codeFloat, ok := code.(float64)
	if !ok {
		return fmt.Errorf("Invalid response format from %s", endpoint)
	}

	if codeFloat != 0 {
		msg, _ := getString(body, "message")
		if msg == "" {
			msg = fmt.Sprintf("Error code %v from %s", codeFloat, endpoint)
		}
		return fmt.Errorf("%s", msg)
	}

	if _, ok := body["data"]; !ok {
		return fmt.Errorf("Missing data in response from %s", endpoint)
	}
	return nil
}

func (s *Sub2ApiAdapter) parseSub2ApiEnvelopeRaw(body map[string]interface{}, endpoint string) (interface{}, error) {
	if err := s.parseSub2ApiEnvelope(body, endpoint); err != nil {
		return nil, err
	}
	return body["data"], nil
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

func (s *Sub2ApiAdapter) resolveExpiresInDays(expiredTime int64) int {
	if expiredTime <= 0 {
		return 0
	}

	var expiresAtMs int64
	if expiredTime > 10_000_000_000 {
		expiresAtMs = expiredTime
	} else {
		expiresAtMs = expiredTime * 1000
	}

	deltaMs := expiresAtMs - time.Now().UnixMilli()
	days := int(math.Max(1, math.Ceil(float64(deltaMs)/float64(24*60*60*1000))))
	if days > 3650 {
		days = 3650
	}
	return days
}

func (s *Sub2ApiAdapter) resolveManagementBaseURL(baseURL string) string {
	normalized := normalizeBaseURL(baseURL)
	if normalized == "" {
		return normalized
	}

	suffixes := []string{
		"/models", "/antigravity", "/antigravity/v1beta", "/antigravity/v1",
		"/api/v1", "/v1beta", "/v1",
	}

	changed := true
	for changed {
		changed = false
		for _, suffix := range suffixes {
			lower := strings.ToLower(normalized)
			if !strings.HasSuffix(lower, suffix) {
				continue
			}
			trimmed := normalizeBaseURL(normalized[:len(normalized)-len(suffix)])
			if trimmed == "" || trimmed == normalized {
				continue
			}
			normalized = trimmed
			changed = true
			break
		}
	}

	return normalized
}

func (s *Sub2ApiAdapter) listAPIKeys(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) ([]sub2apiKeyItem, error) {
	endpoints := []string{
		"/api/v1/keys?page=1&page_size=100",
		"/api/v1/api-keys?page=1&page_size=100",
	}

	headers := authBearerHeaders(accessToken)
	for _, endpoint := range endpoints {
		resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
		if err != nil {
			continue
		}
		rawData, err := s.parseSub2ApiEnvelopeRaw(resp, endpoint)
		if err != nil {
			continue
		}
		items := s.parseTokenItems(rawData)
		if len(items) > 0 {
			return items, nil
		}
	}

	return nil, nil
}

func (s *Sub2ApiAdapter) parseTokenItems(raw interface{}) []sub2apiKeyItem {
	var rawItems []interface{}
	switch v := raw.(type) {
	case []interface{}:
		rawItems = v
	case map[string]interface{}:
		if items, ok := v["items"].([]interface{}); ok {
			rawItems = items
		} else if items, ok := v["list"].([]interface{}); ok {
			rawItems = items
		} else if items, ok := v["data"].([]interface{}); ok {
			rawItems = items
		}
	}

	items := make([]sub2apiKeyItem, 0, len(rawItems))
	for _, rawItem := range rawItems {
		m, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}

		key, _ := getString(m, "key")
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		idFloat, ok := m["id"].(float64)
		if !ok || idFloat <= 0 {
			continue
		}

		name, _ := getString(m, "name")
		name = strings.TrimSpace(name)
		if name == "" {
			name = fmt.Sprintf("token-%d", int(idFloat))
		}

		enabled := true
		if status, ok := m["status"]; ok {
			switch v := status.(type) {
			case bool:
				enabled = v
			case float64:
				enabled = v == 1
			case string:
				lower := strings.ToLower(strings.TrimSpace(v))
				enabled = lower != "inactive" && lower != "disabled" && lower != "false" && lower != "0" && lower != "off"
			}
		}

		tokenGroup := ""
		if gid, ok := m["group_id"].(float64); ok && gid > 0 {
			tokenGroup = fmt.Sprintf("%d", int(gid))
		} else if g, ok := getString(m, "group_name"); ok {
			tokenGroup = g
		} else if g, ok := getString(m, "group"); ok {
			tokenGroup = g
		}

		items = append(items, sub2apiKeyItem{
			id:         int(idFloat),
			key:        key,
			name:       name,
			enabled:    enabled,
			tokenGroup: tokenGroup,
		})
	}
	return items
}

func parseGroupID(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	var id int
	_, err := fmt.Sscanf(raw, "%d", &id)
	return id, err
}
