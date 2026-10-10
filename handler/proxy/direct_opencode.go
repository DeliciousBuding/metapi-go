package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/google/uuid"
)

// Resolve only the already-authorized logical Chat endpoint. Internal wire
// addresses never become grant bits and never borrow a custom endpoint URL.
func resolveDirectOpenCodeEndpoint(endpoint *store.DirectEndpoint, provider, path, model string) (*store.DirectEndpoint, string, string, error) {
	if endpoint == nil || endpoint.Profile != "opencode-go" {
		return endpoint, path, "", nil
	}
	if provider != "opencode_go" || proxy.DirectProtocolForPath(path) != store.DirectProtocolChat || endpoint.ModelWireURLs == nil {
		return nil, "", "", fmt.Errorf("invalid OpenCode Go Chat adapter")
	}
	protocol, bodyProfile := directOpenCodeRouteForModel(model)
	wireEndpoint := *endpoint
	wireEndpoint.ModelWireURLs = nil
	wireEndpoint.Auth = store.DirectAuthBearer
	switch protocol {
	case store.DirectProtocolResponses:
		wireEndpoint.URL = endpoint.ModelWireURLs.Responses
	case store.DirectProtocolMessages:
		wireEndpoint.URL = endpoint.ModelWireURLs.Messages
		wireEndpoint.Auth = store.DirectAuthAPIKey
	}
	if wireEndpoint.URL == "" {
		return nil, "", "", fmt.Errorf("missing OpenCode model wire URL")
	}
	return &wireEndpoint, proxy.PathForEndpoint(directEndpointFromBit(protocol)), bodyProfile, nil
}

// directOpenCodeRouteForModel accepts the final upstream model, after aliases
// have been resolved. Only OpenCode's default model-dispatch endpoint uses this
// selection; explicitly configured formats keep their own protocol and URL.
// The caller owns capability authorization and must retain this wire protocol
// for both request and response conversion.
func directOpenCodeRouteForModel(model string) (protocol int, bodyProfile string) {
	switch {
	case strings.HasPrefix(model, "deepseek"):
		return store.DirectProtocolChat, "deepseek"
	case strings.HasPrefix(model, "grok"), strings.HasPrefix(model, "gpt"):
		return store.DirectProtocolResponses, ""
	case strings.HasPrefix(model, "minimax"), strings.HasPrefix(model, "qwen3"):
		return store.DirectProtocolMessages, ""
	default:
		return store.DirectProtocolChat, ""
	}
}

// prepareDirectOpenCodeWire consumes a body already converted to the selected
// protocol. The resolved endpoint is exact: no URL, authentication or grant is
// inferred here. Custom OpenCode endpoints pass an empty bodyProfile.
func prepareDirectOpenCodeWire(endpoint *store.DirectEndpoint, bodyProfile string, body []byte, downstream http.Header, sessionID string) (*directProviderWire, error) {
	if endpoint == nil {
		return nil, fmt.Errorf("missing selected OpenCode endpoint")
	}
	wire := &directProviderWire{Endpoint: endpoint, Body: body, Headers: make(http.Header), Query: make(url.Values), Profile: endpoint.Profile}
	switch bodyProfile {
	case "":
	case "deepseek":
		var err error
		wire.Body, err = prepareDomesticChatRequest(body, bodyProfile)
		if err != nil {
			return nil, err
		}
		// OpenCode's DeepSeek route accepts JSON-object mode, not JSON Schema.
		var request map[string]json.RawMessage
		if err := json.Unmarshal(wire.Body, &request); err != nil {
			return nil, err
		}
		var format struct {
			Type string `json:"type"`
		}
		if raw, exists := request["response_format"]; exists {
			if err := json.Unmarshal(raw, &format); err != nil {
				return nil, fmt.Errorf("invalid OpenCode response_format")
			}
			if format.Type == "json_schema" {
				request["response_format"] = json.RawMessage(`{"type":"json_object"}`)
				wire.Body, err = json.Marshal(request)
				if err != nil {
					return nil, err
				}
			}
		}
	default:
		return nil, fmt.Errorf("unsupported OpenCode body profile")
	}
	wire.Headers.Set("X-Opencode-Session", directOpenCodeSessionID(downstream, sessionID))
	return wire, nil
}

// A supplied request-scoped session ID keeps retries stable when the client
// does not send an affinity header. Callers resolve that fallback once per
// inbound request; there is no process-global or cross-user session cache.
func directOpenCodeSessionID(headers http.Header, sessionID string) string {
	for _, name := range []string{"X-Opencode-Session", "X-Session-Id", "X-Session-Affinity"} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(sessionID); value != "" {
		return value
	}
	return uuid.NewString()
}
