package oauth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var ErrInvalidDirectCredential = errors.New("invalid direct credential replacement")
var ErrDirectCredentialNotFound = errors.New("direct credential or channel not found")
var ErrDirectCredentialConflict = errors.New("direct credential name already exists")

type DirectOAuthReplacement struct {
	AccessToken string `json:"accessToken"`
	store.DirectOAuthState
}
type DirectCredentialUpdate struct {
	Name    *string                 `json:"name,omitempty"`
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
	var exists int
	if err := db.GetContext(ctx, &exists, db.Rebind(`SELECT 1 FROM upstream_channels WHERE id=?`), channelID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDirectCredentialNotFound
		}
		return nil, ErrDirectCredentialUnavailable
	}
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
	if input.Name == nil && input.Enabled == nil && input.APIKey == nil && input.OAuth == nil {
		return ErrInvalidDirectCredential
	}
	if input.APIKey != nil && input.OAuth != nil {
		return ErrInvalidDirectCredential
	}
	var provider string
	if err := db.GetContext(ctx, &provider, db.Rebind(`SELECT c.provider FROM upstream_credentials k JOIN upstream_channels c ON c.id=k.channel_id WHERE k.id=?`), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDirectCredentialNotFound
		}
		return ErrDirectCredentialUnavailable
	}
	set := []string{}
	args := []any{}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" || len(name) > 200 {
			return ErrInvalidDirectCredential
		}
		set = append(set, "name=?")
		args = append(args, name)
	}
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
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key") {
			return ErrDirectCredentialConflict
		}
		return ErrDirectCredentialUnavailable
	}
	n, err := result.RowsAffected()
	if err != nil {
		return ErrDirectCredentialUnavailable
	}
	if n != 1 {
		return ErrDirectCredentialNotFound
	}
	return nil
}
