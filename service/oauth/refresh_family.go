package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// The commit boundary may lose its acknowledgement after the database has
// committed. Tests inject that external outcome, not the publication logic.
var commitRefreshPublication = func(tx *sqlx.Tx) error { return tx.Commit() }

// Retry storage with the captured result, never the issuer. Each attempt fences
// the same record/family owners and CAS. The retained publication nonce lets a
// lost commit acknowledgement settle without a second update or token exchange.
func publishRefreshedCredential(ctx context.Context, family *store.CredentialRefreshFamily, lease *store.CredentialRefreshLease, reuse bool, nextFingerprint, scope string, id int64, changed error, apply func(*sqlx.Tx) error, verify func(context.Context) error) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = publishRefreshTransaction(ctx, family, lease, reuse, nextFingerprint, scope, id, apply)
		if lastErr == nil {
			family.State = "active"
			return nil
		}
		if committed, err := family.PublicationMatches(ctx, nextFingerprint, scope, id, reuse); err == nil && committed {
			family.State = "active"
			return verify(ctx)
		}
		if errors.Is(lastErr, changed) {
			return changed
		}
		if attempt < 2 {
			if err := sleepCtx(ctx, time.Duration(1<<attempt)*100*time.Millisecond); err != nil {
				return err
			}
		}
	}
	if errors.Is(lastErr, store.ErrCredentialRefreshLeaseLost) || errors.Is(lastErr, store.ErrRefreshFamilyChanged) {
		return changed
	}
	if reuse {
		return ErrDirectCredentialRefresh
	}
	return ErrOAuthRefreshUncertain
}

func publishRefreshTransaction(ctx context.Context, family *store.CredentialRefreshFamily, lease *store.CredentialRefreshLease, reuse bool, nextFingerprint, scope string, id int64, apply func(*sqlx.Tx) error) error {
	var tx *sqlx.Tx
	var err error
	if reuse {
		tx, err = family.BeginReuse(ctx)
	} else {
		tx, err = family.BeginPublication(ctx)
	}
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := lease.FenceTx(ctx, tx); err != nil {
		return err
	}
	if err := apply(tx); err != nil {
		return err
	}
	if reuse {
		err = family.PointResultTx(ctx, tx, scope, id)
	} else {
		err = family.FinishTx(ctx, tx, nextFingerprint, scope, id)
	}
	if err != nil {
		return err
	}
	return commitRefreshPublication(tx)
}

// The issuer binds refresh tokens to a client, not a route or project. A
// project remains the selected credential's own authority during reuse.
func refreshFamilyScope(provider, clientID string) (string, error) {
	var err error
	if strings.TrimSpace(clientID) == "" {
		switch provider {
		case string(ProviderClaude):
			clientID, err = requireClaudeClientID()
		case string(ProviderCodex):
			clientID, err = requireCodexClientID()
		case string(ProviderGeminiCli):
			clientID, _, err = requireGeminiCliOAuthConfig()
		case string(ProviderAntigravity):
			clientID = antigravityClientID
		case string(ProviderGrok):
			clientID = grokClientID
		case string(ProviderCopilot):
			clientID = copilotClientID
		default:
			return "", ErrOAuthRefreshUncertain
		}
	}
	if err != nil {
		return "", err
	}
	data, err := json.Marshal([]string{provider, strings.TrimSpace(clientID)})
	return string(data), err
}

func acquireRefreshFamily(ctx context.Context, db *sqlx.DB, provider, clientID, token string) (*store.CredentialRefreshFamily, error) {
	scope, err := refreshFamilyScope(provider, clientID)
	if err != nil {
		return nil, err
	}
	mode := store.CredentialRefreshConsumable
	if provider == string(ProviderCopilot) {
		// GitHub's durable token obtains short-lived Copilot tokens without
		// consuming itself. Channel settings cannot opt OAuth RTs into this.
		mode = store.CredentialRefreshReusable
	}
	family, err := store.AcquireCredentialRefreshFamily(ctx, db, scope, store.OAuthRefreshFingerprint(scope, token), mode)
	if errors.Is(err, store.ErrRefreshUncertain) {
		return nil, ErrOAuthRefreshUncertain
	}
	return family, err
}

func releaseRefreshFamily(family *store.CredentialRefreshFamily) {
	if err := family.Release(); err != nil {
		slog.Warn("OAuth refresh family lease release failed")
	}
}

// The family records only a reference. Read the current credential, verify
// its fingerprint/client/expiry, then publish into the selected record with
// its own CAS. A donor's disabled state never re-enables any selected record.
func refreshFamilyWinner(ctx context.Context, db *sqlx.DB, f *store.CredentialRefreshFamily, provider, accessToken string, force bool) (*TokenSet, error) {
	if f.Generation == 0 {
		return nil, nil
	}
	var token *TokenSet
	var scope string
	var err error
	switch f.ResultScope {
	case "direct":
		var row directCredentialRow
		err = db.GetContext(ctx, &row, db.Rebind(`SELECT k.secret,k.kind,k.oauth_state,c.provider FROM upstream_credentials k JOIN upstream_channels c ON c.id=k.channel_id WHERE k.id=?`), f.ResultID)
		if err != nil || row.Kind != store.DirectCredentialOAuth || directOAuthProvider(row.Provider) != provider || row.State.Scan(row.RawOAuth) != nil {
			return nil, ErrOAuthRefreshUncertain
		}
		scope, err = refreshFamilyScope(provider, row.State.ClientID)
		token = &TokenSet{AccessToken: row.Secret, RefreshToken: row.State.RefreshToken, IDToken: row.State.IDToken, AccountID: row.State.AccountID, ProjectID: row.State.ProjectID, TokenExpiresAt: row.State.ExpiresAt}
	case "account":
		var account store.Account
		if db.GetContext(ctx, &account, db.Rebind(`SELECT * FROM accounts WHERE id=?`), f.ResultID) != nil {
			return nil, ErrOAuthRefreshUncertain
		}
		info := GetOauthInfoFromAccount(&account)
		if info == nil || info.Provider != provider {
			return nil, ErrOAuthRefreshUncertain
		}
		scope, err = refreshFamilyScope(provider, "")
		token = &TokenSet{AccessToken: account.AccessToken, RefreshToken: info.RefreshToken, IDToken: info.IDToken, AccountID: info.AccountID, AccountKey: info.AccountKey, ProjectID: info.ProjectID, Email: info.Email, PlanType: info.PlanType, ProviderData: info.ProviderData, TokenExpiresAt: info.TokenExpiresAt}
	default:
		return nil, ErrOAuthRefreshUncertain
	}
	valid := err == nil && scope == f.Provider && store.OAuthRefreshFingerprint(scope, token.RefreshToken) == f.CurrentFingerprint && token.AccessToken != "" && token.TokenExpiresAt > time.Now().Add(3*time.Minute).UnixMilli()
	if !valid {
		if f.Retired {
			return nil, ErrOAuthRefreshUncertain
		}
		return nil, nil
	}
	if force && accessToken == token.AccessToken {
		if f.Retired {
			return nil, ErrOAuthRefreshUncertain
		}
		return nil, nil
	}
	return token, nil
}

// Commit the intent for each attempted exchange. Only a typed definite
// rejection/not-sent outcome releases it for a retry. Copilot uses a reusable
// GitHub token, not a consumable OAuth refresh token.
func refreshFamilyWithRetry(ctx context.Context, f *store.CredentialRefreshFamily, provider, accessToken string, fn func(context.Context) (*TokenSet, error)) (*TokenSet, error) {
	ctx = context.WithValue(ctx, refreshExchangeIdentityKey{}, refreshExchangeIdentity{AccessToken: accessToken, IssuerScope: f.Provider, Fingerprint: f.CurrentFingerprint})
	return RefreshWithRetry(ctx, func() (*TokenSet, error) {
		if err := f.Begin(ctx); err != nil {
			return nil, ErrOAuthRefreshUncertain
		}
		token, err := fn(ctx)
		if err == nil {
			return token, nil
		}
		definite := errors.Is(err, errRefreshNotConsumed) || provider == string(ProviderCopilot)
		if failErr := f.Fail(ctx, definite); failErr != nil || !definite {
			return nil, ErrOAuthRefreshUncertain
		}
		return nil, err
	})
}

func markRefreshFamilyUncertain(ctx context.Context, f *store.CredentialRefreshFamily) {
	if err := f.Fail(ctx, false); err != nil {
		slog.Warn("OAuth refresh intent remains unconfirmed")
	}
}
