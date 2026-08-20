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
		// TS-heritage columns (sc2_017–sc2_024): a legacy TS hub.db predates
		// these drizzle migrations, so AutoMigrate must converge them.
		{"sites", "proxy_url"},
		{"sites", "use_system_proxy"},
		{"sites", "custom_headers"},
		{"sites", "external_checkin_url"},
		{"sites", "global_weight"},
		{"sites", "post_refresh_probe_enabled"},
		{"sites", "post_refresh_probe_model"},
		{"sites", "post_refresh_probe_scope"},
		{"sites", "post_refresh_probe_latency_threshold_ms"},
		{"token_routes", "display_name"},
		{"token_routes", "display_icon"},
		{"token_routes", "decision_snapshot"},
		{"token_routes", "decision_refreshed_at"},
		{"token_routes", "route_mode"},
		{"token_routes", "routing_strategy"},
		{"route_channels", "oauth_route_unit_id"},
		{"route_channels", "source_model"},
		{"route_channels", "last_selected_at"},
		{"route_channels", "consecutive_fail_count"},
		{"route_channels", "cooldown_level"},
		{"proxy_logs", "billing_details"},
		{"proxy_logs", "downstream_api_key_id"},
		{"proxy_logs", "client_family"},
		{"proxy_logs", "client_app_id"},
		{"proxy_logs", "client_app_name"},
		{"proxy_logs", "client_confidence"},
		{"proxy_logs", "is_stream"},
		{"proxy_logs", "first_byte_latency_ms"},
		{"accounts", "oauth_provider"},
		{"accounts", "oauth_account_key"},
		{"accounts", "oauth_project_id"},
		{"account_tokens", "token_group"},
		{"account_tokens", "value_status"},
		{"model_availability", "is_manual"},
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
	"CREATE TABLE route_channels (id INTEGER PRIMARY KEY AUTOINCREMENT, route_id INTEGER NOT NULL, account_id INTEGER NOT NULL, created_at TEXT NOT NULL)",
	"CREATE TABLE account_tokens (id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL, name TEXT NOT NULL, token TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)",
	"CREATE TABLE model_availability (id INTEGER PRIMARY KEY AUTOINCREMENT, account_id INTEGER NOT NULL, model_name TEXT NOT NULL, available INTEGER, checked_at TEXT)",
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

// tsHeritageColumns is the set of columns that the TypeScript version added via
// drizzle migrations over its history. Every one of them that the Go schema
// relies on must be converged by the additive registry, otherwise a legacy TS
// hub.db (which predates the newer migrations) crashes at startup with
// "no such column" once Go code SELECTs it. Kept explicit so a future TS
// migration that adds a new column forces a corresponding additive step.
var tsHeritageColumns = []additiveColumnSpec{
	{"account_tokens", "token_group"},
	{"account_tokens", "value_status"},
	{"accounts", "oauth_account_key"},
	{"accounts", "oauth_project_id"},
	{"accounts", "oauth_provider"},
	{"model_availability", "is_manual"},
	{"proxy_logs", "billing_details"},
	{"proxy_logs", "client_app_id"},
	{"proxy_logs", "client_app_name"},
	{"proxy_logs", "client_confidence"},
	{"proxy_logs", "client_family"},
	{"proxy_logs", "downstream_api_key_id"},
	{"proxy_logs", "first_byte_latency_ms"},
	{"proxy_logs", "is_stream"},
	{"route_channels", "consecutive_fail_count"},
	{"route_channels", "cooldown_level"},
	{"route_channels", "last_selected_at"},
	{"route_channels", "oauth_route_unit_id"},
	{"route_channels", "source_model"},
	{"sites", "custom_headers"},
	{"sites", "external_checkin_url"},
	{"sites", "global_weight"},
	{"sites", "post_refresh_probe_enabled"},
	{"sites", "post_refresh_probe_latency_threshold_ms"},
	{"sites", "post_refresh_probe_model"},
	{"sites", "post_refresh_probe_scope"},
	{"sites", "proxy_url"},
	{"sites", "use_system_proxy"},
	{"token_routes", "decision_refreshed_at"},
	{"token_routes", "decision_snapshot"},
	{"token_routes", "display_icon"},
	{"token_routes", "display_name"},
	{"token_routes", "route_mode"},
	{"token_routes", "routing_strategy"},
}

// TestTSHeritageColumnsCoveredByAdditiveRegistry asserts that every column the
// TypeScript version added during its migration history is covered by an
// additive step. This is the guard that was missing when the post-refresh probe
// columns (TS 0025/0026) were added to the Go schema without a corresponding
// additive step — the original gap check only compared against the newest TS
// schema, so TS-only columns that old databases predate slipped through.
func TestTSHeritageColumnsCoveredByAdditiveRegistry(t *testing.T) {
	// additiveColumns() is the authority: every column it lists, including the
	// TS-heritage block, is verified by TestAdditiveUpgradeFromLegacySchema to
	// actually be added on an old schema. Here we just ensure the TS-heritage
	// list is a subset of what the registry converges.
	want := make(map[additiveColumnSpec]bool)
	for _, c := range tsHeritageColumns {
		want[c] = true
	}
	covered := make(map[additiveColumnSpec]bool)
	for _, c := range additiveColumns() {
		covered[c] = true
	}
	for c := range want {
		if !covered[c] {
			t.Errorf("TS-heritage column %s.%s has no additive registry coverage", c.table, c.column)
		}
	}
}
