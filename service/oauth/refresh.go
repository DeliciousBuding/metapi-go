package oauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// ---- Singleflight Token Refresh ----

var (
	ErrAccountChangedDuringRefresh = errors.New("OAuth account changed during refresh; retry selection")
	ErrOAuthAccountDisabled        = errors.New("OAuth account is disabled")
	refreshInFlight                = make(map[int64]*refreshPromise)
	refreshInFlightMu              sync.Mutex
)

type refreshPromise struct {
	done   chan struct{}
	once   sync.Once
	result refreshResult
}

type refreshResult struct {
	AccountID   int64
	AccessToken string
	AccountKey  string
	ExtraConfig *string
	Err         error
}

func newRefreshPromise() *refreshPromise {
	return &refreshPromise{done: make(chan struct{})}
}

func (p *refreshPromise) wait() refreshResult {
	<-p.done
	return p.result
}

func (p *refreshPromise) resolve(result refreshResult) {
	p.once.Do(func() {
		p.result = result
		close(p.done)
	})
}

// RefreshAccessTokenSingleflight refreshes an OAuth access token with singleflight dedup.
func RefreshAccessTokenSingleflight(accountID int64) (*refreshResult, error) {
	refreshInFlightMu.Lock()
	if p, exists := refreshInFlight[accountID]; exists {
		refreshInFlightMu.Unlock()
		result := p.wait()
		return &result, result.Err
	}

	p := newRefreshPromise()
	refreshInFlight[accountID] = p
	refreshInFlightMu.Unlock()

	// Guaranteed cleanup even on panic.
	defer func() {
		refreshInFlightMu.Lock()
		delete(refreshInFlight, accountID)
		refreshInFlightMu.Unlock()
	}()

	result := doRefreshAccessToken(accountID)
	p.resolve(result)

	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func doRefreshAccessToken(accountID int64) refreshResult {
	db := store.GetDB()
	if db == nil {
		return refreshResult{AccountID: accountID, Err: fmt.Errorf("database not initialized")}
	}

	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		return refreshResult{AccountID: accountID, Err: fmt.Errorf("oauth account not found")}
	}
	return refreshManagedAccount(context.Background(), db, account)
}

// The snapshot is taken before waiting for the database owner. A concurrent
// forced refresh must reuse a winner, not consume its newly rotated token.
func refreshManagedAccount(ctx context.Context, db *store.DB, account store.Account) refreshResult {
	accountID := account.ID
	if account.Status == "disabled" {
		return refreshResult{AccountID: accountID, Err: ErrOAuthAccountDisabled}
	}

	oauth := GetOauthInfoFromAccount(&account)
	if oauth == nil || oauth.RefreshToken == "" {
		return refreshResult{AccountID: accountID, Err: fmt.Errorf("oauth refresh token missing")}
	}

	def := GetProviderDefinition(oauth.Provider)
	if def == nil {
		return refreshResult{AccountID: accountID, Err: fmt.Errorf("unsupported oauth provider")}
	}
	if def.RefreshAccessToken == nil {
		return refreshResult{AccountID: accountID, Err: fmt.Errorf("oauth provider cannot refresh")}
	}
	lease, err := store.AcquireCredentialRefreshLease(ctx, db.DB, "account", accountID)
	if err != nil {
		return refreshResult{AccountID: accountID, Err: err}
	}
	defer releaseCredentialRefreshLease(lease)
	if err := ctx.Err(); err != nil {
		return refreshResult{AccountID: accountID, Err: err}
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	var current store.Account
	if err := db.GetContext(ctx, &current, `SELECT * FROM accounts WHERE id=?`, accountID); err != nil {
		return refreshResult{AccountID: accountID, Err: ErrAccountChangedDuringRefresh}
	}
	if current.Status == "disabled" {
		return refreshResult{AccountID: accountID, Err: ErrOAuthAccountDisabled}
	}
	// Only publish an actual persisted successor for the same account identity.
	// A concurrent options-only edit or credential replacement is not a refresh.
	if current.AccessToken != account.AccessToken || current.Status != account.Status ||
		coalesceStrPtr(current.ExtraConfig) != coalesceStrPtr(account.ExtraConfig) ||
		coalesceStrPtr(current.OAuthProvider) != coalesceStrPtr(account.OAuthProvider) ||
		coalesceStrPtr(current.OAuthAccountKey) != coalesceStrPtr(account.OAuthAccountKey) ||
		coalesceStrPtr(current.OAuthProjectID) != coalesceStrPtr(account.OAuthProjectID) {
		next := GetOauthInfoFromAccount(&current)
		if current.Status == "active" && current.AccessToken != "" && current.AccessToken != account.AccessToken && next != nil &&
			next.Provider == oauth.Provider && next.AccountKey == oauth.AccountKey && next.ProjectID == oauth.ProjectID &&
			next.TokenExpiresAt > time.Now().Add(3*time.Minute).UnixMilli() {
			return refreshResult{AccountID: accountID, AccessToken: current.AccessToken, AccountKey: next.AccountKey, ExtraConfig: current.ExtraConfig}
		}
		return refreshResult{AccountID: accountID, Err: ErrAccountChangedDuringRefresh}
	}

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
	// This entrypoint explicitly requests a refresh. Reuse another record's
	// successor, but do not turn sequential forced refreshes into cache reads.
	refreshed, err := refreshFamilyWinner(ctx, db.DB, family, oauth.Provider, account.AccessToken, true)
	if err != nil {
		return refreshResult{AccountID: accountID, Err: safeAccountRefreshFailure(err)}
	}
	reuse := refreshed != nil
	if !reuse {
		refreshed, err = refreshFamilyWithRetry(ctx, family, oauth.Provider, account.AccessToken, func(exchangeCtx context.Context) (*TokenSet, error) {
			return def.RefreshAccessToken(exchangeCtx, RefreshTokenInput{
				RefreshToken: oauth.RefreshToken,
				OAuth:        oauthCtx,
				ProxyURL:     proxyURL,
			})
		})
	}

	// Chain 2: session-token fallback. When the RT refresh failed with a
	// non-retryable error (invalid_grant etc.) and the provider exposes a
	// session-cookie refresh capability, retry once with the stored web
	// session token. This recovers accounts whose RT was revoked but whose
	// ChatGPT web session is still alive — the same self-heal codex2api does.
	if err != nil && errors.Is(err, errRefreshNotConsumed) && IsNonRetryable(err) && def.RefreshWithSessionToken != nil && strings.TrimSpace(oauth.SessionToken) != "" {
		if beginErr := family.Begin(ctx); beginErr != nil {
			return refreshResult{AccountID: accountID, Err: ErrOAuthRefreshUncertain}
		}
		sessionRefreshed, sessionErr := def.RefreshWithSessionToken(ctx, SessionTokenInput{
			SessionToken: oauth.SessionToken,
			ProxyURL:     proxyURL,
		})
		if sessionErr == nil {
			refreshed = sessionRefreshed
			err = nil
		} else if failErr := family.Fail(ctx, true); failErr != nil {
			return refreshResult{AccountID: accountID, Err: ErrOAuthRefreshUncertain}
		}
		// On session-fallback failure, retain the original RT error so the
		// account is marked with the root-cause (invalid_grant), not masked
		// by a session-endpoint transport error.
	}

	if err != nil {
		return refreshResult{AccountID: accountID, Err: safeAccountRefreshFailure(err)}
	}
	if refreshed == nil || strings.TrimSpace(refreshed.AccessToken) == "" {
		return refreshResult{AccountID: accountID, Err: ErrOAuthRefreshUncertain}
	}
	if oauth.AccountID != "" && refreshed.AccountID != "" && oauth.AccountID != refreshed.AccountID {
		return refreshResult{AccountID: accountID, Err: ErrAccountChangedDuringRefresh}
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

func coalesceStrPtr(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func releaseCredentialRefreshLease(lease *store.CredentialRefreshLease) {
	if err := lease.Release(); err != nil {
		// Publication is already settled. The database TTL permits recovery;
		// do not undo a committed rotation or log an SQL error containing DSNs.
		slog.Warn("OAuth refresh lease release failed; expiry will recover ownership")
	}
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

func resolveAccountProxyURL(siteID int64, extraConfig *string) *string {
	proxyURL := GetProxyURLFromExtraConfig(extraConfig)
	if proxyURL != "" {
		return &proxyURL
	}
	return nil
}
