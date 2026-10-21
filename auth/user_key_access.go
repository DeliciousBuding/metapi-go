package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// Reload on every HTTP admission and every WS turn. No identity/group cache.
func resolveManagedKeyUser(key *managedKeyView, db *sqlx.DB) DownstreamTokenAuthResult {
	if key.KeyOwnerType == "system" && key.OwnerUserID == nil {
		return DownstreamTokenAuthResult{OK: true}
	}
	denied := DownstreamTokenAuthResult{StatusCode: 403, Error: "API key user access is disabled", Reason: "user_disabled"}
	if key.KeyOwnerType != "user" || key.OwnerUserID == nil || *key.OwnerUserID <= 0 {
		return denied
	}
	users := store.NewGatewayUserStore(&store.DB{DB: db})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	groupID, routes, err := users.ResolveUserKeyAccess(ctx, *key.OwnerUserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return denied
		}
		return DownstreamTokenAuthResult{StatusCode: 503, Error: "User access is temporarily unavailable", Reason: "unavailable"}
	}
	key.UserGroupID = &groupID
	key.UserRouteIDs = &routes
	return DownstreamTokenAuthResult{OK: true}
}
