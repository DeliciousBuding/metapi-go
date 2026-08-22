package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
)

// UpstreamConfig holds the dependencies needed for upstream forwarding.
type UpstreamConfig struct {
	Router         proxy.TokenRouterInterface
	RouteRefresher proxy.RouteRefreshWorkflow
	Coordinator    *proxy.ProxyChannelCoordinator
	Executor       *proxy.RuntimeExecutor
	// SiteLimiter caps concurrent dispatches per site (sites.max_concurrency).
	// When nil, proxy.DefaultSiteConcurrencyLimiter is used.
	// Orthogonal to Coordinator channel leases.
	SiteLimiter *proxy.SiteConcurrencyLimiter
	// LogProxy persists successful/failed proxy attempts into proxy_logs.
	// When nil, defaultLogProxyWriter uses store.GetDB() (no-op if DB unset).
	LogProxy func(ctx context.Context, entry proxy.ProxyLogEntry) error
}

var upstreamCfg *UpstreamConfig
var unconfiguredUpstreamLogOnce sync.Once

// defaultUpstreamClient is used as a safety fallback when the RuntimeExecutor
// has not been wired (e.g., during tests). It carries a 90s timeout so a hung
// upstream never leaks a goroutine, and the shared RejectCrossOriginRedirect
// policy so a public origin cannot 302 into metadata/loopback when Executor is
// nil. Production deployments should always wire the Executor field via
// SetUpstreamConfig.
var defaultUpstreamClient = &http.Client{
	Timeout:       90 * time.Second,
	CheckRedirect: platform.RejectCrossOriginRedirect,
}

// SetUpstreamConfig sets the package-level upstream forwarding dependencies.
// Called during server startup to wire in the routing engine and HTTP executor.
func SetUpstreamConfig(cfg *UpstreamConfig) {
	upstreamCfg = cfg
}

// getUpstreamConfig returns the configured upstream dependencies.
func getUpstreamConfig() *UpstreamConfig {
	return upstreamCfg
}

// dispatchUpstream forwards a proxy request to the selected upstream channel.
// Implements the spec's 10-step Handler pattern.
func dispatchUpstream(w http.ResponseWriter, r *http.Request, ctx *Ctx) {
	shared.RecordProxyRequest()
	startedAt := time.Now()
	// Parent request/trace id is stable across channel retries and endpoint fallbacks.
	reqCtx, requestID := proxy.EnsureRequestID(r.Context(), "")
	if requestID != "" && r.Context() != reqCtx {
		r = r.WithContext(reqCtx)
	}
	cfg := getUpstreamConfig()
	if cfg == nil {
		if isProxyStubEnabled() {
			writeStubResponse(w, ctx)
			return
		}
		unconfiguredUpstreamLogOnce.Do(func() {
			slog.Error("proxy upstream dependencies are not configured", "request_id", requestID)
		})
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "Proxy upstream is not configured", "server_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUnavailable, ctx != nil && ctx.IsStream, time.Since(startedAt))
		return
	}

	excludeChannelIDs := make([]int64, 0)
	maxRetries := ctx.MaxRetries
	downstreamPolicy := routingPolicyFromAuth(ctx.Policy)
	// multi-tier: best-effort request context estimate for route tier pick.
	// ctx is constructed non-nil by PrepareCtx (which returns a SurfResult on
	// failure instead of a nil Ctx), so it is safe to dereference unguarded.
	downstreamPolicy.RequestedContextTokens = routing.EstimateRequestContextTokens(ctx.Body)
	upstreamPath := ctx.DownstreamPath
	if upstreamPath == "" {
		upstreamPath = r.URL.Path
	}
	// Wave 4 security handoff T1: never forward a downstream path containing
	// ".." segments. Go's http.Client preserves ".." on the wire, and the
	// upstream host would normalize it outside the site API prefix — letting
	// any authenticated downstream key holder reach arbitrary upstream paths.
	// Reject before channel selection; clean paths pass through untouched.
	if proxy.ContainsPathTraversal(upstreamPath) {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, "Invalid request path", "invalid_request_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeClientError, ctx != nil && ctx.IsStream, time.Since(startedAt))
		return
	}
	var pendingFailure *pendingUpstreamFailure

	for retry := 0; retry <= maxRetries; retry++ {
		// Step 6: Channel selection
		selected, err := proxy.SelectProxyChannelForAttempt(
			r.Context(),
			cfg.Router,
			cfg.Coordinator,
			cfg.RouteRefresher,
			proxy.ChannelSelectionInput{
				RequestedModel:    ctx.RequestedModel,
				DownstreamPolicy:  downstreamPolicy,
				ExcludeChannelIDs: excludeChannelIDs,
				RetryCount:        retry,
				ForcedChannelID:   ctx.ForcedChannelID,
			},
		)
		if err != nil || selected == nil {
			slog.Warn("channel selection failed",
				"err", err,
				"model", ctx.RequestedModel,
				"retry", retry,
				"request_id", requestID,
			)
			if pendingFailure != nil {
				pendingFailure.write(w, requestID)
				observeProxyTerminal(ctx, pendingFailure.outcomeStatus(), ctx != nil && ctx.IsStream, time.Since(startedAt))
				return
			}
			writeJSONErrorWithRequest(w, 503, "No available channels", "server_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeUnavailable, ctx != nil && ctx.IsStream, time.Since(startedAt))
			return
		}
		excludeChannelIDs = append(excludeChannelIDs, selected.Channel.ID)

		// Site-scoped concurrency (orthogonal to channel leases).
		// On saturate: skip to next channel/site — do NOT mark expired/cascade.
		// sites.max_concurrency.
		siteLimiter := cfg.SiteLimiter
		if siteLimiter == nil {
			siteLimiter = proxy.DefaultSiteConcurrencyLimiter
		}
		siteSlot, acquired := siteLimiter.TryAcquire(selected.Site.ID, selected.Site.MaxConcurrency)
		if !acquired {
			slog.Debug("site concurrency saturated; skipping channel without failure cascade",
				"site_id", selected.Site.ID,
				"channel_id", selected.Channel.ID,
				"max_concurrency", selected.Site.MaxConcurrency,
				"model", ctx.RequestedModel,
				"retry", retry,
				"request_id", requestID,
			)
			continue
		}

		// Hold the site slot for the full attempt; always release (even on panic path).
		var finished bool
		var nextPending *pendingUpstreamFailure
		func() {
			defer siteSlot.Release()
			finished, nextPending = dispatchSelectedUpstream(w, r, ctx, cfg, selected, upstreamPath, retry, maxRetries, requestID)
		}()
		if finished {
			return
		}
		if nextPending != nil {
			pendingFailure = nextPending
		}
	}

	writeJSONErrorWithRequest(w, 503, "All channels exhausted", "server_error", requestID)
	observeProxyTerminal(ctx, shared.OutcomeUnavailable, ctx != nil && ctx.IsStream, time.Since(startedAt))
}

// observeProxyTerminal records labeled counters, latency histogram, and optional export hook.
// Labels are privacy-safe (endpoint family + outcome); never model/key/body.
func observeProxyTerminal(ctx *Ctx, status string, stream bool, latency time.Duration) {
	endpoint := shared.EndpointOther
	if ctx != nil && ctx.DownstreamPath != "" {
		endpoint = shared.EndpointLabelFromPath(ctx.DownstreamPath)
	}
	shared.ObserveProxyOutcome(shared.ProxyObservation{
		Endpoint: endpoint,
		Status:   status,
		Stream:   stream,
		Latency:  latency,
	})
}

// dispatchSelectedUpstream runs steps 7-9 for one selected channel.
// finished=true means the response was written (success or terminal error).
// finished=false means the caller should continue the retry loop (optionally
// with nextPending as the last soft failure to surface if selection ends).

// Chat-family surfaces iterate multi-protocol endpoint candidates with observed
// first-byte timeout. Failures that fall through to
// the next protocol candidate do not record channel failure so healthy siblings
// are not poisoned by a single protocol miss.
func dispatchSelectedUpstream(
	w http.ResponseWriter,
	r *http.Request,
	ctx *Ctx,
	cfg *UpstreamConfig,
	selected *routing.SelectedChannel,
	upstreamPath string,
	retry int,
	maxRetries int,
	requestID string,
) (finished bool, nextPending *pendingUpstreamFailure) {
	if requestID == "" {
		requestID = proxy.RequestIDFromContext(r.Context())
	}
	// Optional max_tokens / max_output_tokens / generationConfig.maxOutputTokens
	// vs route context_length (known limitation).
	// Enforce on OpenAI chat/completions (+ legacy completions), Anthropic
	// /v1/messages, OpenAI /v1/responses (+ /compact), and Gemini
	// generateContent / streamGenerateContent when the selected route publishes
	// a positive context_length and the body includes a parseable token cap
	// above that limit. Never silent-clamp.
	if ctx != nil && selected != nil && shouldEnforceMaxTokensOnPath(upstreamPath) {
		if err := enforceMaxTokensAgainstContextLength(ctx.Body, selected.ContextLength); err != nil {
			writeJSONErrorWithRequest(w, http.StatusBadRequest, err.Error(), "invalid_request_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeClientError, false, 0)
			return true, nil
		}
	}
	// Step 7: Build upstream request materials
	upstreamModel := selected.ActualModel
	if upstreamModel == "" {
		upstreamModel = ctx.RequestedModel
	}
	runtimeCfg := config.Get()
	// Proxy selection: key proxy > account > site > system > direct
	// See proxy.KeyProxyPrecedence.
	proxyConfig := service.BuildPlatformProxyConfig(runtimeCfg, &selected.Account, &selected.Site)
	if ctx != nil && ctx.Auth != nil {
		proxyConfig = proxy.ApplyKeyProxyOverride(proxyConfig, ctx.Auth.ProxyURL)
	}
	firstByteTimeoutMs := int64(0)
	disableCrossProtocolFallback := false
	if runtimeCfg != nil {
		firstByteTimeoutMs = proxy.FirstByteTimeoutMs(runtimeCfg.ProxyFirstByteTimeoutSec)
		disableCrossProtocolFallback = runtimeCfg.DisableCrossProtocolFallback
	}

	contentType := "application/json"
	var bodyBytes []byte
	var err error
	if ctx.Multipart {
		// Multipart bodies are not multi-protocol rewritten; single-shot only.
		var bodyReader io.Reader
		bodyReader, contentType, err = CloneMultipartBody(r, map[string]string{"model": upstreamModel})
		if err != nil {
			slog.Warn("multipart upstream body construction failed",
				"err", err, "path", upstreamPath, "model", upstreamModel, "request_id", requestID, "retry", retry)
			writeJSONErrorWithRequest(w, 400, "Invalid multipart request body", "invalid_request_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeClientError, false, 0)
			return true, nil
		}
		if bodyReader != nil {
			bodyBytes, err = io.ReadAll(bodyReader)
			if err != nil {
				slog.Warn("multipart upstream body read failed",
					"err", err, "path", upstreamPath, "request_id", requestID, "retry", retry)
				writeJSONErrorWithRequest(w, 400, "Invalid multipart request body", "invalid_request_error", requestID)
				observeProxyTerminal(ctx, shared.OutcomeClientError, false, 0)
				return true, nil
			}
		}
		return dispatchEndpointAttempt(w, r, ctx, cfg, selected, upstreamModel, proxyConfig, upstreamPath, contentType, bodyBytes, firstByteTimeoutMs, retry, maxRetries, true, requestID)
	}
	bodyBytes = swapModelInJSON(ctx.RawBody, upstreamModel)

	// Site protocol preference: responses-only + stream.
	sitePref := proxy.DetectSiteProtocolPreferenceFromSite(
		selected.Site.Platform,
		selected.Site.URL,
		selected.Site.CustomHeaders,
	)
	if errMsg := responsesOnlyClientError(upstreamPath, bodyBytes, sitePref); errMsg != "" {
		// Clear failure for chat/messages clients when site cannot serve without transform.
		writeJSONErrorWithRequest(w, http.StatusBadRequest, errMsg, "invalid_request_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeClientError, false, 0)
		return true, nil
	}

	candidatePaths := resolveUpstreamCandidatePaths(upstreamPath, disableCrossProtocolFallback, sitePref)
	// The shared body pre-scan for the stream rewrite is deferred until a
	// candidate actually needs it, so surfaces without stream gates (embeddings,
	// images, ...) keep the zero-body-touch path.
	var streamHints bodyStreamHints
	hintsReady := false
	var lastPending *pendingUpstreamFailure
	for i, path := range candidatePaths {
		isLast := i >= len(candidatePaths)-1
		attemptBody, sanitizeErr := sanitizeUpstreamJSONBody(bodyBytes, selected.Site.Platform, path, upstreamModel)
		if sanitizeErr != nil {
			// Clear client-facing continuity error.
			writeJSONErrorWithRequest(w, http.StatusBadRequest, sanitizeErr.Error(), "invalid_request_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeClientError, false, 0)
			return true, nil
		}
		// Single-pass stream forcing + include_usage inject (replaces two full
		// map[string]any decode/re-encode rounds per candidate path).
		forceStream, injectUsage := upstreamStreamRewriteGates(selected.Site.Platform, path, sitePref)
		var forcedStream, expectStreamUsage bool
		if forceStream || injectUsage {
			if !hintsReady {
				// Shared pre-scan: all candidate paths reuse one look over the payload.
				streamHints = scanBodyStreamHints(bodyBytes)
				hintsReady = true
			}
			attemptHints := streamHints
			if !sameByteSlice(attemptBody, bodyBytes) {
				// Sanitize rewrote the body for this candidate; rescan it.
				attemptHints = scanBodyStreamHints(attemptBody)
			}
			rewritten, forced, expect, rewriteOK := rewriteUpstreamStreamFlags(attemptBody, attemptHints, forceStream, ctx.IsStream, injectUsage)
			if rewriteOK {
				attemptBody, forcedStream, expectStreamUsage = rewritten, forced, expect
			} else {
				// Irregular body: keep exact legacy decode semantics.
				attemptBody, forcedStream = applyUpstreamStreamPreference(attemptBody, selected.Site.Platform, path, sitePref)
				attemptBody, expectStreamUsage = applyUpstreamStreamIncludeUsage(attemptBody, selected.Site.Platform, path, ctx.IsStream || forcedStream)
			}
		}
		effectiveStream := ctx.IsStream || forcedStream
		finished, pending, cont := dispatchEndpointAttemptWithContinue(
			w, r, ctx, cfg, selected, upstreamModel, proxyConfig,
			path, contentType, attemptBody, firstByteTimeoutMs,
			retry, maxRetries, isLast, disableCrossProtocolFallback, effectiveStream, expectStreamUsage, requestID,
		)
		if finished {
			return true, nil
		}
		if pending != nil {
			lastPending = pending
		}
		if cont {
			// Soft protocol/timeout miss: try next candidate without channel poison.
			continue
		}
		// Channel-level retry or terminal pending for outer loop.
		return false, lastPending
	}
	if lastPending != nil {
		return false, lastPending
	}
	if retry < maxRetries {
		return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Upstream request failed", "upstream_error")
	}
	writeJSONErrorWithRequest(w, 502, "Upstream request failed", "upstream_error", requestID)
	observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, 0)
	return true, nil
}

// resolveUpstreamCandidatePaths returns ordered upstream paths for one channel attempt.
// Non chat-family paths yield the original path only.
// sitePref controls responses-only / prefer-responses ordering.
func resolveUpstreamCandidatePaths(upstreamPath string, disableCrossProtocolFallback bool, sitePref proxy.SiteProtocolPreference) []string {
	candidates := proxy.ResolveEndpointCandidatesWithOptions(upstreamPath, proxy.EndpointCandidateOptions{
		DisableCrossProtocolFallback: disableCrossProtocolFallback,
		Preference:                   sitePref,
	})
	if len(candidates) == 0 {
		return []string{upstreamPath}
	}
	paths := make([]string, 0, len(candidates))
	for _, ep := range candidates {
		if p := proxy.PathForEndpoint(ep); p != "" {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return []string{upstreamPath}
	}
	return paths
}

// responsesOnlyClientError returns a clear client message when a chat/messages
// shaped request hits a responses-only site (no heavy protocol transform in currently).
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
		retry, maxRetries, true, true, ctx != nil && ctx.IsStream, false, requestID,
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
) (finished bool, nextPending *pendingUpstreamFailure, cont bool) {
	if requestID == "" {
		requestID = proxy.RequestIDFromContext(r.Context())
	}
	upstreamURL := proxy.BuildUpstreamURL(selected.Site.URL, upstreamPath)
	startedAt := time.Now()

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytesReader(bodyBytes))
	if err != nil {
		slog.Warn("upstream request construction failed",
			"err", err, "url", upstreamURL, "model", upstreamModel,
			"request_id", requestID, "retry", retry)
		if !isLastEndpoint && !disableCrossProtocolFallback {
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
	// Custom headers first (deny-list skips Authorization/Host/hop-by-hop), then
	// Bearer so site custom_headers can never override the selected token.
	applyProxyCustomHeaders(req, proxyConfig)
	if selected.TokenValue != "" {
		req.Header.Set("Authorization", "Bearer "+selected.TokenValue)
	}

	resp, err := sendUpstreamRequest(cfg, req, proxyConfig, firstByteTimeoutMs)
	latencyMs := time.Since(startedAt).Milliseconds()

	if err != nil {
		// First-byte timeout: continue to next protocol when allowed; do not poison.
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
			if !isLastEndpoint && !disableCrossProtocolFallback {
				return false, nil, true
			}
			// Terminal for this channel attempt.
			errText := err.Error()
			recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, 0, errText)
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, http.StatusRequestTimeout, effectiveStream, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, errText)
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
		if !isLastEndpoint && !disableCrossProtocolFallback {
			// Network error may still be protocol-local; allow next endpoint without poison.
			return false, nil, true
		}
		errText := err.Error()
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, 0, errText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, http.StatusBadGateway, effectiveStream, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, errText)
		if retry < maxRetries {
			return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Upstream request failed", "upstream_error"), false
		}
		writeJSONErrorWithRequest(w, 502, "Upstream request failed", "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, effectiveStream, time.Since(startedAt))
		return true, nil, false
	}

	// Step 9: Handle response
	if effectiveStream {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, readErr := proxy.ReadBufferedResponseBody(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				slog.Warn("failed to read upstream stream error response",
					"err", readErr, "latency_ms", latencyMs, "status", resp.StatusCode,
					"request_id", requestID, "retry", retry)
				if !isLastEndpoint && !disableCrossProtocolFallback {
					return false, nil, true
				}
				errText := readErr.Error()
				recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, http.StatusBadGateway, errText)
				writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, http.StatusBadGateway, true, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, errText)
				if retry < maxRetries {
					return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Failed to read upstream response", "upstream_error"), false
				}
				writeJSONErrorWithRequest(w, 502, "Failed to read upstream response", "upstream_error", requestID)
				observeProxyTerminal(ctx, shared.OutcomeUpstreamError, true, time.Duration(latencyMs)*time.Millisecond)
				return true, nil, false
			}
			rawErrText := string(respBody)
			if shouldContinueEndpointFallback(resp.StatusCode, rawErrText, isLastEndpoint, disableCrossProtocolFallback) {
				return false, nil, true
			}
			// Best-effort usage from error JSON bodies (some gateways still include usage).
			failUsage := ParseUsageFromBody(respBody)
			recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, resp.StatusCode, rawErrText)
			writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, resp.StatusCode, true, failUsage, retry, requestID, truncateErrText(rawErrText))
			if retry < maxRetries && proxy.ShouldRetryProxyRequest(resp.StatusCode, rawErrText) {
				return false, bufferedPendingUpstreamFailure(resp, respBody), false
			}
			relayBufferedUpstreamErrorResponse(w, resp, respBody)
			observeProxyTerminal(ctx, shared.StatusFromHTTP(resp.StatusCode), true, time.Duration(latencyMs)*time.Millisecond)
			return true, nil, false
		}
		// Always close the upstream body, including early client disconnects.
		var streamUsage ParsedUsage
		func() {
			defer resp.Body.Close()
			streamUsage = handleStreamUpstream(w, r, resp, latencyMs)
		}()
		// Observability only: include_usage was on the outbound body but SSE had no usable tokens.
		// Still record zeros / unknown — never invent tokens.
		if expectStreamUsage {
			warnMissingStreamUsageAfterIncludeUsage(upstreamModel, upstreamPath, streamUsage)
		}
		recordUpstreamSuccess(r.Context(), cfg, selected, ctx.RequestedModel, upstreamModel, latencyMs, streamUsage)
		writeSuccessProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, resp.StatusCode, true, streamUsage, retry, requestID)
		observeProxyTerminal(ctx, shared.OutcomeSuccess, true, time.Duration(latencyMs)*time.Millisecond)
		return true, nil, false
	}

	respBody, readErr := proxy.ReadBufferedResponseBody(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		slog.Warn("failed to read upstream response",
			"err", readErr, "latency_ms", latencyMs, "channel_id", selected.Channel.ID,
			"request_id", requestID, "retry", retry)
		if !isLastEndpoint && !disableCrossProtocolFallback {
			return false, nil, true
		}
		errText := readErr.Error()
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, http.StatusBadGateway, errText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, http.StatusBadGateway, false, ParsedUsage{Source: usageSourceUnknown}, retry, requestID, errText)
		if retry < maxRetries {
			return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Failed to read upstream response", "upstream_error"), false
		}
		writeJSONErrorWithRequest(w, 502, "Failed to read upstream response", "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, time.Duration(latencyMs)*time.Millisecond)
		return true, nil, false
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawErrText := string(respBody)
		if shouldContinueEndpointFallback(resp.StatusCode, rawErrText, isLastEndpoint, disableCrossProtocolFallback) {
			return false, nil, true
		}
		// Non-stream HTTP errors: retain any usage object in the error body
		// (measurable under-count after disconnect partial).
		failUsage := ParseUsageFromBody(respBody)
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, resp.StatusCode, rawErrText)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, resp.StatusCode, false, failUsage, retry, requestID, truncateErrText(rawErrText))
		if retry < maxRetries && proxy.ShouldRetryProxyRequest(resp.StatusCode, rawErrText) {
			return false, bufferedPendingUpstreamFailure(resp, respBody), false
		}
		relayBufferedUpstreamResponse(w, resp, respBody)
		observeProxyTerminal(ctx, shared.StatusFromHTTP(resp.StatusCode), false, time.Duration(latencyMs)*time.Millisecond)
		return true, nil, false
	}
	usage := ParseUsageFromBody(respBody)
	failure := proxy.DetectProxyFailure(string(respBody), usage.ToUsageSummary())
	if failure != nil {
		slog.Warn("content-based failure detected",
			"reason", failure.Reason,
			"status", failure.Status,
			"model", upstreamModel,
			"channel_id", selected.Channel.ID,
			"latency_ms", latencyMs,
			"request_id", requestID,
			"retry", retry,
		)
		if shouldContinueEndpointFallback(failure.Status, failure.Reason, isLastEndpoint, disableCrossProtocolFallback) {
			return false, nil, true
		}
		// Content failures often still carry real usage (keyword match / empty-
		// content edge cases with non-zero tokens). Persist failed row + tokens.
		recordUpstreamFailure(r.Context(), cfg, selected, upstreamModel, failure.Status, failure.Reason)
		writeFailureProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, failure.Status, false, usage, retry, requestID, failure.Reason)
		if retry < maxRetries && proxy.ShouldRetryProxyRequest(failure.Status, failure.Reason) {
			return false, jsonPendingUpstreamFailure(failure.Status, "Upstream returned an error response", "upstream_error"), false
		}
		writeJSONErrorWithRequest(w, failure.Status, "Upstream returned an error response", "upstream_error", requestID)
		observeProxyTerminal(ctx, shared.StatusFromHTTP(failure.Status), false, time.Duration(latencyMs)*time.Millisecond)
		return true, nil, false
	}
	recordUpstreamSuccess(r.Context(), cfg, selected, ctx.RequestedModel, upstreamModel, latencyMs, usage)
	writeSuccessProxyLog(r.Context(), cfg, selected, ctx, upstreamModel, upstreamPath, latencyMs, resp.StatusCode, false, usage, retry, requestID)
	// Videos create: map upstream id → publicId before the client sees the body.
	respBody = maybeRewriteVideosCreateResponse(ctx, selected, upstreamPath, respBody)
	relayBufferedUpstreamResponse(w, resp, respBody)
	observeProxyTerminal(ctx, shared.OutcomeSuccess, false, time.Duration(latencyMs)*time.Millisecond)
	return true, nil, false
}

func shouldContinueEndpointFallback(status int, rawErrText string, isLastEndpoint bool, disableCrossProtocolFallback bool) bool {
	if isLastEndpoint || disableCrossProtocolFallback {
		return false
	}
	if proxy.ShouldAbortSameSiteEndpointFallback(status, rawErrText) {
		return false
	}
	if proxy.ShouldDowngradeToNextEndpoint(status, rawErrText) {
		return true
	}
	// First-byte style status=0 should already be handled by error path; treat as continue.
	if status == 0 {
		return true
	}
	return false
}

type pendingUpstreamFailure struct {
	resp        *http.Response
	bodyBytes   []byte
	jsonStatus  int
	jsonMessage string
	jsonType    string
}

func bufferedPendingUpstreamFailure(resp *http.Response, bodyBytes []byte) *pendingUpstreamFailure {
	return &pendingUpstreamFailure{resp: resp, bodyBytes: bodyBytes}
}

func jsonPendingUpstreamFailure(status int, message, typ string) *pendingUpstreamFailure {
	return &pendingUpstreamFailure{
		jsonStatus:  status,
		jsonMessage: message,
		jsonType:    typ,
	}
}

func (p *pendingUpstreamFailure) outcomeStatus() string {
	if p == nil {
		return shared.OutcomeUnavailable
	}
	if p.resp != nil {
		return shared.StatusFromHTTP(p.resp.StatusCode)
	}
	return shared.StatusFromHTTP(p.jsonStatus)
}

func (p *pendingUpstreamFailure) write(w http.ResponseWriter, requestID string) {
	if p == nil {
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "No available channels", "server_error", requestID)
		return
	}
	if p.resp != nil {
		relayBufferedUpstreamResponse(w, p.resp, p.bodyBytes)
		return
	}
	status := p.jsonStatus
	if status == 0 {
		status = http.StatusBadGateway
	}
	message := p.jsonMessage
	if message == "" {
		message = "Upstream request failed"
	}
	typ := p.jsonType
	if typ == "" {
		typ = "upstream_error"
	}
	writeJSONErrorWithRequest(w, status, message, typ, requestID)
}

func applyProxyCustomHeaders(req *http.Request, proxyConfig *platform.ProxyConfig) {
	if proxyConfig == nil {
		return
	}
	// Shared deny-list: Authorization/Host/hop-by-hop/Cookie/Proxy-*/metapi control.
	// site-wins when CustomHeadersOverrideRequest; else request-wins.
	platform.ApplyCustomHeadersWithOptions(req, proxyConfig.CustomHeaders, platform.ApplyCustomHeadersOptions{
		OverrideRequest: proxyConfig.CustomHeadersOverrideRequest,
	})
	// Per-site anti-bot identity: cf_clearance cookie + browser UA override.
	platform.ApplySiteIdentity(req, proxyConfig)
}

// sendUpstreamRequest dispatches an upstream HTTP request with optional observed
// first-byte timeout. firstByteTimeoutMs is milliseconds (0 disables observation).
// Config PROXY_FIRST_BYTE_TIMEOUT_SEC is seconds; convert via proxy.FirstByteTimeoutMs.
func sendUpstreamRequest(cfg *UpstreamConfig, req *http.Request, proxyConfig *platform.ProxyConfig, firstByteTimeoutMs int64) (*http.Response, error) {
	// Executor path: DoWithObservedFirstByte owns the first-byte deadline and
	// does not cancel the body after headers arrive.
	if (proxyConfig == nil || (proxyConfig.ProxyURL == "" && !proxyConfig.InsecureSkipTLS)) && cfg != nil && cfg.Executor != nil {
		return cfg.Executor.DoWithObservedFirstByte(req.Context(), req, firstByteTimeoutMs)
	}

	if firstByteTimeoutMs <= 0 {
		if proxyConfig != nil && (proxyConfig.ProxyURL != "" || proxyConfig.InsecureSkipTLS) {
			return platform.DoWithProxy(req.Context(), req, proxyConfig)
		}
		return defaultUpstreamClient.Do(req)
	}

	// Proxy / fallback client: mirror DoWithObservedFirstByte timer semantics.
	parent := req.Context()
	reqCtx, cancelReq := context.WithCancel(parent)
	req = req.WithContext(reqCtx)
	var timedOut atomic.Bool
	timer := time.AfterFunc(time.Duration(firstByteTimeoutMs)*time.Millisecond, func() {
		timedOut.Store(true)
		cancelReq()
	})

	var (
		resp *http.Response
		err  error
	)
	if proxyConfig != nil && (proxyConfig.ProxyURL != "" || proxyConfig.InsecureSkipTLS) {
		resp, err = platform.DoWithProxy(reqCtx, req, proxyConfig)
	} else {
		resp, err = defaultUpstreamClient.Do(req)
	}
	if err != nil {
		_ = timer.Stop()
		cancelReq()
		if timedOut.Load() && parent.Err() == nil {
			return nil, proxy.ErrObservedFirstByteTimeout
		}
		return nil, err
	}
	_ = timer.Stop()
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancelReq}
	return resp, nil
}

// cancelOnCloseBody cancels the request context when the response body is closed.
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	err := b.ReadCloser.Close()
	if b.cancel != nil {
		b.cancel()
	}
	return err
}

func recordUpstreamFailure(ctx context.Context, cfg *UpstreamConfig, selected *routing.SelectedChannel, modelName string, status int, rawErrText string) {
	if cfg == nil || cfg.Router == nil || selected == nil {
		return
	}
	failureCtx := routing.SiteRuntimeFailureContext{
		ErrorText: &rawErrText,
		ModelName: &modelName,
	}
	if status > 0 {
		failureCtx.Status = &status
	}
	if err := cfg.Router.RecordFailure(ctx, selected.Channel.ID, failureCtx, nil); err != nil {
		slog.Warn("RecordFailure failed", "err", err, "channel_id", selected.Channel.ID, "model", modelName)
	}
}

// recordUpstreamSuccess feeds routing success stats. billingCostName is the
// attribution (requested/canonical) model — the same name proxy_logs and
// billing use, so a K1b redirect (canonical→actual) never skews channel cost
// accumulation; modelName (actual) stays the health-stat label.
func recordUpstreamSuccess(ctx context.Context, cfg *UpstreamConfig, selected *routing.SelectedChannel, billingCostName, modelName string, latencyMs int64, usage ParsedUsage) {
	if cfg == nil || cfg.Router == nil || selected == nil {
		return
	}
	platformName := ""
	if selected.Site.Platform != "" {
		platformName = selected.Site.Platform
	}
	billing := EstimateBillingCostFromUsage(billingCostName, platformName, usage)
	if err := cfg.Router.RecordSuccess(ctx, selected.Channel.ID, float64(latencyMs), billing.EstimatedCost, &modelName, nil); err != nil {
		slog.Warn("RecordSuccess failed", "err", err, "channel_id", selected.Channel.ID, "model", modelName)
	}
	// Soft-feed first-byte EMA: until header timing is plumbed separately, use
	// total latency as an upper bound so faster channels still score better.
	siteID := selected.Account.SiteID
	if siteID != 0 {
		routing.RecordSiteRuntimeSuccess(siteID, float64(latencyMs), &modelName, float64(latencyMs))
	}
}

func isProxyStubEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("METAPI_ENABLE_PROXY_STUB")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// writeStubResponse returns a local stub response only when explicitly enabled
// for tests or demos. Production defaults to 503 if upstream forwarding is not wired.
func writeStubResponse(w http.ResponseWriter, ctx *Ctx) {
	if ctx.IsStream {
		writeSSEHeaders(w)
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		stubChunk := func(content string, finishReason any) []byte {
			payload, err := json.Marshal(map[string]any{
				"id":      "stub-metapi-go",
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   ctx.RequestedModel,
				"choices": []any{
					map[string]any{"index": 0, "delta": map[string]any{"content": content}, "finish_reason": finishReason},
				},
			})
			if err != nil {
				return []byte(`{"error":"stub marshal failed"}`)
			}
			return payload
		}

		w.Write(sseEvent(string(stubChunk("Hello from Metapi Go (stub)", nil))))
		if flusher != nil {
			flusher.Flush()
		}
		w.Write(sseEvent(string(stubChunk("", "stop"))))
		if flusher != nil {
			flusher.Flush()
		}
		w.Write(sseDone())
		if flusher != nil {
			flusher.Flush()
		}
		return
	}

	stubResp := map[string]any{
		"id":      "stub-metapi-go",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   ctx.RequestedModel,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "Hello from Metapi Go (stub)"},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
	writeJSON(w, 200, stubResp)
}

// handleStreamUpstream relays an SSE stream from upstream to the downstream client.
// It performs raw byte passthrough for minimal latency while incrementally
// analyzing bounded SSE event state for error/empty-content detection and
// end-of-stream usage extraction (OpenAI/Anthropic/Gemini/Responses shapes).

// Disables the server-level WriteTimeout via http.ResponseController so long-running
// LLM streams (>60s) are not torn down mid-response.

// Returns best-effort ParsedUsage from SSE events (may be zero/unknown).
func relayBufferedUpstreamResponse(w http.ResponseWriter, resp *http.Response, bodyBytes []byte) {
	for k, v := range resp.Header {
		if k == "Content-Length" || k == "Transfer-Encoding" {
			continue
		}
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)
}

// swapModelInJSON rewrites the top-level "model" field of a JSON request body
// to upstreamModel for one upstream attempt. The dominant no-mapping case (the
// selected channel serves the requested model) short-circuits via an
// allocation-free top-level scan and returns the original bytes untouched; a
// real model_mapping splices only the model value span, keeping the rest of
// the payload byte-identical. Irregular bodies fall back to the legacy
// shallow re-encode (legacySwapModelInJSON).
func swapModelInJSON(bodyBytes []byte, upstreamModel string) []byte {
	if len(bodyBytes) == 0 {
		// Empty body: synthesize a minimal JSON object with only the model field.
		modelJSON, _ := json.Marshal(upstreamModel)
		return append(append([]byte(`{"model":`), modelJSON...), '}')
	}
	s, found, ok := findTopLevelValue(bodyBytes, "model")
	if ok {
		if found {
			if spanStringEquals(bodyBytes, s, upstreamModel) {
				// Zero-copy short-circuit: the body already carries the target
				// model (the no-mapping norm); no rewrite, no allocation.
				return bodyBytes
			}
			// Real model_mapping: splice only the model value, keep the rest of
			// the payload byte-identical.
			modelJSON, _ := json.Marshal(upstreamModel)
			return replaceSpan(bodyBytes, s, string(modelJSON))
		}
		// Clean object without a top-level model key (e.g. Gemini path-model
		// bodies): insert it as the first entry. The scan above proves no
		// escaped-key "model" hiding at the top level.
		openIdx, empty, bracesOK := objectBraces(bodyBytes)
		if bracesOK {
			modelJSON, _ := json.Marshal(upstreamModel)
			return insertTopLevelEntry(bodyBytes, openIdx, `"model":`+string(modelJSON), empty)
		}
	}
	// Irregular body: exact legacy shallow re-encode semantics.
	return legacySwapModelInJSON(bodyBytes, upstreamModel)
}

// legacySwapModelInJSON is the original shallow re-encode kept for irregular
// bodies the allocation-free scanner declines to edit.
func legacySwapModelInJSON(bodyBytes []byte, upstreamModel string) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		// Body is already validated in PrepareCtx; fallback to original bytes.
		return bodyBytes
	}
	modelJSON, _ := json.Marshal(upstreamModel)
	raw["model"] = json.RawMessage(modelJSON)
	result, err := json.Marshal(raw)
	if err != nil {
		return bodyBytes
	}
	return result
}

func bytesReader(b []byte) io.Reader {
	if len(b) == 0 {
		return nil
	}
	return bytes.NewReader(b)
}

func routingPolicyFromAuth(policy auth.DownstreamRoutingPolicy) routing.DownstreamRoutingPolicy {
	refs := mapAuthCredentialRefs(policy.ExcludedCredentialRefs)
	allowedRefs := mapAuthCredentialRefs(policy.AllowedCredentialRefs)

	multipliers := policy.SiteWeightMultipliers
	if multipliers == nil {
		multipliers = map[int64]float64{}
	}

	return routing.DownstreamRoutingPolicy{
		SupportedModels:        policy.SupportedModels,
		AllowedRouteIDs:        policy.AllowedRouteIDs,
		SiteWeightMultipliers:  multipliers,
		KeyWeight:              policy.KeyWeight,
		ExcludedSiteIDs:        policy.ExcludedSiteIDs,
		ExcludedCredentialRefs: refs,
		AllowedSiteIDs:         policy.AllowedSiteIDs,
		AllowedCredentialRefs:  allowedRefs,
		DenyAllWhenEmpty:       policy.DenyAllWhenEmpty,
	}
}

func mapAuthCredentialRefs(in []auth.ExcludedCredentialRef) []routing.CredentialRef {
	refs := make([]routing.CredentialRef, 0, len(in))
	for _, ref := range in {
		tokenID := int64(0)
		if ref.TokenID != nil {
			tokenID = *ref.TokenID
		}
		refs = append(refs, routing.CredentialRef{
			Kind:      string(ref.Kind),
			SiteID:    ref.SiteID,
			AccountID: ref.AccountID,
			TokenID:   tokenID,
		})
	}
	return refs
}
