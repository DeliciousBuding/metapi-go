package store

import "fmt"

func buildGatewayUserGroupsDDL(d string) string {
	pk, boolean := "INTEGER PRIMARY KEY AUTOINCREMENT", "INTEGER"
	if isPG(d) {
		pk, boolean = "BIGSERIAL PRIMARY KEY", "BOOLEAN"
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS gateway_user_groups (
		id %s,
		slug TEXT NOT NULL UNIQUE CHECK (length(slug) BETWEEN 1 AND 64),
		name TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
		enabled %s NOT NULL DEFAULT FALSE CHECK (enabled IN (FALSE, TRUE)),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`, pk, boolean)
}

func buildGatewayUsersDDL(d string) string {
	pk, boolean := "INTEGER PRIMARY KEY AUTOINCREMENT", "INTEGER"
	if isPG(d) {
		pk, boolean = "BIGSERIAL PRIMARY KEY", "BOOLEAN"
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS gateway_users (
		id %s,
		username TEXT NOT NULL UNIQUE CHECK (length(username) BETWEEN 1 AND 64),
		display_name TEXT NOT NULL CHECK (length(display_name) <= 100),
		group_id BIGINT NOT NULL REFERENCES gateway_user_groups(id) ON DELETE RESTRICT,
		enabled %s NOT NULL DEFAULT FALSE CHECK (enabled IN (FALSE, TRUE)),
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`, pk, boolean)
}

func buildGatewayUserGroupRouteGrantsDDL(_ string) string {
	return `CREATE TABLE IF NOT EXISTS gateway_user_group_route_grants (
		group_id BIGINT NOT NULL REFERENCES gateway_user_groups(id) ON DELETE CASCADE,
		route_id BIGINT NOT NULL REFERENCES token_routes(id) ON DELETE CASCADE,
		created_at TEXT NOT NULL,
		PRIMARY KEY (group_id, route_id)
	)`
}

// Migration creates no groups, users or grants and does not adopt existing Keys.
// CreateUser creates the default group only when an administrator needs it.
func migrateGatewayUsers(db *DB) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, query := range []string{
		buildGatewayUserGroupsDDL(db.Dialect),
		buildGatewayUsersDDL(db.Dialect),
		buildGatewayUserGroupRouteGrantsDDL(db.Dialect),
		`CREATE INDEX IF NOT EXISTS gateway_users_group_idx ON gateway_users (group_id, id)`,
		`CREATE INDEX IF NOT EXISTS gateway_user_group_route_grants_route_idx ON gateway_user_group_route_grants (route_id, group_id)`,
	} {
		if _, err := tx.Exec(query); err != nil {
			return err
		}
	}
	return tx.Commit()
}
