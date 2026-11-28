package oauth

import (
	"context"
	"errors"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

func refreshLeasedAccount(ctx context.Context, db *store.DB, lease *store.CredentialRefreshLease, account store.Account, oauth *OauthInfo, def *OAuthProviderDefinition) refreshResult {
	accountID := account.ID
	var oauthCtx *RefreshOAuthContext
	if oauth.ProjectID != "" || oauth.ProviderData != nil {
		oauthCtx = &RefreshOAuthContext{
			ProjectID:    oauth.ProjectID,
			ProviderData: oauth.ProviderData,
		}
	}

	proxyURL := resolveAccountProxyURL(account.SiteID, account.ExtraConfig)

	family, err := acquireRefreshFamily(ctx, db.DB, oauth.Provider, "", oauth.RefreshToken)
	if err != nil {
		return refreshResult{AccountID: accountID, Err: safeAccountRefreshFailure(err)}
	}
	defer releaseRefreshFamily(family)
	published := false
	defer func() {
		if !published && family.State == "refreshing" {
			markRefreshFamilyUncertain(ctx, family)
		}
	}()
	refreshed, reuse, err := exchangeAccountRefresh(ctx, db, family, account.AccessToken, oauth, def, RefreshTokenInput{
		RefreshToken: oauth.RefreshToken,
		OAuth:        oauthCtx,
		ProxyURL:     proxyURL,
	})
	if err != nil {
		return refreshResult{AccountID: accountID, Err: err}
	}

	// Merge refreshed fields with existing.
	nextOauth, err := BuildOauthInfoFromAccount(&account, &OauthInfo{
		Provider:   oauth.Provider,
		AccountID:  coalesceStr(refreshed.AccountID, oauth.AccountID),
		AccountKey: coalesceStr(refreshed.AccountKey, oauth.AccountKey, refreshed.AccountID, oauth.AccountID),
		Email:      coalesceStr(refreshed.Email, oauth.Email),
		PlanType:   coalesceStr(refreshed.PlanType, oauth.PlanType),
		ProjectID: func() string {
			if reuse {
				return oauth.ProjectID
			}
			return coalesceStr(refreshed.ProjectID, oauth.ProjectID)
		}(),
		RefreshToken:   coalesceStr(refreshed.RefreshToken, oauth.RefreshToken),
		TokenExpiresAt: coalesceInt64(refreshed.TokenExpiresAt, oauth.TokenExpiresAt),
		IDToken:        coalesceStr(refreshed.IDToken, oauth.IDToken),
		ProviderData:   mergeProviderData(oauth.ProviderData, refreshed.ProviderData),
	})
	if err != nil {
		return refreshResult{AccountID: accountID, Err: err}
	}

	oauthMap := map[string]interface{}{
		"email":          nextOauth.Email,
		"planType":       nextOauth.PlanType,
		"tokenExpiresAt": nextOauth.TokenExpiresAt,
		"refreshToken":   nextOauth.RefreshToken,
		"idToken":        nextOauth.IDToken,
		"sessionToken":   nextOauth.SessionToken,
	}
	if nextOauth.ProviderData != nil {
		oauthMap["providerData"] = nextOauth.ProviderData
	}

	extraPatch := map[string]interface{}{
		"credentialMode": "session",
		"oauth":          oauthMap,
	}

	extraConfig := MergeAccountExtraConfig(account.ExtraConfig, extraPatch)
	accountKey := nextOauth.AccountKey
	if accountKey == "" {
		accountKey = nextOauth.AccountID
	}
	projectID := nextOauth.ProjectID

	now := time.Now().UTC().Format(time.RFC3339)
	verify := func(checkCtx context.Context) error {
		return verifyAccountRefreshPublication(checkCtx, db, accountID, refreshed.AccessToken, oauth.Provider, accountKey, projectID, extraConfig)
	}
	err = publishRefreshedCredential(ctx, family, lease, reuse, store.OAuthRefreshFingerprint(family.Provider, nextOauth.RefreshToken), "account", accountID, ErrAccountChangedDuringRefresh, func(tx *sqlx.Tx) error {
		updated, err := tx.ExecContext(ctx, db.Rebind(
			`UPDATE accounts SET access_token = ?, oauth_provider = ?, oauth_account_key = ?,
		 oauth_project_id = ?, extra_config = ?, status = 'active', updated_at = ?
		 WHERE id = ? AND access_token = ? AND status = ?
		 AND COALESCE(oauth_provider,'') = COALESCE(?,'')
		 AND COALESCE(oauth_account_key,'') = COALESCE(?,'')
		 AND COALESCE(oauth_project_id,'') = COALESCE(?,'')
		 AND COALESCE(extra_config,'') = COALESCE(?,'')`),
			refreshed.AccessToken, oauth.Provider, accountKey, projectID,
			extraConfig, now, accountID, account.AccessToken, account.Status,
			account.OAuthProvider, account.OAuthAccountKey, account.OAuthProjectID, account.ExtraConfig,
		)
		if err != nil {
			return err
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrAccountChangedDuringRefresh
		}
		return nil
	}, verify)
	if err != nil {
		if errors.Is(err, ErrAccountChangedDuringRefresh) {
			return refreshResult{AccountID: accountID, Err: err}
		}
		return refreshResult{AccountID: accountID, Err: safeAccountRefreshFailure(err)}
	}
	published = true
	// Do not publish a token for a deleted/disabled or superseded record. The
	// current account identity remains authoritative after the provider call.
	if verify(ctx) != nil {
		return refreshResult{AccountID: accountID, Err: ErrAccountChangedDuringRefresh}
	}

	return refreshResult{
		AccountID:   accountID,
		AccessToken: refreshed.AccessToken,
		AccountKey:  accountKey,
		ExtraConfig: extraConfig,
	}
}

func verifyAccountRefreshPublication(ctx context.Context, db *store.DB, id int64, accessToken, provider, accountKey, projectID string, extraConfig *string) error {
	var current store.Account
	if err := db.GetContext(ctx, &current, `SELECT * FROM accounts WHERE id=?`, id); err != nil || current.Status != "active" || current.AccessToken != accessToken ||
		current.OAuthProvider == nil || *current.OAuthProvider != provider || current.OAuthAccountKey == nil || *current.OAuthAccountKey != accountKey ||
		current.OAuthProjectID == nil || *current.OAuthProjectID != projectID || current.ExtraConfig == nil || extraConfig == nil || *current.ExtraConfig != *extraConfig {
		return ErrAccountChangedDuringRefresh
	}
	return nil
}

func coalesceStr(values ...string) string {
	for _, v := range values {
		if stringsTrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func coalesceInt64(values ...int64) int64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func mergeProviderData(existing, refreshed map[string]interface{}) map[string]interface{} {
	if existing == nil && refreshed == nil {
		return nil
	}
	result := make(map[string]interface{})
	for k, v := range existing {
		result[k] = v
	}
	for k, v := range refreshed {
		result[k] = v
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func stringsTrimSpace(s string) string {
	start, end := 0, len(s)
	for start < end {
		c := s[start]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		start++
	}
	for end > start {
		c := s[end-1]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			break
		}
		end--
	}
	return s[start:end]
}
