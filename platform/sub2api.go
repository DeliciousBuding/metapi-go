package platform

import (
	"context"
	"encoding/json"
	"fmt"
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
