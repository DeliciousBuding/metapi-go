package platform

import (
	"context"
	"strings"
)

// OneApiAdapter handles OneAPI platforms (detect: /api/status by version +
// system_name value discriminator against new-api).
// Serves as the base for OneHubAdapter and DoneHubAdapter.
type OneApiAdapter struct {
	*BaseAdapter
}

// Detect probes GET /api/status and checks success===true, data.version is
// present, and system_name (when present) names one-api. one-api v0.5.x has no
// system_name (legacy shape); v0.6.10+ ships system_name (default "One API").
// The value check keeps new-api (default "New API") and one-api apart.
// Requiring version keeps unrelated /api/status endpoints from being labeled
// one-api. Operators who rename SYSTEM_NAME to something unrelated degrade to
// manual platform selection — acceptable for an auto-detect heuristic.
func (o *OneApiAdapter) Detect(ctx context.Context, url string) (bool, error) {
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
	if _, hasVersion := data["version"]; !hasVersion {
		return false, nil
	}
	systemName, hasSystemName := getString(data, "system_name")
	if !hasSystemName {
		// Legacy v0.5 shape (or a non-string system_name): nothing to
		// discriminate on, treat as absent.
		return true, nil
	}
	folded := strings.ToLower(systemName)
	return strings.Contains(folded, "oneapi") || strings.Contains(folded, "one api"), nil
}

// GetModels: GET /v1/models (Bearer auth).
func (o *OneApiAdapter) GetModels(ctx context.Context, baseURL string, apiToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	headers := authBearerHeaders(apiToken)
	resp, err := fetchJSON(ctx, baseURL+"/v1/models", "GET", nil, headers, proxy)
	if err != nil {
		return nil, err
	}

	return extractModelIDsFromData(resp), nil
}
