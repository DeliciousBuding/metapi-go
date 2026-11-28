package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CodexModelsClientVersion is the query param expected by Codex /models.
const CodexModelsClientVersion = "1.0.0"

// CodexModelDiscoveryInput holds inputs for cloud model discovery.
type CodexModelDiscoveryInput struct {
	BaseURL     string
	AccessToken string
	AccountID   string
	ProxyURL    *string
	// HTTPClient overrides the default OAuth HTTP client (tests).
	HTTPClient *http.Client
	// Now is optional clock for tests.
	Now func() time.Time
	// Sleep is optional sleeper for backoff (tests).
	Sleep func(context.Context, time.Duration) error
}

// BuildCodexModelsEndpoint builds GET.../models?client_version=...
func BuildCodexModelsEndpoint(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalized == "" {
		normalized = codexUpstreamBaseURL
	}
	return normalized + "/models?client_version=" + url.QueryEscape(CodexModelsClientVersion)
}

// ExtractCodexModelIDs parses Codex /models JSON payloads (array | models|data|items).
func ExtractCodexModelIDs(payload any) []string {
	collection := extractCodexModelCollection(payload)
	var ids []string
	for _, item := range collection {
		switch v := item.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				ids = append(ids, s)
			}
		case map[string]any:
			id := firstNonEmptyString(
				asNonEmptyString(v["id"]),
				asNonEmptyString(v["slug"]),
				asNonEmptyString(v["model"]),
			)
			if id != "" {
				ids = append(ids, id)
			}
		}
	}
	return NormalizeDiscoveredCodexModels(ids)
}

func extractCodexModelCollection(payload any) []any {
	switch v := payload.(type) {
	case []any:
		return v
	case map[string]any:
		for _, key := range []string{"models", "data", "items"} {
			if arr, ok := v[key].([]any); ok {
				return arr
			}
		}
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// DiscoverCodexModelsFromCloud performs a single GET /models discovery call.
func DiscoverCodexModelsFromCloud(ctx context.Context, input CodexModelDiscoveryInput) ([]string, error) {
	accessToken := strings.TrimSpace(input.AccessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("codex oauth access token missing")
	}
	endpoint := BuildCodexModelsEndpoint(input.BaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Originator", "codex_cli_rs")
	if accountID := strings.TrimSpace(input.AccountID); accountID != "" {
		req.Header.Set("Chatgpt-Account-Id", accountID)
	}

	client := input.HTTPClient
	if client == nil {
		client = newOAuthHTTPClient(nil)
	}
	// Honour proxy via doHTTP when custom client not provided.
	var resp *http.Response
	if input.HTTPClient != nil {
		resp, err = client.Do(req)
	} else {
		resp, err = doHTTP(req, input.ProxyURL, client)
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("codex model discovery timeout (%ds): %w",
				int(CodexModelDiscoveryTimeout/time.Second), ctx.Err())
		}
		return nil, fmt.Errorf("codex model discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := readOAuthErrorResponseBody(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("codex model discovery response read failed: %w", err)
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("codex model discovery returned invalid payload")
	}
	models := ExtractCodexModelIDs(payload)
	if len(models) == 0 {
		return nil, fmt.Errorf("no models available")
	}
	return models, nil
}
