package platform

import (
	"context"
	"net/url"
	"strings"
)

// OpenAiAdapter handles api.openai.com platforms.
type OpenAiAdapter struct {
	*StandardAdapter
}

// Detect matches the exact api.openai.com host (avoids suffix confusion such
// as api.openai.com.evil.example).
func (o *OpenAiAdapter) Detect(ctx context.Context, urlStr string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(urlStr))
	if err != nil {
		return false, nil
	}
	return strings.EqualFold(parsed.Hostname(), "api.openai.com"), nil
}

// GetModels fetches models from the standard /v1/models OpenAI endpoint.
func (o *OpenAiAdapter) GetModels(ctx context.Context, baseURL string, token string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	return o.fetchModelsFromStandardEndpoint(ctx, baseURL, authBearerHeaders(token), proxy)
}
