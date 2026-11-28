package oauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Singleflight Token Refresh ----

var (
	ErrAccountChangedDuringRefresh = errors.New("OAuth account changed during refresh; retry selection")
	ErrOAuthAccountDisabled        = errors.New("OAuth account is disabled")
	refreshInFlight                = make(map[accountRefreshKey]*refreshPromise)
	refreshInFlightMu              sync.Mutex
)

type accountRefreshKey struct {
	db        *store.DB
	accountID int64
}

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
func RefreshAccessTokenSingleflight(db *store.DB, accountID int64) (*refreshResult, error) {
	key := accountRefreshKey{db, accountID}
	refreshInFlightMu.Lock()
	if p, exists := refreshInFlight[key]; exists {
		refreshInFlightMu.Unlock()
		result := p.wait()
		return &result, result.Err
	}

	p := newRefreshPromise()
	refreshInFlight[key] = p
	refreshInFlightMu.Unlock()

	// Guaranteed cleanup even on panic.
	defer func() {
		refreshInFlightMu.Lock()
		delete(refreshInFlight, key)
		refreshInFlightMu.Unlock()
	}()

	result := doRefreshAccessToken(db, accountID)
	p.resolve(result)

	if result.Err != nil {
		return nil, result.Err
	}
	return &result, nil
}

func doRefreshAccessToken(db *store.DB, accountID int64) refreshResult {
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
	return refreshLeasedAccount(ctx, db, lease, account, oauth, def)
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
