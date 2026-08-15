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

// VerifyToken verifies a pasted CLIProxyAPI credential.
//
// CliProxyApiAdapter deliberately overrides VerifyToken instead of inheriting
// BaseAdapter.VerifyToken: the base implementation statically dispatches to
// one-api style GET /api/user/self (absent on CLIProxyAPI) and reports every
// credential as "unknown". CLIProxyAPI credentials are keys: a provider API
// key whose models are served through /v1/models, or the management key
// (validated against the /v0/management API).
func (c *CliProxyApiAdapter) VerifyToken(ctx context.Context, baseURL, token string, platformUserId *int, proxy *ProxyConfig) (*TokenVerifyResult, error) {
	models, err := c.GetModels(ctx, baseURL, token, platformUserId, proxy)
	if err == nil && len(models) > 0 {
		return &TokenVerifyResult{TokenType: "apikey", Models: models}, nil
	}

	// /v1/models is unauthenticated on CLIProxyAPI (HTTP 200 with an empty
	// list for valid and invalid keys alike when no auth files exist), so it
	// cannot distinguish credentials. Probe the management API instead: a
	// valid management key gets 2xx, anything else 401.
	if c.isValidManagementKey(ctx, baseURL, token, proxy) {
		return &TokenVerifyResult{TokenType: "apikey", Models: models}, nil
	}

	return &TokenVerifyResult{TokenType: "unknown"}, nil
}

// isValidManagementKey reports whether token authenticates against the
// CLIProxyAPI management API (GET /v0/management/auth-files).
func (c *CliProxyApiAdapter) isValidManagementKey(ctx context.Context, baseURL, token string, proxy *ProxyConfig) bool {
	normalized := normalizePlatformBaseURL(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalized+"/v0/management/auth-files", nil)
	if err != nil {
		return false
	}
	for k, v := range authBearerHeaders(token) {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", DefaultBrowserUserAgent)
	ApplySiteIdentity(req, proxy)

	resp, err := DoWithProxy(ctx, req, proxy)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}
