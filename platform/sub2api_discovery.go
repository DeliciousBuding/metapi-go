package platform

import (
	"context"
	"fmt"
	"strings"
)

// GetModels: standard OpenAI-compat endpoints, with API key discovery fallback.
func (s *Sub2ApiAdapter) GetModels(ctx context.Context, baseURL, apiToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	normalized := normalizeBaseURL(baseURL)
	managementBase := s.resolveManagementBaseURL(normalized)

	// Surface a terminal fetch error so callers can classify it (unauthorized
	// vs timeout); otherwise return an explicit empty list for "no models".
	finish := func(err error) ([]string, error) {
		if err != nil {
			return nil, err
		}
		return []string{}, nil
	}

	directModels, directErr := s.fetchModelsByToken(ctx, normalized, apiToken, proxy)
	if len(directModels) > 0 {
		return directModels, nil
	}

	// Session JWT cannot access /v1/models directly; discover a user key first.
	// Filter keys by the user's subscription group (from /api/v1/auth/me) so
	// we pick a key from the SAME group as the user's subscription rather than
	// blindly taking the first enabled key, which may belong to a different
	// group and surface the wrong model set (#675).
	userGroup := ""
	if user, err := s.fetchAuthMe(ctx, normalized, apiToken, proxy); err == nil {
		userGroup = user.group
	}
	tokens, _ := s.GetAPITokens(ctx, managementBase, apiToken, platformUserId, proxy)
	discoveredToken := pickKeyForGroup(tokens, userGroup)
	if discoveredToken == nil {
		return finish(directErr)
	}
	if normalizeTokenKeyForCompare(*discoveredToken) == normalizeTokenKeyForCompare(apiToken) {
		return finish(directErr)
	}

	fallbackModels, fallbackErr := s.fetchModelsByToken(ctx, normalized, *discoveredToken, proxy)
	if len(fallbackModels) > 0 {
		return fallbackModels, nil
	}
	if fallbackErr != nil {
		return finish(fallbackErr)
	}
	return finish(directErr)
}

// GetUserGroups: 5 endpoint fallback + key-based inference.
// GetUserGroups asks the upstream which groups exist.
//
// ["default"] is this family's last resort, not an answer: newapi and oneapi
// both reach it only after every rung answered and none of them refused. This
// adapter used to reach it unconditionally, so an unreachable site, an expired
// credential or an upstream refusal all reported a group the upstream never
// advertised. That is worse here than elsewhere, because the caller prefers any
// non-blank upstream answer over the fallback it already has — so the fabricated
// "default" beat GetLocalTokenGroups' truthful DISTINCT token_group, and the
// operator saw one invented group instead of the real ones.
//
// The refusal message this now surfaces was already written and already
// unreachable for sub2api accounts: resolveGroupFetchErrorMessage renders an
// expired session as "账号会话可能已过期，请重新登录后再拉取分组".
func (s *Sub2ApiAdapter) GetUserGroups(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	normalized := normalizeBaseURL(baseURL)

	// The two rungs are a preference order — ask the group endpoint, else derive
	// from existing keys — so the FIRST reason is kept, the decision
	// modelFetchLadder documents. The opposite of the version-skew ladders inside
	// each rung, which keep the last. Both differences are deliberate; do not
	// "unify" them.
	var terminalError string

	directGroups, err := s.listGroups(ctx, normalized, accessToken, proxy)
	if err != nil {
		terminalError = err.Error()
	}
	if len(directGroups) > 0 {
		return directGroups, nil
	}

	inferredFromKeys, err := s.inferGroupsFromKeys(ctx, normalized, accessToken, proxy)
	if err != nil && terminalError == "" {
		terminalError = err.Error()
	}
	if len(inferredFromKeys) > 0 {
		return inferredFromKeys, nil
	}

	if terminalError != "" {
		return nil, fmt.Errorf("%s", terminalError)
	}

	return []string{"default"}, nil
}

func (s *Sub2ApiAdapter) resolveManagementBaseURL(baseURL string) string {
	normalized := normalizeBaseURL(baseURL)
	if normalized == "" {
		return normalized
	}

	suffixes := []string{
		"/models", "/antigravity", "/antigravity/v1beta", "/antigravity/v1",
		"/api/v1", "/v1beta", "/v1",
	}

	changed := true
	for changed {
		changed = false
		for _, suffix := range suffixes {
			lower := strings.ToLower(normalized)
			if !strings.HasSuffix(lower, suffix) {
				continue
			}
			trimmed := normalizeBaseURL(normalized[:len(normalized)-len(suffix)])
			if trimmed == "" || trimmed == normalized {
				continue
			}
			normalized = trimmed
			changed = true
			break
		}
	}

	return normalized
}
