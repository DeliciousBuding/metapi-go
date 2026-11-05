package backup

import (
	"encoding/json"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubCompactDeclaredEndpoint(t *testing.T) {
	channel := AxonHubSourceChannel{Type: "qiniu", BaseURL: "https://provider.invalid", Endpoints: []AxonHubSourceEndpoint{{APIFormat: "openai/responses_compact"}}}
	protocols, endpoints, problems, residuals := resolveChannelEndpoints(channel, axonHubProviderTypes[channel.Type])
	raw, _ := json.Marshal(endpoints)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if len(problems) != 0 || fields["responsesCompact"] == nil || protocols&(1<<24) == 0 || residualContains(residuals, "openai/responses_compact") {
		t.Fatalf("compact endpoint not executable: %s protocols=%d problems=%v residuals=%v", raw, protocols, problems, residuals)
	}
	var endpoint store.DirectEndpoint
	_ = json.Unmarshal(fields["responsesCompact"], &endpoint)
	if endpoint.URL != "https://provider.invalid/v1/responses/compact" || endpoint.Auth != store.DirectAuthBearer {
		t.Fatalf("compact endpoint changed source URL/auth: %+v", endpoint)
	}
}

func TestAxonHubResponsesTransportPermissionAndReimport(t *testing.T) {
	for _, format := range []string{"openai/responses_websocket", "openai/responses"} {
		t.Run(format, func(t *testing.T) {
			db := openAxonHubTestDB(t)
			document := map[string]any{"version": "1.4", "channels": []any{map[string]any{"id": 1, "name": "WS fixture", "type": "openai_responses", "base_url": "https://fixture.invalid", "credentials": map[string]any{"apiKey": "synthetic-ws"}, "supported_models": []string{"model"}, "endpoints": []any{map[string]any{"api_format": format, "transport": "websocket"}}}}, "models": []any{map[string]any{"id": 1, "model_id": "public", "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "model"}}}}}}}
			raw, _ := json.Marshal(document)
			for range 2 {
				if _, err := ImportAxonHubV14(db, raw, "ws-import", false); err != nil {
					t.Fatal(err)
				}
			}
			var endpoints store.DirectEndpoints
			if err := db.Get(&endpoints, `SELECT endpoint_config FROM upstream_channels`); err != nil {
				t.Fatal(err)
			}
			if endpoints.ResponsesWebsocket == nil || endpoints.ResponsesWebsocket.URL != "wss://fixture.invalid/v1/responses" {
				t.Fatalf("WS target missing: %+v", endpoints)
			}
			var mask int
			if err := db.Get(&mask, `SELECT protocols FROM upstream_grants`); err != nil {
				t.Fatal(err)
			}
			if mask&protoResponsesWebsocket == 0 {
				t.Fatal("WS grant missing")
			}
			if format == "openai/responses" && mask&protoResponses != 0 {
				t.Fatal("transport replacement retained unauthorized HTTP")
			}
		})
	}
}
