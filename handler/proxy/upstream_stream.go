package proxyhandler

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	messages "github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

func handleStreamUpstream(w http.ResponseWriter, r *http.Request, resp *http.Response, latencyMs int64) (ParsedUsage, streamOutcome, *proxy.UpstreamVerdict) {
	return handleStreamUpstreamForEndpoint(w, r, resp, latencyMs, r.URL.Path, "", messages.Options{}, nil)
}

func handleStreamUpstreamForEndpoint(w http.ResponseWriter, r *http.Request, resp *http.Response, latencyMs int64, upstreamPath, upstreamModel string, bridgeOptions messages.Options, onFirstOutput func(), direct ...bool) (ParsedUsage, streamOutcome, *proxy.UpstreamVerdict) {
	empty := ParsedUsage{Source: usageSourceUnknown}
	if resp == nil || resp.Body == nil {
		return empty, streamEndedNormally, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		relayUpstreamErrorResponse(w, resp, latencyMs)
		return empty, streamEndedNormally, nil
	}

	// Active SSE stream gauge: Inc when a real stream starts, Dec on every
	// exit path (success, error, byte-limit, or client disconnect). The defer
	// runs after writeSSEHeaders / before the read loop returns so the gauge
	// always tracks a live stream, never a buffered non-stream response.
	shared.IncActiveStreams()
	defer shared.DecActiveStreams()

	// The same single encoding decision the buffered path applies (see
	// normalizeBufferedUpstreamBody): a codec the stdlib implements is decoded
	// inline so every chunk we re-frame is real SSE text, and a body we cannot
	// read is relayed verbatim under its own Content-Encoding with judgement
	// abandoned. writeSSEHeaders deliberately never copies Content-Encoding — a
	// stream is re-framed chunk by chunk, so relaying the upstream's encoding
	// would be a lie. The verbatim branch above is the single exception, and it
	// sets the header itself precisely because there we rewrote nothing.
	bodyReadable := prepareStreamUpstreamBody(resp, w, upstreamBodyIdent{
		requestID: proxy.RequestIDFromContext(r.Context()),
		stream:    true,
	})

	bridgeMessages := isMessagesChatBridge(r.URL.Path, upstreamPath)
	bridgeDirect := len(direct) > 0 && direct[0] && directBridgeNeeded(r.URL.Path, upstreamPath)
	wire := directProviderWireFromContext(r.Context())
	imageTaskStream := wire != nil && wire.DriverImageStreamOptions != nil
	codexImage := wire != nil && wire.Profile == "codex-image"
	routerChatStream := wire != nil && (wire.Profile == "openrouter" || wire.Profile == "cerebras")
	nanoGPTStream := wire != nil && wire.Profile == "nanogpt"
	providerStream := wire != nil && (len(wire.Plugins) > 0 || wire.Driver != nil || wire.Profile == "codex" || wire.Profile == "antigravity" || wire.StripToolPrefix || codexImage || (wire.Profile == "cline" || routerChatStream) && bridgeDirect)
	bailianStream := wire != nil && wire.Profile == "bailian"
	var relayStream protocolEventStream
	if bridgeDirect && wire != nil && wire.ProtocolSession != nil {
		var err error
		relayStream, err = wire.ProtocolSession.NewResponseStream()
		if err != nil {
			writeJSONErrorWithRequest(w, http.StatusBadGateway, "Cannot initialize upstream protocol conversion", "upstream_error", proxy.RequestIDFromContext(r.Context()))
			return empty, streamEndedUpstreamFault, nil
		}
	}
	if (bridgeMessages || bridgeDirect || providerStream || bailianStream || routerChatStream || nanoGPTStream) && !bodyReadable {
		w.Header().Del("Content-Encoding")
		writeJSONErrorWithRequest(w, http.StatusBadGateway, "Cannot decode upstream Chat stream for Messages", "upstream_error", proxy.RequestIDFromContext(r.Context()))
		return empty, streamEndedUpstreamFault, nil
	}

	writeSSEHeaders(w)
	w.WriteHeader(200)

	// A relay must not inherit the server-level WriteTimeout (60s) or the
	// proxy-surface write budget: both bound a whole request, so a healthy long
	// completion would be torn down mid-stream. Each downstream write+flush
	// instead gets its own bounded window (PROXY_STREAM_WRITE_TIMEOUT_SEC), armed
	// immediately before the I/O and cleared immediately after it. The deadline
	// must never outlive a write: net/http's HTTP/2 server turns it into a
	// timer that resets the stream when it fires, which would kill a healthy
	// stream during a long gap between upstream chunks.
	writeBudget := streamWriteTimeout()
	// Drop the whole-request deadline inherited from Server.WriteTimeout /
	// ProxyWriteDeadline before waiting for the first upstream chunk: HTTP/2
	// implements an armed deadline as a per-stream timer, so leaving it running
	// during a slow first chunk would reset a stream that is still healthy.
	clearStreamWriteDeadline(w)

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
	timing := streamTimingFromContext(r.Context())
	idleBody := &streamIdleBody{ReadCloser: resp.Body, timing: timing}
	idleBody.guard = newStreamIdleGuard(idleTimeout, idleBody.closeUnderlying)
	resp.Body = idleBody
	maxStreamBytes := streamResponseByteLimit()
	var messageBridge *messagesChatBody
	if imageTaskStream {
		messageBridge = newDirectImageTaskSSEBody(resp.Body, onFirstOutput)
		resp.Body = messageBridge
	} else if codexImage {
		messageBridge = newDirectCodexImagesBody(resp.Body, r.URL.Path, true, maxStreamBytes)
		messageBridge.original.onFirstOutput = onFirstOutput
		resp.Body = messageBridge
	} else if bridgeDirect || providerStream || bailianStream || routerChatStream || nanoGPTStream {
		var stream protocolEventStream
		if bridgeDirect {
			if relayStream != nil {
				stream = relayStream
			} else {
				stream = directResponseStream(r.URL.Path, upstreamPath, upstreamModel, bridgeOptions)
			}
		}
		if bailianStream {
			if stream == nil {
				stream = newDirectBailianStream()
			} else {
				stream = &chainedProtocolStream{first: newDirectBailianStream(), second: stream}
			}
		}
		if providerStream {
			provider := &directProviderStream{ctx: r.Context(), next: stream}
			if wire.Profile == "codex" {
				provider.codex = &responses.CodexResponseCollector{Limit: maxStreamBytes}
			}
			stream = provider
		}
		var filter protocolEventStream
		if routerChatStream {
			filter = newDirectRouterChatStream()
		} else if nanoGPTStream {
			filter = newDirectNanoGPTStream(wire.NanoGPTTools)
		}
		if filter != nil {
			if stream == nil {
				stream = filter
			} else {
				stream = &chainedProtocolStream{first: filter, second: stream}
			}
		}
		messageBridge = newProtocolBridgeBody(resp.Body, stream, maxStreamBytes)
		observeDirectDriverStream(messageBridge.original, wire)
		if routerChatStream {
			// A 20 MiB image needs about 27 MiB as base64. Keep other Chat
			// profiles at 1 MiB and still enforce the total raw/output budgets.
			messageBridge.frameLimit = min(maxStreamBytes, 32<<20)
		}
		resp.Body = messageBridge
	} else if bridgeMessages {
		messageBridge = newMessagesChatBody(resp.Body, upstreamModel, maxStreamBytes, bridgeOptions)
		resp.Body = messageBridge
	} else if bodyReadable && !strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Disposition")), "attachment") {
		resp.Body = withNativeTerminalBody(resp.Body, r.URL.Path, downstreamResponseModel(r))
	}

	if messageBridge != nil && !codexImage && !imageTaskStream && downstreamResponseModel(r) != "" {
		resp.Body = withNativeTerminalBody(resp.Body, r.URL.Path, downstreamResponseModel(r))
	}
	analyzer := newIncrementalSseAnalyzer()
	analyzer.onFirstOutput = onFirstOutput
	if routerChatStream {
		if native, ok := resp.Body.(*nativeTerminalBody); ok {
			// Native normalization can retain both images and content aliases;
			// allow that bounded expansion. The relay still enforces the total
			// output budget and reports truncation if expansion exceeds it.
			native.frameLimit = 2 * messageBridge.frameLimit
			native.analysis = analyzer
			// The bridge already owns the original provider usage and model.
			native.original = nil
		} else {
			messageBridge.outputAnalysis = analyzer
		}
	}
	if timing != nil && bodyReadable {
		if messageBridge != nil {
			messageBridge.original.timing = timing
		} else if native, ok := resp.Body.(*nativeTerminalBody); ok {
			native.timing = timing
		} else {
			analyzer.timing = timing
		}
	}
	relay := streamChunkRelay{maxBytes: maxStreamBytes, writeBudget: writeBudget, idleGuard: idleBody.guard}
	if bodyReadable && !codexImage && !routerChatStream && !imageTaskStream {
		relay.analyzer = analyzer
	}
	if endpoint, _ := proxy.EndpointFromPath(r.URL.Path); relayStream != nil && endpoint == proxy.EndpointResponses && messageBridge != nil && !codexImage && !imageTaskStream {
		relay.responsesBridge = messageBridge
	}
	end, logDetail := relay.copy(w, r, resp, latencyMs)

	// Post-stream SSE analysis uses bounded incremental state instead of
	// retaining the complete upstream body.
	result := analyzer.Result()
	native, _ := resp.Body.(*nativeTerminalBody)
	if native != nil && native.original != nil {
		result.Usage = native.original.Result().Usage
	}
	if messageBridge != nil {
		// Billing consumes the actual Chat usage, not synthetic Messages start
		// counters or a lossy protocol projection.
		result.Usage = messageBridge.original.Result().Usage
		if codexImage {
			result = messageBridge.original.Result()
		}
	}
	if imageTaskStream {
		result = messageBridge.original.Result()
		// Includes raw polling usage even when no preview/complete event
		// arrived, or the frame itself failed before downstream delivery.
		if wire.DriverImageUsage != nil {
			result.Usage = *wire.DriverImageUsage
		}
	}
	if logDetail {
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
	}
	if end == streamEndedClientDisconnect && !result.HasErrorEvent {
		// Without an explicit upstream error already observed, a client
		// disconnect leaves no complete answer to judge. Do not penalize a
		// channel that never got to finish. Still return
		// any usage already extracted from earlier SSE events (best-effort
		// partial) — never invent tokens.
		return result.Usage, end, nil
	}
	// A native incomplete terminal can legitimately have zero output.
	if native != nil && native.responseIncomplete && !native.terminalFailed && end == streamEndedNormally && !result.HasErrorEvent {
		return result.Usage, end, nil
	}
	verdict := judgeStreamContent(resp.StatusCode, result, latencyMs, !bodyReadable)
	return result.Usage, end, &verdict
}
