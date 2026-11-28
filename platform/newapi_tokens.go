package platform

import (
	"context"
	"errors"
	"fmt"
)

var (
	errNewAPITokenListingUnavailable = errors.New("new-api token listing unavailable")

	// ErrNewAPITokenListingCompletenessUnproven means the legacy listing
	// answered but did not expose a total that can prove the snapshot complete.
	// Callers may still use the tokens for synchronization, but must not use
	// their absence to converge the default relay credential.
	ErrNewAPITokenListingCompletenessUnproven = errors.New("new-api token listing completeness unproven")
)

func (n *NewApiAdapter) GetAPIToken(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*string, error) {
	tokens, err := n.GetAPITokens(ctx, baseURL, accessToken, platformUserId, proxy)
	if err != nil {
		return nil, err
	}
	return findFirstEnabledToken(tokens), nil
}

func (n *NewApiAdapter) GetAPITokens(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, error) {
	tokens, _, err := n.getAPITokensWithUserComplete(ctx, baseURL, accessToken, platformUserId, proxy)
	if errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		return tokens, nil
	}
	return tokens, err
}

// GetAPITokensComplete is the explicit completeness contract consumed by the
// token-sync convergence path. A legacy flat response without a total is
// returned with ErrNewAPITokenListingCompletenessUnproven so callers can sync
// the tokens but must not converge the default from their absence.
func (n *NewApiAdapter) GetAPITokensComplete(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) ([]ApiTokenInfo, bool, error) {
	return n.getAPITokensWithUserComplete(ctx, baseURL, accessToken, platformUserId, proxy)
}

func (n *NewApiAdapter) getAPITokenWithUser(ctx context.Context, baseURL, accessToken string, userID *int, proxy *ProxyConfig) (*string, error) {
	tokens, _, err := n.getAPITokensWithUserComplete(ctx, baseURL, accessToken, userID, proxy)
	if errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		err = nil
	}
	if err != nil {
		if errors.Is(err, errNewAPITokenListingUnavailable) {
			return nil, nil
		}
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	return findFirstEnabledToken(tokens), nil
}

func (n *NewApiAdapter) getAPITokensWithUserComplete(ctx context.Context, baseURL, accessToken string, userID *int, proxy *ProxyConfig) ([]ApiTokenInfo, bool, error) {
	// Try Bearer auth. A successful first page is authoritative: later-page or
	// ownership-hydration failures must surface, not fall through to another
	// credential shape and return a partial list.
	headers := n.authHeaders(accessToken, userID)
	tokens, started, complete, err := n.fetchNewAPITokens(ctx, baseURL, headers, proxy)
	if err == nil || errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		return tokens, complete, err
	}
	if started {
		return nil, false, err
	}

	// Cookie fallback is a transport-shape compatibility path for the same
	// stored credential. Do not probe a different user identity after failure.
	cookieTokens, cookieStarted, cookieComplete, cookieErr := n.getAPITokensByCookie(ctx, baseURL, accessToken, userID, proxy)
	if cookieErr == nil || errors.Is(cookieErr, ErrNewAPITokenListingCompletenessUnproven) {
		return cookieTokens, cookieComplete, cookieErr
	}
	if cookieStarted {
		return nil, false, cookieErr
	}

	if cookieErr != nil {
		err = cookieErr
	}
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", errNewAPITokenListingUnavailable, err)
	}
	return []ApiTokenInfo{}, false, nil
}

func (n *NewApiAdapter) getAPITokensByCookie(ctx context.Context, baseURL, token string, userID *int, proxy *ProxyConfig) ([]ApiTokenInfo, bool, bool, error) {
	var lastErr error
	for _, cookie := range buildCookieCandidates(token) {
		headers := n.cookieHeaders(cookie, userID)

		tokens, started, complete, err := n.fetchNewAPITokens(ctx, baseURL, headers, proxy)
		if err == nil || errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
			return tokens, started, complete, err
		}
		if started {
			return nil, true, false, err
		}
		lastErr = err
	}
	return nil, false, false, lastErr
}

func (n *NewApiAdapter) fetchNewAPITokens(ctx context.Context, baseURL string, headers map[string]string, proxy *ProxyConfig) ([]ApiTokenInfo, bool, bool, error) {
	items, started, complete, err := n.fetchNewAPITokenItems(ctx, baseURL, headers, proxy)
	if err != nil && !errors.Is(err, ErrNewAPITokenListingCompletenessUnproven) {
		return nil, started, false, err
	}
	normalized, err := n.normalizeListedTokens(ctx, baseURL, items, headers, proxy)
	if err != nil {
		return nil, true, false, err
	}
	if complete {
		return normalized, true, true, nil
	}
	return normalized, true, false, ErrNewAPITokenListingCompletenessUnproven
}
