package proxyhandler

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/transform/ollama"
)

// streamChunkRelay owns the downstream byte and write budgets. An optional
// analyzer observes chunks before delivery. A Responses bridge reserves whole
// frames and supplies protocol-specific terminal errors when a frame won't fit.
type streamChunkRelay struct {
	maxBytes        int64
	writeBudget     time.Duration
	idleGuard       *streamIdleGuard
	analyzer        *incrementalSseAnalyzer
	responsesBridge *messagesChatBody
}

// copy reports the ending and whether to log post-stream analysis details.
func (relay streamChunkRelay) copy(w http.ResponseWriter, r *http.Request, resp *http.Response, latencyMs int64) (streamOutcome, bool) {
	sawStreamBytes := false
	var streamedBytes int64
	outcome := streamEndedNormally
	buf := make([]byte, 4096)

	readChunk := func() ([]byte, error) {
		n, err := resp.Body.Read(buf)
		return buf[:n], err
	}
	var responsesFrames *responsesFrameReader
	if relay.responsesBridge != nil {
		// Apply the output budget after conversion and public model mapping.
		// Reserve complete frames so truncation cannot splice an error into JSON.
		responsesFrames = &responsesFrameReader{reader: bufio.NewReader(resp.Body), remaining: relay.maxBytes}
		readChunk = responsesFrames.readFrame
	}

	// emitStreamError attempts the single terminal error event that abnormal
	// endings share. A client that stopped reading is bounded by one write
	// window here too; a failure is logged and never retried.
	emitStreamError := func(message string) {
		if responsesFrames != nil {
			if terminal := relay.responsesBridge.ResponsesFailureEvents(message, responsesFrames.delivered); len(terminal) != 0 {
				if err := writeDownstreamChunk(w, terminal, relay.writeBudget); err != nil {
					slog.Debug("SSE terminal error event not delivered", "err", err, "latency_ms", latencyMs, "streamed_bytes", streamedBytes)
				}
				return
			}
		}
		var sequence int64
		if native, ok := resp.Body.(*nativeTerminalBody); ok {
			sequence = native.nextSequence
		}
		err := writeSSEStreamError(w, r.URL.Path, message, "upstream_error", relay.writeBudget, sequence)
		if err != nil {
			slog.Debug("SSE terminal error event not delivered",
				"err", err,
				"latency_ms", latencyMs,
				"streamed_bytes", streamedBytes,
			)
		}
	}

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
			return streamEndedClientDisconnect, false
		default:
		}

		chunk, err := readChunk()
		if len(chunk) > 0 {
			remaining := relay.maxBytes - streamedBytes
			if remaining <= 0 {
				emitStreamError("stream response exceeded configured byte limit")
				slog.Warn("SSE stream exceeded byte limit",
					"latency_ms", latencyMs,
					"streamed_bytes", streamedBytes,
					"limit_bytes", relay.maxBytes,
				)
				outcome = streamEndedTruncated
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
				// Undecodable bytes are never fed to the analyzer: it would
				// "analyze" noise, find no data events and hand the judge an
				// empty-content fact for a perfectly healthy answer.
				if relay.analyzer != nil {
					relay.analyzer.Push(chunk)
				}
				if writeErr := writeDownstreamChunk(w, chunk, relay.writeBudget); writeErr != nil {
					message := "SSE downstream write failed"
					if isDownstreamWriteTimeout(writeErr) {
						message = "SSE downstream write timed out: client stopped reading"
					}
					slog.Warn(message,
						"err", writeErr,
						"write_timeout_sec", int(relay.writeBudget.Seconds()),
						"latency_ms", latencyMs,
						"streamed_bytes", streamedBytes,
					)
					// Downstream gone or no longer reading: keep any usage already
					// extracted (including the chunk that failed to write), never
					// invent tokens and never record this as an upstream fault.
					// The caller's defer closes the upstream body and releases
					// the site slot, which is what bounds the stalled client.
					return streamEndedClientDisconnect, false
				}
				streamedBytes += int64(len(chunk))
			}
			if exceededLimit {
				emitStreamError("stream response exceeded configured byte limit")
				slog.Warn("SSE stream exceeded byte limit",
					"latency_ms", latencyMs,
					"streamed_bytes", streamedBytes,
					"limit_bytes", relay.maxBytes,
				)
				outcome = streamEndedTruncated
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				switch {
				case (relay.idleGuard.fired.Load() || errors.Is(err, ollama.ErrIdleTimeout) || errors.Is(err, errDirectClineIdleTimeout)) && r.Context().Err() == nil:
					// Upstream stalled: the idle guard closed the body to
					// unblock this read. Emit a distinct final SSE error event
					// and report the idle outcome so the dispatcher records
					// the failure.
					emitStreamError("upstream stream idle timeout")
					slog.Warn("SSE stream idle timeout: upstream sent no chunk within window",
						"idle_timeout_sec", int(relay.idleGuard.idle.Seconds()),
						"latency_ms", latencyMs,
						"streamed_bytes", streamedBytes,
					)
					outcome = streamEndedIdleTimeout
				case r.Context().Err() != nil:
					// Downstream went away while a read was in flight: this read
					// error is a consequence of the client cancel, not an
					// upstream fault. Classified separately so user cancels can
					// never be recorded as channel failures.
					slog.Debug("SSE stream read aborted by downstream disconnect",
						"err", err,
						"latency_ms", latencyMs,
						"streamed_bytes", streamedBytes,
					)
					outcome = streamEndedClientDisconnect
				case errors.Is(err, errResponsesOutputLimit):
					emitStreamError(errResponsesOutputLimit.Error())
					outcome = streamEndedTruncated
				case errors.Is(err, errMessagesChatStreamLimit) || errors.Is(err, ollama.ErrStreamLimit):
					emitStreamError("upstream stream exceeded configured byte limit")
					outcome = streamEndedTruncated
				default:
					// Mid-stream upstream failure (network reset, upstream crash,
					// truncated body): emit a final SSE error event so the client
					// can surface the failure explicitly instead of inferring it
					// from a missing [DONE] marker. Mirrors the byte-limit path.
					emitStreamError("upstream stream interrupted")
					slog.Warn("SSE stream read error", "err", err, "latency_ms", latencyMs)
					outcome = streamEndedUpstreamFault
				}
			}
			break
		}
	}

	return outcome, sawStreamBytes
}

var errResponsesOutputLimit = errors.New("stream response exceeded configured byte limit")

// RelayKit emits LF-delimited SSE events. Inspect bounded line fragments so
// even a single oversized JSON payload is charged before it is buffered.
type responsesFrameReader struct {
	reader    *bufio.Reader
	remaining int64
	delivered int
}

func (r *responsesFrameReader) readFrame() ([]byte, error) {
	var frame []byte
	for {
		fragment, err := r.reader.ReadSlice('\n')
		r.remaining -= int64(len(fragment))
		if r.remaining < 0 {
			return nil, errResponsesOutputLimit
		}
		frame = append(frame, fragment...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			if err == io.EOF && len(frame) > 0 {
				r.delivered++
				return frame, nil
			}
			return nil, err
		}
		if bytes.HasSuffix(frame, []byte("\n\n")) || bytes.HasSuffix(frame, []byte("\r\n\r\n")) {
			r.delivered++
			return frame, nil
		}
	}
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
