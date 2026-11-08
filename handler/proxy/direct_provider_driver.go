package proxyhandler

import (
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Resolve after current credential/route authorization and capture exactly one
// driver for this attempt. The dispatcher retains the original endpoint for
// authentication while the descriptor changes only its same-origin wire URL.
func prepareDirectProviderDriver(ctx context.Context, proxyCtx *Ctx, cfg *UpstreamConfig, wire *directProviderWire, direct *store.DirectUpstreamCandidate, endpoint *store.DirectEndpoint, kind, path, method, model string, stream bool) error {
	binding := direct.RequestConfig.Provider
	protocol := directProviderProtocol(direct, path, endpoint)
	if binding == nil || !slices.Contains(binding.Protocols, protocol) {
		return nil
	}
	if cfg == nil || cfg.ChannelPlugins == nil {
		return channelplugins.ErrUnavailable
	}
	if slices.Contains([]string{"video", "seedanceVideo", "zenmuxVideo"}, protocol) {
		return prepareDirectMediaDriver(ctx, proxyCtx, cfg, wire, direct, endpoint, protocol, kind, path, method, model)
	}
	// Every typed native request uses the existing dispatcher. Persistent video
	// and per-turn WS keep their own consumers; no protocol permission is inferred.
	if !contract.ValidProviderProtocol(protocol) || protocol == "responsesWebsocket" || strings.HasSuffix(path, "/count_tokens") {
		return channelplugins.ErrBinding
	}
	invocation, err := cfg.ChannelPlugins.ResolveDriver(ctx, *binding, protocol, kind)
	if err != nil {
		return err
	}
	hook := "request"
	if invocation.IsImageTask() {
		if stream && !invocation.StreamsImageTask() || direct.RequestConfig.ValidateEndpoints(direct.Endpoints) != nil {
			return channelplugins.ErrBinding
		}
		if stream {
			wire.DriverImageStreamOptions, err = newImageTaskStreamOptions(protocol, wire.Body, wire.DriverUpload != nil)
			if err != nil {
				return err
			}
			// The facade streams; its declared supplier submit is still JSON.
			// Give the driver client intent separately, and do not let a wire
			// stream flag switch the POST to a second SSE consumer.
			wire.Body, err = sjson.DeleteBytes(wire.Body, "stream")
			if err != nil {
				return channelplugin.ErrResult
			}
		}
		if proxyCtx != nil && proxyCtx.Body["response_format"] != nil {
			format, valid := proxyCtx.Body["response_format"].(string)
			if !valid || format != "" && format != "url" && format != "b64_json" {
				return channelplugin.ErrResult
			}
		}
		query, configured := binding.ImageQueries[protocol]
		if !configured || query.Validate() != nil {
			return channelplugins.ErrBinding
		}
		wire.DriverImageQuery = &query
		hook = "submit"
	}
	input := channelplugin.DriverInput{Body: string(wire.Body), Model: model, Stream: stream}
	if wire.DriverUpload != nil {
		input.BodyKind, input.Files = "multipart", wire.DriverUpload.metadata
	}
	raw, err := invocation.Call(ctx, hook, input)
	if err != nil {
		return err
	}
	var descriptor channelplugin.RequestDescriptor
	if requestoverride.Decode(raw, &descriptor) != nil || descriptor.Method != method {
		return channelplugin.ErrResult
	}
	prepared, err := directDriverDescriptorBody(descriptor, wire)
	if err != nil || !sameDriverRequestControls(protocol, wire.Body, prepared) {
		return channelplugin.ErrResult
	}
	url, _ := directRequestURL(direct, path, model, stream, endpoint)
	if err := validateDriverOperationURL(descriptor, url); err != nil {
		return err
	}
	if descriptor.ContentType != "" {
		mediaType, _, err := mime.ParseMediaType(descriptor.ContentType)
		if err != nil || mediaType != "application/json" {
			return channelplugin.ErrResult
		}
	}
	wire.Driver, wire.DriverURL, wire.DriverContentType = &invocation, url, descriptor.ContentType
	wire.Body = prepared
	wire.PluginProtocol, wire.PluginModel, wire.PluginStream = protocol, model, stream
	if wire.Headers == nil {
		wire.Headers = make(http.Header)
	}
	for key, value := range descriptor.Headers {
		wire.Headers.Set(key, value)
	}
	return nil
}

// Bind to the authorized endpoint slot, not merely its downstream path alias.
// Jina embeddings, Ollama and ModelScope share public paths with other slots.
func directProviderProtocol(direct *store.DirectUpstreamCandidate, path string, endpoint *store.DirectEndpoint) string {
	if endpoint != nil {
		for _, entry := range direct.Endpoints.Entries() {
			if entry.Endpoint == endpoint {
				return entry.Key
			}
		}
	}
	if strings.HasPrefix(path, "/v1/videos") {
		return directMediaDriverProtocol(direct, path)
	}
	return directPluginProtocol(path)
}

// Native response projection runs after raw usage observation. It cannot alter
// identity, usage or completion facts; vendor-envelope decoding belongs to the
// provider response consumer, not to this post-observation projection.
func projectDirectDriverResponse(ctx context.Context, wire *directProviderWire, body []byte, hook string) ([]byte, error) {
	if wire.Driver == nil || wire.DriverOperation != "" || wire.DriverImageQuery != nil {
		return body, nil
	}
	raw, err := wire.Driver.Call(ctx, hook, channelplugin.DriverInput{Body: string(body), Model: wire.PluginModel, Stream: wire.PluginStream})
	if err != nil {
		return nil, err
	}
	var result struct {
		Body string `json:"body"`
	}
	if requestoverride.Decode(raw, &result) != nil || requestoverride.RejectDuplicateJSONKeys([]byte(result.Body)) != nil || !sameChannelPluginControlFields(wire.PluginProtocol, body, []byte(result.Body)) {
		return nil, channelplugin.ErrResult
	}
	return []byte(result.Body), nil
}

// WebSocket descriptors contain one native message plus handshake metadata,
// not an HTTP method or another dispatch. The existing WS owner sends it once.
func prepareDirectWebsocketDriver(ctx context.Context, cfg *UpstreamConfig, wire *directProviderWire, direct *store.DirectUpstreamCandidate, endpoint *store.DirectEndpoint, kind, model string) error {
	binding := direct.RequestConfig.Provider
	if binding == nil || !slices.Contains(binding.Protocols, "responsesWebsocket") {
		return nil
	}
	if cfg == nil || cfg.ChannelPlugins == nil {
		return channelplugins.ErrUnavailable
	}
	invocation, err := cfg.ChannelPlugins.ResolveDriver(ctx, *binding, "responsesWebsocket", kind)
	if err != nil {
		return err
	}
	raw, err := invocation.Call(ctx, "request", channelplugin.DriverInput{Body: string(wire.Body), Model: model, Stream: true})
	if err != nil {
		return err
	}
	var descriptor struct {
		Path    string            `json:"path,omitempty"`
		Body    string            `json:"body"`
		Headers map[string]string `json:"headers,omitempty"`
	}
	if requestoverride.Decode(raw, &descriptor) != nil || !sameDriverRequestControls("responsesWebsocket", wire.Body, []byte(descriptor.Body)) {
		return channelplugin.ErrResult
	}
	base, err := url.Parse(endpoint.URL)
	if err != nil || base.Scheme != "ws" && base.Scheme != "wss" {
		return channelplugin.ErrResult
	}
	scheme := base.Scheme
	base.Scheme = "http"
	if scheme == "wss" {
		base.Scheme = "https"
	}
	request := channelplugin.RequestDescriptor{Method: http.MethodPost, Path: descriptor.Path, Body: descriptor.Body, Headers: descriptor.Headers}
	if err := validateDriverOperationURL(request, base.String()); err != nil {
		return err
	}
	wire.Driver, wire.DriverURL = &invocation, endpoint.URL
	wire.Body = []byte(descriptor.Body)
	wire.PluginProtocol, wire.PluginModel, wire.PluginStream = "responsesWebsocket", model, true
	if wire.Headers == nil {
		wire.Headers = make(http.Header)
	}
	for key, value := range descriptor.Headers {
		wire.Headers.Set(key, value)
	}
	return nil
}

func sameDriverRequestControls(protocol string, before, after []byte) bool {
	if requestoverride.RejectDuplicateJSONKeys(after) != nil || !sameChannelPluginRequestControls(protocol, before, after) {
		return false
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(after, &object) != nil || object == nil {
		return false
	}
	// Go DTO decoders may accept case-insensitive aliases while gjson and
	// provider protocols address exact keys. Do not let an alias shadow the
	// model or the budget inputs checked by the existing admission owner.
	canonical := []string{"model", "stream", "stream_options", "messages", "input", "tools", "prompt", "system", "instructions", "max_tokens", "max_completion_tokens", "max_output_tokens", "generationConfig", "generation_config", "type", "previous_response_id", "background", "store"}
	for key := range object {
		for _, field := range canonical {
			if strings.EqualFold(key, field) && key != field {
				return false
			}
		}
	}
	for _, path := range []string{"model", "stream", "stream_options", "max_tokens", "max_completion_tokens", "max_output_tokens", "generationConfig.maxOutputTokens", "generation_config.max_output_tokens"} {
		if gjson.GetBytes(before, path).Raw != gjson.GetBytes(after, path).Raw {
			return false
		}
	}
	return true
}

// Origin equality alone is insufficient: another path can select a different
// model or start another operation with the same credential. Endpoint defaults
// belong in the administrator's saved typed endpoints; a runtime descriptor
// must retain the URL the protocol/operation owner authorized for this attempt.
func validateDriverOperationURL(descriptor channelplugin.RequestDescriptor, authorized string) error {
	resolved, err := descriptor.ResolveURL(authorized)
	if err != nil {
		return err
	}
	base, err := url.Parse(authorized)
	if err != nil || resolved.String() != base.String() {
		return channelplugin.ErrResult
	}
	return nil
}
