package proxyhandler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/proxy"
)

func directUserOperationsWithinBudget(before, after []byte, path string, lifecycle *requestLifecycle, aggregateStream bool) bool {
	var original, transformed map[string]any
	if json.Unmarshal(before, &original) != nil || json.Unmarshal(after, &transformed) != nil {
		return false
	}
	if aggregateStream {
		// The gateway adds these transport controls after ingress admission.
		// Ordered operations cannot change them, and neither carries prompt or
		// output content. Compare the transformed content against the original
		// hold without charging serialization overhead for forced streaming.
		for _, body := range []map[string]any{original, transformed} {
			delete(body, "stream")
			delete(body, "stream_options")
		}
	}
	beforeEstimate, err := userBudgetTokenEstimate(path, original)
	if err != nil {
		return false
	}
	afterEstimate, err := userBudgetTokenEstimate(path, transformed)
	return err == nil && afterEstimate <= beforeEstimate && lifecycle.admitsTransformedUserRequest(afterEstimate)
}

func directTransformProtocol(path string) string {
	endpoint, _ := proxy.EndpointFromPath(path)
	switch endpoint {
	case proxy.EndpointChat:
		return "chat"
	case proxy.EndpointResponses:
		return "responses"
	case proxy.EndpointMessages:
		return "messages"
	case proxy.EndpointGemini:
		return "gemini"
	default:
		return ""
	}
}

func logDirectOperationWarnings(channelID int64, phase string, warnings []requestoverride.Warning) {
	for _, warning := range warnings {
		slog.Warn("channel request operation skipped", "channel_id", channelID, "phase", phase, "operation_index", warning.Index, "reason", warning.Code)
	}
}

func applyDirectRequestHeaderOperations(req *http.Request, channelID int64, wire *directProviderWire) error {
	if wire == nil || len(wire.RequestConfig.Headers)+len(wire.PluginHeaders) == 0 {
		return nil
	}
	operations := append(slices.Clone(wire.PluginHeaders), wire.RequestConfig.Headers...)
	headers, warnings, err := requestoverride.ApplyHeaders(req.Header, operations, wire.RequestContext)
	logDirectOperationWarnings(channelID, "headers", warnings)
	if err != nil {
		return err
	}
	if len(wire.PluginHeaders) > 0 && len(warnings) > 0 {
		return channelplugin.ErrResult
	}
	req.Header = headers
	return nil
}
