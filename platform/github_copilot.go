package platform

import (
	"context"
	"net/url"
	"strings"
)

// GitHubCopilotAdapter handles api.githubcopilot.com. Credentials are managed
// by the Device OAuth provider in service/oauth/copilot.go.
type GitHubCopilotAdapter struct {
	*StandardAdapter
}

// copilotSeedModels mirrors the public GitHub Copilot model surface used by
// the AxonHub reference. It is intentionally a seed list: Copilot does not
// expose a public /v1/models endpoint for this account surface, and seed
// presence is not a real-availability claim.
var copilotSeedModels = []string{
	"gpt-5.4",
	"grok-4.7",
	"gpt-5.4-nano",
	"claude-opus-4.7",
	"kimi-k3",
	"claude-haiku-5.5",
	"gpt-5-mini",
	"claude-opus-4.8",
	"gpt-5.3-codex",
	"gemini-3.6-flash",
	"mai-code-1-flash-picker",
	"mai-code-1.1-flash",
	"gpt-6.1-sol",
	"claude-haiku-4.5",
	"gpt-6-astra",
	"grok-4.5",
	"claude-opus-5",
	"claude-sonnet-5.5",
	"gemini-3.5-flash",
	"claude-fable-5.1",
	"claude-fable-5",
	"gpt-5.4-mini",
	"gpt-5.6-luna",
	"gpt-5.5",
	"claude-sonnet-5",
	"gemini-3.7-flash",
	"claude-sonnet-4.6",
	"gemini-3.8-flash",
	"gpt-6-luna",
	"claude-opus-5.5",
	"gpt-5.6-terra",
	"kimi-k2.7-code",
	"grok-4.6",
	"gpt-5.6-sol",
	"gemini-4-argon",
	"gpt-6-sol",
}

// Detect matches the Copilot chat host and GitHub API URLs whose path already
// identifies a Copilot endpoint.
func (a *GitHubCopilotAdapter) Detect(ctx context.Context, urlStr string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(urlStr))
	if err != nil {
		return false, nil
	}
	hostname := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)
	if hostname == "api.githubcopilot.com" {
		return true, nil
	}
	return hostname == "api.github.com" && strings.Contains(path, "copilot"), nil
}

// GetModels returns the curated seed list; it never claims live availability.
func (a *GitHubCopilotAdapter) GetModels(ctx context.Context, baseURL string, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	models := make([]string, len(copilotSeedModels))
	copy(models, copilotSeedModels)
	return normalizeModelIDs(models), nil
}
