package proxyhandler

import (
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
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
	default:
		return nil
	}
}
