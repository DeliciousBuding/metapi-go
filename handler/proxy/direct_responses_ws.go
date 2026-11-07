package proxyhandler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

type acceptedNativeResponsesIncompleteKey struct{}

func (s *responsesWSSession) sendDirectResponsesWSS(ctx context.Context, body map[string]any, model string, cfg *UpstreamConfig, selected *routing.SelectedChannel, policy routing.DownstreamRoutingPolicy) ([]any, bool, error) {
	reject := func(status int, message string) ([]any, bool, error) {
		if s.directRuntimeKey != "" {
			CloseCodexWebsocketSession(s.directRuntimeKey)
			clearCodexSessionResponseID(s.directRuntimeKey)
			s.directRuntimeKey = ""
		}
		return nil, true, s.writeError(ctx, status, message)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return reject(400, "Invalid Responses request")
	}
	request := s.req.WithContext(auth.WithProxyAuth(ctx, s.auth))
	proxyCtx := &Ctx{Auth: s.auth, Body: body, RawBody: raw, RequestedModel: model, DownstreamPath: "/v1/responses", IsStream: true, lifecycle: requestLifecycleFromContext(ctx)}
	if s.auth != nil {
		proxyCtx.Policy = s.auth.Policy
	}
	permit, admission, err := acquireChannelAdmission(ctx, cfg, selected)
	if err != nil {
		return reject(503, "Selected channel is unavailable")
	}
	defer permit.Release()
	// Every WS turn rechecks the persisted graph, even when the route cache
	// selected it without waiting. A socket never extends a revoked grant.
	selected, status, err := revalidateQueuedChannel(request, proxyCtx, cfg, selected, &policy)
	if err != nil {
		return reject(status, err.Error())
	}
	s.auth = proxyCtx.Auth
	request = request.WithContext(auth.WithProxyAuth(ctx, s.auth))
	if selected.Direct.Endpoints.ResponsesWebsocket == nil {
		return reject(503, "Selected channel has no WebSocket endpoint")
	}
	endpoint := selected.Direct.Endpoints.ResponsesWebsocket
	limiter := cfg.DirectAccountLimiter
	if limiter == nil {
		limiter = proxy.DefaultDirectAccountConcurrencyLimiter
	}
	slot, ok := limiter.TryAcquire(selected.Direct.CredentialID, int64(directAccountConcurrencyLimit()))
	if !ok {
		return reject(503, "Selected credential is busy")
	}
	defer slot.Release()
	proxyConfig := directProxyConfig(selected.Direct.CustomHeader, selected.Direct.ChannelProxy, selected.Direct.UseSystemProxy, request.Header)
	if s.auth != nil {
		proxyConfig = proxy.ApplyKeyProxyOverride(proxyConfig, s.auth.ProxyURL)
	}
	if cfg.ResolveDirectCredential == nil {
		return reject(503, "Selected direct credential is unavailable")
	}
	var proxyURL *string
	if proxyConfig != nil && proxyConfig.ProxyURL != "" {
		proxyURL = &proxyConfig.ProxyURL
	}
	credential, err := cfg.ResolveDirectCredential(ctx, selected.Direct.CredentialID, proxyURL, false)
	if err != nil || credential == nil || credential.AccessToken == "" || (credential.Kind != store.DirectCredentialAPIKey && credential.Kind != store.DirectCredentialOAuth) {
		return reject(503, "Selected direct credential is unavailable")
	}
	if status, err := revalidateDirectCredentialDispatch(request, proxyCtx, cfg, selected, &policy); err != nil {
		return reject(status, err.Error())
	}
	s.auth = proxyCtx.Auth
	request = request.WithContext(auth.WithProxyAuth(ctx, s.auth))
	actualModel := selected.ActualModel
	if actualModel == "" {
		actualModel = model
	}
	wire, err := prepareDirectResponsesWebsocketWire(ctx, cfg, endpoint, selected.Direct, credential, raw, request.Header, downstreamSessionNamespace(s.auth), model, actualModel)
	if err != nil {
		if len(selected.Direct.RequestConfig.Plugins) > 0 || selected.Direct.RequestConfig.Provider != nil && slices.Contains(selected.Direct.RequestConfig.Provider.Protocols, "responsesWebsocket") {
			return reject(channelPluginRequestStatus(err), "Selected channel cannot execute this WebSocket request plugin")
		}
		return reject(400, "Selected channel cannot prepare this WebSocket request")
	}
	if selected.Direct.RequestConfig.Active() && s.auth != nil && s.auth.OwnerUserID != nil &&
		!directUserOperationsWithinBudget(swapModelInJSON(raw, actualModel), wire.Body, "/v1/responses", proxyCtx.lifecycle, false) {
		return reject(403, "Selected channel request operations exceed the admitted user request")
	}
	endpointURL := endpoint.URL
	if wire.DriverURL != "" {
		endpointURL = wire.DriverURL
	}
	upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return reject(503, "Invalid selected WebSocket endpoint")
	}
	applyProxyCustomHeaders(upstreamRequest, proxyConfig)
	applyClientProtocolHeaders(upstreamRequest, request.Header, "/v1/responses")
	applyDirectProviderHeaders(upstreamRequest, wire)
	for _, name := range []string{"X-Api-Key", "X-Goog-Api-Key", "Cookie", "Proxy-Authorization"} {
		upstreamRequest.Header.Del(name)
	}
	upstreamRequest.Header.Set("Authorization", "Bearer "+credential.AccessToken)
	if err := applyDirectRequestHeaderOperations(upstreamRequest, selected.Direct.ChannelID, wire); err != nil {
		return reject(400, "Cannot apply selected channel request headers")
	}
	if wire.Driver != nil {
		lease, err := wire.Driver.ApplySessionAuthentication(upstreamRequest.Header, credential.AccessToken, s.directAuthentication)
		s.directAuthentication = lease
		if err != nil {
			return reject(503, "Selected provider authentication is unavailable")
		}
	} else {
		s.directAuthentication = nil
	}
	// HTTP transport policy also owns explicit proxies and SSRF guarded
	// resolution. Custom headers have already been applied in order.
	client, err := platform.WebsocketHTTPClient(proxyConfig)
	if err != nil {
		return reject(503, "Invalid selected WebSocket proxy")
	}
	headers := map[string]string{}
	for name, values := range upstreamRequest.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	sessionKey := directResponsesSessionKey(s.clientSessionID, s.auth, selected.Direct, credential, headers, proxyConfig)
	if s.directRuntimeKey != "" && s.directRuntimeKey != sessionKey {
		CloseCodexWebsocketSession(s.directRuntimeKey)
		clearCodexSessionResponseID(s.directRuntimeKey)
	}
	s.directRuntimeKey = sessionKey
	var requestBody map[string]any
	decoder := json.NewDecoder(bytes.NewReader(wire.Body))
	decoder.UseNumber()
	if decoder.Decode(&requestBody) != nil {
		return reject(400, "Invalid prepared Responses request")
	}
	// WebSocket uses an event envelope; SSE/HTTP-only controls are omitted.
	delete(requestBody, "stream")
	delete(requestBody, "background")
	var attemptCtx context.Context
	var projectedEvents []map[string]any
	var pluginErr error
	rawDriverUsage := ParsedUsage{Source: usageSourceUnknown}
	var decodeEvent func([]byte) ([]byte, error)
	if wire.Driver != nil {
		decodeEvent = func(raw []byte) ([]byte, error) {
			native, observed, err := decodeDirectDriverPayload(ctx, wire, raw, true, http.StatusOK)
			rawDriverUsage = mergeUsagePreferLater(rawDriverUsage, observed)
			return native, err
		}
	}
	selectedCopy := *selected
	selectedCopy.TokenValue = credential.AccessToken
	selected = &selectedCopy
	_, sendErr := SendCodexWebsocketRequest(ctx, CodexWebsocketSendInput{
		SessionID: sessionKey, RequestURL: endpointURL, Headers: headers, Body: requestBody, HTTPClient: client,
		DecodeEvent: decodeEvent,
		// A direct grant has no authority to replay a request after the peer
		// accepted its event. The old site-specific recovery remains separate.
		DisablePreviousRecovery: true,
		AllowNativeIncomplete:   endpoint.Profile != "codex",
		BeforeAttempt: func() error {
			rateCtx := context.WithValue(auth.WithProxyAuth(ctx, s.auth), channelAdmissionKey{}, admission)
			var err error
			rateCtx, err = prepareChannelRateAttempt(rateCtx)
			if err != nil {
				return err
			}
			attemptCtx = beginGatewayAttempt(rateCtx)
			return nil
		},
		ObserveAttempt: func(events []map[string]any, attemptErr error, duration time.Duration) {
			if attemptCtx == nil {
				return
			}
			usage := responsesWSAttemptUsage(events)
			if wire.Driver != nil {
				usage = rawDriverUsage
			}
			// Account from the original native events. A pinned hook snapshot
			// projects this turn only after the transport/collector settled; hook
			// failure cannot trigger a second send or forge an upstream success.
			projectedEvents, pluginErr = projectDirectWebsocketPluginEvents(ctx, wire, events)
			if attemptErr == nil {
				attemptErr = pluginErr
			}
			if attemptErr != nil {
				status, outcome := 502, shared.OutcomeUpstreamError
				if ctx.Err() != nil {
					status, outcome = 499, shared.OutcomeClientError
				}
				writeFailureProxyLog(context.WithoutCancel(attemptCtx), cfg, selected, proxyCtx, actualModel, "/v1/responses", duration.Milliseconds(), nil, status, true, usage, 0, proxy.RequestIDFromContext(ctx), "upstream websocket attempt failed")
				if proxyCtx.lifecycle != nil {
					proxyCtx.lifecycle.observe(outcome, true)
				}
			} else {
				recordUpstreamSuccess(attemptCtx, cfg, selected, model, actualModel, duration.Milliseconds(), usage)
				writeSuccessProxyLog(attemptCtx, cfg, selected, proxyCtx, actualModel, "/v1/responses", duration.Milliseconds(), nil, 200, true, usage, 0, proxy.RequestIDFromContext(ctx))
			}
			recordChannelRateUsage(attemptCtx, usage)
		},
	})
	if pluginErr != nil {
		if ctx.Err() != nil {
			return nil, true, ctx.Err()
		}
		return reject(502, "Cannot execute selected channel WebSocket event plugin")
	}
	if sendErr != nil {
		if ctx.Err() != nil {
			return nil, true, ctx.Err()
		}
		if admission != nil && admission.denied != nil && attemptCtx == nil {
			if proxyCtx.lifecycle != nil {
				proxyCtx.lifecycle.observe(shared.OutcomeUnavailable, true)
			}
			return reject(503, "Selected channel request limit is exhausted")
		}
		runtimeErr, ok := sendErr.(*CodexWebsocketRuntimeError)
		// A refused upgrade sent no generation event. HTTP fallback performs
		// a fresh, independent grant selection and cannot borrow the WS bit.
		if ok && !runtimeErr.Dispatched && len(runtimeErr.Events) == 0 && attemptCtx == nil {
			CloseCodexWebsocketSession(sessionKey)
			clearCodexSessionResponseID(sessionKey)
			s.directRuntimeKey = ""
			return nil, false, nil
		}
		if runtimeErr != nil {
			for _, event := range projectedEvents {
				if err := s.writeDirectResponsesEvent(ctx, event, model); err != nil {
					return nil, true, err
				}
			}
			for _, event := range projectedEvents {
				if isTerminalResponsesEvent(event) {
					return collectResponsesOutputFromMaps(projectedEvents), true, nil
				}
			}
		}
		if proxyCtx.lifecycle != nil {
			proxyCtx.lifecycle.observe(shared.OutcomeUpstreamError, true)
		}
		return reject(502, "Upstream WebSocket request failed")
	}
	for _, event := range projectedEvents {
		writeCtx := ctx
		// A successful runtime result has already passed the shared collector's
		// native terminal validation. This only classifies downstream lifecycle.
		if endpoint.Profile != "codex" && event["type"] == "response.incomplete" {
			writeCtx = context.WithValue(ctx, acceptedNativeResponsesIncompleteKey{}, true)
		}
		if err := s.writeDirectResponsesEvent(writeCtx, event, model); err != nil {
			return nil, true, err
		}
	}
	if proxyCtx.lifecycle != nil {
		proxyCtx.lifecycle.observe(shared.OutcomeSuccess, true)
	}
	return collectResponsesOutputFromMaps(projectedEvents), true, nil
}

func (s *responsesWSSession) writeDirectResponsesEvent(ctx context.Context, event map[string]any, requestedModel string) error {
	if s.auth != nil && s.auth.Policy.AccessPolicy != nil && s.auth.Policy.AccessPolicy.MapModel(requestedModel) != requestedModel {
		raw, err := json.Marshal(event)
		if err != nil {
			return err
		}
		raw = restoreDownstreamResponseModel(raw, requestedModel)
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var restored map[string]any
		if err := decoder.Decode(&restored); err != nil {
			return err
		}
		event = restored
	}
	return s.writeJSON(ctx, event)
}

func prepareDirectResponsesWebsocketWire(ctx context.Context, cfg *UpstreamConfig, endpoint *store.DirectEndpoint, direct *store.DirectUpstreamCandidate, credential *oauth.DirectCredentialResult, raw []byte, headers http.Header, namespace, requestedModel, actualModel string) (*directProviderWire, error) {
	if err := direct.RequestConfig.ValidateEndpoints(direct.Endpoints); err != nil {
		return nil, err
	}
	context := requestoverride.NewContext(raw, headers, requestedModel, actualModel, "responses")
	body := swapModelInJSON(raw, actualModel)
	body, err := requestoverride.BeforeProtocol(body, "responses", direct.RequestConfig.Options)
	if err == nil {
		body, err = requestoverride.AfterProtocol(body, "responses", direct.RequestConfig.Options, direct.RequestConfig.NativeChatMapping)
	}
	if err == nil {
		body, err = applyDirectParamOverrides(body, direct.ParamOverride)
	}
	if err != nil {
		return nil, err
	}
	wire, err := prepareDirectProviderWire(endpoint, direct.ChannelID, credential, body, headers, namespace, directProviderRequest{Model: actualModel, Stream: true})
	if err != nil {
		return nil, err
	}
	if err := prepareDirectWebsocketDriver(ctx, cfg, wire, direct, endpoint, credential.Kind, actualModel); err != nil {
		return nil, err
	}
	if err := prepareDirectChannelPlugins(ctx, cfg, wire, direct.RequestConfig, "responsesWebsocket", actualModel, true, context); err != nil {
		return nil, err
	}
	wire.Body, err = requestoverride.ClaudeCodeMapping(wire.Body, "responses", direct.RequestConfig.Options, context)
	if err != nil {
		return nil, err
	}
	limit := config.DefaultRequestBodyLimit
	if cfg := config.GetSafe(); cfg != nil && cfg.RequestBodyLimit > 0 {
		limit = cfg.RequestBodyLimit
	}
	var warnings []requestoverride.Warning
	wire.Body, warnings, err = requestoverride.ApplyBody(wire.Body, direct.RequestConfig.Body, context, limit)
	logDirectOperationWarnings(direct.ChannelID, "body", warnings)
	if err != nil {
		return nil, err
	}
	wire.RequestConfig, wire.RequestContext, wire.Endpoint = direct.RequestConfig, context, endpoint
	return wire, nil
}

func directResponsesSessionKey(clientID string, pac *auth.ProxyAuthContext, direct *store.DirectUpstreamCandidate, credential *oauth.DirectCredentialResult, headers map[string]string, proxyConfig *platform.ProxyConfig) string {
	proxyURL := ""
	if proxyConfig != nil {
		proxyURL = proxyConfig.ProxyURL
	}
	// Only the digest enters the runtime key. Rotating a token, endpoint,
	// handshake identity or proxy never reuses the old authorized connection.
	identity, _ := json.Marshal([]any{downstreamSessionNamespace(pac), clientID, direct.ChannelID, direct.CredentialID, direct.Endpoints.ResponsesWebsocket, credential.AccessToken, credential.AccountID, headers, proxyURL})
	digest := sha256.Sum256(identity)
	return "direct-responses:" + hex.EncodeToString(digest[:])
}
