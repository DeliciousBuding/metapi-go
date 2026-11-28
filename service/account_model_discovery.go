package service

import (
	"context"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// resolveAccountModelToken picks the credential used for model-list fetches:
// api_token wins over access_token.
func resolveAccountModelToken(account *store.Account) string {
	if account == nil {
		return ""
	}
	if account.APIToken != nil && strings.TrimSpace(*account.APIToken) != "" {
		return strings.TrimSpace(*account.APIToken)
	}
	return strings.TrimSpace(account.AccessToken)
}

// newAPIAccessTokenFallback returns the already-stored New API dashboard token
// only when the relay credential reached an unavailable model-discovery surface.
// Authentication and timeout failures stay terminal: falling back there would
// report a valid management token as proof that a rejected relay key can infer.
func newAPIAccessTokenFallback(sitePlatform string, account *store.Account, primaryToken string, primaryErr error) (string, bool) {
	if sitePlatform != "new-api" || account == nil || primaryErr == nil {
		return "", false
	}
	accessToken := strings.TrimSpace(account.AccessToken)
	if accessToken == "" || accessToken == strings.TrimSpace(primaryToken) {
		return "", false
	}

	if code, _ := classifyModelRefreshError(primaryErr, nil); code == "unauthorized" || code == "timeout" {
		return "", false
	}

	lower := strings.ToLower(primaryErr.Error())
	if strings.Contains(lower, "http 404") ||
		strings.Contains(lower, "http 405") ||
		strings.Contains(lower, "model-list request") {
		return accessToken, true
	}
	return "", false
}

// resolveModelRefreshPlatformUserIDPtr mirrors the pre-Wave-15 handler
// behavior exactly: only ResolvePlatformUserID (extraConfig platformUserId),
// not the richer ResolvePlatformUserIDPtr username guessing.
func resolveModelRefreshPlatformUserIDPtr(extraConfig *string, username *string) *int {
	id := ResolvePlatformUserID(extraConfig, username)
	if id <= 0 {
		return nil
	}
	v := int(id)
	return &v
}

// cleanModelNames trims whitespace and drops empty/case-insensitive
// duplicates, preserving first-seen casing.
func cleanModelNames(models []string) []string {
	seen := map[string]struct{}{}
	clean := make([]string, 0, len(models))
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		key := strings.ToLower(m)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		clean = append(clean, m)
	}
	return clean
}

func classifyModelRefreshError(err error, ctx context.Context) (code, message string) {
	if err == nil {
		return "unknown", "model fetch failed"
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	timedOut := (ctx != nil && ctx.Err() == context.DeadlineExceeded) ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded")
	if timedOut {
		return "timeout", "model fetch failed (request timed out)"
	}
	if strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401") || strings.Contains(lower, "invalid api key") || strings.Contains(lower, "authentication") {
		return "unauthorized", "model fetch failed: API key is invalid"
	}
	if msg == "" {
		msg = "model fetch failed"
	}
	return "upstream_error", msg
}
