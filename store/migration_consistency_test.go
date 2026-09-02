package store

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// countLegacyTimestampValues counts the values that still carry the TS-era
// 'YYYY-MM-DD HH:MM:SS' shape. It uses the production introspection
// (legacyTimestampColumns) and the production LIKE pattern, so the measurement
// covers exactly the columns the sweep is responsible for — no hand-written
// column list that could hide a miss.
func countLegacyTimestampValues(t *testing.T, db *DB) (int64, []string) {
	t.Helper()
	cols, err := legacyTimestampColumns(db)
	if err != nil {
		t.Fatalf("legacyTimestampColumns: %v", err)
	}
	var total int64
	var detail []string
	for _, tc := range cols {
		var n int64
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %q WHERE %q LIKE ?`, tc.table, tc.column)
		if err := db.QueryRow(db.Rebind(query), legacyTimestampPattern).Scan(&n); err != nil {
			t.Fatalf("count TS-shaped values in %s.%s: %v", tc.table, tc.column, err)
		}
		if n > 0 {
			total += n
			detail = append(detail, fmt.Sprintf("%s.%s=%d", tc.table, tc.column, n))
		}
	}
	return total, detail
}

// TestCopyMigrationNormalizesTSTimestampsSQLite is the #1 acceptance on a
// SQLite target: the golden TS fixture carries TS-shaped timestamps, the copy
// migration runs AutoMigrate on an EMPTY target first and inserts the rows
// afterwards, so zero TS-shaped values may survive in the target.
func TestCopyMigrationNormalizesTSTimestampsSQLite(t *testing.T) {
	measure := copyTSFixture(t)
	src, err := Open(DialectSQLite, measure, false)
	if err != nil {
		t.Fatalf("open fixture copy: %v", err)
	}
	srcTotal, srcDetail := countLegacyTimestampValues(t, src)
	src.Close()
	if srcTotal == 0 {
		t.Fatalf("golden fixture carries no TS-shaped timestamps (%v): the assertion below would be vacuous", srcDetail)
	}
	t.Logf("source fixture carries %d TS-shaped timestamp value(s): %v", srcTotal, srcDetail)

	fixture := copyTSFixture(t)
	target := filepath.Join(t.TempDir(), "target.db")
	var logLines strings.Builder
	if _, err := RunMigration(RunMigrationOptions{
		FromPath:  fixture,
		ToURL:     target,
		Overwrite: true,
		Verify:    true,
		LogWriter: &logLines,
	}); err != nil {
		t.Fatalf("RunMigration: %v\nlog:\n%s", err, logLines.String())
	}

	db, err := Open(DialectSQLite, target, false)
	if err != nil {
		t.Fatalf("open migrated target: %v", err)
	}
	defer db.Close()
	got, detail := countLegacyTimestampValues(t, db)
	if got != 0 {
		t.Errorf("migrated target still carries %d TS-shaped timestamp value(s): %v (source had %d)\nlog:\n%s",
			got, detail, srcTotal, logLines.String())
	}
	if n := countRows(t, db, "sites"); n == 0 {
		t.Error("migrated target has no sites; the copy itself did not happen")
	}
}

// TestCopyMigrationNormalizesTSTimestampsPG is the same acceptance on a
// PostgreSQL target, in a database derived from PG_TEST_DSN.
func TestCopyMigrationNormalizesTSTimestampsPG(t *testing.T) {
	measure := copyTSFixture(t)
	src, err := Open(DialectSQLite, measure, false)
	if err != nil {
		t.Fatalf("open fixture copy: %v", err)
	}
	srcTotal, srcDetail := countLegacyTimestampValues(t, src)
	src.Close()
	if srcTotal == 0 {
		t.Fatalf("golden fixture carries no TS-shaped timestamps (%v): the assertion below would be vacuous", srcDetail)
	}

	targetDSN := pgCompatDSN(t)
	if _, err := RunMigration(RunMigrationOptions{
		FromPath:  copyTSFixture(t),
		ToURL:     targetDSN,
		Overwrite: true,
		Verify:    true,
		LogWriter: io.Discard,
	}); err != nil {
		t.Fatalf("RunMigration pg: %v", err)
	}

	db, err := Open(DialectPostgres, targetDSN, false)
	if err != nil {
		t.Fatalf("open migrated pg target: %v", err)
	}
	defer db.Close()
	got, detail := countLegacyTimestampValues(t, db)
	if got != 0 {
		t.Errorf("migrated pg target still carries %d TS-shaped timestamp value(s): %v (source had %d)", got, detail, srcTotal)
	}
}

// TestAutoMigrateSweepNormalizesLateValuesPG is the #1 acceptance on the
// startup path for PostgreSQL: AutoMigrate an empty database, write a
// TS-shaped value afterwards, run AutoMigrate again (the next boot) and
// require the value to be normalized. This is the shape a journal gate can
// never satisfy, because the gate was booked while the database was empty.
func TestAutoMigrateSweepNormalizesLateValuesPG(t *testing.T) {
	skipIfNoPG(t)
	dsn := pgScratchDSN(t, "_sweep")
	db, err := Open(DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("Open pg: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA public CASCADE`); err != nil {
		t.Fatalf("reset scratch schema: %v", err)
	}
	if _, err := db.Exec(`CREATE SCHEMA public`); err != nil {
		t.Fatalf("recreate scratch schema: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate pg: %v", err)
	}

	if _, err := db.Exec(db.Rebind(`INSERT INTO sites (id, name, url, platform, status, created_at, updated_at)
		VALUES (990004, 'late-ts-pg', 'https://late-ts-pg.example.com', 'openai', 'active', '2026-08-20 12:31:20', '2026-08-20 12:31:20')`)); err != nil {
		t.Fatalf("seed late TS row: %v", err)
	}
	defer func() {
		if _, err := db.Exec(db.Rebind(`DELETE FROM sites WHERE id = 990004`)); err != nil {
			t.Logf("cleanup: %v", err)
		}
	}()

	if before, _ := countLegacyTimestampValues(t, db); before == 0 {
		t.Fatal("seeded TS-shaped value was not counted; the measurement is vacuous")
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second AutoMigrate pg: %v", err)
	}
	var got string
	if err := db.QueryRow(db.Rebind(`SELECT created_at FROM sites WHERE id = 990004`)).Scan(&got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != "2026-08-20T12:31:20Z" {
		t.Errorf("created_at after restart = %q, want RFC3339 '2026-08-20T12:31:20Z'", got)
	}
	if after, detail := countLegacyTimestampValues(t, db); after != 0 {
		t.Errorf("pg database still carries %d TS-shaped value(s) after the second boot: %v", after, detail)
	}
}

// actualUserTables lists the tables a database really holds, measured from the
// catalog (sqlite_master / information_schema.tables) rather than from any
// Go-side list.
func actualUserTables(t *testing.T, db *DB) []string {
	t.Helper()
	query := `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	if db.Dialect == DialectPostgres {
		query = `SELECT table_name FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'
			ORDER BY table_name`
	}
	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		if name == "schema_migrations" {
			continue
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	return out
}

// assertTableSetMatchesCreatedSchema is the #3 acceptance core: the migration
// table set must equal the set of tables AutoMigrate actually creates, minus
// the tables explicitly excluded with a reason. Counts are measured, never
// hardcoded.
func assertTableSetMatchesCreatedSchema(t *testing.T, db *DB) {
	t.Helper()
	created := actualUserTables(t, db)
	createdSet := make(map[string]bool, len(created))
	for _, table := range created {
		createdSet[table] = true
	}

	migrated := AllTableNames()
	migratedSet := make(map[string]bool, len(migrated))
	for _, table := range migrated {
		if migratedSet[table] {
			t.Fatalf("AllTableNames lists %s twice", table)
		}
		migratedSet[table] = true
	}

	var missing, extra []string
	for _, table := range created {
		if _, excluded := migrationExcludedTables[table]; excluded {
			continue
		}
		if !migratedSet[table] {
			missing = append(missing, table)
		}
	}
	for _, table := range migrated {
		if !createdSet[table] {
			extra = append(extra, table)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d created table(s) are not copied by cmd/migrate and are not in migrationExcludedTables: %v", len(missing), missing)
	}
	if len(extra) > 0 {
		t.Errorf("%d copied table(s) do not exist in the schema AutoMigrate created: %v", len(extra), extra)
	}
	for table, reason := range migrationExcludedTables {
		if !createdSet[table] {
			t.Errorf("migrationExcludedTables names %s, which AutoMigrate does not create", table)
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("migrationExcludedTables excludes %s without a reason", table)
		}
	}

	// The registry is the single source: no duplicates, and ClearTableNames
	// must be the same set in the opposite (FK-safe) order.
	registry := SchemaTableNames()
	seen := make(map[string]bool, len(registry))
	for _, table := range registry {
		if seen[table] {
			t.Fatalf("schema registry lists %s twice", table)
		}
		seen[table] = true
	}
	if len(registry) != len(created) {
		t.Errorf("schema registry holds %d tables, AutoMigrate created %d", len(registry), len(created))
	}
	cleared := ClearTableNames()
	if len(cleared) != len(migrated) {
		t.Fatalf("ClearTableNames has %d entries, AllTableNames has %d", len(cleared), len(migrated))
	}
	for i, table := range cleared {
		if table != migrated[len(migrated)-1-i] {
			t.Fatalf("ClearTableNames[%d] = %s, want the reverse of AllTableNames (%s)", i, table, migrated[len(migrated)-1-i])
		}
	}
	t.Logf("measured table coverage: %d/%d created tables copied", len(migrated), len(created))
}

func TestMigrationTableSetMatchesCreatedSchemaSQLite(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	assertTableSetMatchesCreatedSchema(t, db)
}

func TestMigrationTableSetMatchesCreatedSchemaPG(t *testing.T) {
	dsn := pgScratchDSN(t, "_tableset")
	db, err := Open(DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("Open pg: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP SCHEMA public CASCADE`); err != nil {
		t.Fatalf("reset scratch schema: %v", err)
	}
	if _, err := db.Exec(`CREATE SCHEMA public`); err != nil {
		t.Fatalf("recreate scratch schema: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate pg: %v", err)
	}
	assertTableSetMatchesCreatedSchema(t, db)
}

// TestCopyMigrationTransfersEveryRegistryTable is the row-level half of the
// #3 acceptance: a source database holding one row in EVERY table must land
// with the same per-table row counts in the target. Tables the golden fixture
// does not have (oauth_route_units, model_probe_results, the usage
// projections, ...) are exactly the ones that used to be dropped.
func TestCopyMigrationTransfersEveryRegistryTable(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	targetPath := filepath.Join(dir, "target.db")

	src, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("Open source: %v", err)
	}
	if err := AutoMigrate(src); err != nil {
		t.Fatalf("AutoMigrate source: %v", err)
	}
	seedEveryTable(t, src)
	want := make(map[string]int64, len(AllTableNames()))
	for _, table := range AllTableNames() {
		n := countRows(t, src, table)
		if n == 0 {
			t.Fatalf("source table %s is empty before the copy — the assertion would be vacuous", table)
		}
		want[table] = n
	}
	src.Close()

	var logLines strings.Builder
	if _, err := RunMigration(RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     targetPath,
		Overwrite: true,
		Verify:    true,
		LogWriter: &logLines,
	}); err != nil {
		t.Fatalf("RunMigration: %v\nlog:\n%s", err, logLines.String())
	}

	tgt, err := Open(DialectSQLite, targetPath, false)
	if err != nil {
		t.Fatalf("Open target: %v", err)
	}
	defer tgt.Close()

	var mismatch []string
	for _, table := range AllTableNames() {
		if got := countRows(t, tgt, table); got != want[table] {
			mismatch = append(mismatch, fmt.Sprintf("%s: source=%d target=%d", table, want[table], got))
		}
	}
	if len(mismatch) > 0 {
		sort.Strings(mismatch)
		t.Errorf("%d table(s) did not survive the copy:\n  %s\nlog:\n%s",
			len(mismatch), strings.Join(mismatch, "\n  "), logLines.String())
	}
	if len(mismatch) == 0 {
		t.Logf("copied %d/%d tables with matching row counts", len(want), len(AllTableNames()))
	}

	// Booleans must land as SQLite integers, not as the TEXT 'true'/'false'
	// the driver binds a Go bool as: an INTEGER-affinity column keeps
	// non-numeric text as text, so `WHERE enabled = 1` would never match.
	for _, probe := range []struct{ table, column string }{
		{"oauth_route_units", "enabled"},
		{"sites", "use_system_proxy"},
		{"model_availability", "is_manual"},
	} {
		var typeof string
		if err := tgt.QueryRow(fmt.Sprintf(`SELECT typeof(%q) FROM %q`, probe.column, probe.table)).Scan(&typeof); err != nil {
			t.Fatalf("typeof %s.%s: %v", probe.table, probe.column, err)
		}
		if typeof != "integer" && typeof != "null" {
			t.Errorf("%s.%s is stored as %s in the SQLite target, want integer (a Go bool bound directly lands as text)",
				probe.table, probe.column, typeof)
		}
	}
}

// TestCopyMigrationOverwriteClearsEveryRegistryTable pins the --overwrite leg:
// stale rows in tables that used to be missing from the clear list must not
// survive a re-migration and mix with the new data.
func TestCopyMigrationOverwriteClearsEveryRegistryTable(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.db")
	targetPath := filepath.Join(dir, "target.db")

	src, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("Open source: %v", err)
	}
	if err := AutoMigrate(src); err != nil {
		t.Fatalf("AutoMigrate source: %v", err)
	}
	seedEveryTable(t, src)
	src.Close()

	// Pre-pollute the target with stale rows in every table, then re-migrate
	// with --overwrite and require exactly the source counts (stale + new
	// would double them).
	tgt, err := Open(DialectSQLite, targetPath, false)
	if err != nil {
		t.Fatalf("Open target: %v", err)
	}
	if err := AutoMigrate(tgt); err != nil {
		t.Fatalf("AutoMigrate target: %v", err)
	}
	seedEveryTable(t, tgt)
	tgt.Close()

	if _, err := RunMigration(RunMigrationOptions{
		FromPath:  sourcePath,
		ToURL:     targetPath,
		Overwrite: true,
		Verify:    true,
		LogWriter: io.Discard,
	}); err != nil {
		t.Fatalf("RunMigration with overwrite: %v", err)
	}

	src2, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatalf("reopen source: %v", err)
	}
	defer src2.Close()
	tgt2, err := Open(DialectSQLite, targetPath, false)
	if err != nil {
		t.Fatalf("reopen target: %v", err)
	}
	defer tgt2.Close()

	var stale []string
	for _, table := range AllTableNames() {
		s, g := countRows(t, src2, table), countRows(t, tgt2, table)
		if s != g {
			stale = append(stale, fmt.Sprintf("%s: source=%d target=%d", table, s, g))
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("--overwrite left stale rows behind:\n  %s", strings.Join(stale, "\n  "))
	}
}
