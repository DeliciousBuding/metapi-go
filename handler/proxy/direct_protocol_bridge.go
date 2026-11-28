package proxyhandler

import (
	"encoding/json"
	"fmt"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	gemini "github.com/deliciousbuding/metapi-go/transform/gemini/generate_content"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
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
