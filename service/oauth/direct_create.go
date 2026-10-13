package oauth

import (
	"context"
	"github.com/deliciousbuding/metapi-go/internal/gcpsa"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

type DirectCredentialCreate struct {
	Kind    string                  `json:"kind,omitempty"`
	Name    string                  `json:"name"`
	Enabled bool                    `json:"enabled"`
	APIKey  *string                 `json:"apiKey"`
	OAuth   *DirectOAuthReplacement `json:"oauth"`
	// GCPServiceAccount carries the service-account JSON key document when
	// kind is gcp_service_account. Validated at write time, never returned.
	GCPServiceAccount *string `json:"gcpServiceAccount,omitempty"`
}

// CreateDirectCredential permits local credentials under imported channels.
// Credential material is never returned by the management projection.
func CreateDirectCredential(ctx context.Context, db *sqlx.DB, channelID int64, in DirectCredentialCreate) (DirectCredentialSummary, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 200 {
		return DirectCredentialSummary{}, ErrInvalidDirectCredential
	}
	out := DirectCredentialSummary{Name: in.Name, Enabled: in.Enabled, Ownership: "native"}
	err := upstream.Write(ctx, db, func(tx *sqlx.Tx) error {
		var channel struct {
			Provider  string                `db:"provider"`
			Endpoints store.DirectEndpoints `db:"endpoint_config"`
		}
		query := `SELECT provider,endpoint_config FROM upstream_channels WHERE id=?`
		if tx.DriverName() == "pgx" {
			query += " FOR UPDATE"
		}
		if err := tx.GetContext(ctx, &channel, tx.Rebind(query), channelID); err != nil {
			return err
		}
		secret, kind, state, gcpMaterial, err := directCredentialMaterial(channel.Provider, channel.Endpoints, in.Kind, in.APIKey, in.OAuth, in.GCPServiceAccount)
		if err != nil {
			return err
		}
		out.Kind = kind
		out.ExpiresAt = state.ExpiresAt
		out.CanRefresh = state.RefreshToken != ""
		out.ID, err = upstream.InsertNative(ctx, tx, "credentials", channelID, in.Name, secret, in.Enabled, kind, state)
		if err != nil {
			return err
		}
		if gcpMaterial != "" {
			_, err = tx.ExecContext(ctx, tx.Rebind("UPDATE upstream_credentials SET gcp_service_account=? WHERE id=?"), gcpMaterial, out.ID)
		}
		return err
	})
	return out, err
}

func directCredentialMaterial(provider string, endpoints store.DirectEndpoints, kind string, key *string, oauth *DirectOAuthReplacement, gcp *string) (string, string, store.DirectOAuthState, string, error) {
	state := store.DirectOAuthState{}
	if kind == store.DirectCredentialGCPServiceAccount {
		// Durable material is the SA JSON; no access token exists until the
		// first resolve exchanges it, so secret starts empty.
		if key != nil || oauth != nil || gcp == nil {
			return "", "", state, "", ErrInvalidDirectCredential
		}
		material := strings.TrimSpace(*gcp)
		if _, _, err := gcpsa.Parse(material); err != nil {
			return "", "", state, "", ErrInvalidDirectCredential
		}
		return "", store.DirectCredentialGCPServiceAccount, state, material, nil
	}
	if gcp != nil {
		return "", "", state, "", ErrInvalidDirectCredential
	}
	if kind == store.DirectCredentialNone {
		if key != nil || oauth != nil || !store.DirectProviderAllowsAnonymous(provider) || endpoints.AnonymousProtocolMask() == 0 {
			return "", "", state, "", ErrInvalidDirectCredential
		}
		return "", store.DirectCredentialNone, state, "", nil
	}
	if kind != "" && (kind != store.DirectCredentialAPIKey || key == nil) && (kind != store.DirectCredentialOAuth || oauth == nil) {
		return "", "", state, "", ErrInvalidDirectCredential
	}
	if key != nil && oauth != nil || key == nil && oauth == nil {
		return "", "", state, "", ErrInvalidDirectCredential
	}
	if key != nil {
		secret := strings.TrimSpace(*key)
		if secret == "" || strings.HasPrefix(secret, "{") {
			return "", "", state, "", ErrInvalidDirectCredential
		}
		return secret, store.DirectCredentialAPIKey, state, "", nil
	}
	if directOAuthProvider(provider) == "" || strings.TrimSpace(oauth.AccessToken) == "" || oauth.ExpiresAt < 0 {
		return "", "", state, "", ErrInvalidDirectCredential
	}
	return oauth.AccessToken, store.DirectCredentialOAuth, oauth.DirectOAuthState, "", nil
}
