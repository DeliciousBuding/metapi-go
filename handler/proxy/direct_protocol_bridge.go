package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	gemini "github.com/deliciousbuding/metapi-go/transform/gemini/generate_content"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
	"github.com/deliciousbuding/metapi-go/transform/relaykitbridge"
)

func directBridgeNeeded(downstream, upstream string) bool {
	d, dok := proxy.EndpointFromPath(downstream)
	u, uok := proxy.EndpointFromPath(upstream)
	// Ollama's wire normalizer exposes Chat to the existing client bridges.
	if u == proxy.EndpointOllama {
		u = proxy.EndpointChat
	}
	return dok && uok && d != u
}

func dispatchDirectEndpoint(w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig, selected *routing.SelectedChannel, model string, proxyConfig *platform.ProxyConfig, body []byte, contentType string, firstByteTimeoutMs int64, retry, maxRetries int, disableCrossProtocolFallback bool, requestID string) (bool, *pendingUpstreamFailure) {
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
	selected, err = pinDirectVideoTaskCandidate(ctx, selected)
	if err != nil {
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, err.Error(), "upstream_error", requestID)
		return true, nil
	}
	paths, err := directCandidatePaths(selected.Direct, ctx.DownstreamPath, model, ctx.IsStream)
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
	}
	return false, jsonPendingUpstreamFailure(http.StatusBadGateway, "No compatible direct endpoint", "upstream_error")
}

// Every candidate starts from the unchanged downstream body. A failed local
// conversion writes nothing, and an HTTP miss only continues via the narrow
// direct-endpoint rejection policy in direct_fallback.go.
func dispatchDirectEndpointAttempt(w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig, selected *routing.SelectedChannel, model string, proxyConfig *platform.ProxyConfig, path string, body []byte, contentType string, firstByteTimeoutMs int64, retry, maxRetries int, isLastEndpoint bool, requestID string) (bool, *pendingUpstreamFailure, bool) {
	openCode := selected.Direct.Provider == "opencode_go" || selected.Direct.Provider == "opencode_go_anthropic"
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
	} else if selected.Direct.CredentialKind == store.DirectCredentialOAuth || selected.Direct.CredentialKind == store.DirectCredentialNone {
		err = oauth.ErrDirectCredentialUnavailable
	}
	anonymous := credential != nil && credential.Kind == store.DirectCredentialNone && credential.AccessToken == "" && store.DirectProviderAllowsAnonymous(credential.Provider) && endpoint != nil && endpoint.Auth == store.DirectAuthNone
	if err != nil || credential == nil || credential.AccessToken == "" && !anonymous || credential != nil && credential.Kind == store.DirectCredentialNone && !anonymous {
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "Selected direct credential is unavailable", "upstream_error", requestID)
		return true, nil, false
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
	if directBridgeNeeded(ctx.DownstreamPath, path) {
		body, protocolSession, err = prepareDirectProtocolRequest(r.Context(), endpoint, body, ctx.DownstreamPath, path, model, ctx.IsStream, options)
	}
	if ctx.messagesBridgeReplayRequired && (err != nil || replay == nil || !replay.UsedReplay()) {
		writeMessagesReplayFailure(w, ctx, requestID)
		return true, nil, false
	}
	if err == nil && codexImage {
		body, err = prepareDirectCodexImagesRequest(r, ctx, endpoint, model, body)
		contentType = "application/json"
	}
	if err == nil && (!ctx.Multipart || codexImage) && len(body) > 0 {
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
		body, expectUsage = applyUpstreamStreamIncludeUsage(body, "openai", path, ctx.IsStream)
	}
	var wire *directProviderWire
	if endpoint != nil && endpoint.Profile == "openrouter-image" {
		wire, contentType, err = prepareDirectOpenRouterImage(path, contentType, body)
	} else if endpoint != nil && isDirectNativeVideoProfile(endpoint.Profile) {
		wire, contentType, err = prepareDirectNativeVideoProfile(endpoint.Profile, r.Method, path, contentType, body)
	} else if endpoint != nil && isDirectMediaProfile(endpoint.Profile) {
		wire, contentType, err = prepareDirectMediaProfile(endpoint.Profile, path, contentType, body)
	} else if endpoint != nil && endpoint.Profile == "opencode-go" {
		wire, err = prepareDirectOpenCodeWire(endpoint, openCodeBodyProfile, body, r.Header, ctx.directOpenCodeSession)
	} else {
		wire, err = prepareDirectProviderWire(endpoint, selected.Direct.ChannelID, credential, body, r.Header)
	}
	if err != nil {
		if !isLastEndpoint {
			return false, nil, true
		}
		writeJSONErrorWithRequest(w, http.StatusBadRequest, err.Error(), "invalid_request_error", requestID)
		return true, nil, false
	}
	if openCode {
		wire.Headers.Set("X-Opencode-Session", ctx.directOpenCodeSession)
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
	wire.ProtocolSession = protocolSession
	if proxy.DirectProtocolForPath(path) == store.DirectProtocolVideo && (r.Method == http.MethodPost || endpoint != nil && isDirectNativeVideoProfile(endpoint.Profile)) {
		maxRetries = retry
	}
	r = r.WithContext(withDirectProviderWire(r.Context(), wire))
	return dispatchEndpointAttemptWithContinue(w, r, ctx, cfg, selected, model, proxyConfig, path, contentType, wire.Body, firstByteTimeoutMs, retry, maxRetries, isLastEndpoint, true, ctx.IsStream || wire.ForceStream, expectUsage, requestID, options)
}

func directConvertRequest(body []byte, downstream, upstream, model string, stream bool, options messages.Options) ([]byte, error) {
	d, _ := proxy.EndpointFromPath(downstream)
	u, _ := proxy.EndpointFromPath(upstream)
	if d == u {
		return body, nil
	}
	var err error
	switch d {
	case proxy.EndpointChat:
	case proxy.EndpointResponses:
		body, err = responses.ToChatRequest(body)
	case proxy.EndpointMessages:
		body, err = messages.ToChatRequest(body, options)
	case proxy.EndpointGemini:
		body, err = gemini.ToChatRequest(body, model)
	default:
		return nil, fmt.Errorf("unsupported downstream protocol")
	}
	if err != nil {
		return nil, err
	}
	// Native Gemini derives stream from the path, so its Chat intermediate needs
	// the explicit flag expected by the target request serializers.
	if d == proxy.EndpointGemini {
		var obj map[string]json.RawMessage
		if err = json.Unmarshal(body, &obj); err != nil {
			return nil, err
		}
		obj["stream"] = json.RawMessage("false")
		if stream {
			obj["stream"] = json.RawMessage("true")
		}
		body, err = json.Marshal(obj)
		if err != nil {
			return nil, err
		}
	}
	switch u {
	case proxy.EndpointChat, proxy.EndpointOllama:
		return body, nil
	case proxy.EndpointResponses:
		return responses.FromChatRequest(body)
	case proxy.EndpointMessages:
		return messages.FromChatRequest(body)
	case proxy.EndpointGemini:
		return gemini.FromChatRequest(body, model)
	}
	return nil, fmt.Errorf("unsupported upstream protocol")
}

func directConvertResponse(body []byte, downstream, upstream, model string, options messages.Options) ([]byte, error) {
	d, _ := proxy.EndpointFromPath(downstream)
	u, _ := proxy.EndpointFromPath(upstream)
	var err error
	switch u {
	case proxy.EndpointChat, proxy.EndpointOllama:
	case proxy.EndpointResponses:
		body, err = responses.ToChatResponse(body)
	case proxy.EndpointMessages:
		body, err = messages.ToChatResponse(body)
	case proxy.EndpointGemini:
		body, err = gemini.ToChatResponse(body, model)
	default:
		return nil, fmt.Errorf("unsupported upstream protocol")
	}
	if err != nil {
		return nil, err
	}
	switch d {
	case proxy.EndpointChat:
		return body, nil
	case proxy.EndpointResponses:
		return responses.FromChatResponse(body)
	case proxy.EndpointMessages:
		return messages.FromChatResponse(body, options)
	case proxy.EndpointGemini:
		return gemini.FromChatResponse(body)
	}
	return nil, fmt.Errorf("unsupported downstream protocol")
}

type protocolEventStream interface {
	TransformEvent([]byte) ([]byte, error)
	Finish() ([]byte, error)
}
type chainedProtocolStream struct{ first, second protocolEventStream }

func (s *chainedProtocolStream) TransformEvent(frame []byte) ([]byte, error) {
	out, err := s.first.TransformEvent(frame)
	if err != nil {
		return nil, err
	}
	return transformChainedFrames(s.second, out)
}
func (s *chainedProtocolStream) Finish() ([]byte, error) {
	out, err := s.first.Finish()
	if err != nil {
		return nil, err
	}
	converted, err := transformChainedFrames(s.second, out)
	if err != nil {
		return nil, err
	}
	last, err := s.second.Finish()
	return append(converted, last...), err
}
func transformChainedFrames(stream protocolEventStream, frames []byte) ([]byte, error) {
	var out []byte
	for len(frames) > 0 {
		index, sep := nextSseBoundary(string(frames))
		if index < 0 {
			return nil, fmt.Errorf("converter returned an incomplete SSE frame")
		}
		next, err := stream.TransformEvent(frames[:index+sep])
		if err != nil {
			return nil, err
		}
		out = append(out, next...)
		frames = frames[index+sep:]
	}
	return out, nil
}
func directResponseStream(downstream, upstream, model string, options messages.Options) protocolEventStream {
	d, _ := proxy.EndpointFromPath(downstream)
	u, _ := proxy.EndpointFromPath(upstream)
	var first, second protocolEventStream
	switch u {
	case proxy.EndpointResponses:
		first = responses.NewResponsesStream(model)
	case proxy.EndpointMessages:
		first = messages.NewMessagesStream(model)
	case proxy.EndpointGemini:
		first = gemini.NewGeminiStream(model)
	}
	switch d {
	case proxy.EndpointResponses:
		second = responses.NewChatStream(model)
	case proxy.EndpointMessages:
		second = messages.NewChatStream(model, options)
	case proxy.EndpointGemini:
		second = gemini.NewChatStream(model)
	}
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return &chainedProtocolStream{first: first, second: second}
}
