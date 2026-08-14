package platform

import (
	"context"
	"net/url"
	"strings"
)

// GrokAdapter handles xAI Grok platforms (api.x.ai, OAuth-driven).
// StandardAdapter provides the "not supported"/empty defaults for auth,
// checkin and balance methods — Grok credentials are managed via the
// Device OAuth flow registered in service/oauth/grok.go.
type GrokAdapter struct {
	*StandardAdapter
}

// grokSeedModels is the hardcoded model list returned by GetModels when no
// upstream /v1/models probe is available. Mirrors the model surface exposed
// by xAI's public API; update here when xAI ships new Grok variants.
var grokSeedModels = []string{
	"grok-3",
	"grok-3-mini",
	"grok-3-fast",
	"grok-2",
	"grok-2-mini",
	"grok-2-vision",
	"grok-2-latest",
}

// Detect matches the xAI API host. xAI serves its public API from api.x.ai;
// the bare x.ai host is also accepted so users may register either form.
func (g *GrokAdapter) Detect(ctx context.Context, urlStr string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(urlStr))
	if err != nil {
		return false, nil
	}
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "api.x.ai" || hostname == "x.ai" {
		return true, nil
	}
	return false, nil
}

// GetModels returns the hardcoded Grok seed model list. xAI's public API does
// expose /v1/models, but the seed list keeps detection/verification working
// without a live upstream call (mirrors the Codex "empty by default" contract
// while still surfacing a useful model catalog for auto-complete).
func (g *GrokAdapter) GetModels(ctx context.Context, baseURL string, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	seed := make([]string, len(grokSeedModels))
	copy(seed, grokSeedModels)
	return normalizeModelIDs(seed), nil
}
