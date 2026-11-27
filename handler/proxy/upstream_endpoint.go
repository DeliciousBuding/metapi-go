package proxyhandler

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	messages "github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

// dispatchEndpointAttempt sends one selected native endpoint without walking
// protocol candidates. Multipart callers use this single-attempt path.
func dispatchEndpointAttempt(
	w http.ResponseWriter,
	r *http.Request,
	ctx *Ctx,
	cfg *UpstreamConfig,
	selected *routing.SelectedChannel,
	upstreamModel string,
	proxyConfig *platform.ProxyConfig,
	upstreamPath string,
	contentType string,
	bodyBytes []byte,
	firstByteTimeoutMs int64,
	retry int,
	maxRetries int,
	recordFailure bool,
	requestID string,
) (finished bool, nextPending *pendingUpstreamFailure) {
	finished, pending, _ := dispatchEndpointAttemptWithContinue(
		w, r, ctx, cfg, selected, upstreamModel, proxyConfig,
		upstreamPath, contentType, bodyBytes, firstByteTimeoutMs,
		retry, maxRetries, true, true, ctx != nil && ctx.IsStream, false, requestID, messages.Options{},
	)
	if !recordFailure {
		return finished, pending
	}
	return finished, pending
}

// dispatchEndpointAttemptWithContinue runs one endpoint path.
// cont=true means the caller should try the next protocol candidate without
// recording channel failure / without writing a terminal response.
func dispatchEndpointAttemptWithContinue(
	w http.ResponseWriter,
	r *http.Request,
	ctx *Ctx,
	cfg *UpstreamConfig,
	selected *routing.SelectedChannel,
	upstreamModel string,
	proxyConfig *platform.ProxyConfig,
	upstreamPath string,
	contentType string,
	bodyBytes []byte,
	firstByteTimeoutMs int64,
	retry int,
	maxRetries int,
	isLastEndpoint bool,
	disableCrossProtocolFallback bool,
	effectiveStream bool,
	expectStreamUsage bool,
	requestID string,
	bridgeOptions messages.Options,
) (finished bool, nextPending *pendingUpstreamFailure, cont bool) {
	if requestID == "" {
		requestID = proxy.RequestIDFromContext(r.Context())
	}
	if err := validateVideoTaskSelection(ctx, selected, upstreamPath); err != nil {
		writeJSONErrorWithRequest(w, 503, err.Error(), "upstream_error", requestID)
		return true, nil, false
	}
	upstreamBaseURL := selected.Site.URL
	if selected.Direct != nil {
		upstreamBaseURL = selected.Direct.BaseURL
	}
	upstreamURL := siteRequestURL(upstreamBaseURL, upstreamPath, upstreamModel, effectiveStream)
	var directEndpoint *store.DirectEndpoint
	if selected.Direct != nil && ctx != nil {
		var chosen *store.DirectEndpoint
		if wire := directProviderWireFromContext(r.Context()); wire != nil {
			chosen = wire.Endpoint
		}
		upstreamURL, directEndpoint = directRequestURL(selected.Direct, upstreamPath, upstreamModel, effectiveStream, chosen)
		if wire := directProviderWireFromContext(r.Context()); wire != nil && wire.DriverURL != "" {
			upstreamURL = wire.DriverURL
		}
	}
	startedAt := time.Now()
	firstByteTimeoutMs = ctx.remaining(firstByteTimeoutMs)

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytesReader(bodyBytes))
	if err != nil {
		slog.Warn("upstream request construction failed",
			"err", err, "url", upstreamURL, "model", upstreamModel,
			"request_id", requestID, "retry", retry)
		if shouldContinueEndpointFallback(http.StatusBadGateway, err.Error(), isLastEndpoint, disableCrossProtocolFallback, endpointFailureTransport) {
			return false, nil, true
		}
		if retry < maxRetries {
			return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Upstream request failed", "upstream_error"), false
		}
		writeJSONErrorWithRequest(w, 502, "Upstream request failed", "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, effectiveStream, time.Since(startedAt))
		return true, nil, false
	}
	req.Header.Set("Content-Type", contentType)
	if isDirectVideoContentPath(upstreamPath) {
		query := req.URL.Query()
		if variant := r.URL.Query().Get("variant"); variant != "" {
			query.Set("variant", variant)
		}
		req.URL.RawQuery = query.Encode()
		for _, header := range []string{"Range", "If-Range", "Accept"} {
			copyHeaderIfAbsent(req.Header, r.Header, header)
		}
	}
	siteAuthHeader := siteNativeAPIKeyHeader(selected, upstreamPath)
	if siteAuthHeader != "" && selected.TokenValue != "" {
		// The native credential is a request header: an explicit site-wins
		// custom-header setting may replace it, just like other request headers.
		req.Header.Set(siteAuthHeader, selected.TokenValue)
	}
	// Native site credentials above honor the custom-header collision policy.
	// Client protocol headers fill gaps after site headers and identity; Bearer
	// and direct credentials below always use the selected token.
	applyProxyCustomHeaders(req, proxyConfig)
	applyClientProtocolHeaders(req, r.Header, upstreamPath)
	// The value of this header decides whether net/http transparently decodes
	// the answer, i.e. whether we can read the body we are about to bill for and
	// health-check — so it is never site/adapter configurable. Must run after
	// both header builders above (see stripUpstreamAcceptEncoding).
	if selected.Direct == nil {
		stripUpstreamAcceptEncoding(req, proxyConfig, selected.Site.ID, selected.Channel.ID)
	}
	directMessagesRequest := selected.Direct != nil && proxyPathIsMessages(upstreamPath)
	if directMessagesRequest && req.Header.Get("anthropic-version") == "" {
		req.Header.Set("anthropic-version", platform.ClaudeDefaultAnthropicVersion)
	}
	if selected.Direct != nil {
		req.Header.Del("x-api-key")
		req.Header.Del("x-goog-api-key")
		req.Header.Del("Authorization")
	}
	if selected.TokenValue != "" && (directEndpoint == nil || directEndpoint.Auth != store.DirectAuthNone) {
		useAPIKey := directMessagesRequest
		if directEndpoint != nil {
			useAPIKey = directEndpoint.Auth == store.DirectAuthAPIKey
		}
		if directEndpoint != nil && directEndpoint.Auth == store.DirectAuthGoogle {
			req.Header.Set("x-goog-api-key", selected.TokenValue)
		} else if useAPIKey {
			req.Header.Set("x-api-key", selected.TokenValue)
			req.Header.Del("Authorization")
		} else if siteAuthHeader == "" {
			req.Header.Set("Authorization", "Bearer "+selected.TokenValue)
		}
	}
	applyDirectProviderHeaders(req, directProviderWireFromContext(r.Context()))
	if selected.Direct != nil {
		if err := applyDirectRequestHeaderOperations(req, selected.Direct.ChannelID, directProviderWireFromContext(r.Context())); err != nil {
			writeJSONErrorWithRequest(w, http.StatusBadRequest, "Cannot apply selected channel request headers", "invalid_request_error", requestID)
			return true, nil, false
		}
		if wire := directProviderWireFromContext(r.Context()); wire != nil && wire.Driver != nil && directEndpoint != nil && directEndpoint.Auth != store.DirectAuthNone {
			if err := wire.Driver.ApplyAuthentication(req.Header, selected.TokenValue); err != nil {
				writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "Selected provider authentication is unavailable", "upstream_error", requestID)
				return true, nil, false
			}
		}
		// Custom headers were already applied before ordered operations. Do not
		// let the transport reinsert a header explicitly deleted by the channel.
		if proxyConfig != nil && selected.Direct.RequestConfig.Active() {
			configCopy := *proxyConfig
			configCopy.CustomHeaders = nil
			proxyConfig = &configCopy
		}
	}

	rateCtx, rateErr := prepareChannelRateAttempt(r.Context())
	if rateErr != nil {
		return false, nil, false
	}
	r = r.WithContext(rateCtx)
	// Paths that fail before writing an attempt log still report unknown usage.
	// A terminal log wins through the per-attempt once guard.
	defer recordChannelRateUsage(rateCtx, ParsedUsage{Source: usageSourceUnknown})

	// Allocate the observed attempt before dispatch, independent of whether a
	// log writer succeeds or this endpoint falls back to another protocol.
	r = r.WithContext(beginGatewayAttempt(r.Context()))
	if effectiveStream && ctx.IsStream {
		r = r.WithContext(startAttemptStreamTiming(r.Context(), upstreamPath, startedAt))
	}
	req = req.WithContext(r.Context())
	var resp *http.Response
	wire := directProviderWireFromContext(r.Context())
	if wire != nil && (wire.DriverOperation == "remix" || wire.DriverOperation == "deleteTask") {
		// Both mutate a fixed owned task. A transport/response failure cannot
		// prove that the write did not execute, so do not replay or change slot.
		maxRetries = retry
		disableCrossProtocolFallback = true
	}
	if wire != nil && (wire.Profile == "openrouter-image" || isDirectMediaProfile(wire.Profile)) {
		// Replacing the built-in payload/consumer must not remove its image
		// submission replay boundary. A failed POST may already be billable.
		maxRetries = retry
	}
	if wire != nil && wire.DriverImageQuery != nil {
		maxRetries = retry
		disableCrossProtocolFallback = true
		resp, err = sendDirectImageTaskRequest(cfg, req, r, ctx, selected, wire, proxyConfig, firstByteTimeoutMs)
	} else if wire != nil && wire.DriverContentQuery {
		maxRetries = retry
		resp, err = sendDirectProviderContent(cfg, req, ctx, selected, wire, proxyConfig, firstByteTimeoutMs)
	} else if wire != nil && wire.DriverOperation == "" && isDirectNativeVideoProfile(wire.Profile) {
		maxRetries = retry
		taskID := ""
		if ctx.videoTask != nil {
			taskID = ctx.videoTask.UpstreamID
			account := func(usage ParsedUsage) error {
				ctx.videoUsageVerified = true
				_, err := accountDirectVideoUsage(r.Context(), ctx, selected, usage)
				return err
			}
			req = req.WithContext(context.WithValue(req.Context(), directVideoAccountingOperationKey{}, account))
		}
		resp, err = sendDirectNativeVideoRequest(cfg, req, proxyConfig, firstByteTimeoutMs, wire.Profile, taskID)
	} else if wire != nil && wire.Driver == nil && wire.Profile == "openrouter-image" {
		resp, err = sendDirectOpenRouterImageRequest(cfg, req, proxyConfig, firstByteTimeoutMs, wire.ImageResponseFormat)
	} else if wire != nil && wire.Driver == nil && isDirectMediaProfile(wire.Profile) {
		// Async image submission cannot be replayed after a poll/download failure.
		resp, err = sendDirectMediaProfileRequest(cfg, req, proxyConfig, firstByteTimeoutMs, wire.Profile)
	} else {
		resp, err = sendUpstreamRequest(cfg, req, proxyConfig, firstByteTimeoutMs, effectiveStream)
	}
	if err == nil && resp != nil {
		if timing := streamTimingFromContext(r.Context()); timing != nil {
			timing.arrival = time.Now()
			timing.observer.MarkHeader()
		}
		if wire := directProviderWireFromContext(r.Context()); wire != nil {
			switch wire.Profile {
			case "cline":
				err = normalizeDirectClineResponse(r.Context(), resp, effectiveStream)
			case "bedrock":
				err = normalizeDirectBedrockResponse(r.Context(), resp, effectiveStream)
			case "ollama":
				err = normalizeDirectOllamaStream(r.Context(), resp, upstreamModel, effectiveStream)
			}
			if err != nil {
				resp.Body.Close()
			}
		}
	}
	latencyMs := time.Since(startedAt).Milliseconds()
	var firstByteLatencyMs *int64
	if err == nil && resp != nil {
		ctx.armRetryBudget()
		firstByteLatencyMs = int64Ptr(latencyMs)
		if first := directMediaFirstByteLatencyMs(resp); first != nil {
			firstByteLatencyMs = first
		}
	}

	if err != nil {
		// Downstream client cancellation while waiting for upstream headers:
		// never trigger cross-protocol fallback, never poison channel health,
		// and record 499 + client_cancelled consistently across all endpoints.
		if r.Context().Err() != nil {
			reason := "Client canceled request"
			if proxy.IsDirectMediaPath(upstreamPath) {
				reason = "Client canceled media request"
			}
			writeFailureProxyLog(context.WithoutCancel(r.Context()), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 499, effectiveStream, directMediaFailureUsage(err), retry, requestID, reason)
			w.WriteHeader(499)
			observeProxyTerminal(ctx, shared.OutcomeClientError, effectiveStream, time.Since(startedAt))
			return true, nil, false
		}
		// First-byte timeout: continue to next protocol when allowed; do not poison.
		if ctx.expiredBeforeHeaders(err, firstByteTimeoutMs) {
			err = proxy.ErrObservedFirstByteTimeout
		}
		if proxy.IsObservedFirstByteTimeoutError(err) {
			slog.Debug("upstream first-byte timeout",
				"url", upstreamURL,
				"model", upstreamModel,
				"channel_id", selected.Channel.ID,
				"first_byte_timeout_ms", firstByteTimeoutMs,
				"is_last_endpoint", isLastEndpoint,
				"request_id", requestID,
				"retry", retry,
			)
			if shouldContinueEndpointFallback(http.StatusRequestTimeout, err.Error(), isLastEndpoint, disableCrossProtocolFallback, endpointFailureFirstByteTimeout) {
				ctx.armRetryBudget()
				writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusRequestTimeout, effectiveStream, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, truncateErrText(err.Error()))
				return false, nil, true
			}
			// Terminal for this channel attempt.
			errText := err.Error()
			recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, 0, errText)
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusRequestTimeout, effectiveStream, directMediaFailureUsage(err), retry, requestID, errText)
			if retry < maxRetries && proxy.ShouldRetryProxyRequest(408, errText) {
				return false, jsonPendingUpstreamFailure(http.StatusRequestTimeout, "Upstream first-byte timeout", "upstream_error"), false
			}
			writeJSONErrorWithRequest(w, http.StatusRequestTimeout, "Upstream first-byte timeout", "upstream_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeTimeout, effectiveStream, time.Since(startedAt))
			return true, nil, false
		}
		slog.Warn("upstream request failed",
			"err", err, "url", upstreamURL, "model", upstreamModel,
			"channel_id", selected.Channel.ID, "request_id", requestID, "retry", retry)
		errText := err.Error()
		// Transport-level failures go through the same single decision function
		// (and therefore the same same-site abort policy) as HTTP/content
		// failures; the historical bare check let them bypass the abort policy.
		if shouldContinueEndpointFallback(http.StatusBadGateway, errText, isLastEndpoint, disableCrossProtocolFallback, classifyEndpointTransportFailure(err)) {
			// Network error may still be protocol-local; allow next endpoint without poison.
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, effectiveStream, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, truncateErrText(errText))
			return false, nil, true
		}
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, 0, errText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, effectiveStream, directMediaFailureUsage(err), retry, requestID, errText)
		if retry < maxRetries {
			return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Upstream request failed", "upstream_error"), false
		}
		writeJSONErrorWithRequest(w, 502, "Upstream request failed", "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, effectiveStream, time.Since(startedAt))
		return true, nil, false
	}

	// Provider-forced and policy-required streams finish before any JSON output.
	// Their observed usage is retained through the ordinary accounting path.
	var aggregatedUsage *ParsedUsage
	if wire := directProviderWireFromContext(r.Context()); wire != nil && (wire.Profile == "codex" || wire.Profile == "codex-image" || wire.AggregateStream) && !ctx.IsStream && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var collected []byte
		var usage ParsedUsage
		var aggregateErr error
		if wire.Profile == "codex-image" {
			collected, usage, aggregateErr = collectDirectCodexImagesResponse(resp, startedAt, upstreamPath)
		} else if wire.AggregateStream && wire.Profile != "codex" {
			collected, usage, aggregateErr = collectDirectPolicyResponse(r.Context(), resp, startedAt, upstreamPath)
		} else {
			collected, usage, aggregateErr = collectDirectCodexResponse(r.Context(), resp, startedAt)
		}
		latencyMs = time.Since(startedAt).Milliseconds()
		if aggregateErr != nil {
			if (wire.Profile == "codex-image" || wire.AggregateStream) && r.Context().Err() != nil {
				reason := "Client canceled media request"
				if wire.AggregateStream {
					reason = "Client canceled request"
				}
				writeFailureProxyLog(context.WithoutCancel(r.Context()), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, 499, false, usage, retry, requestID, reason)
				w.WriteHeader(499)
				observeProxyTerminal(ctx, shared.OutcomeClientError, false, time.Since(startedAt))
				return true, nil, false
			}
			reason := "Cannot aggregate upstream Codex response: " + aggregateErr.Error()
			if wire.AggregateStream {
				reason = "Cannot aggregate upstream response: " + aggregateErr.Error()
			}
			if channelPluginExecutionFailure(aggregateErr) {
				reason = "Cannot execute selected channel response plugin"
			}
			recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, http.StatusBadGateway, reason)
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, firstByteLatencyMs, http.StatusBadGateway, false, usage, retry, requestID, reason)
			writeJSONErrorWithRequest(w, http.StatusBadGateway, reason, "upstream_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Since(startedAt))
			return true, nil, false
		}
		aggregatedUsage = &usage
		resp.Body = io.NopCloser(bytes.NewReader(collected))
		resp.Header.Set("Content-Type", "application/json")
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.Header.Del("ETag")
		effectiveStream = false
	}
	// Step 9: Handle response
	if proxy.IsDirectMediaPath(upstreamPath) && resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.StatusCode != http.StatusNoContent {
		if isDirectVideoContentPath(upstreamPath) {
			effectiveStream = true
		} else if bit := proxy.DirectProtocolForPath(upstreamPath); bit != store.DirectProtocolVideo && !isNativeJSONProtocol(bit) {
			effectiveStream = !isJSONMediaResponse(resp.Header)
		}
	}
	if effectiveStream && selected.Direct != nil && ctx.videoTask != nil && isDirectVideoContentPath(upstreamPath) && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		ctx.videoUsageVerified = true
		if _, err := accountDirectVideoUsage(r.Context(), ctx, selected, directNativeVideoResponseUsage(resp)); err != nil {
			resp.Body.Close()
			writeJSONErrorWithRequest(w, 503, err.Error(), "upstream_error", requestID)
			return true, nil, false
		}
	}
	reply := endpointReply{
		response:           resp,
		startedAt:          startedAt,
		latencyMs:          latencyMs,
		firstByteLatencyMs: firstByteLatencyMs,
		aggregatedUsage:    aggregatedUsage,
		effectiveStream:    effectiveStream,
		expectStreamUsage:  expectStreamUsage,
		bridgeOptions:      bridgeOptions,
	}
	return reply.relay(w, r, ctx, cfg, selected, upstreamModel, upstreamPath,
		retry, maxRetries, isLastEndpoint, disableCrossProtocolFallback, requestID)
}

type endpointReply struct {
	response           *http.Response
	startedAt          time.Time
	latencyMs          int64
	firstByteLatencyMs *int64
	aggregatedUsage    *ParsedUsage
	effectiveStream    bool
	expectStreamUsage  bool
	bridgeOptions      messages.Options
}
