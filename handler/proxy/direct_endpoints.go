package proxyhandler

import (
	"fmt"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"net/url"
	"strings"
)

func directEndpointForPath(endpoints store.DirectEndpoints, downstreamPath string) *store.DirectEndpoint {
	endpoint, ok := proxy.EndpointFromPath(downstreamPath)
	if !ok {
		return nil
	}
	switch endpoint {
	case proxy.EndpointChat:
		return endpoints.Chat
	case proxy.EndpointResponses:
		return endpoints.Responses
	case proxy.EndpointMessages:
		return endpoints.Messages
	case proxy.EndpointGemini:
		return endpoints.Gemini
	default:
		return nil
	}
}

func directProtocolBit(endpoint proxy.UpstreamEndpoint) int {
	switch endpoint {
	case proxy.EndpointChat:
		return routing.UpstreamProtocolChat
	case proxy.EndpointResponses:
		return routing.UpstreamProtocolResponses
	case proxy.EndpointMessages:
		return routing.UpstreamProtocolAnthropic
	case proxy.EndpointGemini:
		return routing.UpstreamProtocolGemini
	}
	return 0
}
func directEndpointFromBit(bit int) proxy.UpstreamEndpoint {
	switch bit {
	case routing.UpstreamProtocolChat:
		return proxy.EndpointChat
	case routing.UpstreamProtocolResponses:
		return proxy.EndpointResponses
	case routing.UpstreamProtocolAnthropic:
		return proxy.EndpointMessages
	case routing.UpstreamProtocolGemini:
		return proxy.EndpointGemini
	}
	return ""
}

func directSelectedPath(direct *store.DirectUpstreamCandidate, downstreamPath, model string, stream bool) (string, error) {
	client, ok := proxy.EndpointFromPath(downstreamPath)
	if !ok {
		return "", fmt.Errorf("unsupported direct upstream protocol")
	}
	if !direct.Endpoints.IsConfigured() && direct.Protocols&directProtocolBit(client) == 0 {
		return "", fmt.Errorf("direct grant does not authorize the client protocol")
	}
	order := direct.ProtocolOrder
	if len(order) == 0 {
		order = store.DirectProtocolOrder{2, 4, 8, 16}
	}
	selected := proxy.UpstreamEndpoint("")
	for _, bit := range order {
		if direct.Protocols&bit == 0 {
			continue
		}
		endpoint := directEndpointFromBit(bit)
		if selected == "" {
			selected = endpoint
		}
		if endpoint == client {
			selected = client
			break
		}
	}
	if selected == "" {
		return "", fmt.Errorf("direct grant has no authorized outbound protocol")
	}
	if strings.HasSuffix(strings.TrimRight(strings.Split(downstreamPath, "?")[0], "/"), "/count_tokens") {
		if selected != proxy.EndpointMessages {
			return "", fmt.Errorf("token counting requires a native Messages endpoint")
		}
		return "/v1/messages/count_tokens", nil
	}
	if selected == proxy.EndpointGemini {
		action := "generateContent"
		if stream {
			action = "streamGenerateContent"
		}
		return "/v1beta/models/" + url.PathEscape(strings.TrimPrefix(model, "models/")) + ":" + action, nil
	}
	return proxy.PathForEndpoint(selected), nil
}

func directRequestURL(direct *store.DirectUpstreamCandidate, path, model string, stream bool) (string, *store.DirectEndpoint) {
	endpoint := directEndpointForPath(direct.Endpoints, path)
	if endpoint != nil {
		target := endpoint.URL
		if endpoint.ModelPath {
			action := "generateContent"
			if stream {
				action = "streamGenerateContent"
			}
			target = strings.TrimRight(target, "/") + "/" + url.PathEscape(strings.TrimPrefix(model, "models/")) + ":" + action
			if stream {
				target += "?alt=sse"
			}
		} else if strings.HasSuffix(path, "/count_tokens") {
			target = strings.TrimRight(target, "/") + "/count_tokens"
		}
		return target, endpoint
	}
	logical, _ := proxy.EndpointFromPath(path)
	legacy := path
	switch logical {
	case proxy.EndpointChat:
		legacy = direct.ChatPath
	case proxy.EndpointResponses:
		legacy = direct.ResponsesPath
	case proxy.EndpointMessages:
		legacy = direct.AnthropicPath
		if strings.HasSuffix(path, "/count_tokens") {
			legacy = strings.TrimRight(legacy, "/") + "/count_tokens"
		}
	}
	return proxy.BuildUpstreamURL(direct.BaseURL, legacy), nil
}
