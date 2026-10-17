package sqlitetest

import (
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestOpenMatchesProductionSchemaAndPragmas(t *testing.T) {
	want, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { want.Close() })
	if err := store.AutoMigrate(want); err != nil {
		t.Fatal(err)
	}
	got := Open(t)
	type schemaEntry struct {
		Type string `db:"type"`
		Name string `db:"name"`
		SQL  string `db:"sql"`
	}
	var wantSchema, gotSchema []schemaEntry
	const query = "SELECT type, name, COALESCE(sql, '') AS sql FROM sqlite_master ORDER BY type, name"
	if err := want.Select(&wantSchema, query); err != nil {
		t.Fatal(err)
	}
	if err := got.Select(&gotSchema, query); err != nil {
		t.Fatal(err)
	}
	if len(wantSchema) == 0 || !reflect.DeepEqual(wantSchema, gotSchema) {
		t.Fatal("restored schema does not match a freshly migrated database")
	}
	for _, pragma := range []string{"foreign_keys", "busy_timeout", "journal_mode", "synchronous", "cache_size"} {
		var wantValue, gotValue string
		if err := want.Get(&wantValue, "PRAGMA "+pragma); err != nil {
			t.Fatal(err)
		}
		if err := got.Get(&gotValue, "PRAGMA "+pragma); err != nil {
			t.Fatal(err)
		}
		if gotValue != wantValue {
			t.Errorf("%s = %s, want %s", pragma, gotValue, wantValue)
		}
	}
	var wantMigrations, gotMigrations int
	if err := want.Get(&wantMigrations, "SELECT COUNT(*) FROM schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if err := got.Get(&gotMigrations, "SELECT COUNT(*) FROM schema_migrations"); err != nil {
		t.Fatal(err)
	}
	if wantMigrations == 0 || gotMigrations != wantMigrations {
		t.Fatalf("migration journal = %d, want %d", gotMigrations, wantMigrations)
	}
}

func TestOpenIsolatesDataAndSchema(t *testing.T) {
	for range 8 {
		t.Run("independent", func(t *testing.T) {
			t.Parallel()
			db := Open(t)
			// Every database can create the same table and unique row independently.
			if _, err := db.Exec("CREATE TABLE fixture_isolation (id INTEGER PRIMARY KEY, value TEXT UNIQUE)"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO fixture_isolation VALUES (1, 'same-value')"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO fixture_isolation VALUES (2, 'same-value')"); err == nil {
				t.Fatal("unique constraint was not enforced")
			}
			if _, err := db.Exec("CREATE TABLE fixture_child (parent INTEGER REFERENCES fixture_isolation(id))"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO fixture_child VALUES (999)"); err == nil {
				t.Fatal("foreign key constraint was not enforced")
			}
		})
	}
}

func BenchmarkOpen(b *testing.B) {
	if _, err := schemaImage(); err != nil {
		b.Fatal(err)
	}
	b.Run("migrate", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			db, err := store.Open(store.DialectSQLite, ":memory:", false)
			if err != nil {
				b.Fatal(err)
			}
			if err := store.AutoMigrate(db); err != nil {
				db.Close()
				b.Fatal(err)
			}
			if err := db.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("restore", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			db, err := open()
			if err != nil {
				b.Fatal(err)
			}
			if err := db.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
