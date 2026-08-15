package proxyhandler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
)

func handleStreamUpstream(w http.ResponseWriter, r *http.Request, resp *http.Response, latencyMs int64) ParsedUsage {
	empty := ParsedUsage{Source: usageSourceUnknown}
	if resp == nil || resp.Body == nil {
		return empty
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		relayUpstreamErrorResponse(w, resp, latencyMs)
		return empty
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

	analyzer := newIncrementalSseAnalyzer()
	sawStreamBytes := false
	maxStreamBytes := streamResponseByteLimit()
	var streamedBytes int64
	buf := make([]byte, 4096)
	for {
		select {
		case <-r.Context().Done():
			// Client disconnect / request cancel: still return any usage already
			// extracted from earlier SSE events (best-effort partial). Do not
			// invent tokens when upstream never emitted a usage event.
			slog.Info("SSE downstream context ended",
				"err", r.Context().Err(),
				"latency_ms", latencyMs,
				"streamed_bytes", streamedBytes,
			)
			if result := analyzer.Result(); result.Usage.Found {
				return result.Usage
			}
			return empty
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
						return result.Usage
					}
					return empty
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
				// Mid-stream upstream failure (network reset, upstream crash,
				// truncated body): emit a final SSE error event so the client
				// can surface the failure explicitly instead of inferring it
				// from a missing [DONE] marker. Mirrors the byte-limit path.
				writeSSEStreamError(w, flusher, "upstream stream interrupted", "upstream_error")
				slog.Warn("SSE stream read error", "err", err, "latency_ms", latencyMs)
			}
			break
		}
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
			if cfg := config.GetSafe(); cfg != nil && cfg.ProxyEmptyContentFailEnabled {
				slog.Warn("SSE stream contained no data events",
					"latency_ms", latencyMs,
					"event_count", result.EventCount,
					"has_done_marker", result.HasDoneMarker,
				)
			}
		}
		if result.Usage.Found {
			return result.Usage
		}
	}
	return empty
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
