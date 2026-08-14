package proxyhandler

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

// setupBatchTestDB opens an in-memory SQLite DB, auto-migrates it, and
// overrides the process-global store DB so the batch writer (which resolves
// store.GetDB() at flush time) writes here. Restores nil on cleanup.
func setupBatchTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := store.AutoMigrate(db); err != nil {
		db.Close()
		t.Fatalf("migrate: %v", err)
	}
	store.OverrideDB(db)
	t.Cleanup(func() {
		store.OverrideDB(nil)
		db.Close()
	})
	return db
}

// sampleBatchEntry builds a proxy_logs-shaped entry distinct per index so a
// test can assert all N entries landed (not just N copies of one row).
func sampleBatchEntry(index int) proxy.ProxyLogEntry {
	model := fmt.Sprintf("gpt-batch-%d", index)
	return proxy.ProxyLogEntry{
		ModelRequested: model,
		ModelActual:    &model,
		Status:         "success",
		HTTPStatus:     200,
		LatencyMs:      int64(index * 10),
		TotalTokens:    int64Ptr(int64(index + 100)),
		EstimatedCost:  0.01 * float64(index),
		RequestID:      fmt.Sprintf("req-%d", index),
	}
}

// countProxyLogsRows counts proxy_logs rows for a model pattern so parallel
// tests do not double-count each other's data.
func countProxyLogsRows(t *testing.T, db *store.DB, pattern string) int {
	t.Helper()
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM proxy_logs WHERE model_requested LIKE ?", pattern); err != nil {
		t.Fatalf("count proxy_logs: %v", err)
	}
	return count
}

// shutdownWriterSoon waits up to 2s for the writer to drain+flush after a stop
// signal. The loop goroutine closes b.done once it returns, so a successful
// shutdown means every enqueued entry has been flushed.
func shutdownWriterSoon(t *testing.T, b *proxyLogBatchWriter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := b.shutdown(ctx); err != nil {
		t.Fatalf("batch writer shutdown: %v", err)
	}
}

// TestProxyLogBatchWriter_FlushesOnBatchSize enqueues exactly batchSize entries
// with a long flush interval so only the batch-size trigger can flush. After
// shutdown (which would also drain), all entries must be present.
func TestProxyLogBatchWriter_FlushesOnBatchSize(t *testing.T) {
	db := setupBatchTestDB(t)
	const batchSize = 3
	// Long flush interval so only the batch-size trigger fires (not the ticker).
	w := newProxyLogBatchWriter(batchSize, 60_000)
	w.start()
	t.Cleanup(func() { shutdownWriterSoon(t, w) })

	for i := 0; i < batchSize; i++ {
		if err := w.enqueue(context.Background(), sampleBatchEntry(i)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// The batch-size flush happens in the loop goroutine; give it a moment.
	waitFor(t, 1*time.Second, func() bool {
		return countProxyLogsRows(t, db, "gpt-batch-%") == batchSize
	})
}

// TestProxyLogBatchWriter_FlushesOnInterval enqueues fewer than batchSize
// entries, then waits for the flush ticker. The entries must land via the
// interval trigger alone (the batch-size path never fires).
func TestProxyLogBatchWriter_FlushesOnInterval(t *testing.T) {
	db := setupBatchTestDB(t)
	// batchSize high so only the ticker can flush; interval short for speed.
	w := newProxyLogBatchWriter(100, 50)
	w.start()
	t.Cleanup(func() { shutdownWriterSoon(t, w) })

	for i := 0; i < 4; i++ {
		if err := w.enqueue(context.Background(), sampleBatchEntry(i)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Wait for at least one ticker flush (50ms interval → 200ms is ample).
	waitFor(t, 1*time.Second, func() bool {
		return countProxyLogsRows(t, db, "gpt-batch-%") == 4
	})
}

// TestProxyLogBatchWriter_GracefulShutdownDrainsChannel enqueues entries below
// the batch threshold with the ticker effectively disabled, then immediately
// shuts down. The drain path must flush every enqueued entry so no log is lost
// on graceful shutdown.
func TestProxyLogBatchWriter_GracefulShutdownDrainsChannel(t *testing.T) {
	db := setupBatchTestDB(t)
	const enqueued = 7
	// batchSize=100 + 60s interval: neither trigger should fire before shutdown.
	w := newProxyLogBatchWriter(100, 60_000)
	w.start()

	for i := 0; i < enqueued; i++ {
		if err := w.enqueue(context.Background(), sampleBatchEntry(i)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// Shut down immediately — the drain loop must flush all 7 queued entries.
	shutdownWriterSoon(t, w)

	if got := countProxyLogsRows(t, db, "gpt-batch-%"); got != enqueued {
		t.Fatalf("after shutdown drain: proxy_logs rows = %d, want %d (entries were lost)", got, enqueued)
	}
}

// TestProxyLogBatchWriter_BackpressureFallsBackToSync fills the channel past
// capacity. Overflow entries must be written synchronously (not dropped), so
// the final row count equals the total enqueued. The channel contents are then
// drained on shutdown, so no entry is lost.
func TestProxyLogBatchWriter_BackpressureFallsBackToSync(t *testing.T) {
	db := setupBatchTestDB(t)
	// batchSize=100, channel cap = 400. Enqueue 500 → 400 buffered, 100 sync.
	w := newProxyLogBatchWriter(100, 60_000)
	w.start()
	t.Cleanup(func() { shutdownWriterSoon(t, w) })

	const total = 500
	for i := 0; i < total; i++ {
		if err := w.enqueue(context.Background(), sampleBatchEntry(i)); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	// After shutdown drains the channel + flushes, every entry must be present.
	shutdownWriterSoon(t, w)

	if got := countProxyLogsRows(t, db, "gpt-batch-%"); got != total {
		t.Fatalf("after backpressure + shutdown: proxy_logs rows = %d, want %d (logs were dropped under saturation)", got, total)
	}
}

// TestEnqueueProxyLog_SyncModeWhenWriterNil verifies the default/sync path:
// with no global batch writer configured, EnqueueProxyLog writes through
// synchronously so the row is visible immediately (the test/e2e contract).
func TestEnqueueProxyLog_SyncModeWhenWriterNil(t *testing.T) {
	db := setupBatchTestDB(t)
	// Ensure the global writer is nil (sync mode) for this test.
	ConfigureProxyLogWriter(false, 50, 1000)
	t.Cleanup(func() { ConfigureProxyLogWriter(false, 50, 1000) })

	if err := EnqueueProxyLog(context.Background(), sampleBatchEntry(0)); err != nil {
		t.Fatalf("EnqueueProxyLog sync: %v", err)
	}
	if got := countProxyLogsRows(t, db, "gpt-batch-%"); got != 1 {
		t.Fatalf("sync mode rows = %d, want 1 (write-through visibility)", got)
	}
}

// TestEnqueueProxyLog_AsyncViaGlobalWriter verifies the global async path:
// ConfigureProxyLogWriter(true, ...) starts the writer and EnqueueProxyLog
// routes through it, with the entry landing after a flush.
func TestEnqueueProxyLog_AsyncViaGlobalWriter(t *testing.T) {
	db := setupBatchTestDB(t)
	// Small batch + short interval so the entry flushes quickly.
	ConfigureProxyLogWriter(true, 5, 50)
	t.Cleanup(func() {
		// Drain + stop the global writer so it does not leak into other tests.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = ShutdownProxyLogBatchWriter(ctx)
	})

	if err := EnqueueProxyLog(context.Background(), sampleBatchEntry(0)); err != nil {
		t.Fatalf("EnqueueProxyLog async: %v", err)
	}

	waitFor(t, 1*time.Second, func() bool {
		return countProxyLogsRows(t, db, "gpt-batch-%") == 1
	})
}

// TestBatchInsertProxyLogs_MatchesSingleRowInserts verifies the multi-row
// batch INSERT produces rows whose columns match the single-row InsertProxyLog
// path — i.e. the shared args/columns helper did not drift. Each row's tokens
// and cost must round-trip identically.
func TestBatchInsertProxyLogs_MatchesSingleRowInserts(t *testing.T) {
	db := setupBatchTestDB(t)

	// Insert 3 rows as a batch.
	entries := []proxy.ProxyLogEntry{
		sampleBatchEntry(1),
		sampleBatchEntry(2),
		sampleBatchEntry(3),
	}
	if err := batchInsertProxyLogs(context.Background(), db, entries); err != nil {
		t.Fatalf("batchInsertProxyLogs: %v", err)
	}
	// Insert 1 row the single-row path to confirm the same shape.
	if err := InsertProxyLog(context.Background(), db, sampleBatchEntry(9)); err != nil {
		t.Fatalf("InsertProxyLog: %v", err)
	}

	// Read back token totals per model; they must equal what was inserted.
	for _, e := range append(entries, sampleBatchEntry(9)) {
		var got int64
		if err := db.Get(&got, "SELECT COALESCE(total_tokens, 0) FROM proxy_logs WHERE model_requested = ?", e.ModelRequested); err != nil {
			t.Fatalf("select tokens for %s: %v", e.ModelRequested, err)
		}
		if want := *e.TotalTokens; got != want {
			t.Fatalf("total_tokens for %s = %d, want %d (batch/single row drift)", e.ModelRequested, got, want)
		}
	}
}

// TestProxyLogBatchWriter_ConcurrentEnqueueIsThreadSafe hammers the writer from
// several goroutines to exercise the RWMutex/chan contract. After shutdown,
// every enqueued entry must be present exactly once — no doubles, no drops.
func TestProxyLogBatchWriter_ConcurrentEnqueueIsThreadSafe(t *testing.T) {
	db := setupBatchTestDB(t)
	w := newProxyLogBatchWriter(25, 20)
	w.start()
	t.Cleanup(func() { shutdownWriterSoon(t, w) })

	const goroutines = 8
	const perGoroutine = 40
	var total int64
	done := make(chan struct{}, goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			for i := 0; i < perGoroutine; i++ {
				idx := gid*perGoroutine + i
				if err := w.enqueue(context.Background(), sampleBatchEntry(idx)); err != nil {
					t.Errorf("goroutine %d enqueue %d: %v", gid, idx, err)
					return
				}
				atomic.AddInt64(&total, 1)
			}
			done <- struct{}{}
		}(g)
	}
	for g := 0; g < goroutines; g++ {
		<-done
	}

	shutdownWriterSoon(t, w)
	want := int(atomic.LoadInt64(&total))
	if got := countProxyLogsRows(t, db, "gpt-batch-%"); got != want {
		t.Fatalf("concurrent: proxy_logs rows = %d, want %d (lost or duplicated entries)", got, want)
	}
}

// waitFor polls up to timeout for cond to return true, failing the test on
// timeout. Keeps interval-based tests fast but non-flaky.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not satisfied within %v", timeout)
}
