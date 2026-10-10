package proxyhandler

import (
	"io"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
)

// Native audio/video/text streams share the response limits and idle guard with
// SSE, but retain their own bytes and headers. Never append JSON/SSE to a file.
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
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
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
				if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
					return usage, streamEndedClientDisconnect
				}
				received += int64(n)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
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
