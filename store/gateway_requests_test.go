package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
)

func TestGatewayRequestPersistenceAndUpgrade(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == DialectPostgres {
				if os.Getenv("PG_TEST_DSN") == "" {
					t.Skip("PG_TEST_DSN not set")
				}
				dsn = pgScratchDSN(t, "_request_upgrade")
			}
			db, err := Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if dialect == DialectPostgres {
				pgtest.Reset(t, db.DB)
			}
			if err := AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			for _, query := range []string{
				"DROP INDEX proxy_logs_gateway_request_idx",
				"ALTER TABLE proxy_logs DROP COLUMN gateway_request_id",
				"DELETE FROM schema_migrations WHERE version = 'gateway_request_observations_v1'",
				"INSERT INTO proxy_logs (request_id,status) VALUES ('reused-client-id','success')",
			} {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
			}
			var legacy sql.NullString
			if err := db.Get(&legacy, "SELECT gateway_request_id FROM proxy_logs"); err != nil {
				t.Fatal(err)
			}
			if legacy.Valid {
				t.Fatal("invented historical request identity")
			}
			shared := "reused-client-id"
			entry := GatewayRequest{ID: "internal-a", ExternalRequestID: &shared, Transport: "http", TerminalStatus: "completed", OutputStarted: true, LatencyMs: 10, StartedAt: "2026-10-16T08:00:00+08:00", EndedAt: "2026-10-16T00:00:00.01Z"}
			if err := InsertGatewayRequest(context.Background(), db, entry); err != nil {
				t.Fatal(err)
			}
			entry.TerminalStatus = "client_cancelled"
			if err := InsertGatewayRequest(context.Background(), db, entry); err != nil {
				t.Fatal(err)
			}
			entry.ID = "internal-b"
			if err := InsertGatewayRequest(context.Background(), db, entry); err != nil {
				t.Fatal(err)
			}
			var rows []GatewayRequest
			if err := db.Select(&rows, "SELECT * FROM gateway_requests ORDER BY id"); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 || rows[0].TerminalStatus != "completed" || !rows[0].OutputStarted || rows[0].StartedAt != "2026-10-16T00:00:00.000000000Z" || rows[0].FirstOutputLatencyMs != nil {
				t.Fatalf("terminal idempotency/unknown metadata changed: %+v", rows)
			}
			entry.ID = "invalid"
			entry.Transport = "invalid"
			if err := InsertGatewayRequest(context.Background(), db, entry); err == nil {
				t.Fatal("invalid transport accepted")
			}
			entry.ID = ""
			entry.Transport = "http"
			if err := InsertGatewayRequest(context.Background(), db, entry); err == nil {
				t.Fatal("empty identity accepted")
			}
		})
	}
}

func TestGatewayRequestBatchPersistence(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == DialectPostgres {
				if os.Getenv("PG_TEST_DSN") == "" {
					t.Skip("PG_TEST_DSN not set")
				}
				dsn = pgScratchDSN(t, "_request_batch")
			}
			db, err := Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			entries := []GatewayRequest{
				{ID: "batch-a", Transport: "http", TerminalStatus: "completed", StartedAt: "2026-10-16T00:00:00Z", EndedAt: "2026-10-16T00:00:01Z"},
				{ID: "batch-b", Transport: "websocket", TerminalStatus: "upstream_failed", StartedAt: "2026-10-16T00:00:00Z", EndedAt: "2026-10-16T00:00:01Z"},
			}
			for range 2 {
				if err := InsertGatewayRequests(context.Background(), db, entries); err != nil {
					t.Fatal(err)
				}
			}
			var rows []GatewayRequest
			if err := db.Select(&rows, "SELECT * FROM gateway_requests ORDER BY id"); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 || rows[1].HTTPStatus != nil || rows[1].TerminalStatus != "upstream_failed" {
				t.Fatalf("batch records: %+v", rows)
			}
		})
	}
}

func TestGatewayRequestCopyMigration(t *testing.T) {
	source, target := filepath.Join(t.TempDir(), "source.db"), filepath.Join(t.TempDir(), "target.db")
	db, err := Open(DialectSQLite, source, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	entry := GatewayRequest{ID: "copy-request", Transport: "websocket", TerminalStatus: "output_interrupted", AttemptCount: 1, OutputStarted: true, StartedAt: "2026-10-16T00:00:00Z", EndedAt: "2026-10-16T00:00:01Z", LatencyMs: 1000}
	if err := InsertGatewayRequest(context.Background(), db, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO proxy_logs (gateway_request_id,created_at) VALUES (?,?)", entry.ID, entry.EndedAt); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := RunMigration(RunMigrationOptions{FromPath: source, ToURL: target, Verify: true}); err != nil {
		t.Fatal(err)
	}
	db, err = Open(DialectSQLite, target, false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var copied GatewayRequest
	if err := db.Get(&copied, "SELECT * FROM gateway_requests"); err != nil {
		t.Fatal(err)
	}
	var linked string
	if err := db.Get(&linked, "SELECT gateway_request_id FROM proxy_logs"); err != nil {
		t.Fatal(err)
	}
	if copied.ID != entry.ID || copied.TerminalStatus != entry.TerminalStatus || copied.AttemptCount != 1 || linked != entry.ID || copied.HTTPStatus != nil {
		t.Fatalf("migration lost request/link/unknown status: %+v link=%s", copied, linked)
	}
	if tableHasSerialID("gateway_requests") {
		t.Fatal("text request identity treated as serial sequence")
	}
}

func TestGatewayRequestRetentionWindow(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == DialectPostgres {
				if os.Getenv("PG_TEST_DSN") == "" {
					t.Skip("PG_TEST_DSN not set")
				}
				dsn = pgScratchDSN(t, "_request_retention")
			}
			db, err := Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if dialect == DialectPostgres {
				pgtest.Reset(t, db.DB)
			}
			if err := AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			for _, row := range []struct{ id, started, ended string }{
				{"expired", "2026-10-14T00:00:00Z", "2026-10-14T01:00:00Z"},
				{"long-retained", "2026-10-14T00:00:00Z", "2026-10-16T00:00:00Z"},
			} {
				if err := InsertGatewayRequest(context.Background(), db, GatewayRequest{ID: row.id, Transport: "http", TerminalStatus: "completed", StartedAt: row.started, EndedAt: row.ended}); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec("INSERT INTO proxy_logs (gateway_request_id,created_at) VALUES (?,?)", row.id, row.started); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec("INSERT INTO proxy_logs (created_at) VALUES ('2026-10-14T00:00:00Z'),('2026-10-16T00:00:00Z')"); err != nil {
				t.Fatal(err)
			}
			if _, err := PruneProxyRequestLogs(db, "2026-10-15T00:00:00Z"); err != nil {
				t.Fatal(err)
			}
			var requests, attempts, linked int
			if err := db.Get(&requests, "SELECT COUNT(*) FROM gateway_requests"); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&attempts, "SELECT COUNT(*) FROM proxy_logs"); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&linked, "SELECT COUNT(*) FROM proxy_logs WHERE gateway_request_id = 'long-retained'"); err != nil {
				t.Fatal(err)
			}
			if requests != 1 || attempts != 2 || linked != 1 {
				t.Fatalf("partial retention: requests=%d attempts=%d retained-linked=%d", requests, attempts, linked)
			}
		})
	}
}
