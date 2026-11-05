package proxyhandler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/proxy"
)

func directUserOperationsWithinBudget(before, after []byte, path string, lifecycle *requestLifecycle) bool {
	var original, transformed map[string]any
	if json.Unmarshal(before, &original) != nil || json.Unmarshal(after, &transformed) != nil {
		return false
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
	if wire == nil || len(wire.RequestConfig.Headers) == 0 {
		return nil
	}
	headers, warnings, err := requestoverride.ApplyHeaders(req.Header, wire.RequestConfig.Headers, wire.RequestContext)
	logDirectOperationWarnings(channelID, "headers", warnings)
	if err != nil {
		return err
	}
	req.Header = headers
	return nil
}
