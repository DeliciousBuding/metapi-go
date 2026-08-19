package platform

import (
	"context"
)

// VerifyToken verifies a pasted Sub2API credential.
//
// Sub2ApiAdapter deliberately overrides VerifyToken instead of inheriting
// BaseAdapter.VerifyToken: the base implementation dispatches statically to
// BaseAdapter.GetUserInfo (one-api style GET /api/user/self, which does not
// exist on Sub2API) and BaseAdapter.GetModels (always empty), so an inherited
// implementation would report every Sub2API credential as "unknown".
//
// The session path validates the JWT against /api/v1/auth/me and resolves
// user info, balance and the first API key; a pasted API key (sk-...) falls
// back to /v1/models verification with the key-discovery fallback.
func (s *Sub2ApiAdapter) VerifyToken(ctx context.Context, baseURL, token string, platformUserId *int, proxy *ProxyConfig) (*TokenVerifyResult, error) {
	normalized := normalizeBaseURL(baseURL)

	if _, err := s.fetchAuthMe(ctx, normalized, token, proxy); err == nil {
		userInfo, _ := s.GetUserInfo(ctx, normalized, token, platformUserId, proxy)
		balance, _ := s.GetBalance(ctx, normalized, token, platformUserId, proxy)
		apiToken, _ := s.GetAPIToken(ctx, normalized, token, platformUserId, proxy)

		// GetBalance surfaces the auth/me fetch error as a nil BalanceInfo;
		// VerifyToken is best-effort, so keep the result's Balance non-nil.
		if balance == nil {
			balance = &BalanceInfo{}
		}
		apiTokenStr := ""
		if apiToken != nil {
			apiTokenStr = *apiToken
		}
		return &TokenVerifyResult{
			TokenType: "session",
			UserInfo:  userInfo,
			Balance:   balance,
			APIToken:  apiTokenStr,
		}, nil
	}

	models, modelsErr := s.GetModels(ctx, normalized, token, platformUserId, proxy)
	if modelsErr == nil && len(models) > 0 {
		return &TokenVerifyResult{TokenType: "apikey", Models: models}, nil
	}

	return &TokenVerifyResult{TokenType: "unknown"}, nil
}
