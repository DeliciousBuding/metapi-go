package upstream

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// ConnectInput is prepared by the preset and credential owners. It is not an
// HTTP input: endpoint configuration and recommended models are trusted presets.
type ConnectInput struct {
	Channel           ChannelCreate
	AutoName          bool
	Secret            string `json:"-"`
	CredentialKind    string
	OAuthState        store.DirectOAuthState `json:"-"`
	RecommendedModels []string
}

func (ConnectInput) String() string { return "ConnectInput([REDACTED])" }

type Discovery struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type ConnectResult struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Enabled    bool      `json:"enabled"`
	Ownership  string    `json:"ownership"`
	ModelCount int       `json:"modelCount"`
	RouteCount int       `json:"routeCount"`
	Discovery  Discovery `json:"discovery"`
}

// Connect discovers before taking the catalog lock, then commits the entire
// graph through its single writer. Failed discovery and writes leave no rows.
func Connect(ctx context.Context, db *sqlx.DB, in ConnectInput) (ConnectResult, error) {
	channel := in.Channel
	channel.Name = strings.TrimSpace(channel.Name)
	if !validName(channel.Name) || !channel.Endpoints.IsConfigured() {
		return ConnectResult{}, invalid("Invalid upstream connection")
	}
	mask := channel.Endpoints.ProtocolMask()
	if in.CredentialKind == store.DirectCredentialNone {
		mask &= channel.Endpoints.AnonymousProtocolMask()
		if !store.DirectProviderAllowsAnonymous(channel.Provider) || in.Secret != "" {
			return ConnectResult{}, invalid("Platform requires credentials")
		}
	} else if (in.CredentialKind != store.DirectCredentialAPIKey && in.CredentialKind != store.DirectCredentialOAuth) || strings.TrimSpace(in.Secret) == "" || len(in.Secret) > 16384 || strings.ContainsAny(in.Secret, "\r\n\x00") {
		return ConnectResult{}, invalid("Invalid upstream credential")
	}
	if mask == 0 {
		return ConnectResult{}, invalid("Platform has no executable protocols for this credential")
	}
	models, discovery, err := discoverConnectModels(ctx, in)
	if err != nil {
		return ConnectResult{}, err
	}
	out := ConnectResult{Name: channel.Name, Enabled: true, Ownership: "native", ModelCount: len(models), RouteCount: len(models), Discovery: discovery}
	err = Write(ctx, db, func(tx *sqlx.Tx) error {
		var err error
		if in.AutoName {
			baseName := channel.Name
			for suffix := 2; ; suffix++ {
				var exists int
				if err := tx.GetContext(ctx, &exists, tx.Rebind(`SELECT COUNT(*) FROM upstream_channels WHERE origin_key=? AND name=?`), NativeOrigin, channel.Name); err != nil {
					return err
				}
				if exists == 0 {
					break
				}
				channel.Name = fmt.Sprintf("%s (%d)", baseName, suffix)
				if !validName(channel.Name) {
					return conflict("Choose a unique upstream name")
				}
			}
			out.Name = channel.Name
		}
		out.ID, err = InsertNative(ctx, tx, "channels", channel.Name, "generic", channel.Provider, true, channel.BaseURL, channel.Endpoints, "", "", "", channel.UseSystemProxy, channel.ChannelProxy, "[]", "", "")
		if err != nil {
			return err
		}
		credentialID, err := InsertNative(ctx, tx, "credentials", out.ID, "Default", in.Secret, true, in.CredentialKind, in.OAuthState)
		if err != nil {
			return err
		}
		// Match the catalog's entity lock order, including existing groups.
		modelIDs := make([]int64, len(models))
		grantIDs := make([]int64, len(models))
		groupIDs := make([]int64, len(models))
		for i, model := range models {
			modelIDs[i], err = InsertNative(ctx, tx, "models", out.ID, model, true)
			if err != nil {
				return err
			}
		}
		for i, modelID := range modelIDs {
			grantIDs[i], err = InsertNative(ctx, tx, "grants", modelID, credentialID, mask, true)
			if err != nil {
				return err
			}
		}
		for i, model := range models {
			groupIDs[i], err = connectModelGroup(ctx, tx, model)
			if err != nil {
				return err
			}
		}
		for i, groupID := range groupIDs {
			if _, err = InsertNative(ctx, tx, "group_items", groupID, grantIDs[i], int64(0), int64(1), store.DirectProtocolOrder{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ConnectResult{}, err
	}
	return out, nil
}

func connectModelGroup(ctx context.Context, tx *sqlx.Tx, model string) (int64, error) {
	var routes []struct {
		ID      int64  `db:"id"`
		Pattern string `db:"model_pattern"`
		Mode    string `db:"route_mode"`
	}
	if err := tx.SelectContext(ctx, &routes, tx.Rebind(`SELECT id,model_pattern,route_mode FROM token_routes WHERE model_pattern=? OR display_name=? ORDER BY id`), model, model); err != nil {
		return 0, err
	}
	if len(routes) > 1 || len(routes) == 1 && (routes[0].Pattern != model || routes[0].Mode != "pattern") {
		return 0, conflict("Discovered model conflicts with an existing public route")
	}
	var routeID, groupID int64
	if len(routes) == 1 {
		routeID = routes[0].ID
		err := tx.GetContext(ctx, &groupID, tx.Rebind(`SELECT group_id FROM upstream_route_groups WHERE route_id=?`), routeID)
		if err == nil {
			return groupID, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	} else {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := tx.GetContext(ctx, &routeID, tx.Rebind(`INSERT INTO token_routes (model_pattern,display_name,route_mode,routing_strategy,enabled,created_at,updated_at) VALUES (?,'','pattern','weighted',?,?,?) RETURNING id`), model, true, now, now); err != nil {
			return 0, err
		}
	}
	// A group is owned by its public model, so subsequent providers can join
	// without creating competing exact routes or changing existing strategies.
	name := model
	var count int
	if err := tx.GetContext(ctx, &count, tx.Rebind(`SELECT COUNT(*) FROM upstream_groups WHERE origin_key=? AND name=?`), NativeOrigin, name); err != nil {
		return 0, err
	}
	if count != 0 {
		name = fmt.Sprintf("Model route %d", routeID)
	}
	var err error
	groupID, err = InsertNative(ctx, tx, "groups", name, "failover", int64(0), `{}`, true)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO upstream_route_groups (route_id,group_id) VALUES (?,?)`), routeID, groupID)
	return groupID, err
}
