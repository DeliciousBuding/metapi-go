package store

import (
	"testing"
)

// additiveColumnSpec is a table.column pair that enterpriseAdditiveSteps must
// converge on for both fresh installs (base DDL) and old installs (ALTER TABLE).
type additiveColumnSpec struct {
	table  string
	column string
}

// additiveColumns is the authoritative list of every column introduced by the
// additive enterprise upgrade registry (store/additive.go). It is mirrored here
// so tests fail loudly when a step is added, renamed, or removed without the
// corresponding schema change being reconciled.
func additiveColumns() []additiveColumnSpec {
	return []additiveColumnSpec{
		{"downstream_api_keys", "proxy_url"},
		{"sites", "max_concurrency"},
		{"token_routes", "context_length"},
		{"proxy_logs", "request_id"},
		{"downstream_api_keys", "max_rpm"},
		{"downstream_api_keys", "max_tpm"},
		{"token_routes", "sort_order"},
		{"downstream_api_keys", "key_weight"},
		{"sites", "custom_headers_override_request_headers"},
		{"downstream_api_keys", "allowed_site_ids"},
		{"downstream_api_keys", "allowed_credential_refs"},
		{"downstream_api_keys", "ip_allowlist"},
		{"downstream_api_keys", "ip_blocklist"},
		{"accounts", "tags"},
		{"sites", "tags"},
		{"checkin_logs", "failure_reason"},
		{"accounts", "remark"},
	}
}

// legacySchemaDDL is the pre-additive shape of the tables touched by
// enterpriseAdditiveSteps. It intentionally omits every additive column so the
// test exercises the real ALTER TABLE ADD COLUMN upgrade path against an old
// database, rather than the fresh-install no-op path.
var legacySchemaDDL = []string{
	"CREATE TABLE downstream_api_keys (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, key TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)",
	"CREATE TABLE sites (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, url TEXT NOT NULL, platform TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)",
	"CREATE TABLE token_routes (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)",
	"CREATE TABLE proxy_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at TEXT NOT NULL)",
	"CREATE TABLE accounts (id INTEGER PRIMARY KEY AUTOINCREMENT, email TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)",
	"CREATE TABLE checkin_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL, created_at TEXT NOT NULL)",
}

// TestAdditiveUpgradeFromLegacySchema runs the production additive registry
// against a schema that predates the additive columns, and asserts that old rows
// survive and every additive column is added.
func TestAdditiveUpgradeFromLegacySchema(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	for _, ddl := range legacySchemaDDL {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("create legacy table: %v", err)
		}
	}

	legacyRows := []string{
		"INSERT INTO downstream_api_keys (name, key, created_at, updated_at) VALUES ('legacy-key', 'sk-legacy', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z')",
		"INSERT INTO sites (name, url, platform, created_at, updated_at) VALUES ('legacy-site', 'https://example.com', 'openai', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z')",
		"INSERT INTO token_routes (name, created_at, updated_at) VALUES ('legacy-route', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z')",
		"INSERT INTO proxy_logs (created_at) VALUES ('2026-07-01T00:00:00Z')",
		"INSERT INTO accounts (email, created_at, updated_at) VALUES ('legacy@example.com', '2026-07-01T00:00:00Z', '2026-07-01T00:00:00Z')",
		"INSERT INTO checkin_logs (account_id, created_at) VALUES (1, '2026-07-01T00:00:00Z')",
	}
	for _, q := range legacyRows {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("insert legacy row: %v", err)
		}
	}

	if err := ApplyAdditiveMigrations(db); err != nil {
		t.Fatalf("ApplyAdditiveMigrations: %v", err)
	}

	for _, c := range additiveColumns() {
		ok, err := columnExists(db, c.table, c.column)
		if err != nil {
			t.Fatalf("columnExists(%s.%s): %v", c.table, c.column, err)
		}
		if !ok {
			t.Errorf("additive column %s.%s missing after legacy upgrade", c.table, c.column)
		}
	}

	preserved := []struct {
		query string
		want  int
	}{
		{"SELECT COUNT(*) FROM downstream_api_keys WHERE name = 'legacy-key'", 1},
		{"SELECT COUNT(*) FROM sites WHERE name = 'legacy-site'", 1},
		{"SELECT COUNT(*) FROM token_routes WHERE name = 'legacy-route'", 1},
		{"SELECT COUNT(*) FROM proxy_logs", 1},
		{"SELECT COUNT(*) FROM accounts WHERE email = 'legacy@example.com'", 1},
		{"SELECT COUNT(*) FROM checkin_logs WHERE account_id = 1", 1},
	}
	for _, p := range preserved {
		var n int
		if err := db.QueryRow(p.query).Scan(&n); err != nil {
			t.Fatalf("verify preserved row: %v", err)
		}
		if n != p.want {
			t.Errorf("%s: got %d rows, want %d", p.query, n, p.want)
		}
	}

	// Bookkeeping must record every step exactly once.
	var applied int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if applied != len(enterpriseAdditiveSteps) {
		t.Errorf("schema_migrations count = %d, want %d", applied, len(enterpriseAdditiveSteps))
	}
}

// TestBaseSchemaContainsAllAdditiveColumns guards convergence: after a normal
// AutoMigrate, every column introduced by the additive registry must be present
// in the resulting schema.
func TestBaseSchemaContainsAllAdditiveColumns(t *testing.T) {
	db := openTestSQLite(t)
	for _, c := range additiveColumns() {
		ok, err := columnExists(db, c.table, c.column)
		if err != nil {
			t.Fatalf("columnExists(%s.%s): %v", c.table, c.column, err)
		}
		if !ok {
			t.Errorf("additive column %s.%s missing after AutoMigrate", c.table, c.column)
		}
	}
}
