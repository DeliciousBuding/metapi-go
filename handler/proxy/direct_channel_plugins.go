package proxyhandler

import (
	"context"
	"errors"
	"slices"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/tidwall/gjson"
)

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
		if err != nil || len(warnings) > 0 {
			return channelplugin.ErrResult
		}
		wire.Body = body
		wire.PluginHeaders = append(wire.PluginHeaders, result.Headers...)
	}
	headers := append(slices.Clone(wire.PluginHeaders), config.Headers...)
	return (requestoverride.Config{Headers: headers}).Validate()
}

func applyDirectChannelResponsePlugins(ctx context.Context, wire *directProviderWire, body []byte, hook string) ([]byte, error) {
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
	paths := []string{"model", "type", "id", "object", "status", "error", "usage", "stop_reason", "finish_reason", "response.id", "response.model", "response.status", "response.error", "response.usage", "response.incomplete_details"}
	if protocol == "messages" {
		paths = append(paths, "message.id", "message.model", "message.usage", "message.stop_reason", "message.stop_sequence", "delta.stop_reason", "delta.stop_sequence")
	}
	if protocol == "gemini" {
		paths = append(paths, "usageMetadata", "modelVersion")
	}
	for _, path := range paths {
		if gjson.GetBytes(before, path).Raw != gjson.GetBytes(after, path).Raw {
			return false
		}
	}
	for _, pair := range [][2]string{{"choices", "finish_reason"}, {"candidates", "finishReason"}} {
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
	if errors.Is(err, channelplugins.ErrUnavailable) {
		return 503
	}
	return 502
}
