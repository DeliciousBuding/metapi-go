package oauth

import (
	"context"
	"errors"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

func exchangeAccountRefresh(ctx context.Context, db *store.DB, family *store.CredentialRefreshFamily, accessToken string, oauth *OauthInfo, def *OAuthProviderDefinition, input RefreshTokenInput) (*TokenSet, bool, error) {
	// This entrypoint explicitly requests a refresh. Reuse another record's
	// successor, but do not turn sequential forced refreshes into cache reads.
	refreshed, err := refreshFamilyWinner(ctx, db.DB, family, oauth.Provider, accessToken, true)
	if err != nil {
		return nil, false, safeAccountRefreshFailure(err)
	}
	reuse := refreshed != nil
	if !reuse {
		refreshed, err = refreshFamilyWithRetry(ctx, family, oauth.Provider, accessToken, func(exchangeCtx context.Context) (*TokenSet, error) {
			return def.RefreshAccessToken(exchangeCtx, input)
		})
	}

	// Chain 2: session-token fallback. When the RT refresh failed with a
	// non-retryable error (invalid_grant etc.) and the provider exposes a
	// session-cookie refresh capability, retry once with the stored web
	// session token. This recovers accounts whose RT was revoked but whose
	// ChatGPT web session is still alive — the same self-heal codex2api does.
	if err != nil && errors.Is(err, errRefreshNotConsumed) && IsNonRetryable(err) && def.RefreshWithSessionToken != nil && strings.TrimSpace(oauth.SessionToken) != "" {
		if beginErr := family.Begin(ctx); beginErr != nil {
			return nil, false, ErrOAuthRefreshUncertain
		}
		sessionRefreshed, sessionErr := def.RefreshWithSessionToken(ctx, SessionTokenInput{
			SessionToken: oauth.SessionToken,
			ProxyURL:     input.ProxyURL,
		})
		if sessionErr == nil {
			refreshed = sessionRefreshed
			err = nil
		} else if failErr := family.Fail(ctx, true); failErr != nil {
			return nil, false, ErrOAuthRefreshUncertain
		}
		// On session-fallback failure, retain the original RT error so the
		// account is marked with the root-cause (invalid_grant), not masked
		// by a session-endpoint transport error.
	}

	if err != nil {
		return nil, false, safeAccountRefreshFailure(err)
	}
	if refreshed == nil || strings.TrimSpace(refreshed.AccessToken) == "" {
		return nil, false, ErrOAuthRefreshUncertain
	}
	if oauth.AccountID != "" && refreshed.AccountID != "" && oauth.AccountID != refreshed.AccountID {
		return nil, false, ErrAccountChangedDuringRefresh
	}
	return refreshed, reuse, nil
}

func safeAccountRefreshFailure(err error) error {
	if errors.Is(err, ErrOAuthRefreshUncertain) {
		return ErrOAuthRefreshUncertain
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	// Reuse the provider's existing classification, but never expose its body.
	// The fixed OAuth error vocabulary retains scheduler recovery semantics.
	if IsNonRetryable(err) {
		message := "OAuth refresh rejected"
		for _, code := range nonRetryableNeedles {
			if strings.Contains(strings.ToLower(err.Error()), code) {
				message += ": " + code
				break
			}
		}
		return &NonRetryableError{Message: message}
	}
	return errors.New("OAuth refresh failed")
}

func resolveAccountProxyURL(siteID int64, extraConfig *string) *string {
	proxyURL := GetProxyURLFromExtraConfig(extraConfig)
	if proxyURL != "" {
		return &proxyURL
	}
	return nil
}
