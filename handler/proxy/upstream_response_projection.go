package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
	messages "github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func normalizeEndpointJSONResponse(resp *http.Response, body *bufferedUpstreamBody, upstreamPath string, wire *directProviderWire) (bool, error) {
	respBody := body.bytes
	nativeJSON := isNativeJSONProtocol(proxy.DirectProtocolForPath(upstreamPath))
	nativeHasOutput := false
	if nativeJSON || wire != nil && !wire.AggregateStream && (wire.Profile == "ollama" || wire.Profile == "cline" || wire.Profile == "openrouter" || wire.Profile == "cerebras" || wire.Profile == "nanogpt" || wire.Profile == "antigravity") {
		converted := respBody
		var convertErr error
		if nativeJSON {
			if !body.readable || !json.Valid(respBody) {
				convertErr = fmt.Errorf("expected a JSON response")
			} else if proxy.DirectProtocolForPath(upstreamPath) == store.DirectProtocolDecisions {
				valid := false
				valid, nativeHasOutput = decisionsResponseShape(respBody)
				if !valid {
					convertErr = fmt.Errorf("expected a native Decisions response")
				}
			} else if proxy.DirectProtocolForPath(upstreamPath) == store.DirectProtocolResponsesCompact {
				var compact struct {
					Object string            `json:"object"`
					Output []json.RawMessage `json:"output"`
				}
				if json.Unmarshal(respBody, &compact) != nil || compact.Object != "response.compaction" || compact.Output == nil {
					convertErr = fmt.Errorf("expected a native compaction response")
				}
			}
		} else if wire.Profile == "antigravity" {
			if !body.readable {
				convertErr = fmt.Errorf("expected a readable Antigravity response")
			} else if adapter, ok := upstreamadapter.Lookup(wire.Profile); ok {
				converted, convertErr = adapter.NormalizeResponse(respBody, false)
			}
		} else if wire.Profile == "cline" {
			converted, convertErr = upstreamadapter.NormalizeClineJSON(respBody, body.readable)
		} else if wire.Profile == "openrouter" || wire.Profile == "cerebras" {
			converted, convertErr = upstreamadapter.NormalizeRouterChatJSON(respBody, body.readable)
		} else if wire.Profile == "nanogpt" {
			if !body.readable {
				convertErr = fmt.Errorf("expected a readable NanoGPT JSON response")
			} else {
				converted, convertErr = normalizeDirectNanoGPTJSON(respBody, wire.NanoGPTTools)
			}
		} else {
			converted, convertErr = normalizeDirectOllamaJSON(respBody, body.readable)
		}
		if convertErr != nil {
			return false, convertErr
		}
		body.bytes = converted
		resp.Header.Del("Content-Length")
		resp.Header.Del("ETag")
		resp.Header.Set("Content-Type", "application/json")
	}
	return nativeHasOutput, nil
}

func projectEndpointResponse(r *http.Request, resp *http.Response, respBody []byte, readable, direct bool, downstreamPath, upstreamPath, upstreamModel string, bridgeOptions messages.Options) ([]byte, error) {
	directBridge := direct && directBridgeNeeded(downstreamPath, upstreamPath)
	if direct && readable {
		restored := restoreDirectProviderResponse(r.Context(), respBody)
		if !bytes.Equal(restored, respBody) {
			resp.Header.Del("Content-Length")
			resp.Header.Del("ETag")
			respBody = restored
		}
	}
	if directBridge || isMessagesChatBridge(downstreamPath, upstreamPath) {
		var convertErr error
		if readable {
			if wire := directProviderWireFromContext(r.Context()); wire != nil {
				respBody, convertErr = projectDirectProviderResponse(wire, respBody, false)
			}
			if directBridge && convertErr == nil {
				if wire := directProviderWireFromContext(r.Context()); wire != nil && wire.ProtocolSession != nil {
					respBody, convertErr = wire.ProtocolSession.Response(r.Context(), respBody)
				} else {
					respBody, convertErr = directConvertResponse(respBody, downstreamPath, upstreamPath, upstreamModel, bridgeOptions)
				}
			} else if convertErr == nil {
				respBody, convertErr = messages.FromChatResponse(respBody, bridgeOptions)
			}
		} else {
			convertErr = fmt.Errorf("Chat response encoding is not supported by the Messages bridge")
		}
		if convertErr != nil {
			return nil, convertErr
		}
		resp.Header.Del("Content-Length")
		resp.Header.Del("ETag")
		resp.Header.Set("Content-Type", "application/json")
	}
	return respBody, nil
}

func relayBufferedUpstreamResponse(w http.ResponseWriter, resp *http.Response, bodyBytes []byte) {
	relayUpstreamResponseHeaders(w, resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)
}
