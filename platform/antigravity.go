package platform

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// AntigravityAdapter handles Google Antigravity platforms (OAuth-driven).
// StandardAdapter provides the "not supported"/empty defaults for auth,
// checkin and balance methods.
type AntigravityAdapter struct {
	*StandardAdapter
}

// Detect matches the antigravity host only. The bare "antigravity" substring
// previously matched any URL whose path contained /antigravity (e.g. Sub2API
// model endpoints), swallowing them before sub2api was reached.
func (a *AntigravityAdapter) Detect(ctx context.Context, urlStr string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(urlStr))
	if err != nil {
		return false, nil
	}
	return strings.EqualFold(parsed.Hostname(), "antigravity.googleapis.com"), nil
}

// GetModels fetches from /v1internal:fetchAvailableModels with Antigravity-specific headers.
func (a *AntigravityAdapter) GetModels(ctx context.Context, baseURL string, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")

	headers := map[string]string{
		"Authorization":     "Bearer " + accessToken,
		"Accept":            "application/json",
		"User-Agent":        "Antigravity/1.0",
		"X-Goog-Api-Client": "antigravity-client",
		"Client-Metadata":   "antigravity",
	}

	resp, err := fetchJSON(ctx, normalized+"/v1internal:fetchAvailableModels", "POST", map[string]interface{}{}, headers, proxy)
	if err != nil {
		// Propagate: service.classifyModelRefreshError turns this into an
		// actionable reason. Returning an empty list here made an unreachable or
		// unauthorized upstream indistinguishable from "no models", which is
		// reported to the operator as empty_models (#1232 family).
		return nil, fmt.Errorf("fetch available models: %w", err)
	}

	return extractAntigravityModelNames(resp), nil
}

func extractAntigravityModelNames(payload map[string]interface{}) []string {
	modelsObj, ok := payload["models"]
	if !ok {
		return []string{}
	}

	// Object form: {"models": {"model-name": {...},...}}
	if m, ok := modelsObj.(map[string]interface{}); ok {
		names := make([]string, 0, len(m))
		for name := range m {
			if t := strings.TrimSpace(name); t != "" {
				names = append(names, t)
			}
		}
		return normalizeModelIDs(names)
	}

	// Array form: {"models": [{"id": "...", "name": "..."},...]}
	if arr, ok := modelsObj.([]interface{}); ok {
		names := make([]string, 0, len(arr))
		for _, item := range arr {
			switch v := item.(type) {
			case string:
				if t := strings.TrimSpace(v); t != "" {
					names = append(names, t)
				}
			case map[string]interface{}:
				if id, ok := v["id"].(string); ok && strings.TrimSpace(id) != "" {
					names = append(names, strings.TrimSpace(id))
				} else if name, ok := v["name"].(string); ok && strings.TrimSpace(name) != "" {
					names = append(names, strings.TrimSpace(name))
				}
			}
		}
		return normalizeModelIDs(names)
	}

	return []string{}
}
