package store

import "testing"

func TestDirectResponsesTransportSlotsRejectConfusion(t *testing.T) {
	for _, raw := range []string{
		`{"responsesWebsocket":{"url":"wss://fixture.invalid/responses","auth":"bearer","futureTransport":true}}`,
		`{"responsesFuture":{"url":"wss://fixture.invalid/responses","auth":"bearer"}}`,
		`{"responsesWebsocket":{"url":"https://fixture.invalid/responses","auth":"bearer"}}`,
		`{"responses":{"url":"wss://fixture.invalid/responses","auth":"bearer"}}`,
		`{"responsesCompact":{"url":"wss://fixture.invalid/compact","auth":"bearer"}}`,
		`{"responsesCompact":{"url":"https://fixture.invalid/compact","auth":"bearer","profile":"codex"}}`,
		`{"responsesWebsocket":{"url":"wss://fixture.invalid/responses","auth":"none"}}`,
		`{"responsesWebsocket":{"url":"wss://fixture.invalid/responses","auth":"bearer","modelPath":true}}`,
	} {
		var config DirectEndpoints
		if config.Scan(raw) == nil {
			t.Fatalf("cross-transport configuration accepted: %s", raw)
		}
	}
}
