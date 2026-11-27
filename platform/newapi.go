package platform

import (
	"context"
	"fmt"
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

	if data, _ := n.userSelfDataByCookie(ctx, baseURL, accessToken, platformUserId, proxy); data != nil {
		return parseUserInfo(data), nil
	}
	return nil, nil
}

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
				return n.verifiedSession(ctx, baseURL, token, data, getIntPtr(data, "id"), proxy)
			}
		}
	}

	// Both a JSON message and a transport error can request the New-Api-User header.
	message, _ := getString(resp, "message")
	if err != nil {
		message = err.Error()
	}
	if strings.Contains(message, "New-Api-User") {
		result, err := n.verifyWithUserIDHeader(ctx, baseURL, token, platformUserId, proxy)
		if err != nil || result != nil {
			return result, err
		}
	}
	if data, userID := n.userSelfDataByCookie(ctx, baseURL, token, platformUserId, proxy); data != nil {
		return n.verifiedSession(ctx, baseURL, token, data, userID, proxy)
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
	return n.verifiedSession(ctx, baseURL, token, data, userID, proxy)
}

func (n *NewApiAdapter) verifiedSession(ctx context.Context, baseURL, token string, data map[string]interface{}, userID *int, proxy *ProxyConfig) (*TokenVerifyResult, error) {
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
		SourceKey: "notice",
		Title:     "Site notice",
		Content:   content,
		Level:     "info",
	}}, nil
}
