package proxyhandler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// tryCodexUpstreamWSS attempts Codex platform upstream wss. Returns handled=false
// to fall through to HTTP bridge (including dial failures with zero events).
func (s *responsesWSSession) tryCodexUpstreamWSS(ctx context.Context, body map[string]any, requestModel string, preserveIncremental bool) (output []any, handled bool, err error) {
	cfg := getUpstreamConfig()
	if cfg == nil || cfg.Router == nil {
		return nil, false, nil
	}
	runtimeCfg := config.RuntimeSafe()

	policy := RoutingPolicyFromAuth(auth.EmptyDownstreamRoutingPolicy)
	if s.auth != nil {
		policy = RoutingPolicyFromAuth(s.auth.Policy)
	}
	policy.RequestedContextTokens = routing.EstimateRequestContextTokens(body)
	requestStream := true
	policy.RequestStream = &requestStream
	// Native direct Responses WS is already a downstream stream. Keep the
	// original stream fact and never permit required-stream aggregation to
	// turn this transport into an HTTP fallback.
	policy.AllowStreamAggregation = false
	policy.ResolveAssociationRequest = associationRequestFromContext(ctx)
	policy.TraceIdentity = requestTraceIdentity(ctx, s.auth, s.req.Header, requestModel)
	policy.RequiredUpstreamProtocol = store.DirectProtocolResponsesWebsocket
	selected, selErr := cfg.Router.SelectChannel(ctx, requestModel, policy)
	if selErr != nil || selected == nil {
		return nil, false, nil
	}
	if selected.Direct != nil {
		return s.sendDirectResponsesWSS(ctx, body, requestModel, cfg, selected, policy)
	}
	if runtimeCfg == nil || !runtimeCfg.CodexUpstreamWebsocketEnabled {
		return nil, false, nil
	}
	if !SelectedChannelSupportsCodexWebsocketTransport(selected.Site.Platform, selected.Account.ExtraConfig, requestModel) {
		return nil, false, nil
	}

	upstreamModel := selected.ActualModel
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = requestModel
	}
	upBody := cloneMap(body)
	upBody["model"] = upstreamModel
	upBody["stream"] = true

	upstreamURL := proxyBuildUpstreamURL(selected.Site.URL, "/v1/responses")
	if upstreamURL == "" {
		return nil, false, nil
	}

	headers := map[string]string{}
	if selected.TokenValue != "" {
		headers["Authorization"] = "Bearer " + selected.TokenValue
	}
	if s.req != nil {
		for _, h := range []string{"user-agent", "originator", "session-id", "session_id", "openai-beta", wsTurnStateHeader} {
			if v := strings.TrimSpace(s.req.Header.Get(h)); v != "" {
				headers[h] = v
			}
		}
	}
	headers[responsesWebsocketTransportHeader] = "1"
	if preserveIncremental {
		headers[responsesWebsocketModeHeader] = "incremental"
	}

	sessionKey := BuildCodexSessionResponseStoreKey(s.clientSessionID, selected.Site.ID, selected.Account.ID, selected.Channel.ID)
	if sessionKey == "" {
		sessionKey = s.clientSessionID
	}
	found := false
	for _, k := range s.codexRuntimeKeys {
		if k == sessionKey {
			found = true
			break
		}
	}
	if !found {
		s.codexRuntimeKeys = append(s.codexRuntimeKeys, sessionKey)
	}

	// Resin sticky identity for native wss reverse-proxy rewrite. The wss
	// dial bypasses BuildPlatformProxyConfig, so we pass the identity through
	// the input; sendCodexWSSessionAttempt rewrites the URL + sets
	// X-Resin-Account when Resin is enabled. Platform = cfg.ResinPlatformName
	// (fallback site.Platform); Account = acc-{selected.Account.ID}.
	resinAccount := ""
	resinPlatform := ""
	if runtimeCfg := safeConfigGet(); runtimeCfg != nil && service.ResinEnabled(runtimeCfg, &selected.Site) {
		resinAccount = service.AccountFor(&selected.Account, &selected.Site)
		resinPlatform = service.ResolveResinPlatform(runtimeCfg, &selected.Site)
		if resinAccount != "" {
			service.TouchResinLease(resinAccount)
		}
	}

	result, sendErr := SendCodexWebsocketRequest(ctx, CodexWebsocketSendInput{
		SessionID:     sessionKey,
		RequestURL:    upstreamURL,
		Headers:       headers,
		Body:          upBody,
		ResinAccount:  resinAccount,
		ResinPlatform: resinPlatform,
		ObserveAttempt: func(events []map[string]any, attemptErr error, duration time.Duration) {
			// The WS loop context carries no ProxyAuth; seed this turn's auth
			// so the proxy-log sink can settle its quota reservation.
			attemptCtx := ctx
			if s.auth != nil {
				attemptCtx = auth.WithProxyAuth(attemptCtx, s.auth)
			}
			attemptCtx = beginGatewayAttempt(attemptCtx)
			proxyCtx := &Ctx{Auth: s.auth, RequestedModel: requestModel, DownstreamPath: "/v1/responses", IsStream: true, lifecycle: requestLifecycleFromContext(ctx)}
			usage := responsesWSAttemptUsage(events)
			requestID := proxy.RequestIDFromContext(ctx)
			if attemptErr != nil {
				writeFailureProxyLog(attemptCtx, cfg, selected, proxyCtx, upstreamModel, "/v1/responses", duration.Milliseconds(), nil, 0, true, usage, 0, requestID, "upstream websocket attempt failed")
				proxyCtx.lifecycle.observe(shared.OutcomeUpstreamError, true)
			} else {
				writeSuccessProxyLog(attemptCtx, cfg, selected, proxyCtx, upstreamModel, "/v1/responses", duration.Milliseconds(), nil, 0, true, usage, 0, requestID)
				proxyCtx.lifecycle.observe(shared.OutcomeSuccess, true)
			}
		},
	})
	if sendErr != nil {
		runtimeErr, ok := sendErr.(*CodexWebsocketRuntimeError)
		if ok && !runtimeErr.Dispatched && len(runtimeErr.Events) == 0 {
			LogCodexWebsocketDialFallback(sessionKey, sendErr)
			return nil, false, nil
		}
		if ok {
			sawTerminal := false
			for _, ev := range runtimeErr.Events {
				if werr := s.writeJSON(ctx, ev); werr != nil {
					return nil, true, werr
				}
				sawTerminal = sawTerminal || isResponsesTerminalType(asJSONString(ev["type"]))
			}
			if !sawTerminal {
				status := runtimeErr.Status
				if status == 0 {
					status = 408
				}
				_ = s.writeError(ctx, status, runtimeErr.Error())
			}
			return collectResponsesOutputFromMaps(runtimeErr.Events), true, nil
		}
		return nil, true, sendErr
	}

	for _, ev := range result.Events {
		if werr := s.writeJSON(ctx, ev); werr != nil {
			return nil, true, werr
		}
	}
	return collectResponsesOutputFromMaps(result.Events), true, nil
}

// Keep native WS's existing latest reported usage snapshot, while an invalid
// counter in any frame remains an accounting fault for user budgets.
func responsesWSAttemptUsage(events []map[string]any) ParsedUsage {
	usage := ParsedUsage{Source: usageSourceUnknown}
	invalid := false
	for _, event := range events {
		data, _ := json.Marshal(event)
		parsed := ParseUsageFromBody(data)
		invalid = invalid || parsed.invalidForUserBudget
		if parsed.Found {
			usage = parsed
		}
	}
	usage.invalidForUserBudget = invalid
	return usage
}

func proxyBuildUpstreamURL(siteURL, path string) string {
	base := strings.TrimRight(strings.TrimSpace(siteURL), "/")
	if base == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
