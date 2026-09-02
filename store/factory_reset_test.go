package store

import (
	"fmt"
	"strings"
	"testing"
)

// probeColumn is the minimum introspection needed to synthesize a valid row
// for any table without hardcoding 37 INSERT statements.
type probeColumn struct {
	name       string
	kind       string
	notNull    bool
	hasDefault bool
	isSerialPK bool
	// isTextPK marks a TEXT PRIMARY KEY. SQLite reports notnull=0 for
	// `key TEXT PRIMARY KEY` (the nullable-text-PK gap the schema audit
	// tracks separately), yet a NULL primary key is not a usable row, so the
	// probe supplies a value for it anyway.
	isTextPK bool
}

// probeLiteral renders a syntactically valid value for a column kind. Later
// attempts try other plausible literals so a CHECK constraint on one column
// does not make the whole probe impossible.
func probeLiteral(kind string, attempt int, dialect string) string {
	boolTrue, boolFalse := "1", "0"
	if dialect == DialectPostgres {
		boolTrue, boolFalse = "TRUE", "FALSE"
	}
	switch kind {
	case kindBool:
		if attempt%2 == 0 {
			return boolTrue
		}
		return boolFalse
	case kindInt:
		switch attempt {
		case 0:
			return "1"
		case 1:
			return "0"
		default:
			return "2"
		}
	case kindFloat:
		if attempt == 0 {
			return "1"
		}
		return "1.5"
	default:
		switch attempt {
		case 0:
			return "'probe'"
		case 1:
			return "'active'"
		case 2:
			return "'pending'"
		default:
			return "'success'"
		}
	}
}

// seedEveryTable inserts one row into every table of the schema registry, in
// FK-safe order, and fails loudly for any table that could not be seeded — a
// reset/copy assertion over an unseeded table would be vacuous.
func seedEveryTable(t *testing.T, db *DB) {
	t.Helper()
	var failed []string
	// Two passes: registry order is parents-first, but a self-contained
	// second pass catches any edge the first pass hit too early.
	for pass := 0; pass < 2; pass++ {
		failed = nil
		for _, table := range AllTableNames() {
			var n int
			if err := db.QueryRow(db.Rebind(fmt.Sprintf(`SELECT COUNT(*) FROM %q`, table))).Scan(&n); err != nil {
				t.Fatalf("count %s: %v", table, err)
			}
			if n > 0 {
				continue
			}
			insertProbeRowQuiet(db, table, &failed)
		}
		if len(failed) == 0 {
			return
		}
	}
	t.Fatalf("could not seed %d table(s): %v", len(failed), failed)
}

// insertProbeRowQuiet is insertProbeRow without t.Fatalf, so seedEveryTable
// can retry a table on a second pass.
func insertProbeRowQuiet(db *DB, table string, failed *[]string) {
	cols := make([]probeColumn, 0)
	if db.Dialect == DialectPostgres {
		rows, err := db.Query(db.Rebind(
			`SELECT column_name, is_nullable, COALESCE(column_default, '')
			 FROM information_schema.columns
			 WHERE table_schema = current_schema() AND table_name = ?
			 ORDER BY ordinal_position`), table)
		if err != nil {
			*failed = append(*failed, fmt.Sprintf("%s: %v", table, err))
			return
		}
		spec, _ := schemaColumns(table)
		kinds := make(map[string]string, len(spec))
		for _, c := range spec {
			kinds[c.name] = c.kind
		}
		for rows.Next() {
			var c probeColumn
			var nullable, def string
			if err := rows.Scan(&c.name, &nullable, &def); err != nil {
				rows.Close()
				*failed = append(*failed, fmt.Sprintf("%s: %v", table, err))
				return
			}
			c.kind = kinds[c.name]
			c.notNull = strings.EqualFold(nullable, "NO")
			c.hasDefault = strings.TrimSpace(def) != ""
			c.isSerialPK = strings.Contains(def, "nextval(")
			cols = append(cols, c)
		}
		rows.Close()
	} else {
		spec, _ := schemaColumns(table)
		kinds := make(map[string]string, len(spec))
		for _, c := range spec {
			kinds[c.name] = c.kind
		}
		rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
		if err != nil {
			*failed = append(*failed, fmt.Sprintf("%s: %v", table, err))
			return
		}
		for rows.Next() {
			var cid, notNull, pk int
			var name, colType string
			var dflt *string
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
				rows.Close()
				*failed = append(*failed, fmt.Sprintf("%s: %v", table, err))
				return
			}
			serialPK := pk == 1 && (kinds[name] == kindInt || strings.Contains(strings.ToUpper(colType), "INT"))
			cols = append(cols, probeColumn{
				name:       name,
				kind:       kinds[name],
				notNull:    notNull == 1,
				hasDefault: dflt != nil,
				isSerialPK: serialPK,
				isTextPK:   pk == 1 && !serialPK,
			})
		}
		rows.Close()
	}

	var names, kindsList []string
	for _, c := range cols {
		if c.isSerialPK || c.hasDefault || (!c.notNull && !c.isTextPK) {
			continue
		}
		names = append(names, fmt.Sprintf("%q", c.name))
		kindsList = append(kindsList, c.kind)
	}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		var query string
		if len(names) == 0 {
			query = fmt.Sprintf(`INSERT INTO %q DEFAULT VALUES`, table)
		} else {
			values := make([]string, len(kindsList))
			for i, kind := range kindsList {
				values[i] = probeLiteral(kind, attempt, db.Dialect)
			}
			query = fmt.Sprintf(`INSERT INTO %q (%s) VALUES (%s)`,
				table, strings.Join(names, ", "), strings.Join(values, ", "))
		}
		if _, err := db.Exec(query); err != nil {
			lastErr = err
			continue
		}
		return
	}
	*failed = append(*failed, fmt.Sprintf("%s: %v", table, lastErr))
}

// countRows returns the row count of a table.
func countRows(t *testing.T, db *DB, table string) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(db.Rebind(fmt.Sprintf(`SELECT COUNT(*) FROM %q`, table))).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestFactoryResetWipesEveryRegistryTableSQLite is the #7 acceptance on
// SQLite: seed one row into every table, reset, and require every table —
// admin_sessions included — to be empty.
func TestFactoryResetWipesEveryRegistryTableSQLite(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	seedEveryTable(t, db)
	for _, table := range FactoryResetTableNames() {
		if got := countRows(t, db, table); got == 0 {
			t.Fatalf("table %s is empty before the reset — the assertion would be vacuous", table)
		}
	}
	if got := countRows(t, db, "schema_migrations"); got == 0 {
		t.Fatal("schema_migrations is empty before the reset; AutoMigrate did not book its steps")
	}
	journalBefore := countRows(t, db, "schema_migrations")

	deleted, err := FactoryReset(db.DB)
	if err != nil {
		t.Fatalf("FactoryReset: %v", err)
	}

	var nonEmpty []string
	for _, table := range FactoryResetTableNames() {
		if got := countRows(t, db, table); got != 0 {
			nonEmpty = append(nonEmpty, fmt.Sprintf("%s=%d", table, got))
		}
		if _, ok := deleted[table]; !ok {
			t.Errorf("FactoryReset did not report a deleted count for %s", table)
		}
	}
	if len(nonEmpty) > 0 {
		t.Errorf("tables still holding rows after factory reset: %v", nonEmpty)
	}
	adminSessions := countRows(t, db, "admin_sessions")
	t.Logf("rows after factory reset: admin_sessions=%d, tables wiped=%d", adminSessions, len(deleted))
	if adminSessions != 0 {
		t.Errorf("admin_sessions holds %d row(s) after factory reset — pre-reset admin cookies stay valid", adminSessions)
	}
	if got := countRows(t, db, "schema_migrations"); got != journalBefore {
		t.Errorf("schema_migrations rows = %d after reset, want %d (the journal is bookkeeping, not business data)", got, journalBefore)
	}

	// A second reset on an already-empty database must not fail.
	if _, err := FactoryReset(db.DB); err != nil {
		t.Fatalf("second FactoryReset on an empty database: %v", err)
	}
}

// TestFactoryResetWipesEveryRegistryTablePG is the same acceptance on
// PostgreSQL, in a database derived from PG_TEST_DSN so the shared integration
// database is never wiped.
func TestFactoryResetWipesEveryRegistryTablePG(t *testing.T) {
	dsn := pgScratchDSN(t, "_factory_reset")
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

	seedEveryTable(t, db)
	for _, table := range FactoryResetTableNames() {
		if got := countRows(t, db, table); got == 0 {
			t.Fatalf("table %s is empty before the reset — the assertion would be vacuous", table)
		}
	}

	if _, err := FactoryReset(db.DB); err != nil {
		t.Fatalf("FactoryReset pg: %v", err)
	}
	var nonEmpty []string
	for _, table := range FactoryResetTableNames() {
		if got := countRows(t, db, table); got != 0 {
			nonEmpty = append(nonEmpty, fmt.Sprintf("%s=%d", table, got))
		}
	}
	if len(nonEmpty) > 0 {
		t.Errorf("tables still holding rows after pg factory reset: %v", nonEmpty)
	}
	if got := countRows(t, db, "admin_sessions"); got != 0 {
		t.Errorf("admin_sessions holds %d row(s) after pg factory reset", got)
	}
	if _, err := FactoryReset(db.DB); err != nil {
		t.Fatalf("second FactoryReset on an empty pg database: %v", err)
	}
}

// TestFactoryResetOnTakeoverDatabase is the data-safety rule: a factory reset
// must run to completion on a database that was not created by this version
// (the golden TS fixture, taken over in place), and must run twice.
func TestFactoryResetOnTakeoverDatabase(t *testing.T) {
	fixture := copyTSFixture(t)
	db, err := Open(DialectSQLite, fixture, false)
	if err != nil {
		t.Fatalf("Open fixture: %v", err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate on TS takeover database: %v", err)
	}
	if got := countRows(t, db, "sites"); got == 0 {
		t.Fatal("fixture has no sites; the takeover database is not the golden fixture")
	}
	if _, err := FactoryReset(db.DB); err != nil {
		t.Fatalf("FactoryReset on TS takeover database: %v", err)
	}
	for _, table := range FactoryResetTableNames() {
		if got := countRows(t, db, table); got != 0 {
			t.Errorf("%s holds %d row(s) after resetting a takeover database", table, got)
		}
	}
	if _, err := FactoryReset(db.DB); err != nil {
		t.Fatalf("second FactoryReset on takeover database: %v", err)
	}
}

// TestFactoryResetTableSetIsDerivedAndFKSafe pins the derivation: the reset
// set is the registry minus the explicit exclusion map, it is FK-safe
// (children deleted before parents), it never contains the migration journal,
// and it does contain admin_sessions.
func TestFactoryResetTableSetIsDerivedAndFKSafe(t *testing.T) {
	reset := FactoryResetTableNames()
	resetSet := make(map[string]bool, len(reset))
	for _, table := range reset {
		if resetSet[table] {
			t.Fatalf("factory reset list contains %s twice", table)
		}
		resetSet[table] = true
	}

	facts := schemaFactsVal
	if facts == nil {
		facts = buildSchemaFacts()
	}
	if facts.err != nil {
		t.Fatalf("schema registry: %v", facts.err)
	}
	for _, table := range facts.registry {
		_, excluded := factoryResetExcludedTables[table]
		if resetSet[table] == excluded {
			t.Errorf("table %s: in reset set = %t, excluded = %t — every registry table must be reset unless explicitly excluded with a reason",
				table, resetSet[table], excluded)
		}
		if excluded && strings.TrimSpace(factoryResetExcludedTables[table]) == "" {
			t.Errorf("table %s is excluded from the factory reset without a reason", table)
		}
	}
	for table, reason := range factoryResetExcludedTables {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("excluded table %s has an empty reason", table)
		}
		if resetSet[table] {
			t.Errorf("excluded table %s is still in the reset set", table)
		}
	}
	if resetSet["schema_migrations"] {
		t.Error("schema_migrations must not be wiped by a factory reset")
	}
	if !resetSet["admin_sessions"] {
		t.Error("admin_sessions must be wiped by a factory reset (pre-reset admin cookies would stay valid)")
	}

	// FK-safe: every referencing table is deleted before the table it points at.
	position := make(map[string]int, len(reset))
	for i, table := range reset {
		position[table] = i
	}
	for _, table := range reset {
		for _, parent := range facts.parents[table] {
			p, ok := position[parent]
			if !ok {
				continue
			}
			if position[table] > p {
				t.Errorf("%s is deleted after its FK parent %s", table, parent)
			}
		}
	}
}
