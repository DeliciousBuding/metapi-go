package proxyhandler

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
)

// Native audio/video/text streams share the response limits, idle guard and
// bounded per-write downstream window with SSE, but retain their own bytes and
// headers. Never append JSON/SSE to a file.
func handleMediaByteStream(w http.ResponseWriter, r *http.Request, resp *http.Response) (ParsedUsage, streamOutcome) {
	usage := ParsedUsage{Source: usageSourceUnknown}
	limit := streamResponseByteLimit()
	if resp.ContentLength > limit {
		writeJSONError(w, 502, "Upstream media exceeds response byte limit", "upstream_error")
		return usage, streamEndedTruncated
	}
	shared.IncActiveStreams()
	defer shared.DecActiveStreams()
	body := &streamIdleBody{ReadCloser: resp.Body}
	body.guard = newStreamIdleGuard(streamIdleTimeout(), body.closeUnderlying)
	defer body.Close()
	relayUpstreamResponseHeaders(w, resp.Header)
	w.WriteHeader(resp.StatusCode)
	// The relay must not inherit the whole-request write deadline: a healthy
	// long download outlives it. Each chunk instead gets its own bounded
	// write+flush window, cleared again before the next upstream read.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	writeBudget := streamWriteTimeout()
	var received int64
	buffer := make([]byte, 32<<10)
	for {
		if r.Context().Err() != nil {
			return usage, streamEndedClientDisconnect
		}
		n, err := body.Read(buffer)
		if n > 0 {
			remaining := limit - received
			exceeded := int64(n) > remaining
			if exceeded {
				n = int(remaining)
			}
			if n > 0 {
				if writeErr := writeDownstreamChunk(w, buffer[:n], writeBudget); writeErr != nil {
					message := "media downstream write failed"
					if isDownstreamWriteTimeout(writeErr) {
						message = "media downstream write timed out: client stopped reading"
					}
					slog.Warn(message,
						"err", writeErr,
						"write_timeout_sec", int(writeBudget.Seconds()),
						"streamed_bytes", received,
					)
					// The client is gone: keep unknown usage, never record this
					// as an upstream fault. The caller's defer closes the
					// upstream body and releases the slot.
					return usage, streamEndedClientDisconnect
				}
				received += int64(n)
			}
			if exceeded {
				return usage, streamEndedTruncated
			}
		}
		if err != nil {
			if r.Context().Err() != nil {
				return usage, streamEndedClientDisconnect
			}
			if body.guard.fired.Load() {
				return usage, streamEndedIdleTimeout
			}
			if err == io.EOF {
				return usage, streamEndedNormally
			}
			return usage, streamEndedUpstreamFault
		}
	}
}
