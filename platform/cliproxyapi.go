package platform

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// CliProxyApiAdapter handles CLIProxy API (port 8317 + x-cpa-* headers).
type CliProxyApiAdapter struct {
	*StandardAdapter
}

// Detect uses 3 conditions: port 8317, "cliproxy" keyword, or HTTP probe with x-cpa-* headers.
func (c *CliProxyApiAdapter) Detect(ctx context.Context, url string) (bool, error) {
	lower := strings.ToLower(url)

	// Condition 1: port 8317
	if strings.Contains(lower, ":8317/") || strings.HasSuffix(lower, ":8317") {
		return true, nil
	}

	// Condition 2: "cliproxy" / "cli-proxy-api" keywords (the hyphenated form
	// is an alias in the registry but was previously not matched).
	if strings.Contains(lower, "cliproxy") || strings.Contains(lower, "cli-proxy-api") {
		return true, nil
	}

	// Condition 3: HTTP probe for x-cpa-* headers
	base := normalizePlatformBaseURL(url)
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx2, "GET", base+"/v0/management/openai-compatibility", nil)
	if err != nil {
		return false, nil
	}

	resp, err := DoWithProxy(ctx2, req, nil)
	if err != nil {
		return false, nil
	}
	defer resp.Body.Close()

	hasCpaHeaders := resp.Header.Get("x-cpa-version") != "" ||
		resp.Header.Get("x-cpa-commit") != "" ||
		resp.Header.Get("x-cpa-build-date") != ""

	if hasCpaHeaders {
		return resp.StatusCode == 200 || resp.StatusCode == 401 || resp.StatusCode == 403, nil
	}

	return false, nil
}

// GetModels fetches models from the standard /v1/models endpoint.
func (c *CliProxyApiAdapter) GetModels(ctx context.Context, baseURL string, apiToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	return c.fetchModelsFromStandardEndpoint(ctx, baseURL, authBearerHeaders(apiToken), proxy)
}
