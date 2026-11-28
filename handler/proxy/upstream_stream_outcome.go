package proxyhandler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// streamOutcome classifies how an SSE relay ended so the dispatcher can
// record upstream faults distinctly from clean or client-driven endings.
// Every value except streamEndedNormally and streamEndedClientDisconnect is an
// upstream-side fault and MUST be recorded through the failure path.
type streamOutcome int

const (
	// streamEndedNormally covers the clean EOF case: the upstream finished the
	// stream and the whole body was relayed.
	streamEndedNormally streamOutcome = iota
	// streamEndedIdleTimeout means the upstream stopped sending chunks for
	// longer than PROXY_STREAM_IDLE_TIMEOUT_SEC and the idle guard aborted
	// the stream. The dispatcher records this via the failure path.
	streamEndedIdleTimeout
	// streamEndedUpstreamFault means the upstream body failed mid-stream
	// (network reset, upstream crash, truncated transfer) after the relay had
	// already begun. The client got a partial answer plus an SSE error event.
	streamEndedUpstreamFault
	// streamEndedTruncated means the relay hit PROXY_MAX_STREAM_RESPONSE_BYTES
	// and stopped early, so the client received an incomplete answer.
	streamEndedTruncated
	// streamEndedClientDisconnect means the downstream client cancelled or its
	// write side went away. This ending alone is NOT an upstream fault:
	// without an explicit upstream error, recording it as a failure would
	// poison channel health with user cancel behaviour. Usage already
	// extracted is still returned for accounting.
	streamEndedClientDisconnect
)

// String keeps log lines and test failure messages readable now that the
// classification has more than two members.
func (o streamOutcome) String() string {
	switch o {
	case streamEndedNormally:
		return "normally"
	case streamEndedIdleTimeout:
		return "idleTimeout"
	case streamEndedUpstreamFault:
		return "upstreamFault"
	case streamEndedTruncated:
		return "truncated"
	case streamEndedClientDisconnect:
		return "clientDisconnect"
	}
	return "unknown"
}

// judgeStreamContent feeds the streaming path parsed facts into the same pure
// content judge the buffered path calls (proxy.JudgeUpstreamContent). It
// replaces the historical log-only parallel implementation that merely warned
// about missing data events while the dispatcher still recorded a success.
//
// bodyUnreadable is the streaming half of the encoding-honesty contract: when
// the upstream used a Content-Encoding we do not decode, the analyzer saw bytes
// nobody could parse, so HasOutput=false here means "no evidence" and not "empty
// answer". It is passed to the judge as an explicit Unreadable fact rather than
// being special-cased here, keeping one owner of the verdict.
func judgeStreamContent(statusCode int, result incrementalSseAnalysisResult, latencyMs int64, bodyUnreadable bool) proxy.UpstreamVerdict {
	verdict := proxy.JudgeUpstreamContent(proxy.UpstreamContentFacts{
		StatusCode:    statusCode,
		Streaming:     true,
		RawText:       sseErrorEventText(result.ErrorEvents),
		HasOutput:     result.HasGeneratedOutput,
		HasErrorEvent: result.HasErrorEvent,
		Usage:         result.Usage.ToUsageSummary(),
		Unreadable:    bodyUnreadable,
	})
	if verdict.Failed {
		slog.Warn("stream content-based failure detected",
			"code", string(verdict.Code),
			"reason", verdict.Reason,
			"status", verdict.Status,
			"latency_ms", latencyMs,
			"event_count", result.EventCount,
			"has_done_marker", result.HasDoneMarker,
		)
	} else if !result.HasDataEvent {
		slog.Debug("SSE stream contained no data events",
			"latency_ms", latencyMs,
			"event_count", result.EventCount,
			"has_done_marker", result.HasDoneMarker,
		)
	}
	return verdict
}

// sseErrorEventText joins the SSE error-event payloads the analyzer kept. The
// bounded analyzer never retains the raw stream body, so error events are the
// only content signal the streaming path can hand to the shared judge.
func sseErrorEventText(events []SseEvent) string {
	if len(events) == 0 {
		return ""
	}
	parts := make([]string, 0, len(events))
	for _, ev := range events {
		if data := strings.TrimSpace(ev.Data); data != "" {
			parts = append(parts, data)
		}
	}
	return strings.Join(parts, "\n")
}

func relayUpstreamErrorResponse(w http.ResponseWriter, resp *http.Response, latencyMs int64) {
	bodyBytes, err := proxy.ReadBufferedResponseBody(resp.Body)
	if err != nil {
		slog.Warn("failed to read upstream error response", "err", err, "latency_ms", latencyMs, "status", resp.StatusCode)
		writeJSONError(w, 502, "Failed to read upstream response", "upstream_error")
		return
	}

	relayUpstreamResponseHeaders(w, resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)
}

func relayBufferedUpstreamErrorResponse(w http.ResponseWriter, resp *http.Response, bodyBytes []byte) {
	relayBufferedUpstreamResponse(w, resp, bodyBytes)
}
