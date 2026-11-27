package balance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
)

// income log fallback config
const incomeLogResponseBodyLimit = 1 << 20

var supportedIncomeLogPlatforms = map[string]bool{
	"new-api":   true,
	"anyrouter": true,
	"one-api":   true,
	"veloera":   true,
}

func supportsTodayIncomeLogFallback(platform string) bool {
	return supportedIncomeLogPlatforms[strings.ToLower(platform)]
}

func resolveQuotaConversionFactor(platform string) float64 {
	if strings.ToLower(platform) == "veloera" {
		return 1_000_000
	}
	return 500_000
}

// fetchTodayIncomeFromLogs fetches today's income from /api/log/self endpoints.
func fetchTodayIncomeFromLogs(baseURL, accessToken, platformName string, platformUserID int64, proxyConfig *platform.ProxyConfig) (float64, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	accessToken = strings.TrimSpace(accessToken)
	if baseURL == "" || accessToken == "" {
		return 0, fmt.Errorf("empty baseURL or accessToken")
	}

	start, end := service.GetTodayUnixSecondsRange(time.Now())
	conversionFactor := resolveQuotaConversionFactor(platformName)
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
		"Content-Type":  "application/json",
	}
	if platformUserID > 0 {
		headers["New-Api-User"] = strconv.FormatInt(platformUserID, 10)
	}

	logTypes := []int{1, 4}
	const pageSize = 100
	const maxPages = 6
	hasAnyResponse := false
	totalIncome := 0.0

	for _, logType := range logTypes {
		for page := 1; page <= maxPages; page++ {
			query := fmt.Sprintf("p=%d&page_size=%d&type=%d&token_name=&model_name=&start_timestamp=%d&end_timestamp=%d&group=",
				page, pageSize, logType, start, end)
			reqURL := baseURL + "/api/log/self?" + query

			req, err := http.NewRequest(http.MethodGet, reqURL, nil)
			if err != nil {
				break
			}
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			resp, err := platform.DoWithProxy(context.Background(), req, proxyConfig)
			if err != nil || resp.StatusCode != http.StatusOK {
				if resp != nil {
					resp.Body.Close()
				}
				break
			}

			var payload map[string]any
			if err := decodeIncomeLogPayload(resp.Body, &payload); err != nil {
				resp.Body.Close()
				break
			}
			resp.Body.Close()

			if payload == nil {
				break
			}
			hasAnyResponse = true

			items := extractLogItems(payload)
			for _, item := range items {
				quotaRaw := parsePositiveNumberAny(item["quota"])
				if quotaRaw > 0 {
					totalIncome += quotaRaw / conversionFactor
					continue
				}
				if content, ok := item["content"].(string); ok {
					totalIncome += parseIncomeFromContent(content)
				}
			}

			total := extractLogTotal(payload)
			if len(items) == 0 {
				break
			}
			if total != nil && page*pageSize >= *total {
				break
			}
		}
	}

	if !hasAnyResponse {
		return 0, fmt.Errorf("no log responses")
	}
	return math.Round(totalIncome*1_000_000) / 1_000_000, nil
}

func decodeIncomeLogPayload(body io.Reader, payload *map[string]any) error {
	limited := &io.LimitedReader{R: body, N: incomeLogResponseBodyLimit + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return err
	}
	if int64(len(data)) > incomeLogResponseBodyLimit {
		return fmt.Errorf("income log response exceeds %d bytes", incomeLogResponseBodyLimit)
	}
	return json.Unmarshal(data, payload)
}

func extractLogItems(payload map[string]any) []map[string]any {
	if data, ok := payload["data"].(map[string]any); ok {
		if items, ok := data["items"].([]any); ok {
			result := make([]map[string]any, 0, len(items))
			for _, item := range items {
				if m, ok := item.(map[string]any); ok {
					result = append(result, m)
				}
			}
			return result
		}
	}
	if items, ok := payload["items"].([]any); ok {
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	}
	if data, ok := payload["data"].([]any); ok {
		result := make([]map[string]any, 0, len(data))
		for _, item := range data {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}

func extractLogTotal(payload map[string]any) *int {
	candidates := []any{}
	if data, ok := payload["data"].(map[string]any); ok {
		candidates = append(candidates, data["total"])
	}
	candidates = append(candidates, payload["total"])

	for _, v := range candidates {
		switch val := v.(type) {
		case float64:
			if val >= 0 {
				t := int(val)
				return &t
			}
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(val)); err == nil && n >= 0 {
				return &n
			}
		}
	}
	return nil
}

func parsePositiveNumberAny(v any) float64 {
	switch val := v.(type) {
	case float64:
		if val > 0 && !math.IsNaN(val) && !math.IsInf(val, 0) {
			return val
		}
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err == nil && n > 0 && !math.IsNaN(n) && !math.IsInf(n, 0) {
			return n
		}
	}
	return 0
}

func parseIncomeFromContent(content string) float64 {
	// Extract the first number from content text.
	// Matches TS behavior: scans for digit patterns in log content.
	normalized := strings.ReplaceAll(content, ",", "")
	re := regexp.MustCompile(`\d+(?:\.\d+)?`)
	match := re.FindString(normalized)
	if match == "" {
		return 0
	}
	val, err := strconv.ParseFloat(match, 64)
	if err != nil || val <= 0 || math.IsNaN(val) || math.IsInf(val, 0) {
		return 0
	}
	return val
}
