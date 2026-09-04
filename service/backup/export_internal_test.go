package backup

// The exported QueryTableAsJSON wrapper is gone: nothing in production called
// it, and its only caller was a test in the external test package, so the public
// surface existed to be tested rather than to be used. The guard behind it is
// still load-bearing -- a table name reaching queryTableAsJSON is interpolated
// into the statement, and IsKnownTable is the only thing between it and the SQL
// parser -- so it is proven here against the function production actually calls.

import (
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

func setupInternalBackupDB(t *testing.T) *sqlx.DB {
	t.Helper()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return db.DB
}

func TestQueryTableAsJSONRejectsUnknownTable(t *testing.T) {
	db := setupInternalBackupDB(t)

	for _, table := range []string{
		"settings; DROP TABLE settings",
		"settings--",
		"settings WHERE 1=1",
		"not_a_table",
		"",
	} {
		rows, err := queryTableAsJSON(db, table, nil)
		if err == nil {
			t.Errorf("queryTableAsJSON(%q) returned %d rows and no error, want an unknown-table rejection", table, len(rows))
			continue
		}
		if !strings.Contains(err.Error(), "unknown table") {
			t.Errorf("queryTableAsJSON(%q) error = %v, want it to name the unknown table", table, err)
		}
	}
}

func TestQueryTableAsJSONReadsAKnownTable(t *testing.T) {
	db := setupInternalBackupDB(t)

	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('theme', '"dark"')`); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	rows, err := queryTableAsJSON(db, "settings", nil)
	if err != nil {
		t.Fatalf("queryTableAsJSON(settings) = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want the one seeded row", len(rows))
	}
}
