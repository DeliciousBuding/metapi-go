package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/tidwall/gjson"
)

func directPluginProtocol(path string) string {
	for _, entry := range (store.DirectEndpoints{}).Entries() {
		if entry.Protocol == proxy.DirectProtocolForPath(path) {
			return entry.Key
		}
	}
	return directTransformProtocol(path)
}

func prepareDirectChannelPlugins(ctx context.Context, cfg *UpstreamConfig, wire *directProviderWire, config store.DirectRequestConfig, protocol, model string, stream bool, requestContext requestoverride.Context) error {
	if len(config.Plugins) == 0 {
		return nil
	}
	if cfg == nil || cfg.ChannelPlugins == nil {
		return channelplugins.ErrUnavailable
	}
	invocations, err := cfg.ChannelPlugins.Resolve(ctx, config.Plugins, protocol)
	if err != nil {
		return err
	}
	wire.Plugins = invocations
	wire.PluginProtocol, wire.PluginModel, wire.PluginStream = protocol, model, stream
	for _, invocation := range invocations {
		result, err := invocation.Program.Call(ctx, "request", channelplugin.Input{Protocol: protocol, Body: string(wire.Body), Model: model, Stream: stream, Options: invocation.Binding.Options})
		if err != nil {
			return err
		}
		body, warnings, err := requestoverride.ApplyBody(wire.Body, result.Body, requestContext, channelplugin.MaxInputBytes)
		if err != nil || len(warnings) > 0 || !sameChannelPluginRequestControls(protocol, wire.Body, body) {
			return channelplugin.ErrResult
		}
		wire.Body = body
		wire.PluginHeaders = append(wire.PluginHeaders, result.Headers...)
	}
	headers := append(slices.Clone(wire.PluginHeaders), config.Headers...)
	return (requestoverride.Config{Headers: headers}).Validate()
}

func sameChannelPluginRequestControls(protocol string, before, after []byte) bool {
	if protocol != "responsesCompact" && protocol != "responsesWebsocket" {
		return true
	}
	// These transports carry opaque continuation and native event envelopes.
	// Vendor configuration is extensible; changing identity or control mode is
	// not a wire normalization, and must not happen after authorization.
	for _, path := range []string{"model", "type", "input", "previous_response_id", "stream", "background", "store"} {
		if gjson.GetBytes(before, path).Raw != gjson.GetBytes(after, path).Raw {
			return false
		}
	}
	return true
}

func projectDirectWebsocketPluginEvents(ctx context.Context, wire *directProviderWire, events []map[string]any) ([]map[string]any, error) {
	if len(wire.Plugins) == 0 && wire.Driver == nil {
		return events, nil
	}
	out := make([]map[string]any, 0, len(events))
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			return nil, channelplugin.ErrResult
		}
		raw, err = applyDirectChannelResponsePlugins(ctx, wire, raw, "event")
		if err != nil {
			return nil, err
		}
		var projected map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&projected) != nil || projected == nil {
			return nil, channelplugin.ErrResult
		}
		out = append(out, projected)
	}
	return out, nil
}

func applyDirectChannelResponsePlugins(ctx context.Context, wire *directProviderWire, body []byte, hook string) ([]byte, error) {
	var err error
	body, err = projectDirectDriverResponse(ctx, wire, body, hook)
	if err != nil {
		return nil, err
	}
	for _, invocation := range wire.Plugins {
		result, err := invocation.Program.Call(ctx, hook, channelplugin.Input{Protocol: wire.PluginProtocol, Body: string(body), Model: wire.PluginModel, Stream: wire.PluginStream, Options: invocation.Binding.Options})
		if err != nil {
			return nil, err
		}
		out, warnings, err := requestoverride.ApplyBody(body, result.Body, requestoverride.Context{}, channelplugin.MaxInputBytes)
		if err != nil || len(warnings) > 0 || !sameChannelPluginControlFields(wire.PluginProtocol, body, out) {
			return nil, channelplugin.ErrResult
		}
		body = out
	}
	return body, nil
}

// Plugins may normalize vendor/content fields, not turn an upstream failure
// into a success, forge usage or drop native terminal/control events. Usage is
// separately recorded from the original upstream bytes before these hooks.
func sameChannelPluginControlFields(protocol string, before, after []byte) bool {
	if !responseControlAliasesMatch(before, after) {
		return false
	}
	return channelPluginControlsMatch(protocol, before, after, false)
}

func channelPluginControlsMatch(protocol string, before, after []byte, presentOnly bool) bool {
	paths := []string{"model", "type", "id", "object", "status", "error", "usage", "stop_reason", "finish_reason", "response.id", "response.model", "response.status", "response.error", "response.usage", "response.incomplete_details"}
	if protocol == "messages" {
		paths = append(paths, "message.id", "message.model", "message.usage", "message.stop_reason", "message.stop_sequence", "delta.stop_reason", "delta.stop_sequence")
	}
	if protocol == "gemini" {
		paths = append(paths, "usageMetadata", "modelVersion")
	}
	if protocol == "responsesCompact" || protocol == "responsesWebsocket" {
		paths = append(paths, "output", "response.output", "item", "part", "output_index", "content_index", "item_id", "call_id", "sequence_number", "previous_response_id")
		if strings.HasPrefix(gjson.GetBytes(before, "type").String(), "response.function_call_arguments.") {
			paths = append(paths, "delta", "arguments", "name")
		}
	}
	for _, path := range paths {
		if presentOnly && !gjson.GetBytes(before, path).Exists() {
			continue
		}
		if gjson.GetBytes(before, path).Raw != gjson.GetBytes(after, path).Raw {
			return false
		}
	}
	for _, pair := range [][2]string{{"choices", "finish_reason"}, {"candidates", "finishReason"}} {
		if presentOnly && !gjson.GetBytes(before, pair[0]).Exists() {
			continue
		}
		a, b := gjson.GetBytes(before, pair[0]).Array(), gjson.GetBytes(after, pair[0]).Array()
		if len(a) != len(b) {
			return false
		}
		for index := range a {
			if a[index].Get(pair[1]).Raw != b[index].Get(pair[1]).Raw {
				return false
			}
		}
	}
	return true
}

func channelPluginRequestStatus(err error) int {
	if errors.Is(err, channelplugin.ErrRequestKind) {
		return 415
	}
	if errors.Is(err, channelplugins.ErrUnavailable) {
		return 503
	}
	return 502
}

func channelPluginExecutionFailure(err error) bool {
	return errors.Is(err, channelplugin.ErrExecution) || errors.Is(err, channelplugin.ErrResult) || errors.Is(err, channelplugin.ErrManifest) || errors.Is(err, channelplugins.ErrBinding) || errors.Is(err, channelplugins.ErrUnavailable)
}
