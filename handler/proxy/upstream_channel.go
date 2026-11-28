package proxyhandler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	messages "github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

// UpstreamConfig holds the dependencies needed for upstream forwarding.
type UpstreamConfig struct {
	DB       *store.DB
	Router   proxy.TokenRouterInterface
	Executor *proxy.RuntimeExecutor
	// SiteLimiter caps concurrent dispatches per site (sites.max_concurrency).
	// When nil, proxy.DefaultSiteConcurrencyLimiter is used.
	SiteLimiter *proxy.SiteConcurrencyLimiter
	// DirectAccountLimiter caps concurrent dispatches per direct upstream
	// account (upstream_credentials.id), since direct channels have no site
	// row to carry MaxConcurrency. The limit comes from
	// PROXY_DIRECT_ACCOUNT_MAX_CONCURRENCY; 0 leaves direct dispatch
	// unlimited. When nil, proxy.DefaultDirectAccountConcurrencyLimiter is
	// used.
	DirectAccountLimiter *proxy.SiteConcurrencyLimiter
	// ChannelLimits is the shared per-channel RPM/TPM/FIFO runtime. Routing
	// must receive the same owner; nil uses the process-local default.
	ChannelLimits *channellimit.Manager
	// LogProxy persists successful/failed proxy attempts into proxy_logs.
	// When nil, defaultLogProxyWriter uses the request-scoped DB (no-op if unset).
	LogProxy                func(ctx context.Context, entry proxy.ProxyLogEntry) error
	ResolveDirectCredential func(context.Context, int64, *string, bool) (*oauth.DirectCredentialResult, error)
	ChannelPlugins          *channelplugins.Manager
}

var upstreamCfg *UpstreamConfig
var unconfiguredUpstreamLogOnce sync.Once

// SetUpstreamConfig sets the package-level upstream forwarding dependencies.
// Called during server startup to wire in the routing engine and HTTP executor.
func SetUpstreamConfig(cfg *UpstreamConfig) {
	upstreamCfg = cfg
}

// getUpstreamConfig returns the configured upstream dependencies.
func getUpstreamConfig() *UpstreamConfig {
	return upstreamCfg
}

// directAccountConcurrencyLimit returns the configured per-direct-account
// concurrency cap from the startup-loaded config singleton. <=0 (default)
// means unlimited, preserving the pre-existing direct dispatch behavior;
// tests that construct a partial config stay unlimited unless they opt in.
func directAccountConcurrencyLimit() int {
	if cfg := config.GetSafe(); cfg != nil && cfg.ProxyDirectAccountMaxConcurrency > 0 {
		return cfg.ProxyDirectAccountMaxConcurrency
	}
	return 0
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
	defer releaseDirectVideoCapture(ctx, requestID)
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
	staticCfg := config.Get()
	// Proxy selection precedence: key proxy > account > site > system > direct.
	proxyConfig := service.BuildPlatformProxyConfig(staticCfg, &selected.Account, &selected.Site)
	if selected.Direct != nil {
		proxyConfig = directProxyConfig(selected.Direct.CustomHeader, selected.Direct.ChannelProxy, selected.Direct.UseSystemProxy, r.Header)
	}
	if ctx != nil && ctx.Auth != nil {
		proxyConfig = proxy.ApplyKeyProxyOverride(proxyConfig, ctx.Auth.ProxyURL)
	}
	// Runtime-mutable knobs come from the atomic settings snapshot so a
	// live settings change applies on the very next request.
	firstByteTimeoutMs := int64(0)
	disableCrossProtocolFallback := false
	if rt := config.RuntimeSafe(); rt != nil {
		firstByteTimeoutMs = proxy.FirstByteTimeoutMs(rt.ProxyFirstByteTimeoutSec)
		disableCrossProtocolFallback = rt.DisableCrossProtocolFallback
	}
	configuredFirstByteTimeoutMs := firstByteTimeoutMs
	ctx.startRetryBudget(time.Duration(configuredFirstByteTimeoutMs) * time.Millisecond)
	firstByteTimeoutMs = ctx.remaining(configuredFirstByteTimeoutMs)

	if ctx.messagesBridgeReplayRequired && disableCrossProtocolFallback {
		writeMessagesReplayFailure(w, ctx, requestID)
		return true, nil
	}

	contentType := "application/json"
	if proxy.IsDirectMediaPath(upstreamPath) && r.Header.Get("Content-Type") != "" {
		contentType = r.Header.Get("Content-Type")
	}
	var bodyBytes []byte
	var err error
	if ctx.Multipart {
		ctx.directProviderUpload = nil
		if hasDirectMultipartDriver(selected.Direct, upstreamPath) {
			ctx.directProviderUpload, bodyBytes, err = prepareDirectProviderUpload(r, selected.Direct, upstreamModel)
			if err != nil {
				writeJSONErrorWithRequest(w, 400, "Invalid provider multipart metadata", "invalid_request_error", requestID)
				return true, nil
			}
			return dispatchDirectEndpoint(w, r, ctx, cfg, selected, upstreamModel, proxyConfig, bodyBytes, contentType, firstByteTimeoutMs, configuredFirstByteTimeoutMs, retry, maxRetries, disableCrossProtocolFallback, requestID)
		}
		// Multipart bodies are not multi-protocol rewritten; single-shot only.
		var bodyReader io.Reader
		overrides := map[string]string{"model": upstreamModel}
		if selected.Direct != nil {
			overrides, err = directMultipartOverrides(upstreamModel, selected.Direct.ParamOverride)
			if err != nil {
				writeJSONErrorWithRequest(w, 400, err.Error(), "invalid_request_error", requestID)
				return true, nil
			}
		}
		bodyReader, contentType, err = CloneMultipartBody(r, overrides)
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
		if selected.Direct != nil {
			return dispatchDirectEndpoint(w, r, ctx, cfg, selected, upstreamModel, proxyConfig, bodyBytes, contentType, firstByteTimeoutMs, configuredFirstByteTimeoutMs, retry, maxRetries, disableCrossProtocolFallback, requestID)
		}
		return dispatchEndpointAttempt(w, r, ctx, cfg, selected, upstreamModel, proxyConfig, upstreamPath, contentType, bodyBytes, firstByteTimeoutMs, retry, maxRetries, true, requestID)
	}
	bodyBytes = swapModelInJSON(ctx.RawBody, upstreamModel)
	if bit := proxy.DirectProtocolForPath(upstreamPath); bit == store.DirectProtocolGemini || bit == store.DirectProtocolGeminiEmbeddings {
		// Native Gemini carries its model in the URL, not the request body.
		if _, found, valid := findTopLevelValue(ctx.RawBody, "model"); valid && !found {
			bodyBytes = ctx.RawBody
		}
	}
	if mediaRequestPreservesBodyModel(upstreamPath, r.Method) {
		bodyBytes = ctx.RawBody
	}
	if selected.Direct != nil {
		return dispatchDirectEndpoint(w, r, ctx, cfg, selected, upstreamModel, proxyConfig, bodyBytes, contentType, firstByteTimeoutMs, configuredFirstByteTimeoutMs, retry, maxRetries, disableCrossProtocolFallback, requestID)
	}

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
	if ctx.messagesBridgeReplayRequired && (len(candidatePaths) < 2 || !isMessagesChatBridge(upstreamPath, candidatePaths[1])) {
		writeMessagesReplayFailure(w, ctx, requestID)
		return true, nil
	}
	var chatBody []byte
	var bridgeOptions messages.Options
	if len(candidatePaths) > 1 && isMessagesChatBridge(upstreamPath, candidatePaths[1]) {
		// Check fallback eligibility without modifying the native payload. If a
		// Messages feature has no lossless Chat representation, keep the native
		// endpoint terminal so its real failure is recorded, not silently skipped.
		bridge := newMessagesBridgeRequest(r, ctx, selected)
		if bridge.ready() == nil {
			bridgeOptions = bridge.Options()
		} else if ctx.messagesBridgeReplayRequired {
			writeMessagesReplayFailure(w, ctx, requestID)
			return true, nil
		}
		// Stateless text/tools remain usable without a session; a response that
		// needs hidden reasoning cannot silently discard it when callbacks are absent.
		chatBody, err = messages.ToChatRequest(bodyBytes, bridgeOptions)
		if err != nil {
			if ctx.messagesBridgeReplayRequired || bridge.UsedReplay() {
				writeMessagesReplayFailure(w, ctx, requestID)
				return true, nil
			}
			slog.Debug("Messages request requires a native upstream", "reason", err, "request_id", requestID)
			candidatePaths = candidatePaths[:1]
		} else if ctx.messagesBridgeReplayRequired && !bridge.UsedReplay() {
			writeMessagesReplayFailure(w, ctx, requestID)
			return true, nil
		} else if bridge.UsedReplay() {
			// A previous Chat reply deliberately kept its reasoning off the Messages
			// wire. Send the restored continuation only through its matching Chat bridge.
			candidatePaths = candidatePaths[1:]
		}
	}
	// The shared body pre-scan for the stream rewrite is deferred until a
	// candidate actually needs it, so surfaces without stream gates (embeddings,
	// images, ...) keep the zero-body-touch path.
	var streamHints bodyStreamHints
	hintsReady := false
	var lastPending *pendingUpstreamFailure
	for i, path := range candidatePaths {
		isLast := i >= len(candidatePaths)-1
		candidateBody := bodyBytes
		if isMessagesChatBridge(upstreamPath, path) {
			candidateBody = chatBody
		}
		upstreamPlatform := selected.Site.Platform
		attemptBody, sanitizeErr := sanitizeUpstreamJSONBody(candidateBody, upstreamPlatform, path, upstreamModel)
		if sanitizeErr == nil {
			attemptBody, sanitizeErr = upstreamadapter.PrepareDomesticChatRequest(attemptBody, nativeChatRequestProfile(selected.Site, path))
		}
		if sanitizeErr != nil {
			// Clear client-facing continuity error.
			writeJSONErrorWithRequest(w, http.StatusBadRequest, sanitizeErr.Error(), "invalid_request_error", requestID)
			observeProxyTerminal(ctx, shared.OutcomeClientError, false, 0)
			return true, nil
		}
		// Single-pass stream forcing + include_usage inject (replaces two full
		// map[string]any decode/re-encode rounds per candidate path).
		forceStream, injectUsage := upstreamStreamRewriteGates(upstreamPlatform, path, sitePref)
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
				attemptBody, forcedStream = applyUpstreamStreamPreference(attemptBody, upstreamPlatform, path, sitePref)
				attemptBody, expectStreamUsage = applyUpstreamStreamIncludeUsage(attemptBody, upstreamPlatform, path, ctx.IsStream || forcedStream)
			}
		}
		effectiveStream := ctx.IsStream || forcedStream
		finished, pending, cont := dispatchEndpointAttemptWithContinue(
			w, r, ctx, cfg, selected, upstreamModel, proxyConfig,
			path, contentType, attemptBody, firstByteTimeoutMs,
			retry, maxRetries, isLast, disableCrossProtocolFallback, effectiveStream, expectStreamUsage, requestID, bridgeOptions,
		)
		if finished {
			return true, nil
		}
		if pending != nil {
			lastPending = pending
		}
		if cont {
			// Transport errors keep sharing the remaining header budget; HTTP
			// answers and timeout-authorized protocol fallbacks clear the prior
			// deadline first so the fallback endpoint gets a real header wait.
			ctx.startRetryBudget(time.Duration(configuredFirstByteTimeoutMs) * time.Millisecond)
			firstByteTimeoutMs = ctx.remaining(configuredFirstByteTimeoutMs)
			continue
		}
		// Channel-level retry or terminal pending for outer loop.
		return false, lastPending
	}
	if lastPending != nil {
		lastPending.recordFirstFailure(ctx)
		return false, lastPending
	}
	if retry < maxRetries {
		return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "Upstream request failed", "upstream_error")
	}
	writeJSONErrorWithRequest(w, 502, "Upstream request failed", "upstream_error", requestID)
	observeProxyTerminal(ctx, shared.OutcomeUpstreamError, false, 0)
	return true, nil
}

// resolveUpstreamCandidatePaths admits only protocols this dispatcher can serve.
// Preference cannot manufacture a missing request/response converter. Native
// stays first; only Messages -> Chat has a complete JSON/SSE return bridge.
func resolveUpstreamCandidatePaths(upstreamPath string, disableCrossProtocolFallback bool, sitePref proxy.SiteProtocolPreference) []string {
	if strings.HasSuffix(strings.TrimRight(strings.Split(upstreamPath, "?")[0], "/"), "/count_tokens") {
		return []string{upstreamPath}
	}
	primary, ok := proxy.EndpointFromPath(upstreamPath)
	if !ok {
		return []string{upstreamPath}
	}
	if primary == proxy.EndpointGemini {
		return []string{upstreamPath}
	}
	if sitePref.ResponsesOnly {
		// responsesOnlyClientError already rejected native Chat/Messages bodies.
		// A Responses-shaped body at a legacy alias needs only this path rewrite.
		return []string{proxy.PathForEndpoint(proxy.EndpointResponses)}
	}
	paths := []string{proxy.PathForEndpoint(primary)}
	if primary == proxy.EndpointMessages && !disableCrossProtocolFallback {
		paths = append(paths, proxy.PathForEndpoint(proxy.EndpointChat))
	}
	return paths
}

func proxyPathIsMessages(path string) bool {
	endpoint, ok := proxy.EndpointFromPath(path)
	return ok && endpoint == proxy.EndpointMessages
}

// RoutingPolicyFromAuth converts the auth-layer downstream policy into the
// routing-layer policy by delegating to the shared proxy package owner.
func RoutingPolicyFromAuth(policy auth.DownstreamRoutingPolicy) routing.DownstreamRoutingPolicy {
	return proxy.RoutingPolicyFromAuth(policy)
}
