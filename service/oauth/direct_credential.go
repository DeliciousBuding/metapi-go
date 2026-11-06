package oauth

import (
	"context"
	"errors"
	"github.com/deliciousbuding/metapi-go/internal/gcpsa"
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
	ProjectID   string `json:"projectId,omitempty"`
}

func (DirectCredentialResult) String() string { return "DirectCredentialResult([REDACTED])" }

type directCredentialRow struct {
	ChannelID      int64                  `db:"channel_id"`
	Secret         string                 `db:"secret"`
	Kind           string                 `db:"kind"`
	RawOAuth       string                 `db:"oauth_state"`
	GCPMaterial    *string                `db:"gcp_service_account"`
	Provider       string                 `db:"provider"`
	Enabled        bool                   `db:"enabled"`
	ChannelEnabled bool                   `db:"channel_enabled"`
	State          store.DirectOAuthState `db:"-"`
	RawEndpoints   string                 `db:"endpoint_config"`
	Endpoints      store.DirectEndpoints  `db:"-"`
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
	case "antigravity":
		return string(ProviderAntigravity)
	case "github_copilot":
		return string(ProviderCopilot)
	case "xai_subscription":
		return string(ProviderGrok)
	default:
		return ""
	}
}

func loadDirectCredential(ctx context.Context, db *sqlx.DB, id int64) (*directCredentialRow, error) {
	if db == nil || id <= 0 {
		return nil, ErrDirectCredentialUnavailable
	}
	var row directCredentialRow
	if err := db.GetContext(ctx, &row, db.Rebind(`SELECT k.channel_id,k.secret,k.kind,k.oauth_state,k.gcp_service_account,k.enabled,c.provider,c.endpoint_config,c.enabled AS channel_enabled FROM upstream_credentials k JOIN upstream_channels c ON c.id=k.channel_id WHERE k.id=?`), id); err != nil {
		return nil, ErrDirectCredentialUnavailable
	}
	if !row.Enabled || !row.ChannelEnabled {
		return nil, ErrDirectCredentialUnavailable
	}
	if row.Endpoints.Scan(row.RawEndpoints) != nil {
		return nil, ErrDirectCredentialUnavailable
	}
	if row.Kind == store.DirectCredentialNone {
		if !store.DirectProviderAllowsAnonymous(row.Provider) || row.Endpoints.AnonymousProtocolMask() == 0 || row.Secret != "" || row.State.Scan(row.RawOAuth) != nil || row.State != (store.DirectOAuthState{}) {
			return nil, ErrDirectCredentialUnavailable
		}
		return &row, nil
	}
	if row.Kind == store.DirectCredentialGCPServiceAccount {
		// The live token cache may be empty until the first exchange; the
		// material is what makes the credential resolvable.
		if row.GCPMaterial == nil {
			return nil, ErrDirectCredentialUnavailable
		}
		if _, _, err := gcpsa.Parse(*row.GCPMaterial); err != nil {
			return nil, ErrDirectCredentialUnavailable
		}
		if row.State.Scan(row.RawOAuth) != nil {
			return nil, ErrDirectCredentialUnavailable
		}
		return &row, nil
	}
	if row.Kind != store.DirectCredentialAPIKey && row.Kind != store.DirectCredentialOAuth {
		return nil, ErrDirectCredentialUnavailable
	}
	if row.Provider == "antigravity" && row.Kind != store.DirectCredentialOAuth {
		return nil, ErrDirectCredentialUnavailable
	}
	if row.Kind == store.DirectCredentialOAuth {
		if directOAuthProvider(row.Provider) == "" || row.State.Scan(row.RawOAuth) != nil {
			return nil, ErrDirectCredentialUnavailable
		}
		// Copilot stores the durable GitHub token only in oauth_state; the
		// short-lived Copilot token is exchanged lazily into secret.
		if strings.TrimSpace(row.Secret) == "" && directOAuthProvider(row.Provider) != string(ProviderCopilot) &&
			directOAuthProvider(row.Provider) != string(ProviderAntigravity) {
			return nil, ErrDirectCredentialUnavailable
		}
		if row.Provider == "antigravity" {
			if _, err := ParseAntigravityImportMaterial(row.State.RefreshToken + "|" + row.State.ProjectID); err != nil || row.State.ClientID != antigravityClientID {
				return nil, ErrDirectCredentialUnavailable
			}
		}
	} else if strings.TrimSpace(row.Secret) == "" {
		return nil, ErrDirectCredentialUnavailable
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
	return &DirectCredentialResult{AccessToken: row.Secret, Kind: row.Kind, Provider: row.Provider, AccountID: accountID, ProjectID: row.State.ProjectID}
}

// ResolveDirectCredential reads current storage after selection. Refresh tokens
// never enter route caches. Concurrent requests share one refresh per DB/id;
// caller cancellation cannot cancel another request's refresh.
func ResolveDirectCredential(ctx context.Context, db *sqlx.DB, id int64, proxyURL *string, forceRefresh bool) (*DirectCredentialResult, error) {
	row, err := loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if row.Kind == store.DirectCredentialAPIKey || row.Kind == store.DirectCredentialNone {
		return directCredentialResult(row), nil
	}
	if row.Kind == store.DirectCredentialGCPServiceAccount && row.State.RefreshToken == "" {
		// Cached-token fast path; the slow path exchanges via the singleflight
		// below. The refresh-token distinction does not apply to SA exchange.
		now := time.Now().UnixMilli()
		if !forceRefresh && row.State.ExpiresAt > now+3*60*1000 && row.Secret != "" {
			return directCredentialResult(row), nil
		}
		return resolveGCPCredential(ctx, db, id, proxyURL)
	}
	now := time.Now().UnixMilli()
	if !forceRefresh && row.Secret != "" && row.State.ExpiresAt > now+3*60*1000 {
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

// resolveGCPCredential exchanges the service-account material for an access
// token under the shared per-credential singleflight, then caches it in
// secret + oauth_state.expiresAt with the same optimistic-concurrency guard
// as OAuth refresh.
func resolveGCPCredential(ctx context.Context, db *sqlx.DB, id int64, proxyURL *string) (*DirectCredentialResult, error) {
	key := directRefreshKey{db: db, id: id}
	directRefreshMu.Lock()
	promise := directRefreshes[key]
	if promise == nil {
		promise = &directRefreshPromise{done: make(chan struct{})}
		directRefreshes[key] = promise
		go func() {
			exchangeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			promise.result, promise.err = exchangeGCPCredential(exchangeCtx, db, id, proxyURL)
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

func exchangeGCPCredential(ctx context.Context, db *sqlx.DB, id int64, proxyURL *string) (*DirectCredentialResult, error) {
	row, err := loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != store.DirectCredentialGCPServiceAccount || row.GCPMaterial == nil {
		return nil, ErrDirectCredentialUnavailable
	}
	token, expiresAt, err := gcpsa.Exchange(ctx, *row.GCPMaterial, proxyURL)
	if err != nil {
		if errors.Is(err, gcpsa.ErrExchangeRejected) {
			return nil, ErrDirectCredentialRevoked
		}
		return nil, ErrDirectCredentialRefresh
	}
	next := store.DirectOAuthState{ExpiresAt: expiresAt}
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE upstream_credentials SET secret=?,oauth_state=? WHERE id=? AND kind=? AND enabled=? AND secret=? AND oauth_state=?`), token, next, id, store.DirectCredentialGCPServiceAccount, true, row.Secret, row.RawOAuth)
	if err != nil {
		return nil, ErrDirectCredentialRefresh
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return nil, ErrDirectCredentialChanged
	}
	// Recheck enablement after the exchange; a mid-flight replacement must
	// never authorize the just-superseded token.
	current, err := loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	return directCredentialResult(current), nil
}

func refreshDirectCredential(ctx context.Context, db *sqlx.DB, id int64, proxyURL *string, force bool) (*DirectCredentialResult, error) {
	row, err := loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	return refreshDirectCredentialSnapshot(ctx, db, id, proxyURL, force, row)
}

func refreshDirectCredentialSnapshot(ctx context.Context, db *sqlx.DB, id int64, proxyURL *string, force bool, row *directCredentialRow) (*DirectCredentialResult, error) {
	if row.Kind != store.DirectCredentialOAuth || (!force && row.Secret != "" && row.State.ExpiresAt > time.Now().Add(3*time.Minute).UnixMilli()) {
		return directCredentialResult(row), nil
	}
	snapshot := *row
	lease, err := store.AcquireCredentialRefreshLease(ctx, db, "direct", id)
	if err != nil {
		return nil, ErrDirectCredentialRefresh
	}
	defer releaseCredentialRefreshLease(lease)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	criticalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
	defer cancel()
	ctx = criticalCtx
	row, err = loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	if row.Kind != snapshot.Kind || row.Provider != snapshot.Provider || row.ChannelID != snapshot.ChannelID || row.RawEndpoints != snapshot.RawEndpoints {
		return nil, ErrDirectCredentialChanged
	}
	if row.Secret != snapshot.Secret || row.RawOAuth != snapshot.RawOAuth {
		if row.Secret != "" && row.State.ExpiresAt > time.Now().Add(3*time.Minute).UnixMilli() {
			return directCredentialResult(row), nil
		}
		return nil, ErrDirectCredentialChanged
	}
	def := GetProviderDefinition(directOAuthProvider(row.Provider))
	if def == nil || def.RefreshAccessToken == nil || row.State.RefreshToken == "" {
		return nil, ErrDirectCredentialUnavailable
	}
	fresh, err := RefreshWithRetry(ctx, func() (*TokenSet, error) {
		return def.RefreshAccessToken(ctx, RefreshTokenInput{RefreshToken: row.State.RefreshToken, ClientID: row.State.ClientID, OAuth: &RefreshOAuthContext{ProjectID: row.State.ProjectID}, ProxyURL: proxyURL})
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
	next.ProjectID = coalesceStr(fresh.ProjectID, next.ProjectID)
	next.ExpiresAt = fresh.TokenExpiresAt
	tx, err := lease.BeginFencedTx(ctx)
	if err != nil {
		return nil, ErrDirectCredentialChanged
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, db.Rebind(`UPDATE upstream_credentials SET secret=?,oauth_state=?
		WHERE id=? AND kind=? AND enabled=? AND secret=? AND oauth_state=? AND channel_id=?
		AND EXISTS(SELECT 1 FROM upstream_channels c WHERE c.id=upstream_credentials.channel_id AND c.enabled=? AND c.provider=? AND c.endpoint_config=?)`),
		fresh.AccessToken, next, id, store.DirectCredentialOAuth, true, row.Secret, row.RawOAuth, row.ChannelID, true, row.Provider, row.RawEndpoints)
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
	if err := tx.Commit(); err != nil {
		return nil, ErrDirectCredentialRefresh
	}
	// Also recheck channel/credential enablement after the refresh. A source
	// replacement must never authorize sending the just-superseded token.
	current, err := loadDirectCredential(ctx, db, id)
	if err != nil {
		return nil, err
	}
	expectedOAuth, err := next.Value()
	if err != nil || current.Kind != row.Kind || current.Provider != row.Provider || current.ChannelID != row.ChannelID ||
		current.RawEndpoints != row.RawEndpoints || current.Secret != fresh.AccessToken || current.RawOAuth != expectedOAuth {
		return nil, ErrDirectCredentialChanged
	}
	return directCredentialResult(current), nil
}
