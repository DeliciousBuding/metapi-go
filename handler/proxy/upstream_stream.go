package proxyhandler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
)

// streamOutcome classifies how an SSE relay ended so the dispatcher can
// record upstream faults distinctly from clean or client-driven endings.
type streamOutcome int

const (
	// streamEndedNormally covers clean EOF, byte-limit termination, mid-stream
	// read errors, downstream disconnects and writes — the historical paths.
	streamEndedNormally streamOutcome = iota
	// streamEndedIdleTimeout means the upstream stopped sending chunks for
	// longer than PROXY_STREAM_IDLE_TIMEOUT_SEC and the idle guard aborted
	// the stream. The dispatcher records this via the failure path.
	streamEndedIdleTimeout
)

func handleStreamUpstream(w http.ResponseWriter, r *http.Request, resp *http.Response, latencyMs int64) (ParsedUsage, streamOutcome) {
	empty := ParsedUsage{Source: usageSourceUnknown}
	if resp == nil || resp.Body == nil {
		return empty, streamEndedNormally
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		relayUpstreamErrorResponse(w, resp, latencyMs)
		return empty, streamEndedNormally
	}

	// Active SSE stream gauge: Inc when a real stream starts, Dec on every
	// exit path (success, error, byte-limit, or client disconnect). The defer
	// runs after writeSSEHeaders / before the read loop returns so the gauge
	// always tracks a live stream, never a buffered non-stream response.
	shared.IncActiveStreams()
	defer shared.DecActiveStreams()

	writeSSEHeaders(w)
	w.WriteHeader(200)

	// Disable server-level WriteTimeout for SSE streaming.
	// Without this, any stream exceeding app.Server.WriteTimeout (60s) gets
	// forcibly closed — a hard break for reasoning models and long completions.
	if rc, ok := w.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = rc.SetWriteDeadline(time.Time{})
	}

	flusher, _ := w.(http.Flusher)

	// Copy upstream Content-Type if SSE; writeSSEHeaders already sets it.
	if ct := resp.Header.Get("Content-Type"); ct != "" && strings.Contains(ct, "text/event-stream") {
		_ = ct // SSE content type already handled by writeSSEHeaders
	}

	// Chunk-gap (stream idle) guard: every relayed chunk pushes the deadline
	// forward; when no chunk arrives within the window the guard closes the
	// upstream body, unblocking the read loop below. The upstream dispatch
	// clients carry no whole-request timeout for streams precisely so this
	// per-chunk timer — not total elapsed time — governs stream liveness.
	idleTimeout := streamIdleTimeout()
	idleBody := &streamIdleBody{ReadCloser: resp.Body}
	idleBody.guard = newStreamIdleGuard(idleTimeout, idleBody.closeUnderlying)
	resp.Body = idleBody

	analyzer := newIncrementalSseAnalyzer()
	sawStreamBytes := false
	maxStreamBytes := streamResponseByteLimit()
	var streamedBytes int64
	idleTimedOut := false
	buf := make([]byte, 4096)
	for {
		select {
		case <-r.Context().Done():
			// Client disconnect / request cancel: still return any usage already
			// extracted from earlier SSE events (best-effort partial). Do not
			// invent tokens when upstream never emitted a usage event.
			slog.Debug("SSE downstream context ended",
				"err", r.Context().Err(),
				"latency_ms", latencyMs,
				"streamed_bytes", streamedBytes,
			)
			if result := analyzer.Result(); result.Usage.Found {
				return result.Usage, streamEndedNormally
			}
			return empty, streamEndedNormally
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			remaining := maxStreamBytes - streamedBytes
			if remaining <= 0 {
				writeSSEStreamError(w, flusher, "stream response exceeded configured byte limit", "upstream_error")
				slog.Warn("SSE stream exceeded byte limit",
					"latency_ms", latencyMs,
					"streamed_bytes", streamedBytes,
					"limit_bytes", maxStreamBytes,
				)
				break
			}
			exceededLimit := int64(len(chunk)) > remaining
			if exceededLimit {
				chunk = chunk[:int(remaining)]
			}

			sawStreamBytes = true
			if len(chunk) > 0 {
				// Extract usage before downstream write so client disconnect
				// on the final usage-bearing chunk still counts tokens.
				analyzer.Push(chunk)
				if _, writeErr := w.Write(chunk); writeErr != nil {
					slog.Warn("SSE downstream write failed",
						"err", writeErr,
						"latency_ms", latencyMs,
						"streamed_bytes", streamedBytes,
					)
					// Downstream gone: keep any usage already extracted (including
					// the chunk that failed to write). Never invent tokens.
					if result := analyzer.Result(); result.Usage.Found {
						return result.Usage, streamEndedNormally
					}
					return empty, streamEndedNormally
				}
				streamedBytes += int64(len(chunk))
			}
			if flusher != nil {
				flusher.Flush()
			}
			if exceededLimit {
				writeSSEStreamError(w, flusher, "stream response exceeded configured byte limit", "upstream_error")
				slog.Warn("SSE stream exceeded byte limit",
					"latency_ms", latencyMs,
					"streamed_bytes", streamedBytes,
					"limit_bytes", maxStreamBytes,
				)
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				if idleBody.guard.fired.Load() && r.Context().Err() == nil {
					// Upstream stalled: the idle guard closed the body to
					// unblock this read. Emit a distinct final SSE error event
					// and report the idle outcome so the dispatcher records
					// the failure. A concurrent downstream disconnect keeps
					// its historical (non-idle) classification.
					writeSSEStreamError(w, flusher, "upstream stream idle timeout", "upstream_error")
					slog.Warn("SSE stream idle timeout: upstream sent no chunk within window",
						"idle_timeout_sec", int(idleTimeout.Seconds()),
						"latency_ms", latencyMs,
						"streamed_bytes", streamedBytes,
					)
					idleTimedOut = true
				} else {
					// Mid-stream upstream failure (network reset, upstream crash,
					// truncated body): emit a final SSE error event so the client
					// can surface the failure explicitly instead of inferring it
					// from a missing [DONE] marker. Mirrors the byte-limit path.
					writeSSEStreamError(w, flusher, "upstream stream interrupted", "upstream_error")
					slog.Warn("SSE stream read error", "err", err, "latency_ms", latencyMs)
				}
			}
			break
		}
	}

	outcome := streamEndedNormally
	if idleTimedOut {
		outcome = streamEndedIdleTimeout
	}

	// Post-stream SSE analysis uses bounded incremental state instead of
	// retaining the complete upstream body.
	result := analyzer.Result()
	if sawStreamBytes {
		if result.DroppedOversizedEvent {
			slog.Warn("SSE stream event exceeded analysis buffer",
				"latency_ms", latencyMs,
				"pending_limit_bytes", maxIncrementalSsePendingBytes,
			)
		}

		// Log SSE error events at WARN level
		if result.HasErrorEvent {
			LogSseErrorEvents(result.ErrorEvents)
		}

		// Check for empty content (stream ended with no data events).
		// ProxyEmptyContentFailEnabled is read from the startup-loaded config
		// singleton instead of os.Getenv per stream request.
		if !result.HasDataEvent {
			if rt := config.RuntimeSafe(); rt != nil && rt.ProxyEmptyContentFailEnabled {
				slog.Warn("SSE stream contained no data events",
					"latency_ms", latencyMs,
					"event_count", result.EventCount,
					"has_done_marker", result.HasDoneMarker,
				)
			}
		}
		if result.Usage.Found {
			return result.Usage, outcome
		}
	}
	return empty, outcome
}

// streamResponseByteLimit returns the configured max SSE stream response size
// in bytes from the startup-loaded config singleton. Falls back to
// DefaultProxyMaxStreamResponseBytes when config is not yet loaded (e.g. tests
// that bypass TestMain) or when the operator left the value at zero/negative.
// Reading a struct field per request is materially cheaper than re-parsing
// os.Getenv on every SSE stream and keeps the hot path allocation-free here.
func streamResponseByteLimit() int64 {
	if cfg := config.GetSafe(); cfg != nil && cfg.ProxyMaxStreamResponseBytes > 0 {
		return int64(cfg.ProxyMaxStreamResponseBytes)
	}
	return int64(config.DefaultProxyMaxStreamResponseBytes)
}

func writeSSEStreamError(w http.ResponseWriter, flusher http.Flusher, message, typ string) {
	payload, _ := json.Marshal(map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    typ,
		},
	})
	_, _ = w.Write(sseEvent(string(payload)))
	_, _ = w.Write(sseDone())
	if flusher != nil {
		flusher.Flush()
	}
}

func relayUpstreamErrorResponse(w http.ResponseWriter, resp *http.Response, latencyMs int64) {
	bodyBytes, err := proxy.ReadBufferedResponseBody(resp.Body)
	if err != nil {
		slog.Warn("failed to read upstream error response", "err", err, "latency_ms", latencyMs, "status", resp.StatusCode)
		writeJSONError(w, 502, "Failed to read upstream response", "upstream_error")
		return
	}

	for k, v := range resp.Header {
		if k == "Content-Length" || k == "Transfer-Encoding" {
			continue
		}
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)
}

func relayBufferedUpstreamErrorResponse(w http.ResponseWriter, resp *http.Response, bodyBytes []byte) {
	relayBufferedUpstreamResponse(w, resp, bodyBytes)
}

// streamIdleTimeout returns the configured SSE chunk-gap timeout from the
// startup-loaded config singleton, falling back to
// DefaultProxyStreamIdleTimeoutSec when config is not yet loaded (tests that
// bypass TestMain) or the operator value is non-positive. Mirrors the
// per-request config.GetSafe() pattern of streamResponseByteLimit; like the
// other PROXY_* timeouts the value itself is frozen at startup.
func streamIdleTimeout() time.Duration {
	sec := config.DefaultProxyStreamIdleTimeoutSec
	if cfg := config.GetSafe(); cfg != nil && cfg.ProxyStreamIdleTimeoutSec > 0 {
		sec = cfg.ProxyStreamIdleTimeoutSec
	}
	return time.Duration(sec) * time.Second
}

// streamIdleBody wraps the upstream SSE body with the chunk-gap guard. Read
// pushes the idle deadline forward for every non-empty chunk; Close stops
// the watchdog and closes the underlying body exactly once. When the idle
// deadline passes with no chunk, the watchdog closes the underlying body to
// unblock a pending Read with an error, which the relay loop classifies via
// the guard's fired flag.
type streamIdleBody struct {
	io.ReadCloser
	guard     *streamIdleGuard
	closeOnce sync.Once
	closeErr  error
}

func (b *streamIdleBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.guard.reset()
	}
	return n, err
}

// closeUnderlying closes the wrapped body exactly once. Invoked both by the
// idle watchdog (to interrupt a blocked Read) and by Close.
func (b *streamIdleBody) closeUnderlying() {
	b.closeOnce.Do(func() { b.closeErr = b.ReadCloser.Close() })
}

func (b *streamIdleBody) Close() error {
	if b.guard != nil {
		b.guard.stop()
	}
	b.closeUnderlying()
	return b.closeErr
}

// streamIdleGuard bounds the gap between upstream SSE chunks. A single
// watchdog goroutine sleeps until the current deadline; every relayed chunk
// stores a later deadline (atomic, lock-free on the hot path). When the
// deadline passes with no new chunk the guard fires closeFn exactly once.
type streamIdleGuard struct {
	idle     time.Duration
	closeFn  func()
	deadline atomic.Int64 // unixnano of the current idle cutoff
	fired    atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
}

func newStreamIdleGuard(idle time.Duration, closeFn func()) *streamIdleGuard {
	g := &streamIdleGuard{
		idle:    idle,
		closeFn: closeFn,
		stopCh:  make(chan struct{}),
	}
	g.deadline.Store(time.Now().Add(idle).UnixNano())
	go g.watch()
	return g
}

// reset extends the idle window; called for every non-empty relayed chunk.
func (g *streamIdleGuard) reset() {
	g.deadline.Store(time.Now().Add(g.idle).UnixNano())
}

func (g *streamIdleGuard) stop() {
	g.stopOnce.Do(func() { close(g.stopCh) })
}

func (g *streamIdleGuard) watch() {
	for {
		wait := time.Until(time.Unix(0, g.deadline.Load()))
		if wait <= 0 {
			if g.tryFire() {
				return
			}
			continue // a chunk moved the deadline; re-arm
		}
		timer := time.NewTimer(wait)
		select {
		case <-g.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			if g.tryFire() {
				return
			}
			// A chunk moved the deadline while the timer was running; loop
			// and re-arm with the remaining window.
		}
	}
}

// tryFire fires the guard unless a chunk arrived between timer expiry and
// now. Returns true when the guard fired (or had already fired) so the
// watchdog can exit.
func (g *streamIdleGuard) tryFire() bool {
	if time.Now().UnixNano() < g.deadline.Load() {
		return false
	}
	if g.fired.CompareAndSwap(false, true) {
		g.closeFn()
	}
	return true
}
