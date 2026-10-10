package oauth

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/upstream"
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
	Kind    string                  `json:"kind,omitempty"`
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
	Ownership  string `json:"ownership"`
}

func ListDirectCredentials(ctx context.Context, db *sqlx.DB, channelID int64) ([]DirectCredentialSummary, error) {
	var exists int
	if err := db.GetContext(ctx, &exists, db.Rebind(`SELECT 1 FROM upstream_channels WHERE id=?`), channelID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDirectCredentialNotFound
		}
		return nil, ErrDirectCredentialUnavailable
	}
	rows, err := db.QueryxContext(ctx, db.Rebind(`SELECT id,name,enabled,kind,oauth_state,origin_key FROM upstream_credentials WHERE channel_id=? ORDER BY id`), channelID)
	if err != nil {
		return nil, ErrDirectCredentialUnavailable
	}
	defer rows.Close()
	out := []DirectCredentialSummary{}
	for rows.Next() {
		var item DirectCredentialSummary
		var state store.DirectOAuthState
		var origin string
		if err := rows.Scan(&item.ID, &item.Name, &item.Enabled, &item.Kind, &state, &origin); err != nil {
			return nil, ErrDirectCredentialUnavailable
		}
		item.ExpiresAt = state.ExpiresAt
		item.CanRefresh = state.RefreshToken != ""
		item.Ownership = upstream.Ownership(origin)
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
	return upstream.Write(ctx, db, func(tx *sqlx.Tx) error {
		return updateDirectCredentialTx(ctx, tx, id, input)
	})
}

func updateDirectCredentialTx(ctx context.Context, db *sqlx.Tx, id int64, input DirectCredentialUpdate) error {
	if input.Name == nil && input.Enabled == nil && input.APIKey == nil && input.OAuth == nil && input.Kind == "" {
		return ErrInvalidDirectCredential
	}
	if input.APIKey != nil && input.OAuth != nil {
		return ErrInvalidDirectCredential
	}
	var channel struct {
		Provider  string                `db:"provider"`
		Endpoints store.DirectEndpoints `db:"endpoint_config"`
	}
	if err := db.GetContext(ctx, &channel, db.Rebind(`SELECT c.provider,c.endpoint_config FROM upstream_credentials k JOIN upstream_channels c ON c.id=k.channel_id WHERE k.id=?`), id); err != nil {
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
	if input.APIKey != nil || input.OAuth != nil || input.Kind != "" {
		secret, kind, state, err := directCredentialMaterial(channel.Provider, channel.Endpoints, input.Kind, input.APIKey, input.OAuth)
		if err != nil {
			return err
		}
		if kind == store.DirectCredentialNone {
			var masks []int
			if err := db.SelectContext(ctx, &masks, db.Rebind(`SELECT protocols FROM upstream_grants WHERE credential_id=?`), id); err != nil {
				return ErrDirectCredentialUnavailable
			}
			for _, mask := range masks {
				if mask&channel.Endpoints.AnonymousProtocolMask() != mask {
					return ErrInvalidDirectCredential
				}
			}
		}
		set = append(set, "secret=?", "kind=?", "oauth_state=?")
		args = append(args, secret, kind, state)
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
