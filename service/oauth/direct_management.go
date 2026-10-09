package oauth

import (
	"context"
	"errors"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var ErrInvalidDirectCredential = errors.New("invalid direct credential replacement")

type DirectOAuthReplacement struct {
	AccessToken string `json:"accessToken"`
	store.DirectOAuthState
}
type DirectCredentialUpdate struct {
	Enabled *bool                   `json:"enabled,omitempty"`
	APIKey  *string                 `json:"apiKey,omitempty"`
	OAuth   *DirectOAuthReplacement `json:"oauth,omitempty"`
}
type DirectCredentialSummary struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Kind       string `json:"kind"`
	ExpiresAt  int64  `json:"expiresAt,omitempty"`
	CanRefresh bool   `json:"canRefresh"`
}

func ListDirectCredentials(ctx context.Context, db *sqlx.DB, channelID int64) ([]DirectCredentialSummary, error) {
	rows, err := db.QueryxContext(ctx, db.Rebind(`SELECT id,name,enabled,kind,oauth_state FROM upstream_credentials WHERE channel_id=? ORDER BY id`), channelID)
	if err != nil {
		return nil, ErrDirectCredentialUnavailable
	}
	defer rows.Close()
	out := []DirectCredentialSummary{}
	for rows.Next() {
		var item DirectCredentialSummary
		var state store.DirectOAuthState
		if err := rows.Scan(&item.ID, &item.Name, &item.Enabled, &item.Kind, &state); err != nil {
			return nil, ErrDirectCredentialUnavailable
		}
		item.ExpiresAt = state.ExpiresAt
		item.CanRefresh = state.RefreshToken != ""
		out = append(out, item)
	}
	if rows.Err() != nil {
		return nil, ErrDirectCredentialUnavailable
	}
	return out, nil
}

// UpdateDirectCredential replaces credential material atomically. Omitting a
// replacement edits only enablement. A refresh in flight compares old bytes
// and cannot overwrite this update.
func UpdateDirectCredential(ctx context.Context, db *sqlx.DB, id int64, input DirectCredentialUpdate) error {
	if input.Enabled == nil && input.APIKey == nil && input.OAuth == nil {
		return ErrInvalidDirectCredential
	}
	if input.APIKey != nil && input.OAuth != nil {
		return ErrInvalidDirectCredential
	}
	var provider string
	if err := db.GetContext(ctx, &provider, db.Rebind(`SELECT c.provider FROM upstream_credentials k JOIN upstream_channels c ON c.id=k.channel_id WHERE k.id=?`), id); err != nil {
		return ErrDirectCredentialUnavailable
	}
	set := []string{}
	args := []any{}
	if input.Enabled != nil {
		set = append(set, "enabled=?")
		args = append(args, *input.Enabled)
	}
	if input.APIKey != nil {
		key := strings.TrimSpace(*input.APIKey)
		if key == "" || strings.HasPrefix(key, "{") {
			return ErrInvalidDirectCredential
		}
		set = append(set, "secret=?", "kind=?", "oauth_state=?")
		args = append(args, key, store.DirectCredentialAPIKey, store.DirectOAuthState{})
	}
	if input.OAuth != nil {
		if directOAuthProvider(provider) == "" || strings.TrimSpace(input.OAuth.AccessToken) == "" || input.OAuth.ExpiresAt < 0 {
			return ErrInvalidDirectCredential
		}
		set = append(set, "secret=?", "kind=?", "oauth_state=?")
		args = append(args, input.OAuth.AccessToken, store.DirectCredentialOAuth, input.OAuth.DirectOAuthState)
	}
	args = append(args, id)
	result, err := db.ExecContext(ctx, db.Rebind(`UPDATE upstream_credentials SET `+strings.Join(set, ",")+` WHERE id=?`), args...)
	if err != nil {
		return ErrDirectCredentialUnavailable
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return ErrDirectCredentialUnavailable
	}
	return nil
}
