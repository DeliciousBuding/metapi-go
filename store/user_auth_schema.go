package store

func buildUserCredentialsDDL(string) string {
	return `CREATE TABLE IF NOT EXISTS gateway_user_credentials (
		user_id BIGINT PRIMARY KEY REFERENCES gateway_users(id) ON DELETE CASCADE,
		password_hash TEXT NOT NULL,
		version TEXT NOT NULL,
		activation_hash TEXT,
		activation_expires_at TEXT,
		updated_at TEXT NOT NULL
	)`
}

func buildUserSessionsDDL(string) string {
	return `CREATE TABLE IF NOT EXISTS gateway_user_sessions (
		token_hash TEXT PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES gateway_users(id) ON DELETE CASCADE,
		credential_version TEXT NOT NULL,
		created_at TEXT NOT NULL,
		last_seen_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		idle_expires_at TEXT NOT NULL
	)`
}

func migrateUserAuth(db *DB) error {
	for _, query := range []string{buildUserCredentialsDDL(db.Dialect), buildUserSessionsDDL(db.Dialect),
		`CREATE INDEX IF NOT EXISTS gateway_user_sessions_user_idx ON gateway_user_sessions(user_id)`} {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}
	return nil
}
