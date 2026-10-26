package proxyhandler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/relaykitbridge"
)

type directProviderWire struct {
	Endpoint        *store.DirectEndpoint
	Body            []byte
	Headers         http.Header
	Query           url.Values
	ForceStream     bool
	StripToolPrefix bool
	Profile         string
	// ImageResponseFormat is the downstream output choice, never sent upstream.
	ImageResponseFormat string
	NanoGPTTools        *upstreamadapter.NanoGPTTools
	ProtocolSession     *relaykitbridge.Session
	removeHeaders       []string
	removeEmptyHeaders  []string
}
type directProviderWireKey struct{}

func withDirectProviderWire(ctx context.Context, wire *directProviderWire) context.Context {
	return context.WithValue(ctx, directProviderWireKey{}, wire)
}
func directProviderWireFromContext(ctx context.Context) *directProviderWire {
	wire, _ := ctx.Value(directProviderWireKey{}).(*directProviderWire)
	return wire
}

// prepareDirectProviderWire adapts the selected storage/credential records to
// the shared wire owner after protocol conversion and parameter overrides.
// Credential resolution, endpoint selection and transport stay with the caller.
func prepareDirectProviderWire(endpoint *store.DirectEndpoint, channelID int64, credential *oauth.DirectCredentialResult, body []byte, downstream http.Header, sessionNamespace string) (*directProviderWire, error) {
	profile := ""
	input := upstreamadapter.Input{Body: body, Downstream: downstream, ChannelID: channelID, SessionNamespace: sessionNamespace}
	if endpoint != nil {
		profile, input.EndpointURL = endpoint.Profile, endpoint.URL
	}
	if credential != nil {
		input.OAuth = credential.Kind == store.DirectCredentialOAuth
		input.AccountID = credential.AccountID
	}
	adapter, ok := upstreamadapter.Lookup(profile)
	if !ok {
		return nil, fmt.Errorf("unsupported direct provider profile")
	}
	prepared, err := adapter.Prepare(input)
	if err != nil {
		return nil, err
	}
	return &directProviderWire{
		Body: prepared.Body, Headers: prepared.Headers, Query: prepared.Query,
		ForceStream: prepared.ForceStream, StripToolPrefix: prepared.StripToolPrefix,
		Profile: profile, NanoGPTTools: prepared.NanoGPTTools,
		removeHeaders: prepared.RemoveHeaders, removeEmptyHeaders: prepared.RemoveEmptyHeaders,
	}, nil
}

// applyDirectProviderHeaders runs after generic request header construction.
// No authorization values are sourced from downstream/provider metadata.
func applyDirectProviderHeaders(req *http.Request, wire *directProviderWire) {
	if wire == nil {
		return
	}
	for _, name := range wire.removeHeaders {
		req.Header.Del(name)
	}
	for name, values := range wire.Headers {
		req.Header[name] = append([]string(nil), values...)
	}
	query := req.URL.Query()
	for name, values := range wire.Query {
		query[name] = append([]string(nil), values...)
	}
	req.URL.RawQuery = query.Encode()
	for _, name := range wire.removeEmptyHeaders {
		if wire.Headers.Get(name) == "" {
			req.Header.Del(name)
		}
	}
}

func restoreDirectProviderResponse(ctx context.Context, body []byte) []byte {
	if wire := directProviderWireFromContext(ctx); wire != nil {
		if adapter, ok := upstreamadapter.Lookup(wire.Profile); ok {
			return adapter.RestoreResponse(body, wire.StripToolPrefix)
		}
	}
	return body
}
