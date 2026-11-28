package proxyhandler

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/alert"
	"github.com/deliciousbuding/metapi-go/store"
)

// dispatchUpstream forwards a proxy request to the selected upstream channel.
// Implements the spec's 10-step Handler pattern.
func dispatchUpstream(w http.ResponseWriter, r *http.Request, ctx *Ctx) {
	if ctx.lifecycle != nil {
		ctx.lifecycle.bind(ctx.Auth, ctx.DownstreamPath, ctx.RequestedModel)
		ctx.lifecycle.mu.Lock()
		ctx.lifecycle.dispatched = true
		ctx.lifecycle.mu.Unlock()
	}
	r = withDownstreamResponseModel(r, ctx)
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
		if ctx.lifecycle != nil {
			ctx.lifecycle.mu.Lock()
			ctx.lifecycle.gatewayFailure = "upstream_not_configured"
			ctx.lifecycle.mu.Unlock()
		}
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "Proxy upstream is not configured", "server_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeUnavailable, ctx != nil && ctx.IsStream, time.Since(startedAt))
		return
	}

	ctx.retryBudget = &upstreamRetryBudget{}
	maxRetries := ctx.MaxRetries
	downstreamPolicy := RoutingPolicyFromAuth(ctx.Policy)
	requestStream := ctx.IsStream
	downstreamPolicy.RequestStream = &requestStream
	downstreamPolicy.TraceIdentity = requestTraceIdentity(r.Context(), ctx.Auth, r.Header, ctx.RequestedModel)
	// multi-tier: best-effort request context estimate for route tier pick.
	// ctx is constructed non-nil by PrepareCtx (which returns a SurfResult on
	// failure instead of a nil Ctx), so it is safe to dereference unguarded.
	downstreamPolicy.RequestedContextTokens = routing.EstimateRequestContextTokens(ctx.Body)
	upstreamPath := ctx.DownstreamPath
	if upstreamPath == "" {
		upstreamPath = r.URL.Path
	}
	downstreamPolicy.RequiredUpstreamProtocol, downstreamPolicy.AllowUpstreamProtocolConversion = proxy.RoutingProtocolForPath(upstreamPath)
	downstreamPolicy.AllowStreamAggregation = directStreamAggregateProtocol(upstreamPath) != "" && !strings.HasSuffix(upstreamPath, "/count_tokens")
	downstreamPolicy.ResolveAssociationRequest = associationRequestFromContext(r.Context())
	if downstreamPolicy.ResolveAssociationRequest == nil {
		downstreamPolicy.ResolveAssociationRequest = lazyAssociationRequest(upstreamPath, ctx.RawBody, r.Header, ctx.IsStream, time.Now())
	}
	ctx.dispatchPolicy = &downstreamPolicy
	// Never forward a downstream path that can escape the selected site's API
	// prefix. Go's http.Client preserves ".." on the wire, and the upstream host
	// would normalize it outside the prefix — letting any authenticated
	// downstream key holder reach arbitrary upstream paths. chi hands back the
	// raw segment when RawPath is set, so the guard also decodes encoded
	// separators/dots and re-checks the files/videos resource prefixes after
	// path.Clean. Traversal-free paths pass through byte-for-byte.
	if !validUpstreamDispatchPath(upstreamPath) {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, "Invalid request path", "invalid_request_error", requestID)
		observeProxyTerminal(ctx, shared.OutcomeClientError, ctx != nil && ctx.IsStream, time.Since(startedAt))
		return
	}
	if endpoint, ok := proxy.EndpointFromPath(upstreamPath); ok && endpoint == proxy.EndpointMessages {
		channelID, required, err := messagesBridgePreferredChannel(ctx)
		ctx.messagesBridgeReplayRequired = required
		if err != nil || (required && (channelID == nil || *channelID == 0)) {
			writeMessagesReplayFailure(w, ctx, requestID)
			return
		}
		if required {
			if ctx.ForcedChannelID != nil && *ctx.ForcedChannelID != 0 && *ctx.ForcedChannelID != *channelID {
				writeMessagesReplayFailure(w, ctx, requestID)
				return
			}
			// This is not an authorization bypass: the normal preferred-channel
			// selector still applies current downstream policy and availability.
			ctx.ForcedChannelID = channelID
			maxRetries = 0
		}
	}
	dispatchChannelAttempts(w, r, ctx, cfg, upstreamPath, maxRetries, requestID, startedAt)
}

func resolveUpstreamDB(ctx *Ctx, cfg *UpstreamConfig) *store.DB {
	if ctx != nil && ctx.db != nil {
		return ctx.db
	}
	if cfg != nil {
		return cfg.DB
	}
	return nil
}

// reportProxyAllFailedTerminal reports a terminal all-channels-failed outcome
// so operators get an event + notification instead of a silent 5xx. Dedup is
// handled inside alert.ReportProxyAllFailed (per-model cooldown) so a dead
// upstream cannot spam on every request. No-op when the DB is not wired.
func reportProxyAllFailedTerminal(ctx *Ctx, cfg *UpstreamConfig, reason string) {
	model := ""
	if ctx != nil {
		model = ctx.RequestedModel
	}
	reportProxyAllFailed(resolveUpstreamDB(ctx, cfg), model, reason)
}

func reportProxyAllFailed(db *store.DB, model, reason string) {
	if db == nil {
		return
	}
	alert.ReportProxyAllFailed(config.RuntimeSafe(), db.DB, alert.ProxyAllFailedParams{Model: model, Reason: reason})
}

func selectedHealthAccountID(selected *routing.SelectedChannel) *int64 {
	if selected.Direct != nil || selected.Account.ID <= 0 {
		return nil
	}
	return &selected.Account.ID
}
