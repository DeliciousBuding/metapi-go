package platform

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

func (s *Sub2ApiAdapter) fetchSubscriptionSummary(ctx context.Context, baseURL, accessToken string, proxy *ProxyConfig) (*SubscriptionSummary, error) {
	headers := authBearerHeaders(accessToken)
	summaryEndpoint := "/api/v1/subscriptions/summary"

	resp, err := fetchJSON(ctx, baseURL+summaryEndpoint, "GET", nil, headers, proxy)
	if err != nil {
		// Try fallback
		return s.trySubscriptionFallback(ctx, baseURL, headers, proxy)
	}

	data, err := s.parseSub2ApiEnvelopeRaw(resp, summaryEndpoint)
	if err != nil {
		return s.trySubscriptionFallback(ctx, baseURL, headers, proxy)
	}

	return s.buildSubscriptionSummary(data), nil
}

func (s *Sub2ApiAdapter) trySubscriptionFallback(ctx context.Context, baseURL string, headers map[string]string, proxy *ProxyConfig) (*SubscriptionSummary, error) {
	fallbackEndpoints := []string{"/api/v1/subscriptions/active"}
	for _, endpoint := range fallbackEndpoints {
		resp, err := fetchJSON(ctx, baseURL+endpoint, "GET", nil, headers, proxy)
		if err != nil {
			continue
		}
		data, err := s.parseSub2ApiEnvelopeRaw(resp, endpoint)
		if err != nil {
			continue
		}
		return s.buildSubscriptionSummary(data), nil
	}
	return nil, nil
}

func (s *Sub2ApiAdapter) buildSubscriptionSummary(raw interface{}) *SubscriptionSummary {
	subscriptions := s.parseSubscriptionItems(raw)

	body, _ := raw.(map[string]interface{})
	var activeCount int
	var totalUsedUsd float64

	if body != nil {
		if ac, ok := getFloat(body, "active_count"); ok {
			activeCount = int(ac)
		} else if ac, ok := getFloat(body, "activeCount"); ok {
			activeCount = int(ac)
		}

		if tu, ok := getFloat(body, "total_used_usd"); ok {
			totalUsedUsd = tu
		} else if tu, ok := getFloat(body, "totalUsedUsd"); ok {
			totalUsedUsd = tu
		}
	}

	if activeCount == 0 {
		activeCount = len(subscriptions)
	}

	if totalUsedUsd == 0 {
		for _, sub := range subscriptions {
			if sub.MonthlyUsedUsd != nil {
				totalUsedUsd += *sub.MonthlyUsedUsd
			}
		}
		totalUsedUsd = math.Round(totalUsedUsd*1e6) / 1e6
	}

	return &SubscriptionSummary{
		ActiveCount:   activeCount,
		TotalUsedUsd:  totalUsedUsd,
		Subscriptions: subscriptions,
	}
}

func (s *Sub2ApiAdapter) parseSubscriptionItems(raw interface{}) []SubscriptionPlanSummary {
	var rawItems []interface{}
	switch v := raw.(type) {
	case []interface{}:
		rawItems = v
	case map[string]interface{}:
		if arr, ok := v["subscriptions"].([]interface{}); ok {
			rawItems = arr
		} else if arr, ok := v["items"].([]interface{}); ok {
			rawItems = arr
		} else if arr, ok := v["list"].([]interface{}); ok {
			rawItems = arr
		} else if arr, ok := v["data"].([]interface{}); ok {
			rawItems = arr
		}
	}

	result := make([]SubscriptionPlanSummary, 0, len(rawItems))
	for _, item := range rawItems {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if summary := s.parseSingleSubscription(m); summary != nil {
			result = append(result, *summary)
		}
	}
	return result
}

func (s *Sub2ApiAdapter) parseSingleSubscription(item map[string]interface{}) *SubscriptionPlanSummary {
	summary := SubscriptionPlanSummary{}

	if id := getIntPtr(item, "id"); id != nil {
		summary.ID = id
	}

	// Try group_id from nested group object or direct
	groupObj, _ := getMap(item, "group")
	if gid := getIntPtr(item, "group_id"); gid != nil {
		summary.GroupID = gid
	} else if gid := getIntPtr(item, "groupId"); gid != nil {
		summary.GroupID = gid
	} else if groupObj != nil {
		if gid := getIntPtr(groupObj, "id"); gid != nil {
			summary.GroupID = gid
		}
	}

	// Group name from multiple candidates
	groupNameCandidates := []string{}
	if s, _ := getString(item, "group_name"); s != "" {
		groupNameCandidates = append(groupNameCandidates, s)
	}
	if s, _ := getString(item, "groupName"); s != "" {
		groupNameCandidates = append(groupNameCandidates, s)
	}
	if s, _ := getString(item, "name"); s != "" {
		groupNameCandidates = append(groupNameCandidates, s)
	}
	if s, _ := getString(item, "title"); s != "" {
		groupNameCandidates = append(groupNameCandidates, s)
	}
	if groupObj != nil {
		if s, _ := getString(groupObj, "name"); s != "" {
			groupNameCandidates = append(groupNameCandidates, s)
		}
		if s, _ := getString(groupObj, "title"); s != "" {
			groupNameCandidates = append(groupNameCandidates, s)
		}
	}
	if len(groupNameCandidates) > 0 {
		summary.GroupName = groupNameCandidates[0]
	}

	if s, _ := getString(item, "status"); s != "" {
		summary.Status = s
	}

	// Expires at
	expiresAt := s.parseDateTime(
		s.getRawString(item, "expires_at"),
		s.getRawString(item, "expiresAt"),
		s.getRawString(item, "expired_at"),
		s.getRawString(item, "expiredAt"),
		s.getRawString(item, "end_at"),
		s.getRawString(item, "endAt"),
		s.getRawString(item, "end_time"),
		s.getRawString(item, "endTime"),
		s.getRawString(item, "current_period_end"),
		s.getRawString(item, "currentPeriodEnd"),
	)
	if expiresAt != "" {
		summary.ExpiresAt = expiresAt
	}

	// Daily
	if v := s.parseNonNegativeNumber(s.getRaw(item, "daily_used_usd"), s.getRaw(item, "dailyUsedUsd")); v != nil {
		summary.DailyUsedUsd = v
	}
	if v := s.parseNonNegativeNumber(s.getRaw(item, "daily_limit_usd"), s.getRaw(item, "dailyLimitUsd")); v != nil {
		summary.DailyLimitUsd = v
	}

	// Weekly
	if v := s.parseNonNegativeNumber(s.getRaw(item, "weekly_used_usd"), s.getRaw(item, "weeklyUsedUsd")); v != nil {
		summary.WeeklyUsedUsd = v
	}
	if v := s.parseNonNegativeNumber(s.getRaw(item, "weekly_limit_usd"), s.getRaw(item, "weeklyLimitUsd")); v != nil {
		summary.WeeklyLimitUsd = v
	}

	// Monthly
	if v := s.parseNonNegativeNumber(
		s.getRaw(item, "monthly_used_usd"), s.getRaw(item, "monthlyUsedUsd"),
		s.getRaw(item, "used_usd"), s.getRaw(item, "usedUsd"),
		s.getRaw(item, "total_used_usd"), s.getRaw(item, "totalUsedUsd"),
	); v != nil {
		summary.MonthlyUsedUsd = v
	}
	if v := s.parseNonNegativeNumber(
		s.getRaw(item, "monthly_limit_usd"), s.getRaw(item, "monthlyLimitUsd"),
		s.getRaw(item, "limit_usd"), s.getRaw(item, "limitUsd"),
		s.getRaw(item, "total_limit_usd"), s.getRaw(item, "totalLimitUsd"),
	); v != nil {
		summary.MonthlyLimitUsd = v
	}

	// If nothing parsed, return nil
	if summary.ID == nil && summary.GroupID == nil && summary.GroupName == "" &&
		summary.Status == "" && summary.ExpiresAt == "" &&
		summary.DailyUsedUsd == nil && summary.MonthlyUsedUsd == nil {
		return nil
	}

	return &summary
}

func (s *Sub2ApiAdapter) getRaw(item map[string]interface{}, key string) interface{} {
	return item[key]
}

func (s *Sub2ApiAdapter) getRawString(item map[string]interface{}, key string) string {
	v := item[key]
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case float64:
		return fmt.Sprintf("%v", val)
	}
	return ""
}

func (s *Sub2ApiAdapter) parseNonNegativeNumber(values ...interface{}) *float64 {
	for _, v := range values {
		if v == nil {
			continue
		}
		switch val := v.(type) {
		case float64:
			if val >= 0 {
				result := math.Round(val*1e6) / 1e6
				return &result
			}
		case string:
			trimmed := strings.TrimSpace(val)
			if trimmed == "" {
				continue
			}
			// Try parse
			var f float64
			if _, err := fmt.Sscanf(trimmed, "%f", &f); err == nil && f >= 0 {
				result := math.Round(f*1e6) / 1e6
				return &result
			}
		}
	}
	return nil
}

func (s *Sub2ApiAdapter) parseDateTime(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}

		// Try numeric (unix timestamp)
		var numeric float64
		if _, err := fmt.Sscanf(v, "%f", &numeric); err == nil && numeric > 0 {
			ms := numeric
			if ms < 10_000_000_000 {
				ms *= 1000
			}
			return time.UnixMilli(int64(ms)).Format(time.RFC3339)
		}

		// Try date parsing
		t, err := time.Parse(time.RFC3339, v)
		if err == nil {
			return t.Format(time.RFC3339)
		}
	}
	return ""
}
