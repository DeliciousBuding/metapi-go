package store

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
)

func TestDirectDecisionsIndependentEndpoint(t *testing.T) {
	var endpoints DirectEndpoints
	if err := endpoints.Scan(`{"decisions":{"url":"https://fixture.invalid/custom/decide","auth":"bearer"}}`); err != nil {
		t.Fatal(err)
	}
	const bit = 1 << 26
	if endpoints.ProtocolMask() != bit || endpoints.ForProtocol(bit) == nil || !ValidDirectProtocol(bit) {
		t.Fatal("Decisions must retain its own persisted grant and endpoint")
	}
	value, err := endpoints.Value()
	if err != nil {
		t.Fatal(err)
	}
	var loaded DirectEndpoints
	if err := loaded.Scan(value); err != nil || loaded.ProtocolMask() != bit {
		t.Fatalf("round trip failed: %v", err)
	}
	for _, raw := range []string{
		`{"decisions":{"url":"wss://fixture.invalid/decide","auth":"bearer"}}`,
		`{"decisions":{"url":"https://fixture.invalid/decide","auth":"x-api-key"}}`,
		`{"decisions":{"url":"https://fixture.invalid/decide","auth":"bearer","profile":"codex"}}`,
		`{"decisions":{"url":"https://fixture.invalid/decide","auth":"bearer","modelPath":true}}`,
	} {
		if loaded.Scan(raw) == nil {
			t.Fatal("accepted invalid Decisions transport/profile")
		}
	}
}

func TestDirectDecisionsAllowsOrderedJSONOperations(t *testing.T) {
	config := DirectRequestConfig{}
	config.Body = []requestoverride.Operation{{Op: "set", Path: "metadata.fixture", Value: "true"}}
	endpoints := DirectEndpoints{Decisions: &DirectEndpoint{URL: "https://fixture.invalid/decide", Auth: DirectAuthBearer}}
	if err := config.ValidateEndpoints(endpoints); err != nil {
		t.Fatal(err)
	}
	endpoints.AudioTranscription = &DirectEndpoint{URL: "https://fixture.invalid/audio", Auth: DirectAuthBearer}
	if err := config.ValidateEndpoints(endpoints); err == nil {
		t.Fatal("JSON operations acquired a multipart endpoint permission")
	}
}
