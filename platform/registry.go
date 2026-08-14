package platform

import (
	"strings"
)

// PLATFORM_ALIASES maps user-facing platform names to canonical identifiers.
var PlatformAliases = map[string]string{
	"anyrouter":     "anyrouter",
	"vo-api":        "new-api",
	"super-api":     "new-api",
	"rix-api":       "new-api",
	"neo-api":       "new-api",
	"newapi":        "new-api",
	"new api":       "new-api",
	"new-api":       "new-api",
	"oneapi":        "one-api",
	"one api":       "one-api",
	"one-api":       "one-api",
	"onehub":        "one-hub",
	"one-hub":       "one-hub",
	"donehub":       "done-hub",
	"done-hub":      "done-hub",
	"veloera":       "veloera",
	"sub2api":       "sub2api",
	"openai":        "openai",
	"codex":         "codex",
	"chatgpt-codex": "codex",
	"chatgpt codex": "codex",
	"anthropic":     "claude",
	"claude":        "claude",
	"gemini":        "gemini",
	"gemini-cli":    "gemini-cli",
	"antigravity":   "antigravity",
	"anti-gravity":  "antigravity",
	"google":        "gemini",
	"cliproxyapi":   "cliproxyapi",
	"cpa":           "cliproxyapi",
	"cli-proxy-api": "cliproxyapi",
}

// orderedPlatformNames defines the spec-required adapter registration order.
// "Specific forks before generic adapters for better auto-detection."
// OneApi last (its HTTP probe is the broadest condition, serving as catch-all).
var orderedPlatformNames = []string{
	"openai",
	"codex",
	"claude",
	"gemini",
	"gemini-cli",
	"antigravity",
	"cliproxyapi",
	"anyrouter",
	"done-hub",
	"one-hub",
	"veloera",
	"new-api",
	"sub2api",
	"one-api",
}

// registry holds all platform adapters in detection order.
// Populated deterministically by init() following orderedPlatformNames.
var registry []PlatformAdapter

func init() {
	for _, name := range orderedPlatformNames {
		registry = append(registry, buildAdapter(name))
	}
}

// buildAdapter constructs the adapter registered for a canonical platform name.
func buildAdapter(name string) PlatformAdapter {
	switch name {
	case "openai":
		return &OpenAiAdapter{StandardAdapter: NewStandardAdapter("openai")}
	case "codex":
		return &CodexAdapter{StandardAdapter: &StandardAdapter{
			BaseAdapter:               NewBaseAdapter("codex"),
			LoginUnsupportedMessage:   "codex oauth login is managed via OAuth flow",
			CheckinUnsupportedMessage: "codex oauth connections do not support checkin",
		}}
	case "claude":
		return &ClaudeAdapter{StandardAdapter: NewStandardAdapter("claude")}
	case "gemini":
		return &GeminiAdapter{StandardAdapter: NewStandardAdapter("gemini")}
	case "gemini-cli":
		return &GeminiCliAdapter{GeminiAdapter: &GeminiAdapter{StandardAdapter: NewStandardAdapter("gemini-cli")}}
	case "antigravity":
		return &AntigravityAdapter{StandardAdapter: NewStandardAdapter("antigravity")}
	case "cliproxyapi":
		return &CliProxyApiAdapter{StandardAdapter: &StandardAdapter{
			BaseAdapter:               NewBaseAdapter("cliproxyapi"),
			LoginUnsupportedMessage:   "CLIProxyAPI does not support login",
			CheckinUnsupportedMessage: "CLIProxyAPI does not support checkin",
		}}
	case "anyrouter":
		return &AnyRouterAdapter{NewApiAdapter: &NewApiAdapter{BaseAdapter: NewBaseAdapter("anyrouter")}}
	case "done-hub":
		return &DoneHubAdapter{OneHubAdapter: &OneHubAdapter{OneApiAdapter: &OneApiAdapter{BaseAdapter: NewBaseAdapter("done-hub")}}}
	case "one-hub":
		return &OneHubAdapter{OneApiAdapter: &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-hub")}}
	case "veloera":
		return &VeloeraAdapter{BaseAdapter: NewBaseAdapter("veloera")}
	case "new-api":
		return &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
	case "sub2api":
		return &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	case "one-api":
		return &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	}
	return nil
}

// NormalizePlatformAlias maps a raw platform string to its canonical form.
func NormalizePlatformAlias(platform string) string {
	raw := strings.ToLower(strings.TrimSpace(platform))
	if raw == "" {
		return ""
	}
	if canonical, ok := PlatformAliases[raw]; ok {
		return canonical
	}
	return raw
}

// GetAdapter returns the registered adapter for a given canonical platform name.
func GetAdapter(platform string) PlatformAdapter {
	normalized := NormalizePlatformAlias(platform)
	for _, a := range registry {
		if a.PlatformName() == normalized {
			return a
		}
	}
	return nil
}

// ListAdapters returns all registered adapters (for diagnostics).
func ListAdapters() []PlatformAdapter {
	return append([]PlatformAdapter{}, registry...)
}
