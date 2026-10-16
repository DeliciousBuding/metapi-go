package proxyhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

// Compare equal concurrency with/without request persistence in both write
// modes. Includes shutdown drain so enqueue speed cannot hide unfinished I/O.
// This isolates local SQLite overhead, not real upstream network latency.
func BenchmarkGatewayLifecycleSQLite(b *testing.B) {
	for _, async := range []bool{false, true} {
		mode := "sync"
		if async {
			mode = "async"
		}
		b.Run(mode, func(b *testing.B) {
			for _, enabled := range []bool{false, true} {
				name := "attempts_only"
				if enabled {
					name = "requests_and_attempts"
				}
				b.Run(name, func(b *testing.B) {
					path := filepath.Join(b.TempDir(), "bench.db")
					db, err := store.Open(store.DialectSQLite, path, false)
					if err != nil {
						b.Fatal(err)
					}
					defer db.Close()
					if err := store.AutoMigrate(db); err != nil {
						b.Fatal(err)
					}
					checkpoint := func() int64 {
						if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
							b.Fatal(err)
						}
						stat, err := os.Stat(path)
						if err != nil {
							b.Fatal(err)
						}
						return stat.Size()
					}
					initialBytes := checkpoint()
					writeAttempt := func(ctx context.Context, entry proxy.ProxyLogEntry) error { return InsertProxyLog(ctx, db, entry) }
					writeRequest := func(ctx context.Context, entry store.GatewayRequest) error {
						return store.InsertGatewayRequest(ctx, db, entry)
					}
					var writer *proxyLogBatchWriter
					if async {
						store.OverrideDB(db)
						defer store.OverrideDB(nil)
						writer = newProxyLogBatchWriter(50, 1000)
						writer.start()
						defer writer.shutdown(context.Background())
						writeAttempt = writer.enqueue
						writeRequest = writer.enqueueRequest
					}
					next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						entry := proxy.ProxyLogEntry{Status: shared.OutcomeSuccess, HTTPStatus: 200}
						state := requestLifecycleFromContext(r.Context())
						state.recordAttempt(r.Context(), &entry)
						if err := writeAttempt(r.Context(), entry); err != nil {
							b.Error(err)
						}
						_, _ = w.Write([]byte(`{"synthetic":"completed"}`))
						state.observe(shared.OutcomeSuccess, false)
					})
					var handler http.Handler = next
					if enabled {
						handler = requestLifecycleMiddleware(next, func(ctx context.Context, entry store.GatewayRequest) error {
							err := writeRequest(ctx, entry)
							if err != nil {
								b.Error(err)
							}
							return err
						})
					}
					durations := make([]int64, b.N)
					var index atomic.Int64
					b.ReportAllocs()
					b.ResetTimer()
					b.RunParallel(func(pb *testing.PB) {
						for pb.Next() {
							i := index.Add(1) - 1
							started := time.Now()
							handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/responses", nil))
							durations[i] = time.Since(started).Nanoseconds()
						}
					})
					if writer != nil {
						if err := writer.shutdown(context.Background()); err != nil {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					var attempts int
					if err := db.Get(&attempts, "SELECT COUNT(*) FROM proxy_logs"); err != nil || attempts != b.N {
						b.Fatalf("persisted attempts=%d expected=%d err=%v", attempts, b.N, err)
					}
					if enabled {
						var requests int
						if err := db.Get(&requests, "SELECT COUNT(*) FROM gateway_requests"); err != nil || requests != b.N {
							b.Fatalf("persisted requests=%d expected=%d err=%v", requests, b.N, err)
						}
					}
					sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
					b.ReportMetric(float64(durations[(b.N-1)*95/100])/1000, "p95-us")
					b.ReportMetric(float64(durations[(b.N-1)*99/100])/1000, "p99-us")
					b.ReportMetric(float64(checkpoint()-initialBytes)/float64(b.N), "db-B/request")
				})
			}
		})
	}
}
