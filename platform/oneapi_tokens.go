package platform

import (
	"context"
	"fmt"
	"strings"
)

// GetAPITokens: GET /api/token/?p=0&size=100 (Bearer auth).
func (o *OneApiAdapter) GetAPITokens(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, baseURL+apiTokenListPath(), "GET", nil, headers, proxy)
	if err != nil {
		// Propagate: an unreachable or unauthorized token listing is not the same
		// fact as "this account has no tokens". Returning an empty list here made
		// a failed sync report zero tokens and no error, so the operator saw a
		// successful sync of nothing.
		return nil, fmt.Errorf("list upstream tokens: %w", err)
	}

	items := parseTokenItemsFromMap(resp)
	if len(items) == 0 {
		return []ApiTokenInfo{}, nil // the upstream answered: genuinely no tokens
	}
	return normalizeTokenItems(items), nil
}

// GetAPIToken returns the first enabled token.
func (o *OneApiAdapter) GetAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := o.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		// Propagate: GetAPITokens now distinguishes "the upstream answered with no
		// tokens" (nil, nil below) from "we could not ask". Returning nil, nil here
		// re-hid the failure one level up, which is why the empty answer survived.
		return nil, err
	}
	return findFirstEnabledToken(tokens), nil
}

// CreateAPIToken: POST /api/token/ (Bearer auth).
//
// A failed fetch is an error, not "not created". The caller answers 502 for both,
// but only the error path logs the reason and puts it in the response, so
// `false, nil` here reported "we asked and the upstream said no" for a call that
// never reached anyone. An answered `success:false` stays (false, nil): that is
// the upstream's own verdict.
func (o *OneApiAdapter) CreateAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, options *CreateAPITokenOptions, proxy *ProxyConfig) (bool, error) {
	payload := buildDefaultTokenPayload(options)
	headers := authBearerHeaders(accessToken)
	resp, err := fetchJSON(ctx, baseURL+"/api/token/", "POST", payload, headers, proxy)
	if err != nil {
		return false, fmt.Errorf("create upstream token: %w", err)
	}
	success, _ := getBool(resp, "success")
	return success, nil
}

// DeleteAPIToken: list -> find key -> DELETE /api/token/{id} (with trailing-slash fallback).
//
// nil means "that key is gone upstream, or was never there". A failed listing and
// a failed delete are both errors: the caller deletes the local row only when this
// returns nil, so reporting success for a call that never landed left a live
// upstream credential with no local row tracking it. NewApiAdapter.DeleteAPIToken
// in this package already drew that line.
func (o *OneApiAdapter) DeleteAPIToken(ctx context.Context, baseURL, accessToken, tokenKey string, platformUserId *int, proxy *ProxyConfig) error {
	targetKey := normalizeTokenKeyForCompare(tokenKey)
	if targetKey == "" {
		return nil // nothing to match upstream
	}

	headers := authBearerHeaders(accessToken)

	// List tokens
	resp, err := fetchJSON(ctx, baseURL+apiTokenListPath(), "GET", nil, headers, proxy)
	if err != nil {
		return fmt.Errorf("list upstream tokens: %w", err)
	}

	items := parseTokenItemsFromMap(resp)
	tokenID := pickTokenID(items, targetKey)
	if tokenID == nil {
		return nil // already absent upstream: nothing to delete
	}

	// Double-DELETE: OneApi serves this route with and without the trailing slash,
	// so one of the two failing is expected. Both failing is not.
	reason := ""
	for _, url := range []string{
		fmt.Sprintf("%s/api/token/%d", baseURL, *tokenID),
		fmt.Sprintf("%s/api/token/%d/", baseURL, *tokenID),
	} {
		delResp, err := fetchJSON(ctx, url, "DELETE", nil, headers, proxy)
		if err != nil {
			reason = err.Error()
			continue
		}
		if success, _ := getBool(delResp, "success"); success {
			return nil
		}
		reason, _ = getString(delResp, "message")
		if strings.TrimSpace(reason) == "" {
			reason = "upstream reported failure"
		}
	}
	return fmt.Errorf("delete upstream token %d: %s", *tokenID, reason)
}

func buildDefaultTokenPayload(options *CreateAPITokenOptions) map[string]interface{} {
	name := "metapi"
	if options != nil && strings.TrimSpace(options.Name) != "" {
		name = strings.TrimSpace(options.Name)
	}

	unlimitedQuota := true
	if options != nil {
		unlimitedQuota = options.UnlimitedQuota
	}

	remainQuota := 0.0
	if options != nil && options.RemainQuota > 0 {
		remainQuota = options.RemainQuota
	}

	expiredTime := int64(-1)
	if options != nil && options.ExpiredTime != 0 {
		expiredTime = options.ExpiredTime
	}

	allowIPs := ""
	modelLimits := ""
	group := ""
	modelLimitsEnabled := false
	if options != nil {
		allowIPs = options.AllowIPs
		modelLimits = options.ModelLimits
		group = options.Group
		modelLimitsEnabled = options.ModelLimitsEnabled
	}

	return map[string]interface{}{
		"name":                 name,
		"unlimited_quota":      unlimitedQuota,
		"expired_time":         expiredTime,
		"remain_quota":         remainQuota,
		"allow_ips":            allowIPs,
		"model_limits_enabled": modelLimitsEnabled,
		"model_limits":         modelLimits,
		"group":                group,
	}
}
