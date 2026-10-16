package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestGatewayRequestsShareUsageRetention(t *testing.T) {
	for _, owner := range []string{"main", "legacy"} {
		t.Run(owner, func(t *testing.T) {
			db := openRetentionTestDB(t)
			oldAt := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
			keepAt := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
			for _, row := range []struct{ id, at string }{{"old-request", oldAt}, {"keep-request", keepAt}} {
				if err := store.InsertGatewayRequest(context.Background(), db, store.GatewayRequest{ID: row.id, Transport: "http", TerminalStatus: "completed", StartedAt: row.at, EndedAt: row.at}); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec("INSERT INTO proxy_logs (gateway_request_id,created_at) VALUES (?,?)", row.id, row.at); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec("INSERT INTO proxy_logs (request_id,created_at) VALUES ('legacy-old',?),('legacy-keep',?)", oldAt, keepAt); err != nil {
				t.Fatal(err)
			}
			if owner == "main" {
				NewLogCleanupScheduler(&config.Config{}).runJobLocked(db, &config.RuntimeSettings{LogCleanupUsageLogsEnabled: true, LogCleanupRetentionDays: 1})
			} else {
				NewProxyLogRetentionScheduler(&config.Config{ProxyLogRetentionDays: 1}).runCleanupLocked(db, 1)
			}
			if got := countRows(t, db, "SELECT COUNT(*) FROM gateway_requests WHERE id = 'keep-request'"); got != 1 {
				t.Fatal("retained request removed")
			}
			if got := countRows(t, db, "SELECT COUNT(*) FROM gateway_requests"); got != 1 {
				t.Fatalf("requests=%d", got)
			}
			if got := countRows(t, db, "SELECT COUNT(*) FROM proxy_logs"); got != 2 {
				t.Fatalf("attempts=%d", got)
			}
			if got := countRows(t, db, "SELECT COUNT(*) FROM proxy_logs WHERE gateway_request_id IS NOT NULL AND gateway_request_id NOT IN (SELECT id FROM gateway_requests)"); got != 0 {
				t.Fatal("retention left linked orphan attempts")
			}
		})
	}
}

func TestGatewayRequestRetentionRollsBackOnFailure(t *testing.T) {
	db := openRetentionTestDB(t)
	at := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	if err := store.InsertGatewayRequest(context.Background(), db, store.GatewayRequest{ID: "request", Transport: "http", TerminalStatus: "completed", StartedAt: at, EndedAt: at}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO proxy_logs (gateway_request_id,created_at) VALUES ('request',?)", at); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER deny_request_delete BEFORE DELETE ON gateway_requests BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PruneProxyRequestLogs(db, time.Now().UTC().Format(time.RFC3339)); err == nil {
		t.Fatal("delete failure hidden")
	}
	if got := countRows(t, db, "SELECT COUNT(*) FROM proxy_logs"); got != 1 {
		t.Fatal("attempt deletion committed without request deletion")
	}
}
