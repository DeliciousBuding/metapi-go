package proxyhandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	"github.com/deliciousbuding/metapi-go/transform/relaykitbridge"
)

// Every candidate starts from the unchanged downstream body. A failed local
// conversion writes nothing, and an HTTP miss only continues via the narrow
// direct-endpoint rejection policy in direct_fallback.go.
func dispatchDirectEndpointAttempt(w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig, selected *routing.SelectedChannel, model string, proxyConfig *platform.ProxyConfig, path string, body []byte, contentType string, firstByteTimeoutMs int64, retry, maxRetries int, isLastEndpoint bool, requestID string) (bool, *pendingUpstreamFailure, bool) {
	aggregateStream := !ctx.IsStream && selected.Direct.StreamPolicy == store.DirectStreamRequire
	upstreamStream := ctx.IsStream || aggregateStream
	if aggregateStream && (directStreamAggregateProtocol(ctx.DownstreamPath) == "" || directStreamAggregateProtocol(path) == "") {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, "Selected channel requires streaming for an unsupported request protocol", "invalid_request_error", requestID)
		return true, nil, false
	}
	openCode := selected.Direct.Provider == "opencode_go" || selected.Direct.Provider == "opencode_go_anthropic"
	// Scope forwarded session headers to the downstream key so shared upstream
	// credentials cannot be resumed across keys (audit L28).
	sessionNamespace := downstreamSessionNamespace(ctx.Auth)
	var err error
	if ctx.Multipart && !proxy.IsDirectMediaPath(path) {
		writeJSONErrorWithRequest(w, 400, "Multipart requires a media endpoint", "invalid_request_error", requestID)
		return true, nil, false
	}
	endpoint := directEndpointForPath(selected.Direct.Endpoints, path)
	if proxy.IsDirectMediaPath(path) {
		endpoint, err = directSelectedMediaEndpoint(selected.Direct, path)
		if err != nil {
			writeJSONErrorWithRequest(w, 400, err.Error(), "invalid_request_error", requestID)
			return true, nil, false
		}
	}
	var openCodeBodyProfile string
	endpoint, path, openCodeBodyProfile, err = resolveDirectOpenCodeEndpoint(endpoint, selected.Direct.Provider, path, model)
	if err != nil {
		writeJSONErrorWithRequest(w, 400, err.Error(), "invalid_request_error", requestID)
		return true, nil, false
	}
	codexImage := endpoint != nil && endpoint.Profile == "codex-image"
	credential := &oauth.DirectCredentialResult{AccessToken: selected.TokenValue, Kind: store.DirectCredentialAPIKey, Provider: selected.Direct.Provider}
	if cfg.ResolveDirectCredential != nil && selected.Direct.CredentialID > 0 {
		var proxyURL *string
		if proxyConfig != nil && proxyConfig.ProxyURL != "" {
			value := proxyConfig.ProxyURL
			proxyURL = &value
		}
		credential, err = cfg.ResolveDirectCredential(r.Context(), selected.Direct.CredentialID, proxyURL, false)
	} else if selected.Direct.CredentialKind == store.DirectCredentialOAuth || selected.Direct.CredentialKind == store.DirectCredentialNone || selected.Direct.CredentialKind == store.DirectCredentialGCPServiceAccount {
		err = oauth.ErrDirectCredentialUnavailable
	}
	anonymous := credential != nil && credential.Kind == store.DirectCredentialNone && credential.AccessToken == "" && store.DirectProviderAllowsAnonymous(credential.Provider) && endpoint != nil && endpoint.Auth == store.DirectAuthNone
	if err != nil || credential == nil || credential.AccessToken == "" && !anonymous || credential != nil && credential.Kind == store.DirectCredentialNone && !anonymous {
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "Selected direct credential is unavailable", "upstream_error", requestID)
		return true, nil, false
	}
	if status, err := revalidateDirectCredentialDispatch(r, ctx, cfg, selected, credential.Kind, ctx.dispatchPolicy); err != nil {
		writeJSONErrorWithRequest(w, status, err.Error(), "permission_error", requestID)
		return true, nil, false
	}
	if ctx.Auth != nil {
		r = r.WithContext(auth.WithProxyAuth(r.Context(), ctx.Auth))
	}
	selectedCopy := *selected
	selectedCopy.TokenValue = credential.AccessToken
	selected = &selectedCopy
	options := messages.Options{}
	var replay *messagesBridgeRequest
	if proxyPathIsMessages(ctx.DownstreamPath) && directBridgeNeeded(ctx.DownstreamPath, path) {
		replay = newMessagesBridgeRequest(r, ctx, selected)
		if replay.ready() == nil {
			options = replay.Options()
		} else if ctx.messagesBridgeReplayRequired {
			writeMessagesReplayFailure(w, ctx, requestID)
			return true, nil, false
		}
	} else if ctx.messagesBridgeReplayRequired {
		writeMessagesReplayFailure(w, ctx, requestID)
		return true, nil, false
	}
	var protocolSession *relaykitbridge.Session
	requestContext := requestoverride.NewContext(ctx.RawBody, r.Header, ctx.RequestedModel, model, directTransformProtocol(ctx.DownstreamPath))
	body, err = requestoverride.BeforeProtocol(body, directTransformProtocol(ctx.DownstreamPath), selected.Direct.RequestConfig.Options)
	if err == nil && aggregateStream {
		body, err = requireDirectStreamBody(body, ctx.DownstreamPath)
	}
	if err == nil && directBridgeNeeded(ctx.DownstreamPath, path) {
		body, protocolSession, err = prepareDirectProtocolRequest(r.Context(), endpoint, body, ctx.DownstreamPath, path, model, upstreamStream, options)
	}
	if err == nil && selected.Direct.RequestConfig.Options.Active() {
		body, err = requestoverride.AfterProtocol(body, directTransformProtocol(path), selected.Direct.RequestConfig.Options, selected.Direct.RequestConfig.NativeChatMapping)
	}
	if ctx.messagesBridgeReplayRequired && (err != nil || replay == nil || !replay.UsedReplay()) {
		writeMessagesReplayFailure(w, ctx, requestID)
		return true, nil, false
	}
	providerUpload := ctx.Multipart && hasDirectMultipartDriver(selected.Direct, path)
	if err == nil && codexImage && !providerUpload {
		body, err = prepareDirectCodexImagesRequest(r, ctx, endpoint, model, body)
		contentType = "application/json"
	}
	if err == nil && (!ctx.Multipart || codexImage) && !providerUpload && len(body) > 0 {
		body, err = applyDirectParamOverrides(body, selected.Direct.ParamOverride)
	}
	if err == nil && proxy.DirectProtocolForPath(path) == store.DirectProtocolGeminiEmbeddings {
		body, err = prepareGeminiEmbeddings(body, path, model)
	}
	if err != nil {
		status := http.StatusBadRequest
		if isRequestBodyTooLarge(err) {
			status = http.StatusRequestEntityTooLarge
		}
		if !isLastEndpoint && !ctx.messagesBridgeReplayRequired {
			return false, nil, true
		}
		writeJSONErrorWithRequest(w, status, "Cannot convert request for selected upstream: "+err.Error(), "invalid_request_error", requestID)
		return true, nil, false
	}
	// Native Gemini streaming lives in the URL, never an invented body field.
	if endpoint, _ := proxy.EndpointFromPath(path); endpoint == proxy.EndpointGemini && directBridgeNeeded(ctx.DownstreamPath, path) {
		var obj map[string]json.RawMessage
		if json.Unmarshal(body, &obj) == nil {
			delete(obj, "model")
			delete(obj, "stream")
			body, _ = json.Marshal(obj)
		}
	}
	expectUsage := false
	if endpoint, _ := proxy.EndpointFromPath(path); endpoint == proxy.EndpointChat {
		body, expectUsage = applyUpstreamStreamIncludeUsage(body, "openai", path, upstreamStream)
	}
	var wire *directProviderWire
	providerBinding := selected.Direct.RequestConfig.Provider
	boundProvider := providerBinding != nil && slices.Contains(providerBinding.Protocols, directProviderProtocol(selected.Direct, path, endpoint))
	if providerUpload {
		if ctx.directProviderUpload == nil {
			writeJSONErrorWithRequest(w, 400, "Missing provider upload metadata", "invalid_request_error", requestID)
			return true, nil, false
		}
		wire = &directProviderWire{Body: body, Headers: make(http.Header), DriverUpload: ctx.directProviderUpload}
		if endpoint != nil {
			wire.Profile = endpoint.Profile
		}
		wire.DriverUpload.profile = wire.Profile
	} else if endpoint != nil && boundProvider && (endpoint.Profile == "openrouter-image" || isDirectMediaProfile(endpoint.Profile) || isDirectNativeVideoProfile(endpoint.Profile)) {
		// An explicit driver owns its vendor payload and delivery mode. Keep
		// endpoint credentials/transport in the host, but do not layer the
		// built-in media conversion or asynchronous submission defaults on it.
		wire = &directProviderWire{Body: body, Headers: make(http.Header), Profile: endpoint.Profile}
	} else if endpoint != nil && endpoint.Profile == "openrouter-image" {
		wire, contentType, err = prepareDirectOpenRouterImage(path, contentType, body)
	} else if endpoint != nil && isDirectNativeVideoProfile(endpoint.Profile) {
		wire, contentType, err = prepareDirectNativeVideoProfile(endpoint.Profile, r.Method, path, contentType, body)
	} else if endpoint != nil && isDirectMediaProfile(endpoint.Profile) {
		wire, contentType, err = prepareDirectMediaProfile(endpoint.Profile, path, contentType, body)
	} else if endpoint != nil && endpoint.Profile == "opencode-go" {
		wire, err = prepareDirectOpenCodeWire(endpoint, openCodeBodyProfile, body, r.Header, ctx.directOpenCodeSession, sessionNamespace)
	} else {
		wire, err = prepareDirectProviderWire(endpoint, selected.Direct.ChannelID, credential, body, r.Header, sessionNamespace, directProviderRequest{Model: model, Stream: upstreamStream})
	}
	if err != nil {
		if !isLastEndpoint {
			return false, nil, true
		}
		writeJSONErrorWithRequest(w, http.StatusBadRequest, err.Error(), "invalid_request_error", requestID)
		return true, nil, false
	}
	beforeOperations := append([]byte(nil), wire.Body...)
	if err = prepareDirectProviderDriver(r.Context(), ctx, cfg, wire, selected.Direct, endpoint, credential.Kind, path, r.Method, model, upstreamStream); err != nil {
		message, kind := "Cannot execute selected provider driver", "upstream_error"
		if errors.Is(err, channelplugin.ErrRequestKind) {
			message, kind = "Selected provider does not accept this request format", "invalid_request_error"
		}
		writeJSONErrorWithRequest(w, channelPluginRequestStatus(err), message, kind, requestID)
		return true, nil, false
	}
	if wire.DriverContentType != "" {
		contentType = wire.DriverContentType
	}
	if err = prepareDirectChannelPlugins(r.Context(), cfg, wire, selected.Direct.RequestConfig, directPluginProtocol(path), model, upstreamStream, requestContext); err != nil {
		writeJSONErrorWithRequest(w, channelPluginRequestStatus(err), "Cannot execute selected channel request plugin", "upstream_error", requestID)
		return true, nil, false
	}
	wire.Body, err = requestoverride.ClaudeCodeMapping(wire.Body, directTransformProtocol(path), selected.Direct.RequestConfig.Options, requestContext)
	if err != nil {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, "Cannot apply selected channel reasoning mapping", "invalid_request_error", requestID)
		return true, nil, false
	}
	if len(selected.Direct.RequestConfig.Body) > 0 {
		var warnings []requestoverride.Warning
		limit := config.DefaultRequestBodyLimit
		if cfg := config.GetSafe(); cfg != nil && cfg.RequestBodyLimit > 0 {
			limit = cfg.RequestBodyLimit
		}
		wire.Body, warnings, err = requestoverride.ApplyBody(wire.Body, selected.Direct.RequestConfig.Body, requestContext, limit)
		logDirectOperationWarnings(selected.Direct.ChannelID, "body", warnings)
		if err != nil {
			writeJSONErrorWithRequest(w, http.StatusBadRequest, "Cannot apply selected channel request operations", "invalid_request_error", requestID)
			return true, nil, false
		}
	}
	wire.RequestConfig = selected.Direct.RequestConfig
	wire.RequestContext = requestContext
	wire.ForceStream = wire.ForceStream || aggregateStream
	wire.AggregateStream = aggregateStream
	if (len(selected.Direct.RequestConfig.Body) > 0 || selected.Direct.RequestConfig.Options.Active() || len(wire.Plugins) > 0 || wire.Driver != nil) && ctx.Auth != nil && ctx.Auth.OwnerUserID != nil &&
		!directUserOperationsWithinBudget(beforeOperations, wire.Body, path, ctx.lifecycle, aggregateStream) {
		writeJSONErrorWithRequest(w, http.StatusForbidden, "Selected channel request operations exceed the admitted user request", "permission_error", requestID)
		return true, nil, false
	}
	if openCode {
		wire.Headers.Set("X-Opencode-Session", upstreamadapter.NamespaceSessionID(sessionNamespace, ctx.directOpenCodeSession))
		// Source channel headers are explicit overrides of provider defaults.
		if proxyConfig != nil {
			for name, value := range proxyConfig.CustomHeaders {
				if strings.EqualFold(name, "X-Opencode-Session") {
					wire.Headers.Set("X-Opencode-Session", value)
				}
			}
		}
	}
	wire.Endpoint = endpoint
	if wire.DriverUpload != nil {
		contentType, err = finalizeDirectProviderUpload(r.Context(), wire)
		if err != nil {
			writeJSONErrorWithRequest(w, channelPluginRequestStatus(err), "Cannot encode selected provider upload", "upstream_error", requestID)
			return true, nil, false
		}
	}
	wire.ProtocolSession = protocolSession
	if wire.DriverOperation != "" || proxy.DirectProtocolForPath(path) == store.DirectProtocolVideo && (r.Method == http.MethodPost || endpoint != nil && isDirectNativeVideoProfile(endpoint.Profile)) {
		maxRetries = retry
	}
	r = r.WithContext(withDirectProviderWire(r.Context(), wire))
	return dispatchEndpointAttemptWithContinue(w, r, ctx, cfg, selected, model, proxyConfig, path, contentType, wire.Body, firstByteTimeoutMs, retry, maxRetries, isLastEndpoint, true, ctx.IsStream || wire.ForceStream, expectUsage, requestID, options)
}
