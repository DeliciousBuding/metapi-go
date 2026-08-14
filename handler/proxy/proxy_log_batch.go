package proxyhandler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

// proxyLogBatchWriter drains proxy.ProxyLogEntry values from a buffered channel
// and INSERTs them in batches, decoupling the hot proxy path from DB write
// latency (especially SQLite's single-writer lock contention).
//
// Trade-off: entries are visible in proxy_logs up to flushInterval (default 1s)
// after they are produced. Operators that need write-through visibility (e2e
// tests, audit-on-write) set PROXY_LOG_ASYNC=false to bypass the writer.
//
// Reliability contract:
//   - On enqueue: a non-blocking send fills the channel; when the channel is
//     full (backpressure) the entry is written synchronously so logs are never
//     dropped due to saturation.
//   - On graceful shutdown: the loop drains everything still in the channel +
//     the in-memory buffer and flushes before returning, so no log is lost
//     when the server is stopped between flush ticks.
//   - On batch INSERT failure: the batch falls back to per-row inserts so a
//     single malformed row cannot sink the rest of the batch.
type proxyLogBatchWriter struct {
	ch            chan proxy.ProxyLogEntry
	batchSize     int
	flushInterval time.Duration
	stop          chan struct{}
	done          chan struct{}
	stopOnce      sync.Once
	wg            sync.WaitGroup
}

// newProxyLogBatchWriter constructs a writer. batchSize is clamped to a sane
// floor; flushIntervalMs likewise. The channel capacity is a multiple of the
// batch size to absorb short bursts before backpressure kicks in.
func newProxyLogBatchWriter(batchSize int, flushIntervalMs int) *proxyLogBatchWriter {
	if batchSize < 1 {
		batchSize = defaultProxyLogBatchSize
	}
	if flushIntervalMs < 1 {
		flushIntervalMs = defaultProxyLogFlushIntervalMs
	}
	// Channel headroom: 4× the batch size gives ~4 batches of burst capacity
	// before enqueue falls back to synchronous writes. Bounded to keep memory
	// predictable under sustained overload.
	channelCapacity := batchSize * 4
	return &proxyLogBatchWriter{
		ch:            make(chan proxy.ProxyLogEntry, channelCapacity),
		batchSize:     batchSize,
		flushInterval: time.Duration(flushIntervalMs) * time.Millisecond,
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
}

// start launches the background drain goroutine. It must be called exactly
// once before any enqueue; shutdown must be called exactly once to stop it.
func (b *proxyLogBatchWriter) start() {
	b.wg.Add(1)
	go b.loop()
}

// enqueue pushes entry onto the channel. When the channel is full it falls
// back to a synchronous InsertProxyLog so logs are never dropped under
// backpressure. Returns nil when there is no DB to write to (mirrors
// InsertProxyLog's nil-DB no-op).
func (b *proxyLogBatchWriter) enqueue(ctx context.Context, entry proxy.ProxyLogEntry) error {
	select {
	case b.ch <- entry:
		return nil
	default:
	}
	// Channel full: write through synchronously rather than blocking the hot
	// path or dropping the log. This is the intended backpressure escape valve.
	db := store.GetDB()
	if db == nil {
		return nil
	}
	return InsertProxyLog(ctx, db, entry)
}

// loop is the background drain goroutine. It accumulates entries into a buffer
// and flushes when the buffer reaches batchSize OR the flush ticker fires,
// whichever comes first. On stop it drains the channel + buffer one last time.
func (b *proxyLogBatchWriter) loop() {
	defer b.wg.Done()
	defer close(b.done)

	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	buf := make([]proxy.ProxyLogEntry, 0, b.batchSize)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		b.flushBatch(context.Background(), buf)
		buf = buf[:0]
	}

	for {
		select {
		case entry := <-b.ch:
			buf = append(buf, entry)
			if len(buf) >= b.batchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-b.stop:
			// Graceful shutdown: drain anything still queued so no log is lost
			// between the last flush tick and process exit.
			draining := true
			for draining {
				select {
				case entry := <-b.ch:
					buf = append(buf, entry)
					if len(buf) >= b.batchSize {
						flush()
					}
				default:
					draining = false
				}
			}
			flush()
			return
		}
	}
}

// shutdown signals the loop to stop and blocks until it has finished draining
// and flushing, or until ctx expires. Safe to call once (guarded by stopOnce);
// subsequent calls are no-ops.
func (b *proxyLogBatchWriter) shutdown(ctx context.Context) error {
	b.stopOnce.Do(func() { close(b.stop) })
	select {
	case <-b.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// flushBatch writes entries as a single multi-row INSERT. On failure it falls
// back to per-row inserts (via InsertProxyLog) so one bad row cannot sink the
// whole batch. The DB is resolved at flush time via store.GetDB() so runtime
// overrides (tests) are honored.
func (b *proxyLogBatchWriter) flushBatch(ctx context.Context, entries []proxy.ProxyLogEntry) {
	if len(entries) == 0 {
		return
	}
	db := store.GetDB()
	if db == nil {
		slog.Warn("proxy_log batch writer: no DB at flush, dropping batch", "count", len(entries))
		return
	}
	if len(entries) == 1 {
		if err := InsertProxyLog(ctx, db, entries[0]); err != nil {
			slog.Warn("proxy_log single insert failed during flush", "err", err)
		}
		return
	}
	if err := batchInsertProxyLogs(ctx, db, entries); err != nil {
		slog.Warn("proxy_log batch insert failed, falling back to per-row", "err", err, "count", len(entries))
		for _, entry := range entries {
			if err := InsertProxyLog(ctx, db, entry); err != nil {
				slog.Warn("proxy_log per-row insert failed",
					"err", err,
					"status", entry.Status,
					"model", entry.ModelRequested,
					"request_id", entry.RequestID,
				)
			}
		}
	}
}

// batchInsertProxyLogs builds a single multi-row INSERT statement for the given
// entries. Placeholders are ?-shaped; store.DB.ExecContext rebinds them to $N
// for PostgreSQL, so the same query works across dialects. All rows share one
// created_at timestamp (the flush instant) — within flushInterval of when
// each entry was actually produced, which is well inside the 24h / 60s query
// windows that consume the column.
func batchInsertProxyLogs(ctx context.Context, db *store.DB, entries []proxy.ProxyLogEntry) error {
	var sb strings.Builder
	sb.WriteString("INSERT INTO proxy_logs (")
	sb.WriteString(proxyLogInsertColumns)
	sb.WriteString(") VALUES ")

	// One timestamp for the whole batch; the per-row skew vs. real occurrence
	// is bounded by the flush interval and is irrelevant to 24h/60s windows.
	createdAt := time.Now().UTC().Format(time.RFC3339)
	args := make([]any, 0, len(entries)*proxyLogInsertArgCount)
	for i, entry := range entries {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(proxyLogSingleRowPlaceholders)
		args = append(args, proxyLogEntryArgs(entry, createdAt)...)
	}
	_, err := db.ExecContext(ctx, sb.String(), args...)
	return err
}

// proxyLogInsertArgCount is the number of positional args per proxy_logs row.
// Kept as a constant so the batch writer can pre-size the args slice without a
// runtime count of proxyLogEntryArgs output.
const proxyLogInsertArgCount = 24

// Default tunables for the proxy_log batch writer. Mirrors the env defaults in
// config.Load so the writer is usable in isolation (tests, ad-hoc tooling).
const (
	defaultProxyLogBatchSize        = 50
	defaultProxyLogFlushIntervalMs = 1000
)

// ---- Global writer singleton ----
//
// The process-wide writer is configured by app.ConfigureProxyUpstream from the
// PROXY_LOG_ASYNC / PROXY_LOG_BATCH_SIZE / PROXY_LOG_FLUSH_INTERVAL_MS env
// vars. When async is disabled the writer stays nil and EnqueueProxyLog writes
// through synchronously — the test/e2e path that needs immediate visibility.
var (
	proxyLogBatchWriterMu sync.Mutex
	proxyLogBatchWriterInst *proxyLogBatchWriter
)

// ConfigureProxyLogWriter sets up the global proxy log writer. When async is
// true a batch writer is started; when false any existing writer is shut down
// (drained) and the system reverts to synchronous writes. Idempotent: calling
// again first drains the previous writer so reconfigure does not leak a
// goroutine or lose in-flight entries.
func ConfigureProxyLogWriter(async bool, batchSize int, flushIntervalMs int) {
	proxyLogBatchWriterMu.Lock()
	defer proxyLogBatchWriterMu.Unlock()

	if proxyLogBatchWriterInst != nil {
		// Drain the previous writer before replacing it so reconfigure never
		// loses entries or strands a goroutine. A bounded wait is enough: the
		// loop drains the channel in O(channel depth) before returning.
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := proxyLogBatchWriterInst.shutdown(drainCtx); err != nil {
			slog.Warn("proxy_log batch writer shutdown during reconfigure timed out", "err", err)
		}
		cancel()
		proxyLogBatchWriterInst = nil
	}

	if !async {
		return
	}

	w := newProxyLogBatchWriter(batchSize, flushIntervalMs)
	w.start()
	proxyLogBatchWriterInst = w
	slog.Info("proxy_log batch writer enabled",
		"batch_size", w.batchSize,
		"flush_interval_ms", int(w.flushInterval/time.Millisecond),
		"channel_capacity", cap(w.ch),
	)
}

// ShutdownProxyLogBatchWriter drains and stops the global batch writer if one
// is running. Called from app graceful shutdown BEFORE store.CloseDatabase so
// the writer can flush its last batch against a still-open DB. No-op when the
// writer is nil (sync mode).
func ShutdownProxyLogBatchWriter(ctx context.Context) error {
	proxyLogBatchWriterMu.Lock()
	w := proxyLogBatchWriterInst
	proxyLogBatchWriterInst = nil
	proxyLogBatchWriterMu.Unlock()

	if w == nil {
		return nil
	}
	return w.shutdown(ctx)
}

// EnqueueProxyLog writes a proxy_logs entry, either via the async batch writer
// (when configured) or synchronously (sync mode / tests). This is the
// production write path wired into UpstreamConfig.LogProxy by
// app.ConfigureProxyUpstream. Backpressure (channel full) falls back to a
// synchronous InsertProxyLog so logs are never dropped.
func EnqueueProxyLog(ctx context.Context, entry proxy.ProxyLogEntry) error {
	proxyLogBatchWriterMu.Lock()
	w := proxyLogBatchWriterInst
	proxyLogBatchWriterMu.Unlock()

	if w == nil {
		// Sync mode: write through immediately. This is the test/e2e path and
		// the PROXY_LOG_ASYNC=false operator opt-out.
		return InsertProxyLog(ctx, store.GetDB(), entry)
	}
	return w.enqueue(ctx, entry)
}
