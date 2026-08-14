package platform

import (
	"context"
	"regexp"
	"strings"
)

func (s *Sub2ApiAdapter) resolveModelEndpoints(baseURL string) []string {
	normalized := normalizeBaseURL(baseURL)
	if normalized == "" {
		return nil
	}

	if strings.HasSuffix(strings.ToLower(normalized), "/models") {
		return []string{normalized}
	}

	if regexp.MustCompile(`(?i)/(?:antigravity/)?v\d+(?:\.\d+)?(?:beta)?$`).MatchString(normalized) {
		return []string{normalized + "/models"}
	}

	if strings.HasSuffix(strings.ToLower(normalized), "/antigravity") {
		return []string{
			normalized + "/v1/models",
			normalized + "/v1beta/models",
		}
	}

	return []string{
		normalized + "/v1/models",
		normalized + "/api/v1/models",
		normalized + "/v1beta/models",
		normalized + "/antigravity/v1beta/models",
	}
}

func (s *Sub2ApiAdapter) fetchModelsByToken(ctx context.Context, baseURL, token string, proxy *ProxyConfig) ([]string, error) {
	authToken := stripBearerPrefix(token)
	if authToken == "" {
		return nil, nil
	}

	endpoints := s.resolveModelEndpoints(baseURL)
	var lastErr error
	for _, url := range endpoints {
		// Fast-fail: a slow first endpoint must not burn the whole budget.
		// Checking ctx.Err() between attempts lets a cancelled/deadline-exceeded
		// context short-circuit the remaining endpoints (#675).
		if ctx.Err() != nil {
			break
		}
		headers := map[string]string{"Authorization": "Bearer " + authToken}
		resp, err := fetchJSON(ctx, url, "GET", nil, headers, proxy)
		if err != nil {
			lastErr = err
			continue
		}
		models := extractModelIDs(resp)
		if len(models) > 0 {
			return models, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return []string{}, nil
}

func extractModelIDs(payload map[string]interface{}) []string {
	var source interface{} = payload
	if data, ok := payload["data"]; ok {
		if m, ok := data.(map[string]interface{}); ok {
			source = m
		} else {
			source = data
		}
	}

	var rawModels []interface{}
	if source != nil {
		switch v := source.(type) {
		case []interface{}:
			rawModels = v
		case map[string]interface{}:
			if items, ok := v["items"].([]interface{}); ok {
				rawModels = items
			} else if models, ok := v["models"].([]interface{}); ok {
				rawModels = models
			}
		}
	}

	seen := make(map[string]bool)
	result := make([]string, 0, len(rawModels))
	for _, item := range rawModels {
		var name string
		switch v := item.(type) {
		case string:
			name = strings.TrimSpace(v)
		case map[string]interface{}:
			if id, ok := v["id"].(string); ok {
				name = strings.TrimSpace(id)
			} else if n, ok := v["name"].(string); ok {
				name = strings.TrimSpace(n)
			}
		}
		name = strings.TrimPrefix(name, "models/")
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return normalizeModelIDs(result)
}
