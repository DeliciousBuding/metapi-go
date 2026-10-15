package store

import (
	"database/sql"
	"os"
	"testing"
)

func TestDownstreamPathUpgrade(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == DialectPostgres {
				if os.Getenv("PG_TEST_DSN") == "" {
					t.Skip("PG_TEST_DSN not set")
				}
				dsn = pgScratchDSN(t, "_ingress_upgrade")
			}
			db, err := Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{
				"ALTER TABLE proxy_logs DROP COLUMN downstream_path",
				"DELETE FROM schema_migrations WHERE version = 'proxy_log_downstream_path_v1'",
				"INSERT INTO proxy_logs (model_requested, error_message) VALUES ('legacy-ingress', '[downstream:/v1/messages] old error')",
			} {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			// Repeat startup to check the upgrade is idempotent and does not infer old metadata.
			for range 2 {
				if err := AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
			}
			var path sql.NullString
			if err := db.Get(&path, "SELECT downstream_path FROM proxy_logs WHERE model_requested = 'legacy-ingress'"); err != nil {
				t.Fatal(err)
			}
			if path.Valid {
				t.Fatalf("historical ingress was invented: %s", path.String)
			}
		})
	}
}
