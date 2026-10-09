package oauth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var (
	ErrDirectCredentialUnavailable = errors.New("direct credential unavailable")
	ErrDirectCredentialExpired     = errors.New("direct OAuth credential expired; replace or refresh the credential")
	ErrDirectCredentialRefresh     = errors.New("direct OAuth refresh failed")
	ErrDirectCredentialRevoked     = errors.New("direct OAuth refresh rejected; replace the credential")
	ErrDirectCredentialChanged     = errors.New("direct credential changed during refresh; retry selection")
)

type DirectCredentialResult struct {
	AccessToken string `json:"-"`
	Kind        string `json:"kind"`
	Provider    string `json:"provider"`
	AccountID   string `json:"accountId,omitempty"`
}

func (DirectCredentialResult) String() string { return "DirectCredentialResult([REDACTED])" }

type directCredentialRow struct {
	Secret         string                 `db:"secret"`
	Kind           string                 `db:"kind"`
	RawOAuth       string                 `db:"oauth_state"`
	Provider       string                 `db:"provider"`
	Enabled        bool                   `db:"enabled"`
	ChannelEnabled bool                   `db:"channel_enabled"`
	State          store.DirectOAuthState `db:"-"`
}

type directRefreshKey struct {
	db *sqlx.DB
	id int64
}
type directRefreshPromise struct {
	done   chan struct{}
	result *DirectCredentialResult
	err    error
}

var directRefreshMu sync.Mutex
var directRefreshes = map[directRefreshKey]*directRefreshPromise{}

func directOAuthProvider(provider string) string {
	switch provider {
	case "codex", "fenno":
		return string(ProviderCodex)
	case "claudecode":
		return string(ProviderClaude)
	default:
		return ""
	}
}

func loadDirectCredential(ctx context.Context, db *sqlx.DB, id int64) (*directCredentialRow, error) {
	if db == nil || id <= 0 {
		return nil, ErrDirectCredentialUnavailable
	}
	var row directCredentialRow
	if err := db.GetContext(ctx, &row, db.Rebind(`SELECT k.secret,k.kind,k.oauth_state,k.enabled,c.provider,c.enabled AS channel_enabled FROM upstream_credentials k JOIN upstream_channels c ON c.id=k.channel_id WHERE k.id=?`), id); err != nil {
		return nil, ErrDirectCredentialUnavailable
	}
	if !row.Enabled || !row.ChannelEnabled || strings.TrimSpace(row.Secret) == "" {
		return nil, ErrDirectCredentialUnavailable
	}
	if row.Kind != store.DirectCredentialAPIKey && row.Kind != store.DirectCredentialOAuth {
		return nil, ErrDirectCredentialUnavailable
	}
	if row.Kind == store.DirectCredentialOAuth {
		if directOAuthProvider(row.Provider) == "" || row.State.Scan(row.RawOAuth) != nil {
			return nil, ErrDirectCredentialUnavailable
		}
	}
	return &row, nil
}

func directCredentialResult(row *directCredentialRow) *DirectCredentialResult {
	accountID := row.State.AccountID
	if accountID == "" && directOAuthProvider(row.Provider) == string(ProviderCodex) {
		if identity := ParseCodexAccessToken(row.Secret); identity != nil {
			accountID = identity.ChatGPTAccountID
		}
	}
	return &DirectCredentialResult{AccessToken: row.Secret, Kind: row.Kind, Provider: row.Provider, AccountID: accountID}
}

// ResolveDirectCredential reads current storage after selection. Refresh tokens
// never enter route caches. Concurrent requests share one refresh per DB/id;
// caller cancellation cannot cancel another request's refresh.
func ResolveDirectCredential(ctx context.Context, db *sqlx.DB, id int64, proxyURL *string, forceRefresh bool) (*DirectCredentialResult, error) {
	row, err := loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if row.Kind == store.DirectCredentialAPIKey {
		return directCredentialResult(row), nil
	}
	now := time.Now().UnixMilli()
	if !forceRefresh && row.State.ExpiresAt > now+3*60*1000 {
		return directCredentialResult(row), nil
	}
	if row.State.RefreshToken == "" {
		if !forceRefresh && (row.State.ExpiresAt == 0 || row.State.ExpiresAt > now) {
			return directCredentialResult(row), nil
		}
		return nil, ErrDirectCredentialExpired
	}
	key := directRefreshKey{db: db, id: id}
	directRefreshMu.Lock()
	promise := directRefreshes[key]
	if promise == nil {
		promise = &directRefreshPromise{done: make(chan struct{})}
		directRefreshes[key] = promise
		go func() {
			refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			promise.result, promise.err = refreshDirectCredential(refreshCtx, db, id, proxyURL, forceRefresh)
			directRefreshMu.Lock()
			delete(directRefreshes, key)
			close(promise.done)
			directRefreshMu.Unlock()
		}()
	}
	directRefreshMu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-promise.done:
		return promise.result, promise.err
	}
}

func refreshDirectCredential(ctx context.Context, db *sqlx.DB, id int64, proxyURL *string, force bool) (*DirectCredentialResult, error) {
	row, err := loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != store.DirectCredentialOAuth || (!force && row.State.ExpiresAt > time.Now().Add(3*time.Minute).UnixMilli()) {
		return directCredentialResult(row), nil
	}
	def := GetProviderDefinition(directOAuthProvider(row.Provider))
	if def == nil || def.RefreshAccessToken == nil || row.State.RefreshToken == "" {
		return nil, ErrDirectCredentialUnavailable
	}
	fresh, err := RefreshWithRetry(ctx, func() (*TokenSet, error) {
		return def.RefreshAccessToken(ctx, RefreshTokenInput{RefreshToken: row.State.RefreshToken, ClientID: row.State.ClientID, ProxyURL: proxyURL})
	})
	if err != nil {
		// Providers can return response bodies containing tokens. Never return
		// their raw errors to proxy logging or management endpoints.
		if IsNonRetryable(err) {
			return nil, ErrDirectCredentialRevoked
		}
		return nil, ErrDirectCredentialRefresh
	}
	if fresh == nil || strings.TrimSpace(fresh.AccessToken) == "" || fresh.TokenExpiresAt <= time.Now().UnixMilli() {
		return nil, ErrDirectCredentialRefresh
	}
	next := row.State
	next.RefreshToken = coalesceStr(fresh.RefreshToken, next.RefreshToken)
	next.IDToken = coalesceStr(fresh.IDToken, next.IDToken)
	next.AccountID = coalesceStr(fresh.AccountID, next.AccountID)
	next.ExpiresAt = fresh.TokenExpiresAt
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE upstream_credentials SET secret=?,oauth_state=? WHERE id=? AND kind=? AND enabled=? AND secret=? AND oauth_state=?`), fresh.AccessToken, next, id, store.DirectCredentialOAuth, true, row.Secret, row.RawOAuth)
	if err != nil {
		return nil, ErrDirectCredentialRefresh
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, ErrDirectCredentialRefresh
	}
	if count != 1 {
		return nil, ErrDirectCredentialChanged
	}
	// Also recheck channel/credential enablement after the refresh. A source
	// replacement must never authorize sending the just-superseded token.
	current, err := loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	return directCredentialResult(current), nil
}
