package oauth

import (
	"context"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

type DirectCredentialCreate struct {
	Name    string                  `json:"name"`
	Enabled bool                    `json:"enabled"`
	APIKey  *string                 `json:"apiKey"`
	OAuth   *DirectOAuthReplacement `json:"oauth"`
}

// CreateDirectCredential permits local credentials under imported channels.
// Credential material is never returned by the management projection.
func CreateDirectCredential(ctx context.Context, db *sqlx.DB, channelID int64, in DirectCredentialCreate) (DirectCredentialSummary, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 200 || in.APIKey == nil && in.OAuth == nil || in.APIKey != nil && in.OAuth != nil {
		return DirectCredentialSummary{}, ErrInvalidDirectCredential
	}
	out := DirectCredentialSummary{Name: in.Name, Enabled: in.Enabled, Ownership: "native"}
	err := upstream.Write(ctx, db, func(tx *sqlx.Tx) error {
		var provider string
		query := `SELECT provider FROM upstream_channels WHERE id=?`
		if tx.DriverName() == "pgx" {
			query += " FOR UPDATE"
		}
		if err := tx.GetContext(ctx, &provider, tx.Rebind(query), channelID); err != nil {
			return err
		}
		secret, kind, state, err := directCredentialMaterial(provider, in.APIKey, in.OAuth)
		if err != nil {
			return err
		}
		out.Kind = kind
		out.ExpiresAt = state.ExpiresAt
		out.CanRefresh = state.RefreshToken != ""
		out.ID, err = upstream.InsertNative(ctx, tx, "credentials", channelID, in.Name, secret, in.Enabled, kind, state)
		return err
	})
	return out, err
}

func directCredentialMaterial(provider string, key *string, oauth *DirectOAuthReplacement) (string, string, store.DirectOAuthState, error) {
	state := store.DirectOAuthState{}
	if key != nil && oauth != nil || key == nil && oauth == nil {
		return "", "", state, ErrInvalidDirectCredential
	}
	if key != nil {
		secret := strings.TrimSpace(*key)
		if secret == "" || strings.HasPrefix(secret, "{") {
			return "", "", state, ErrInvalidDirectCredential
		}
		return secret, store.DirectCredentialAPIKey, state, nil
	}
	if directOAuthProvider(provider) == "" || strings.TrimSpace(oauth.AccessToken) == "" || oauth.ExpiresAt < 0 {
		return "", "", state, ErrInvalidDirectCredential
	}
	return oauth.AccessToken, store.DirectCredentialOAuth, oauth.DirectOAuthState, nil
}
