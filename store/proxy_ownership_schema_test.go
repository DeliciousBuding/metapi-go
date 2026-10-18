package store

import "testing"

// openOwnershipSchemaDB opens an isolated database on the requested dialect
// with the production schema applied. PostgreSQL uses a scratch database so
// the shared PG_TEST_DSN fixture cannot collide with other packages.
func openOwnershipSchemaDB(t *testing.T, dialect string) *DB {
	t.Helper()
	if dialect == DialectPostgres {
		dsn := pgScratchDSN(t, "_ownership")
		db, err := Open(DialectPostgres, dsn, false)
		if err != nil {
			t.Fatalf("open pg: %v", err)
		}
		if _, err := db.Exec(`DROP SCHEMA public CASCADE`); err != nil {
			db.Close()
			t.Fatalf("reset schema: %v", err)
		}
		if _, err := db.Exec(`CREATE SCHEMA public`); err != nil {
			db.Close()
			t.Fatalf("recreate schema: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		if err := AutoMigrate(db); err != nil {
			t.Fatalf("AutoMigrate pg: %v", err)
		}
		return db
	}
	return openTestSQLite(t)
}

// TestProxyOwnershipSchemaConverges pins both halves of the storage contract
// for the Files/videos ownership fix: a fresh install gets the new table and
// column from the base DDL, and an install that predates them converges through
// the additive registry. It runs on both dialects because the DDL and the
// ON CONFLICT insert are dialect-specific.
func TestProxyOwnershipSchemaConverges(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := openOwnershipSchemaDB(t, dialect)

			for _, table := range []string{"proxy_file_objects"} {
				exists, err := tableExists(db, table)
				if err != nil || !exists {
					t.Fatalf("table %s missing after AutoMigrate (err=%v)", table, err)
				}
			}
			for _, column := range []string{"file_id", "owner"} {
				ok, err := columnExists(db, "proxy_file_objects", column)
				if err != nil || !ok {
					t.Fatalf("proxy_file_objects.%s missing after AutoMigrate (err=%v)", column, err)
				}
			}
			ok, err := columnExists(db, "proxy_video_tasks", "owner")
			if err != nil || !ok {
				t.Fatalf("proxy_video_tasks.owner missing after AutoMigrate (err=%v)", err)
			}

			// Simulate a database created before the ownership fix: the column is
			// gone and the additive step is not recorded yet.
			if _, err := db.Exec(`ALTER TABLE proxy_video_tasks DROP COLUMN owner`); err != nil {
				t.Fatalf("drop owner: %v", err)
			}
			if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 'proxy_video_task_owner_v1'`); err != nil {
				t.Fatalf("forget additive step: %v", err)
			}
			if err := ApplyAdditiveMigrations(db); err != nil {
				t.Fatalf("ApplyAdditiveMigrations: %v", err)
			}
			converged, err := columnExists(db, "proxy_video_tasks", "owner")
			if err != nil || !converged {
				t.Fatalf("proxy_video_tasks.owner not re-added by the additive step (err=%v)", err)
			}

			// Legacy rows keep NULL: the handler treats that as "no owner" and
			// denies access, never as a shared row.
			if _, err := db.Exec(`INSERT INTO proxy_video_tasks (public_id, upstream_video_id, site_url, token_value)
				VALUES ('video_schema_legacy', 'upstream_schema_legacy', '', '')`); err != nil {
				t.Fatalf("insert legacy row: %v", err)
			}
			var owner *string
			if err := db.QueryRow(`SELECT owner FROM proxy_video_tasks WHERE public_id = 'video_schema_legacy'`).Scan(&owner); err != nil {
				t.Fatalf("read legacy owner: %v", err)
			}
			if owner != nil {
				t.Fatalf("legacy owner = %q, want NULL", *owner)
			}

			// The upload-side insert keeps the first owner when an id repeats.
			if _, err := db.Exec(`INSERT INTO proxy_file_objects (file_id, owner, created_at, updated_at)
				VALUES ('file_schema_shared', 'owner-one', '', '')`); err != nil {
				t.Fatalf("insert file owner: %v", err)
			}
			res, err := db.Exec(`INSERT INTO proxy_file_objects (file_id, owner, created_at, updated_at)
				VALUES ('file_schema_shared', 'owner-two', '', '')
				ON CONFLICT(file_id) DO NOTHING`)
			if err != nil {
				t.Fatalf("conflicting file owner insert: %v", err)
			}
			if n, err := res.RowsAffected(); err != nil || n != 0 {
				t.Fatalf("conflicting insert rows = %d (err=%v), want 0", n, err)
			}
			var stored string
			if err := db.QueryRow(`SELECT owner FROM proxy_file_objects WHERE file_id = 'file_schema_shared'`).Scan(&stored); err != nil {
				t.Fatalf("read file owner: %v", err)
			}
			if stored != "owner-one" {
				t.Fatalf("owner after conflicting insert = %q, want owner-one", stored)
			}
		})
	}
}
