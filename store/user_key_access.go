package store

import (
	"context"
	"database/sql"
)

// Distinguishes user-Key-owned files/tasks even in portable row payloads.
const UserKeyResourceOwnerPrefix = "user-key:"

// ResolveUserKeyAccess takes one database snapshot of identity, group and
// grants, including active users whose current group has no routes.
func (s *GatewayUserStore) ResolveUserKeyAccess(ctx context.Context, userID int64) (int64, []int64, error) {
	if err := s.available(); err != nil {
		return 0, nil, err
	}
	rows, err := s.db.QueryxContext(ctx, s.db.Rebind(`SELECT u.group_id, r.id FROM gateway_users u
		JOIN gateway_user_groups g ON g.id=u.group_id
		LEFT JOIN gateway_user_group_route_grants gr ON gr.group_id=g.id
		LEFT JOIN token_routes r ON r.id=gr.route_id AND r.enabled=TRUE
		WHERE u.id=? AND u.enabled=TRUE AND g.enabled=TRUE ORDER BY r.id`), userID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	groupID, routes := int64(0), []int64{}
	for rows.Next() {
		var route sql.NullInt64
		if err := rows.Scan(&groupID, &route); err != nil {
			return 0, nil, err
		}
		if route.Valid {
			routes = append(routes, route.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}
	if groupID == 0 {
		return 0, nil, sql.ErrNoRows
	}
	return groupID, routes, nil
}

// Ownership is never inferred from group_name (a legacy display label).
// Losing the explicit user binding must not create an unrestricted system Key.
func migrateUserKeyAccess(db *DB) error {
	if err := EnsureColumn(db, "downstream_api_keys", "key_owner_type", "TEXT", "TEXT", "NOT NULL DEFAULT 'system' CHECK (key_owner_type IN ('system','user'))"); err != nil {
		return err
	}
	if err := EnsureColumn(db, "downstream_api_keys", "owner_user_id", "BIGINT", "BIGINT", "CHECK ((key_owner_type = 'system' AND owner_user_id IS NULL) OR (key_owner_type = 'user' AND owner_user_id IS NOT NULL AND owner_user_id > 0))"); err != nil {
		return err
	}
	for _, table := range []string{"gateway_requests", "proxy_logs"} {
		for _, column := range []string{"owner_user_id", "user_group_id"} {
			if err := EnsureColumn(db, table, column, "BIGINT", "BIGINT", ""); err != nil {
				return err
			}
		}
		if err := EnsureIndex(db, table+"_user_idx", "CREATE INDEX IF NOT EXISTS "+table+"_user_idx ON "+table+" (owner_user_id)"); err != nil {
			return err
		}
	}
	return EnsureIndex(db, "downstream_api_keys_user_idx", "CREATE INDEX IF NOT EXISTS downstream_api_keys_user_idx ON downstream_api_keys (owner_user_id)")
}
