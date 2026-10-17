package pgtest

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

func TestResetCurrentSchema(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	// Keep session search_path on one connection; neither schema touches the
	// integration database's existing fixtures.
	db.SetMaxOpenConns(1)
	suffix := uuid.NewString()
	own := pgx.Identifier{`pgtest_own_"` + suffix}.Sanitize()
	sibling := pgx.Identifier{"pgtest_sibling_" + suffix}.Sanitize()
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("execute fixture SQL: %v", err)
		}
	}
	for _, schema := range []string{own, sibling} {
		exec("CREATE SCHEMA " + schema)
		t.Cleanup(func() {
			if _, err := db.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Errorf("drop fixture schema: %v", err)
			}
		})
	}
	exec("SET search_path TO " + own + ", " + sibling)
	Reset(t, db) // A fresh schema is a valid reset target.

	// Quoted table/schema identifiers exercise the generated TRUNCATE statement.
	exec(`CREATE TABLE "parent""rows" (id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY)`)
	exec(`CREATE TABLE children (parent_id integer REFERENCES "parent""rows"(id))`)
	exec(`CREATE TABLE schema_migrations (version integer PRIMARY KEY)`)
	exec(`INSERT INTO "parent""rows" DEFAULT VALUES`)
	exec(`INSERT INTO children VALUES (1)`)
	exec(`INSERT INTO schema_migrations VALUES (42)`)
	exec("CREATE TABLE " + sibling + ".sentinel (value text)")
	exec("INSERT INTO " + sibling + ".sentinel VALUES ('untouched')")

	Reset(t, db)
	for _, table := range []string{`"parent""rows"`, "children"} {
		var count int
		if err := db.Get(&count, "SELECT count(*) FROM "+table); err != nil {
			t.Fatalf("count reset table: %v", err)
		}
		if count != 0 {
			t.Errorf("%s row count = %d, want 0", table, count)
		}
	}
	var version int
	if err := db.Get(&version, "SELECT version FROM schema_migrations"); err != nil {
		t.Fatalf("read preserved migration journal: %v", err)
	}
	if version != 42 {
		t.Errorf("migration version = %d, want 42", version)
	}
	var sentinel string
	if err := db.Get(&sentinel, "SELECT value FROM "+sibling+".sentinel"); err != nil {
		t.Fatalf("read sibling sentinel: %v", err)
	}
	if sentinel != "untouched" {
		t.Errorf("sibling sentinel = %q, want untouched", sentinel)
	}
	var id int
	if err := db.Get(&id, `INSERT INTO "parent""rows" DEFAULT VALUES RETURNING id`); err != nil {
		t.Fatalf("insert after reset: %v", err)
	}
	if id != 1 {
		t.Errorf("identity after reset = %d, want 1", id)
	}

	t.Run("reject cross-schema foreign keys", func(t *testing.T) {
		exec("CREATE TABLE " + sibling + `.references_own (parent_id integer REFERENCES ` + own + `."parent""rows"(id))`)
		exec("INSERT INTO " + sibling + ".references_own VALUES (1)")
		err := reset(db)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "0A000" {
			t.Fatalf("reset error = %v, want PostgreSQL foreign-key TRUNCATE refusal (0A000)", err)
		}
		for _, table := range []string{own + `."parent""rows"`, sibling + ".references_own", sibling + ".sentinel"} {
			var count int
			if err := db.Get(&count, "SELECT count(*) FROM "+table); err != nil {
				t.Fatalf("count after refused reset: %v", err)
			}
			if count != 1 {
				t.Errorf("%s row count after refused reset = %d, want 1", table, count)
			}
		}
	})
}
