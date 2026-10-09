package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	gemini "github.com/deliciousbuding/metapi-go/transform/gemini/generate_content"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

func directBridgeNeeded(downstream, upstream string) bool {
	d, dok := proxy.EndpointFromPath(downstream)
	u, uok := proxy.EndpointFromPath(upstream)
	return dok && uok && d != u
}

func dispatchDirectEndpoint(w http.ResponseWriter, r *http.Request, ctx *Ctx, cfg *UpstreamConfig, selected *routing.SelectedChannel, model string, proxyConfig *platform.ProxyConfig, body []byte, firstByteTimeoutMs int64, retry, maxRetries int, requestID string) (bool, *pendingUpstreamFailure) {
	if ctx.DownstreamPath == "" {
		copyCtx := *ctx
		copyCtx.DownstreamPath = r.URL.Path
		ctx = &copyCtx
	}
	path, err := directSelectedPath(selected.Direct, ctx.DownstreamPath, model, ctx.IsStream)
	if err != nil {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, err.Error(), "invalid_request_error", requestID)
		return true, nil
	}
	options := messages.Options{}
	if ctx.messagesBridgeReplayRequired {
		writeMessagesReplayFailure(w, ctx, requestID)
		return true, nil
	}
	if directBridgeNeeded(ctx.DownstreamPath, path) {
		body, err = directConvertRequest(body, ctx.DownstreamPath, path, model, ctx.IsStream, options)
	}
	if err == nil {
		body, err = applyDirectParamOverrides(body, selected.Direct.ParamOverride)
	}
	if err != nil {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, "Cannot convert request for selected upstream: "+err.Error(), "invalid_request_error", requestID)
		return true, nil
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
	credential := &oauth.DirectCredentialResult{AccessToken: selected.TokenValue, Kind: store.DirectCredentialAPIKey, Provider: selected.Direct.Provider}
	if cfg.ResolveDirectCredential != nil && selected.Direct.CredentialID > 0 {
		var proxyURL *string
		if proxyConfig != nil && proxyConfig.ProxyURL != "" {
			value := proxyConfig.ProxyURL
			proxyURL = &value
		}
		credential, err = cfg.ResolveDirectCredential(r.Context(), selected.Direct.CredentialID, proxyURL, false)
	} else if selected.Direct.CredentialKind == store.DirectCredentialOAuth {
		err = oauth.ErrDirectCredentialUnavailable
	}
	if err != nil || credential == nil || credential.AccessToken == "" {
		writeJSONErrorWithRequest(w, http.StatusServiceUnavailable, "Selected direct credential is unavailable", "upstream_error", requestID)
		return true, nil
	}
	selectedCopy := *selected
	selectedCopy.TokenValue = credential.AccessToken
	selected = &selectedCopy
	endpoint := directEndpointForPath(selected.Direct.Endpoints, path)
	wire, err := prepareDirectProviderWire(endpoint, selected.Direct.ChannelID, credential, body, r.Header)
	if err != nil {
		writeJSONErrorWithRequest(w, http.StatusBadRequest, err.Error(), "invalid_request_error", requestID)
		return true, nil
	}
	r = r.WithContext(withDirectProviderWire(r.Context(), wire))
	finished, pending, _ := dispatchEndpointAttemptWithContinue(w, r, ctx, cfg, selected, model, proxyConfig, path, "application/json", wire.Body, firstByteTimeoutMs, retry, maxRetries, true, true, ctx.IsStream || wire.ForceStream, expectUsage, requestID, options)
	return finished, pending
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
		var obj map[string]any
		if err = json.Unmarshal(body, &obj); err != nil {
			return nil, err
		}
		obj["stream"] = stream
		body, err = json.Marshal(obj)
		if err != nil {
			return nil, err
		}
	}
	switch u {
	case proxy.EndpointChat:
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
	case proxy.EndpointChat:
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
