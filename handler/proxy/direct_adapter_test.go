package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
	"github.com/deliciousbuding/metapi-go/store"
)

func adapterTestObject(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil || object == nil {
		t.Fatalf("expected JSON object: %s (%v)", body, err)
	}
	return object
}

// Existing body fixtures now enter through the same registry as production.
// These test-only adapters retain their focused byte/error assertions without
// exporting individual preparation implementations from the new owner.
func prepareAdapterBody(profile string, body []byte) ([]byte, error) {
	adapter, ok := upstreamadapter.Lookup(profile)
	if !ok {
		return nil, fmt.Errorf("unsupported direct provider profile")
	}
	wire, err := adapter.Prepare(upstreamadapter.Input{Body: body})
	if err != nil {
		return nil, err
	}
	return wire.Body, nil
}

func prepareAdapterProfileBody(body []byte, profile string) ([]byte, error) {
	return prepareAdapterBody(profile, body)
}

func projectAdapterBody(profile string, body []byte, stream bool) ([]byte, error) {
	adapter, ok := upstreamadapter.Lookup(profile)
	if !ok {
		return nil, fmt.Errorf("unsupported direct provider profile")
	}
	return adapter.ProjectResponse(body, stream)
}

func directSessionHeaders(t *testing.T, profile string, downstream http.Header, namespace string) http.Header {
	t.Helper()
	body := []byte(`{"model":"fixture","input":"hello","max_tokens":8,"messages":[{"role":"user","content":"hello"}]}`)
	wire, err := prepareDirectProviderWire(&store.DirectEndpoint{Profile: profile, URL: "https://chatgpt.com/backend-api/codex/responses"}, 1, nil, body, downstream, namespace)
	if err != nil {
		t.Fatal(err)
	}
	return wire.Headers
}
