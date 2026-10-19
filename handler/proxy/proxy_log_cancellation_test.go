package proxyhandler

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestCancelledCallerPreservesAttemptAndRequest(t *testing.T) {
	for _, fullQueue := range []bool{false, true} {
		name := "synchronous"
		if fullQueue {
			name = "async-overflow"
		}
		t.Run(name, func(t *testing.T) {
			db := sqlitetest.Open(t)
			previousDB := store.GetDB()
			store.OverrideDB(db)
			t.Cleanup(func() { store.OverrideDB(previousDB) })
			var writer *proxyLogBatchWriter
			if fullQueue {
				writer = newProxyLogBatchWriter(1, 60_000)
				// Keep the consumer stopped until the real completion writes
				// have taken the synchronous overflow path.
				for i := range cap(writer.ch) {
					writer.ch <- proxyLogWrite{attempt: batchEntryPointer(i)}
				}
			}
			proxyLogBatchWriterMu.Lock()
			previousWriter := proxyLogBatchWriterInst
			proxyLogBatchWriterInst = writer
			proxyLogBatchWriterMu.Unlock()
			t.Cleanup(func() {
				proxyLogBatchWriterMu.Lock()
				proxyLogBatchWriterInst = previousWriter
				proxyLogBatchWriterMu.Unlock()
			})

			caller, cancel := context.WithCancel(context.Background())
			req := httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(caller)
			state, req := newRequestLifecycle(req, "http", defaultGatewayRequestWriter)
			cancel()
			entry := sampleBatchEntry(1000)
			logProxy(req.Context(), &UpstreamConfig{LogProxy: EnqueueProxyLog}, entry)
			state.finish(req.Context())

			var request store.GatewayRequest
			if err := db.Get(&request, `SELECT * FROM gateway_requests WHERE id=?`, state.entry.ID); err != nil {
				t.Fatal(err)
			}
			var linked int
			if err := db.Get(&linked, `SELECT COUNT(*) FROM proxy_logs WHERE gateway_request_id=?`, request.ID); err != nil {
				t.Fatal(err)
			}
			if linked != 1 || request.AttemptCount != linked || request.TerminalStatus != "client_cancelled" {
				t.Fatalf("cancelled request truth: terminal=%s declared attempts=%d persisted attempts=%d", request.TerminalStatus, request.AttemptCount, linked)
			}
			if writer != nil {
				writer.start()
				shutdownWriterSoon(t, writer)
			}
		})
	}
}

func batchEntryPointer(index int) *proxy.ProxyLogEntry {
	entry := sampleBatchEntry(index)
	return &entry
}

func TestAttemptCompletionWriterKeepsMetadataAndFiniteDeadline(t *testing.T) {
	type metadataKey struct{}
	caller, cancel := context.WithCancel(context.WithValue(context.Background(), metadataKey{}, "retained"))
	cancel()
	var observed context.Context
	logProxy(caller, &UpstreamConfig{LogProxy: func(ctx context.Context, _ proxy.ProxyLogEntry) error {
		observed = ctx
		if ctx.Err() != nil || ctx.Value(metadataKey{}) != "retained" {
			t.Fatalf("completion inherited cancellation or lost metadata: err=%v value=%v", ctx.Err(), ctx.Value(metadataKey{}))
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > time.Second {
			t.Fatalf("completion write needs a finite fresh deadline: %v %v", deadline, ok)
		}
		return nil
	}}, sampleBatchEntry(2000))
	if observed == nil || observed.Err() != context.Canceled {
		t.Fatal("completion write context was not released")
	}
}
