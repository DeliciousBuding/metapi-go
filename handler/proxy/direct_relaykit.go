package proxyhandler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	"github.com/deliciousbuding/metapi-go/transform/relaykitbridge"
)

// Choose one conversion owner before sending the attempt. Provider-specific
// wire contracts and Messages reasoning replay keep their existing codecs;
// ordinary conversations use RelayKit with one request/response session.
func prepareDirectProtocolRequest(ctx context.Context, endpoint *store.DirectEndpoint, body []byte, downstream, upstream, model string, stream bool, options messages.Options) ([]byte, *relaykitbridge.Session, error) {
	d, dok := proxy.EndpointFromPath(downstream)
	u, uok := proxy.EndpointFromPath(upstream)
	if endpoint != nil && endpoint.Profile == "" && dok && uok && d != proxy.EndpointMessages && u != proxy.EndpointOllama {
		session, converted, err := relaykitbridge.ConvertRequest(ctx, relaykitbridge.Format(d), relaykitbridge.Format(u), model, body)
		if !errors.Is(err, relaykitbridge.ErrSpecialized) {
			if err != nil {
				return nil, nil, err
			}
			// Gemini expresses both fields in the request URL. Other APIs need
			// the selected model and the actual HTTP stream mode in their JSON.
			if u != proxy.EndpointGemini {
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(converted, &payload); err != nil {
					return nil, nil, err
				}
				payload["model"], _ = json.Marshal(model)
				payload["stream"], _ = json.Marshal(stream)
				converted, err = json.Marshal(payload)
			}
			return converted, session, err
		}
	}
	converted, err := directConvertRequest(body, downstream, upstream, model, stream, options)
	return converted, nil, err
}
