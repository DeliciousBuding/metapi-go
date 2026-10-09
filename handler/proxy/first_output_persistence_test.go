package proxyhandler

import (
	"context"
	"os"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestFirstOutputPersistence(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == store.DialectPostgres {
				dsn = os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
			}
			db, err := store.Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			defer func() {
				if _, err := db.Exec("DELETE FROM proxy_logs WHERE model_requested=?", "fixture-first-output"); err != nil {
					t.Errorf("clean timing fixture: %v", err)
				}
			}()
			if dialect == store.DialectPostgres {
				pgtest.Reset(t, db.DB)
			}
			if err := store.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			for index, output := range []*int64{nil, int64Ptr(280)} {
				entry := proxy.ProxyLogEntry{ModelRequested: "fixture-first-output", Status: "success", HTTPStatus: 200, IsStream: boolPtr(true), FirstByteLatencyMs: int64Ptr(10), FirstOutputLatencyMs: output, LatencyMs: 400, RequestID: "fixture-timing"}
				if err := InsertProxyLog(context.Background(), db, entry); err != nil {
					t.Fatal(err)
				}
				var row store.ProxyLog
				if err := db.Get(&row, "SELECT first_byte_latency_ms, first_output_latency_ms, latency_ms FROM proxy_logs WHERE model_requested = ? ORDER BY id DESC LIMIT 1", entry.ModelRequested); err != nil {
					t.Fatal(err)
				}
				if index == 0 && row.FirstOutputLatencyMs != nil {
					t.Fatal("missing observation converted to zero")
				}
				if index == 1 && (row.FirstOutputLatencyMs == nil || *row.FirstOutputLatencyMs != 280) {
					t.Fatal("first output was lost or copied from headers")
				}
			}
		})
	}
}
