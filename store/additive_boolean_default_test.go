package store

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
)

// This file pins the boolean-default dialect contract for the additive
// migration registry.
//
// Registry entries are written once and executed on both dialects. SQLite
// stores booleans as INTEGER and accepts either spelling of a default, while
// PostgreSQL type-checks the default expression against the column: a legacy
// database that still needs `ADD COLUMN flag BOOLEAN DEFAULT 0` fails with
// "column is of type boolean but default expression is of type integer"
// (SQLSTATE 42804), which aborts AutoMigrate and stops the process from
// starting. Fresh installs never hit it because CREATE TABLE already carries
// the column, so the failure only surfaces when upgrading an old database —
// exactly the path the tests below exercise.

func TestNormalizeBooleanDefault(t *testing.T) {
	cases := []struct {
		name    string
		colType string
		in      string
		want    string
	}{
		{"boolean numeric zero", "BOOLEAN", "DEFAULT 0", "DEFAULT FALSE"},
		{"boolean numeric one", "BOOLEAN", "DEFAULT 1", "DEFAULT TRUE"},
		{"lowercase column type", "boolean", "DEFAULT 0", "DEFAULT FALSE"},
		{"padded clause", " BOOLEAN ", "  DEFAULT   0  ", "DEFAULT FALSE"},
		{"trailing modifiers preserved", "BOOLEAN", "DEFAULT 0 NOT NULL", "DEFAULT FALSE NOT NULL"},
		{"standard spelling untouched", "BOOLEAN", "DEFAULT FALSE", "DEFAULT FALSE"},
		{"empty clause untouched", "BOOLEAN", "", ""},
		{"integer column untouched", "INTEGER", "DEFAULT 0", "DEFAULT 0"},
		{"text column untouched", "TEXT", "DEFAULT 0", "DEFAULT 0"},
		{"quoted literal untouched", "BOOLEAN", "DEFAULT 'off'", "DEFAULT 'off'"},
		{"missing default keyword untouched", "BOOLEAN", "NOT NULL", "NOT NULL"},
		{"bare literal without keyword untouched", "BOOLEAN", "0", "0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeBooleanDefault(tc.colType, tc.in); got != tc.want {
				t.Errorf("normalizeBooleanDefault(%q, %q) = %q, want %q", tc.colType, tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildAddColumnDDL(t *testing.T) {
	cases := []struct {
		name    string
		table   string
		column  string
		colType string
		def     string
		want    string
	}{
		{
			name:    "with default",
			table:   "sites",
			column:  "use_system_proxy",
			colType: "BOOLEAN",
			def:     "DEFAULT FALSE",
			want:    "ALTER TABLE sites ADD COLUMN use_system_proxy BOOLEAN DEFAULT FALSE",
		},
		{
			name:    "nullable without default",
			table:   "sites",
			column:  "resin_enabled",
			colType: "BOOLEAN",
			def:     "",
			want:    "ALTER TABLE sites ADD COLUMN resin_enabled BOOLEAN",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildAddColumnDDL(tc.table, tc.column, tc.colType, tc.def); got != tc.want {
				t.Errorf("buildAddColumnDDL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBooleanColumnDDLIsDialectSafe walks the boolean shapes used by the
// registry and asserts the statement each dialect would execute. The numeric
// spelling must never reach a PostgreSQL BOOLEAN column.
func TestBooleanColumnDDLIsDialectSafe(t *testing.T) {
	cases := []struct {
		column        string
		sqliteType    string
		pgType        string
		defaultClause string
		wantSQLite    string
		wantPG        string
	}{
		{
			column:        "use_system_proxy",
			sqliteType:    "INTEGER",
			pgType:        "BOOLEAN",
			defaultClause: "DEFAULT 0",
			wantSQLite:    "ALTER TABLE sites ADD COLUMN use_system_proxy INTEGER DEFAULT 0",
			wantPG:        "ALTER TABLE sites ADD COLUMN use_system_proxy BOOLEAN DEFAULT FALSE",
		},
		{
			column:        "post_refresh_probe_enabled",
			sqliteType:    "INTEGER",
			pgType:        "BOOLEAN",
			defaultClause: "DEFAULT 0",
			wantSQLite:    "ALTER TABLE sites ADD COLUMN post_refresh_probe_enabled INTEGER DEFAULT 0",
			wantPG:        "ALTER TABLE sites ADD COLUMN post_refresh_probe_enabled BOOLEAN DEFAULT FALSE",
		},
		{
			column:        "is_manual",
			sqliteType:    "INTEGER",
			pgType:        "BOOLEAN",
			defaultClause: "DEFAULT 1",
			wantSQLite:    "ALTER TABLE model_availability ADD COLUMN is_manual INTEGER DEFAULT 1",
			wantPG:        "ALTER TABLE model_availability ADD COLUMN is_manual BOOLEAN DEFAULT TRUE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.column, func(t *testing.T) {
			table := "sites"
			if tc.column == "is_manual" {
				table = "model_availability"
			}

			gotSQLite := buildAddColumnDDL(table, tc.column,
				strings.TrimSpace(tc.sqliteType),
				normalizeBooleanDefault(strings.TrimSpace(tc.sqliteType), strings.TrimSpace(tc.defaultClause)))
			if gotSQLite != tc.wantSQLite {
				t.Errorf("sqlite DDL = %q, want %q", gotSQLite, tc.wantSQLite)
			}

			gotPG := buildAddColumnDDL(table, tc.column,
				strings.TrimSpace(tc.pgType),
				normalizeBooleanDefault(strings.TrimSpace(tc.pgType), strings.TrimSpace(tc.defaultClause)))
			if gotPG != tc.wantPG {
				t.Errorf("postgres DDL = %q, want %q", gotPG, tc.wantPG)
			}
			if strings.Contains(gotPG, "BOOLEAN DEFAULT 0") || strings.Contains(gotPG, "BOOLEAN DEFAULT 1") {
				t.Errorf("postgres DDL carries a numeric boolean default: %q", gotPG)
			}
		})
	}
}

// TestEnsureColumnBooleanNumericDefaultSQLite exercises the normalization on a
// live SQLite database, including the "BOOLEAN" column spelling, and pins that
// a repeated call stays a no-op.
func TestEnsureColumnBooleanNumericDefaultSQLite(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE boolean_probe (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}

	if err := EnsureColumn(db, "boolean_probe", "flag", "BOOLEAN", "BOOLEAN", "DEFAULT 0"); err != nil {
		t.Fatalf("EnsureColumn with numeric boolean default failed: %v", err)
	}
	if err := EnsureColumn(db, "boolean_probe", "flag", "BOOLEAN", "BOOLEAN", "DEFAULT 0"); err != nil {
		t.Fatalf("EnsureColumn re-run failed: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO boolean_probe (name) VALUES ('legacy')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	var flag bool
	if err := db.QueryRow(`SELECT flag FROM boolean_probe WHERE name = ?`, "legacy").Scan(&flag); err != nil {
		t.Fatalf("read flag: %v", err)
	}
	if flag {
		t.Errorf("flag = true, want false (DEFAULT 0 must land as false)")
	}
}

// TestAdditiveRegistryAvoidsNumericBooleanDefaults is a source gate over the
// registry: a BOOLEAN column must not be declared with a numeric default. The
// runtime normalizer already makes such a call safe, but the registry is read
// by humans reasoning about schema, so the two spellings are kept consistent.
func TestAdditiveRegistryAvoidsNumericBooleanDefaults(t *testing.T) {
	src, err := os.ReadFile("additive.go")
	if err != nil {
		t.Fatalf("read additive.go: %v", err)
	}

	callRe := regexp.MustCompile(`EnsureColumn\(\s*db\s*,\s*"([^"]+)"\s*,\s*"([^"]+)"\s*,\s*"([^"]*)"\s*,\s*"([^"]*)"\s*,\s*"([^"]*)"\s*\)`)
	matches := callRe.FindAllStringSubmatch(string(src), -1)
	if len(matches) < 20 {
		t.Fatalf("source gate matched only %d EnsureColumn call sites; the regexp no longer fits the registry", len(matches))
	}

	for _, m := range matches {
		table, column, sqliteType, pgType, defaultClause := m[1], m[2], m[3], m[4], m[5]
		for _, dialect := range []struct {
			name    string
			colType string
		}{{"sqlite", sqliteType}, {"postgres", pgType}} {
			if !strings.EqualFold(strings.TrimSpace(dialect.colType), "BOOLEAN") {
				continue
			}
			literal := strings.ToUpper(strings.TrimSpace(defaultClause))
			if literal == "DEFAULT 0" || literal == "DEFAULT 1" {
				t.Errorf("%s.%s: %s BOOLEAN column declared with numeric default %q; use DEFAULT FALSE/TRUE",
					table, column, dialect.name, defaultClause)
			}
		}
	}
}

// pgScratchDSN derives a dedicated database from PG_TEST_DSN so a legacy-shape
// probe never collides with the converged schema the rest of the PG suite
// builds in the shared integration database.
func pgScratchDSN(t *testing.T, suffix string) string {
	t.Helper()
	skipIfNoPG(t)

	base := pgDSN()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse PG_TEST_DSN: %v", err)
	}
	dbName := strings.TrimPrefix(u.Path, "/") + suffix
	if dbName == suffix || strings.ContainsAny(dbName, `"`+"`"+`'`) {
		t.Fatalf("unsafe derived db name %q", dbName)
	}

	admin, err := Open(DialectPostgres, base, false)
	if err != nil {
		t.Fatalf("open admin PG connection: %v", err)
	}
	defer admin.Close()

	var exists bool
	if err := admin.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = ?)`,
		dbName,
	).Scan(&exists); err != nil {
		t.Fatalf("check pg_database for %s: %v", dbName, err)
	}
	if !exists {
		if _, err := admin.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, dbName)); err != nil {
			t.Fatalf("create scratch database %s: %v — the PG_TEST_DSN user needs CREATEDB", dbName, err)
		}
	}

	u.Path = "/" + dbName
	return u.String()
}

// TestPostgresEnsureColumnBooleanNumericDefaultNormalized is the live probe for
// the safety net: a legacy-shaped PostgreSQL table plus the numeric spelling a
// SQLite-first registry entry would carry. Before normalization this failed
// with SQLSTATE 42804 and the process could not start.
func TestPostgresEnsureColumnBooleanNumericDefaultNormalized(t *testing.T) {
	skipIfNoPG(t)

	db, err := Open(DialectPostgres, pgDSN(), false)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	const table = "boolean_default_probe"
	if _, err := db.Exec(`DROP TABLE IF EXISTS ` + table + ` CASCADE`); err != nil {
		t.Fatalf("drop probe table: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP TABLE IF EXISTS ` + table + ` CASCADE`) })

	if _, err := db.Exec(`CREATE TABLE ` + table + ` (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create probe table: %v", err)
	}

	if err := EnsureColumn(db, table, "flag", "INTEGER", "BOOLEAN", "DEFAULT 0"); err != nil {
		t.Fatalf("EnsureColumn BOOLEAN + numeric default failed on postgres: %v", err)
	}
	if err := EnsureColumn(db, table, "flag", "INTEGER", "BOOLEAN", "DEFAULT 0"); err != nil {
		t.Fatalf("EnsureColumn re-run failed on postgres: %v", err)
	}

	var colType, colDefault string
	if err := db.QueryRow(
		`SELECT data_type, column_default FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
		table, "flag").Scan(&colType, &colDefault); err != nil {
		t.Fatalf("read column metadata: %v", err)
	}
	if colType != "boolean" {
		t.Errorf("column type = %q, want boolean", colType)
	}
	if colDefault != "false" {
		t.Errorf("column default = %q, want false", colDefault)
	}

	if _, err := db.Exec(`INSERT INTO ` + table + ` (name) VALUES ('legacy')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	var flag bool
	if err := db.QueryRow(`SELECT flag FROM `+table+` WHERE name = ?`, "legacy").Scan(&flag); err != nil {
		t.Fatalf("read flag: %v", err)
	}
	if flag {
		t.Errorf("flag = true, want false")
	}
}

// TestPostgresLegacyBooleanColumnsConverge replays the real registry steps that
// declare boolean columns against a PostgreSQL database shaped like an old
// install (tables present, additive columns absent). This is the upgrade path
// that could not start before the numeric defaults were normalized.
func TestPostgresLegacyBooleanColumnsConverge(t *testing.T) {
	dsn := pgScratchDSN(t, "_legacy_boolean")

	db, err := Open(DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("open scratch postgres database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	legacy := []string{
		`DROP TABLE IF EXISTS sites CASCADE`,
		`DROP TABLE IF EXISTS model_availability CASCADE`,
		// Pre-additive shapes: the columns every boolean step adds are absent.
		`CREATE TABLE sites (
			id BIGSERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			platform TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE model_availability (
			id BIGSERIAL PRIMARY KEY,
			account_id BIGINT NOT NULL,
			model_name TEXT NOT NULL,
			available BOOLEAN,
			checked_at TEXT
		)`,
	}
	for _, ddl := range legacy {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("prepare legacy schema (%s): %v", firstLine(ddl), err)
		}
	}

	if _, err := db.Exec(`INSERT INTO sites (name, url, platform) VALUES ('legacy-site', 'https://example.com', 'openai')`); err != nil {
		t.Fatalf("insert legacy site: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO model_availability (account_id, model_name, available) VALUES (1, 'gpt-4o', true)`); err != nil {
		t.Fatalf("insert legacy availability row: %v", err)
	}

	steps := []string{
		"sc2_017_ts_legacy_sites_columns",
		"sc2_018_site_post_refresh_probe",
		"sc2_024_model_availability_is_manual",
	}
	for _, version := range steps {
		step := findAdditiveStep(t, version)
		if err := step.Apply(db); err != nil {
			t.Fatalf("apply %s on a legacy postgres database: %v", version, err)
		}
		// Idempotent re-run: a crash between DDL and bookkeeping must not wedge
		// the next startup.
		if err := step.Apply(db); err != nil {
			t.Fatalf("re-apply %s on a legacy postgres database: %v", version, err)
		}
	}

	wantBoolean := []struct{ table, column string }{
		{"sites", "use_system_proxy"},
		{"sites", "post_refresh_probe_enabled"},
		{"model_availability", "is_manual"},
	}
	for _, col := range wantBoolean {
		var colType, colDefault string
		if err := db.QueryRow(
			`SELECT data_type, column_default FROM information_schema.columns
			 WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			col.table, col.column).Scan(&colType, &colDefault); err != nil {
			t.Fatalf("read %s.%s metadata: %v", col.table, col.column, err)
		}
		if colType != "boolean" {
			t.Errorf("%s.%s type = %q, want boolean", col.table, col.column, colType)
		}
		if colDefault != "false" {
			t.Errorf("%s.%s default = %q, want false", col.table, col.column, colDefault)
		}
	}

	var useSystemProxy, probeEnabled, isManual bool
	if err := db.QueryRow(`SELECT use_system_proxy, post_refresh_probe_enabled FROM sites WHERE name = ?`, "legacy-site").
		Scan(&useSystemProxy, &probeEnabled); err != nil {
		t.Fatalf("read legacy site booleans: %v", err)
	}
	if useSystemProxy || probeEnabled {
		t.Errorf("legacy site booleans = %v/%v, want false/false", useSystemProxy, probeEnabled)
	}
	if err := db.QueryRow(`SELECT is_manual FROM model_availability WHERE model_name = ?`, "gpt-4o").Scan(&isManual); err != nil {
		t.Fatalf("read legacy availability boolean: %v", err)
	}
	if isManual {
		t.Errorf("model_availability.is_manual = true, want false")
	}
}

func findAdditiveStep(t *testing.T, version string) AdditiveStep {
	t.Helper()
	for _, step := range enterpriseAdditiveSteps {
		if step.Version == version {
			return step
		}
	}
	t.Fatalf("additive step %q not found in the registry", version)
	return AdditiveStep{}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
