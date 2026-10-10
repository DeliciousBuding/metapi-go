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
	return endpoints.ForProtocol(proxy.DirectProtocolForPath(downstreamPath))
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
	bit := proxy.DirectProtocolForPath(downstreamPath)
	if bit != 0 && bit&store.DirectGenerationProtocols == 0 {
		if _, err := directSelectedMediaEndpoint(direct, downstreamPath); err != nil {
			return "", err
		}
		return downstreamPath, nil
	}
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
		if bit&store.DirectGenerationProtocols == 0 || direct.Protocols&bit == 0 {
			continue
		}
		endpoint := directEndpointFromBit(bit)
		if direct.Endpoints.IsConfigured() && direct.Endpoints.ForProtocol(bit) == nil {
			continue
		}
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

func directSelectedMediaEndpoint(direct *store.DirectUpstreamCandidate, path string) (*store.DirectEndpoint, error) {
	allowed := direct.Protocols & proxy.DirectProtocolMaskForPath(path)
	order := direct.ProtocolOrder
	if len(order) == 0 {
		for _, entry := range direct.Endpoints.Entries() {
			if entry.Endpoint != nil {
				order = append(order, entry.Protocol)
			}
		}
	}
	for _, bit := range order {
		if allowed&bit != 0 {
			if endpoint := direct.Endpoints.ForProtocol(bit); endpoint != nil {
				return endpoint, nil
			}
		}
	}
	return nil, fmt.Errorf("direct grant lacks an authorized media endpoint")
}

func directRequestURL(direct *store.DirectUpstreamCandidate, path, model string, stream bool, chosen ...*store.DirectEndpoint) (string, *store.DirectEndpoint) {
	endpoint := directEndpointForPath(direct.Endpoints, path)
	if proxy.IsDirectMediaPath(path) {
		endpoint, _ = directSelectedMediaEndpoint(direct, path)
	}
	if len(chosen) > 0 && chosen[0] != nil {
		endpoint = chosen[0]
	}
	if endpoint != nil {
		target := endpoint.URL
		if endpoint.ModelPath {
			action := "generateContent"
			if proxy.DirectProtocolForPath(path) == store.DirectProtocolGeminiEmbeddings {
				action = "embedContent"
				if strings.HasSuffix(path, ":batchEmbedContents") {
					action = "batchEmbedContents"
				}
			} else if stream {
				action = "streamGenerateContent"
			}
			target = strings.TrimRight(target, "/") + "/" + url.PathEscape(strings.TrimPrefix(model, "models/")) + ":" + action
			if stream && action == "streamGenerateContent" {
				target += "?alt=sse"
			}
		} else if proxy.DirectProtocolForPath(path) == store.DirectProtocolVideo {
			target = strings.TrimRight(target, "/") + strings.TrimPrefix(path, "/v1/videos")
		} else if strings.HasSuffix(path, "/count_tokens") {
			target = strings.TrimRight(target, "/") + "/count_tokens"
		}
		return target, endpoint
	}
	if proxy.IsDirectMediaPath(path) {
		return "", nil
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
