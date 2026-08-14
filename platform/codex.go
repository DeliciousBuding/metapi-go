package platform

import (
	"context"
	"strings"
)

// CodexAdapter handles chatgpt.com/backend-api/codex (OAuth-driven).
// StandardAdapter provides the "not supported"/empty defaults for auth,
// checkin and balance methods.
type CodexAdapter struct {
	*StandardAdapter
}

// Detect matches URL keyword: chatgpt.com/backend-api/codex.
func (c *CodexAdapter) Detect(ctx context.Context, url string) (bool, error) {
	normalized := strings.TrimRight(strings.ToLower(strings.TrimSpace(url)), "/")
	return strings.Contains(normalized, "chatgpt.com/backend-api/codex"), nil
}
