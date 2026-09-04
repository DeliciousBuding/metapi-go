package platform

import (
	"context"
	"net/url"
	"strings"
)

// ClaudeDefaultAnthropicVersion is the anthropic-version header value used when
// a caller does not supply one. Single source of truth for every path that
// speaks the Anthropic-native protocol (platform adapters, OAuth flows, and the
// /v1 proxy data plane).
const ClaudeDefaultAnthropicVersion = "2023-06-01"

// ClaudeAdapter handles api.anthropic.com platforms (native + OpenAI-compat gateways).
type ClaudeAdapter struct {
	*StandardAdapter
}

// Detect matches the exact api.anthropic.com host, or anthropic.com with a
// /v1 path prefix (avoids anthropic.com/v1-docs style false positives).
func (c *ClaudeAdapter) Detect(ctx context.Context, urlStr string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(urlStr))
	if err != nil {
		return false, nil
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "api.anthropic.com" {
		return true, nil
	}
	return host == "anthropic.com" && strings.HasPrefix(parsed.Path, "/v1"), nil
}

// GetModels tries native Anthropic endpoint first, then falls back to OpenAI-compat
// by stripping the /anthropic suffix from the base URL.
func (c *ClaudeAdapter) GetModels(ctx context.Context, baseURL string, token string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	openAICompatBaseURL := resolveOpenAICompatibleBaseURL(baseURL)

	// Try native Anthropic endpoint
	claudeHeaders := map[string]string{
		"x-api-key":         token,
		"anthropic-version": ClaudeDefaultAnthropicVersion,
	}
	lad := &modelFetchLadder{}

	models, err := c.fetchModelsFromStandardEndpoint(ctx, baseURL, claudeHeaders, proxy)
	if err != nil {
		lad.fail(err.Error())
	} else {
		lad.answer()
		if len(models) > 0 {
			return models, nil
		}
	}

	// Fallback: strip /anthropic suffix and try OpenAI-compat. Without one, the
	// anthropic-shaped read above is the whole ladder, so its failure has to be
	// reported rather than flattened into an empty model list.
	if openAICompatBaseURL == "" {
		return lad.result()
	}

	return c.fetchModelsFromStandardEndpoint(ctx, openAICompatBaseURL, authBearerHeaders(token), proxy)
}

// resolveOpenAICompatibleBaseURL strips the /anthropic suffix to get the OpenAI-compat base.
func resolveOpenAICompatibleBaseURL(baseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lower := strings.ToLower(normalized)
	if strings.HasSuffix(lower, "/anthropic") {
		return normalized[:len(normalized)-len("/anthropic")]
	}
	return ""
}
