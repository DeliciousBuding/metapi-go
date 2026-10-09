package backup

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubProtocolOrderRetainsRouteScope(t *testing.T) {
	db := openAxonHubTestDB(t)
	var payload map[string]any
	_ = json.Unmarshal(axonHubSettingsPayload(`{"modelMappings":[{"from":"matched","to":"gpt-6"}],"modelProtocols":[{"model":"external","apiFormats":["gemini/contents","anthropic/messages"]},{"model":"matched","apiFormats":["openai/responses","gemini/contents"]},{"model":"gpt-6","apiFormats":["openai/chat_completions"]},{"model":"other","apiFormats":["openai/responses"]}]}`), &payload)
	ch := payload["channels"].([]any)[0].(map[string]any)
	ch["endpoints"] = []any{map[string]any{"api_format": "gemini/contents"}, map[string]any{"api_format": "anthropic/messages"}, map[string]any{"api_format": "openai/responses"}}
	model := func(id int, name, matched string) any {
		return map[string]any{"id": id, "model_id": name, "type": "chat", "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": matched}}}}}
	}
	payload["models"] = []any{model(1, "external", "matched"), model(2, "other", "gpt-6")}
	raw, _ := json.Marshal(payload)
	if _, err := ImportAxonHubV14(db, raw, "protocol-order", false); err != nil {
		t.Fatal(err)
	}
	var routeIDs []int64
	if err := db.Select(&routeIDs, `SELECT id FROM token_routes ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	candidates, err := service.NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), routeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates=%d", len(candidates))
	}
	if candidates[0].Channel.Direct.GrantID != candidates[1].Channel.Direct.GrantID {
		t.Fatal("same credential/model should retain shared grant")
	}
	wants := []store.DirectProtocolOrder{{16, 8, 4, 2}, {4, 2}}
	for i, candidate := range candidates {
		if !reflect.DeepEqual(candidate.Channel.Direct.ProtocolOrder, wants[i]) {
			t.Fatalf("route %d order=%v want %v", i, candidate.Channel.Direct.ProtocolOrder, wants[i])
		}
	}
}

func TestAxonHubProtocolOverrideDoesNotEnableMissingSurface(t *testing.T) {
	for _, tc := range []struct {
		settings string
		routes   int
	}{
		{`{"modelProtocols":[{"model":"gpt-6","apiFormats":["openai/embeddings"]}]}`, 0},
		{`{"modelProtocols":[{"model":"gpt-6","apiFormats":["openai/embeddings"],"enabled":false}]}`, 1},
		{`{"modelProtocols":[{"model":"gpt-6","apiFormats":["future/not-configured"]}]}`, 1},
	} {
		src, err := ParseAxonHubSource(axonHubSettingsPayload(tc.settings))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := CompileAxonHubPlan(src)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.routes) != tc.routes {
			t.Fatalf("routes=%d want=%d residuals=%v", len(plan.routes), tc.routes, plan.residuals)
		}
	}
	src, err := ParseAxonHubSource(axonHubSettingsPayload(`{"modelProtocols":[{"model":"gpt-6","apiFormats":["openai/chat_completions"],"future":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.skipped) != 1 {
		t.Fatal("unknown nested protocol semantics silently accepted")
	}
}
