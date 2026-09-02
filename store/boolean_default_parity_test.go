package store

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// #12: a boolean column must carry the SAME default text on a fresh install
// and on a database that was converged by the additive registry. The two are
// value-equivalent (SQLite accepts DEFAULT 0 and DEFAULT FALSE for the same
// INTEGER column) but the schema text is what an operator, a diff tool and
// information_schema see, so drift here means the three database shapes
// (fresh / takeover / copy-migrated) are not provably identical.
//
// The comparison set is derived from the DDL registry (every column whose
// PostgreSQL spelling is BOOLEAN), never hardcoded, so a new boolean column
// is covered automatically.

// booleanSpecColumns lists (table, column) pairs the schema declares as
// BOOLEAN on PostgreSQL.
func booleanSpecColumns(t *testing.T) []tableColumn {
	t.Helper()
	var out []tableColumn
	for _, table := range SchemaTableNames() {
		cols, err := schemaColumns(table)
		if err != nil {
			t.Fatalf("schemaColumns(%s): %v", table, err)
		}
		for _, c := range cols {
			if c.kind == kindBool {
				out = append(out, tableColumn{table: table, column: c.name})
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("the schema registry declares no boolean column; the parity assertion would be vacuous")
	}
	return out
}

// sqliteDefaults reads PRAGMA table_info dflt_value for the given columns.
// A column the database does not have is omitted from the map.
func sqliteDefaults(t *testing.T, db *DB, cols []tableColumn) map[string]string {
	t.Helper()
	out := make(map[string]string)
	byTable := make(map[string][]string)
	for _, c := range cols {
		byTable[c.table] = append(byTable[c.table], c.column)
	}
	tables := make([]string, 0, len(byTable))
	for table := range byTable {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		exists, err := tableExists(db, table)
		if err != nil {
			t.Fatalf("tableExists(%s): %v", table, err)
		}
		if !exists {
			continue
		}
		rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%q)`, table))
		if err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		for rows.Next() {
			var cid, notNull, pk int
			var name, colType string
			var dflt *string
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
				rows.Close()
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			for _, want := range byTable[table] {
				if want == name {
					value := ""
					if dflt != nil {
						value = strings.TrimSpace(*dflt)
					}
					out[table+"."+name] = value
				}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iterate table_info(%s): %v", table, err)
		}
		rows.Close()
	}
	return out
}

// pgDefaults reads information_schema.columns.column_default for the given
// columns, which is exactly the text an operator compares between databases.
func pgDefaults(t *testing.T, db *DB, cols []tableColumn) map[string]string {
	t.Helper()
	out := make(map[string]string)
	rows, err := db.Query(`SELECT table_name, column_name, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND data_type = 'boolean'
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		t.Fatalf("read pg boolean defaults: %v", err)
	}
	defer rows.Close()
	live := make(map[string]string)
	for rows.Next() {
		var table, column, def string
		if err := rows.Scan(&table, &column, &def); err != nil {
			t.Fatalf("scan pg column default: %v", err)
		}
		live[table+"."+column] = strings.TrimSpace(def)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate pg column defaults: %v", err)
	}
	for _, c := range cols {
		if def, ok := live[c.table+"."+c.column]; ok {
			out[c.table+"."+c.column] = def
		}
	}
	return out
}

// assertDefaultParity compares two snapshots over their intersection and
// requires the intersection to be large enough to be meaningful.
func assertDefaultParity(t *testing.T, fresh, converged map[string]string) {
	t.Helper()
	keys := make([]string, 0, len(fresh))
	for key := range fresh {
		if _, ok := converged[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) < 4 {
		t.Fatalf("only %d boolean column(s) are comparable between the two shapes (%v); the parity assertion would be near-vacuous", len(keys), keys)
	}
	var drift []string
	withDefault := 0
	for _, key := range keys {
		if fresh[key] != converged[key] {
			drift = append(drift, fmt.Sprintf("%s: fresh=%q converged=%q", key, fresh[key], converged[key]))
		}
		if fresh[key] != "" {
			withDefault++
		}
	}
	if withDefault == 0 {
		t.Errorf("no comparable boolean column carries a default at all; nothing was actually compared")
	}
	if len(drift) > 0 {
		t.Errorf("%d boolean column(s) have different default text on a fresh install vs a converged one:\n  %s",
			len(drift), strings.Join(drift, "\n  "))
	}
	t.Logf("compared %d boolean column default(s), %d of them non-empty", len(keys), withDefault)
}

// TestBooleanDefaultParitySQLite compares a fresh AutoMigrate database with a
// legacy-shape database converged by the additive registry (the takeover path
// the registry exists for).
func TestBooleanDefaultParitySQLite(t *testing.T) {
	cols := booleanSpecColumns(t)

	fresh, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open fresh: %v", err)
	}
	defer fresh.Close()
	if err := AutoMigrate(fresh); err != nil {
		t.Fatalf("AutoMigrate fresh: %v", err)
	}
	freshDefaults := sqliteDefaults(t, fresh, cols)

	converged, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open legacy: %v", err)
	}
	defer converged.Close()
	for _, ddl := range legacySchemaDDL {
		if _, err := converged.Exec(ddl); err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	if err := ApplyAdditiveMigrations(converged); err != nil {
		t.Fatalf("ApplyAdditiveMigrations on legacy schema: %v", err)
	}
	convergedDefaults := sqliteDefaults(t, converged, cols)

	assertDefaultParity(t, freshDefaults, convergedDefaults)
}

// TestBooleanDefaultParityPG compares information_schema.columns.column_default
// between a freshly migrated database and the same database after its boolean
// columns were dropped and re-added by the additive registry — the PostgreSQL
// takeover shape. Both snapshots come from one derived database, so nothing
// else in the PG suite is touched.
func TestBooleanDefaultParityPG(t *testing.T) {
	dsn := pgScratchDSN(t, "_bool_parity")
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

	cols := booleanSpecColumns(t)
	freshDefaults := pgDefaults(t, db, cols)
	if len(freshDefaults) < 4 {
		t.Fatalf("fresh pg database reports only %d boolean column(s): %v", len(freshDefaults), freshDefaults)
	}

	// Takeover shape: drop every boolean column, forget the journal so the
	// registry re-adds them through the real EnsureColumn path, and re-converge.
	for _, c := range cols {
		if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE %q DROP COLUMN IF EXISTS %q`, c.table, c.column)); err != nil {
			t.Fatalf("drop %s.%s: %v", c.table, c.column, err)
		}
	}
	if _, err := db.Exec(`DELETE FROM schema_migrations`); err != nil {
		t.Fatalf("clear migration journal: %v", err)
	}
	if err := ApplyAdditiveMigrations(db); err != nil {
		t.Fatalf("ApplyAdditiveMigrations on pg takeover shape: %v", err)
	}
	convergedDefaults := pgDefaults(t, db, cols)

	assertDefaultParity(t, freshDefaults, convergedDefaults)
}
