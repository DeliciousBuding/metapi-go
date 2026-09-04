package platform

import (
	"context"
	"fmt"
	"strings"
)

// DoneHubAdapter extends OneHubAdapter with checkin unsupported, remaining-quota balance, and /api/notice.
type DoneHubAdapter struct {
	*OneHubAdapter
}

// Detect: URL keyword match "donehub" or "done-hub" (title-first platform).
func (d *DoneHubAdapter) Detect(ctx context.Context, url string) (bool, error) {
	lower := strings.ToLower(url)
	return strings.Contains(lower, "donehub") || strings.Contains(lower, "done-hub"), nil
}

// Checkin: always unsupported for DoneHub.
func (d *DoneHubAdapter) Checkin(ctx context.Context, url, accessToken string, platformUserId *int, proxy *ProxyConfig) (*CheckinResult, error) {
	return &CheckinResult{Success: false, Message: "checkin endpoint not found"}, nil
}

// GetBalance: quota=remaining, total=quota+used, divisor=500000.
func (d *DoneHubAdapter) GetBalance(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*BalanceInfo, error) {
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, headers, proxy)
	if err != nil {
		// Propagate. A zero BalanceInfo with a nil error is stored as the
		// account's balance, and service/balance drives runtime health, the
		// token-expired alert and the auto-relogin retry from err != nil — so
		// swallowing here wrote balance=0 and skipped all three.
		return nil, fmt.Errorf("fetch balance: %w", err)
	}

	balance := parseOneApiStyleBalance(resp, 500000, true)
	return &balance, nil
}

// GetSiteAnnouncements: GET /api/notice.
func (d *DoneHubAdapter) GetSiteAnnouncements(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]SiteAnnouncement, error) {
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
		SourceKey:  buildNoticeSourceKey(content),
		Title:      "Site notice",
		Content:    content,
		Level:      "info",
		SourceURL:  "/api/notice",
		RawPayload: nil,
	}}, nil
}
