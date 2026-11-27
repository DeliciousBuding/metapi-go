package store

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestProxyAttemptTelemetryUpgrade(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := migrationTestDB(t, dialect)
			for _, q := range []string{`ALTER TABLE proxy_logs DROP COLUMN conversion_details`, `ALTER TABLE proxy_logs DROP COLUMN stream_timing`, `INSERT INTO proxy_logs (status) VALUES ('legacy')`} {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version='proxy_attempt_telemetry_v1'`); err != nil {
					t.Fatal(err)
				}
				if err := AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
				var conversion, timing sql.NullString
				if err := db.QueryRow(`SELECT conversion_details,stream_timing FROM proxy_logs WHERE status='legacy'`).Scan(&conversion, &timing); err != nil || conversion.Valid || timing.Valid {
					t.Fatal(err, conversion, timing)
				}
			}
		})
	}
}

func TestProxyAttemptTelemetrySQLiteToPostgres(t *testing.T) {
	pg := migrationTestDB(t, DialectPostgres)
	var schema string
	if err := pg.Get(&schema, `SELECT current_schema()`); err != nil {
		t.Fatal(err)
	}
	dsn, err := url.Parse(os.Getenv("PG_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("search_path", schema)
	dsn.RawQuery = query.Encode()
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	db, err := Open(DialectSQLite, sourcePath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	const conversion = `{"owner":"relaykit"}`
	const timing = `{"decodeStatus":"interrupted"}`
	if _, err = db.Exec(`INSERT INTO proxy_logs (conversion_details,stream_timing) VALUES (?,?),(NULL,NULL)`, conversion, timing); err != nil {
		t.Fatal(err)
	}
	if _, err = RunMigration(RunMigrationOptions{FromPath: sourcePath, ToURL: dsn.String(), Verify: true}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pg.Get(&count, `SELECT COUNT(*) FROM proxy_logs WHERE (id=1 AND conversion_details=? AND stream_timing=?) OR (id=2 AND conversion_details IS NULL AND stream_timing IS NULL)`, conversion, timing); err != nil || count != 2 {
		t.Fatal(count, err)
	}
}
