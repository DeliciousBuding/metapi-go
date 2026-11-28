package proxyhandler

import (
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func dispatchDirectEndpoint(w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig, selected *routing.SelectedChannel, model string, proxyConfig *platform.ProxyConfig, body []byte, contentType string, firstByteTimeoutMs, configuredFirstByteTimeoutMs int64, retry, maxRetries int, disableCrossProtocolFallback bool, requestID string) (bool, *pendingUpstreamFailure) {
	openCode := selected.Direct.Provider == "opencode_go" || selected.Direct.Provider == "opencode_go_anthropic"
	if openCode && ctx.directOpenCodeSession == "" {
		ctx.directOpenCodeSession = directOpenCodeSessionID(r.Header, ctx.ClientCtx.SessionID)
	}
	if ctx.DownstreamPath == "" {
		copyCtx := *ctx
		copyCtx.DownstreamPath = r.URL.Path
		ctx = &copyCtx
	}
	if selected.Direct.CredentialKind == store.DirectCredentialNone {
		copySelected, copyDirect := *selected, *selected.Direct
		copyDirect.Protocols &= copyDirect.Endpoints.AnonymousProtocolMask()
		if !store.DirectProviderAllowsAnonymous(copyDirect.Provider) {
			copyDirect.Protocols = 0
		}
		copySelected.Direct = &copyDirect
		selected = &copySelected
	}
	var err error
	if err := selected.Direct.RequestConfig.ValidateEndpoints(selected.Direct.Endpoints); err != nil {
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "Selected channel request operations are incompatible with its endpoints", "upstream_error", requestID)
		return true, nil
	}
	selected, err = pinDirectVideoTaskCandidate(ctx, selected)
	if err != nil {
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, err.Error(), "upstream_error", requestID)
		return true, nil
	}
	upstreamStream := ctx.IsStream || selected.Direct.StreamPolicy == store.DirectStreamRequire
	paths, err := directCandidatePaths(selected.Direct, ctx.DownstreamPath, model, upstreamStream)
	if err != nil {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, err.Error(), "invalid_request_error", requestID)
		return true, nil
	}
	if disableCrossProtocolFallback && len(paths) > 1 {
		paths = paths[:1]
	}
	if ctx.messagesBridgeReplayRequired {
		var bridged []string
		for _, path := range paths {
			if directBridgeNeeded(ctx.DownstreamPath, path) {
				bridged = append(bridged, path)
			}
		}
		paths = bridged
		if len(paths) == 0 {
			writeMessagesReplayFailure(w, ctx, requestID)
			return true, nil
		}
	}
	for i, path := range paths {
		finished, pending, cont := dispatchDirectEndpointAttempt(w, r, ctx, cfg, selected, model, proxyConfig, path, body, contentType, firstByteTimeoutMs, retry, maxRetries, i == len(paths)-1, requestID)
		if finished || !cont {
			return finished, pending
		}
		// Match the site-channel candidate loop: refresh the shared first-byte
		// retry budget after a fallback-authorized response so the next protocol
		// gets a fresh bounded window (never leaving deadline zeroed or reusing
		// a stale clamped remainder).
		ctx.startRetryBudget(time.Duration(configuredFirstByteTimeoutMs) * time.Millisecond)
		firstByteTimeoutMs = ctx.remaining(configuredFirstByteTimeoutMs)
	}
	return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "No compatible direct endpoint", "upstream_error")
}
