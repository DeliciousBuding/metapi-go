package proxyhandler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
)

func (reply endpointReply) relayStream(
	w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig,
	selected *routing.SelectedChannel, upstreamModel, upstreamPath string, retry int, requestID string,
) {
	resp, startedAt, latencyMs := reply.response, reply.startedAt, reply.latencyMs
	// Always close the upstream body, including early client disconnects.
	var streamUsage ParsedUsage
	var streamEnd streamOutcome
	var streamVerdict *proxy.UpstreamVerdict
	func() {
		defer resp.Body.Close()
		var firstOutput *int64
		if proxy.IsDirectMediaPath(upstreamPath) && (!isSSEMediaResponse(resp.Header) || isDirectVideoContentPath(upstreamPath)) {
			streamUsage, streamEnd = handleMediaByteStream(w, r, resp)
		} else {
			streamUsage, streamEnd, streamVerdict = handleStreamUpstreamForEndpoint(w, r, resp, latencyMs, upstreamPath, upstreamModel, reply.bridgeOptions, func() { firstOutput = int64Ptr(time.Since(startedAt).Milliseconds()); ctx.lifecycle.firstOutput() }, selected.Direct != nil)
		}
		if timing := streamTimingFromContext(r.Context()); timing != nil {
			timing.interrupted = streamEnd != streamEndedNormally
		}
		streamUsage.FirstOutputLatencyMs = firstOutput
	}()
	if !streamUsage.Found {
		if usage := directNativeVideoResponseUsage(resp); usage.Found {
			streamUsage = usage
		}
	}
	streamUsage = applyDirectVideoAccounting(ctx, streamUsage)
	latencyMs = time.Since(startedAt).Milliseconds()
	if status, errText, terminal, failed := streamFailureVerdict(streamEnd, int(streamIdleTimeout().Seconds())); failed {
		// Any non-normal, non-client-driven ending is an upstream-side
		// fault: idle timeout, mid-stream interruption or byte-limit
		// truncation. The client already received the relayed prefix plus a
		// final SSE error event, so the response is terminal (no retry once
		// streaming has begun). Record via the failure path — channel
		// health, failure proxy_log and the terminal outcome — with the
		// full attempt duration, not just the header latency. Partial usage
		// already extracted from the stream is still accounted.
		totalLatencyMs := time.Since(startedAt).Milliseconds()
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, status, errText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, totalLatencyMs, reply.firstByteLatencyMs, status, true, streamUsage, retry, requestID, truncateErrText(errText))
		observeProxyTerminal(ctx, terminal, true, time.Since(startedAt))
		return
	}
	if streamVerdict != nil && streamVerdict.Failed {
		// A content-level failure remains authoritative even if the client
		// disconnected after an explicit upstream error was observed. The
		// shared content judge owns this verdict. Streaming already began,
		// so this is terminal; partial usage is still accounted.
		totalLatencyMs := time.Since(startedAt).Milliseconds()
		slog.Warn("stream content-based failure recorded",
			"code", string(streamVerdict.Code),
			"reason", streamVerdict.Reason,
			"model", upstreamModel,
			"channel_id", selected.Channel.ID,
			"request_id", requestID,
			"latency_ms", totalLatencyMs,
		)
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, streamVerdict.Status, streamVerdict.Reason)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, totalLatencyMs, reply.firstByteLatencyMs, streamVerdict.Status, true, streamUsage, retry, requestID, truncateErrText(streamVerdict.Reason))
		observeProxyTerminal(ctx, shared.StatusFromHTTP(streamVerdict.Status), true, time.Since(startedAt))
		return
	}
	// Observability only: include_usage was on the outbound body but SSE had no usable tokens.
	// Still record zeros / unknown — never invent tokens.
	if reply.expectStreamUsage {
		warnMissingStreamUsageAfterIncludeUsage(upstreamModel, upstreamPath, streamUsage)
	}
	recordUpstreamSuccess(r.Context(), cfg, selected, ctx.RequestedModel, upstreamModel, latencyMs, streamUsage)
	writeSuccessProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, reply.firstByteLatencyMs, resp.StatusCode, true, streamUsage, retry, requestID)
	observeProxyTerminal(ctx, shared.OutcomeSuccess, true, time.Duration(latencyMs)*time.Millisecond)
}
