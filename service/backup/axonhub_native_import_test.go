package backup

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func axonHubNativeFixture(t *testing.T) []byte {
	t.Helper()
	channels := []map[string]any{
		{"type": "ollama", "base_url": "http://localhost:11434", "credentials": map[string]any{}},
		{"type": "ollama_anthropic", "base_url": "http://127.0.0.1:11434", "credentials": map[string]any{}},
		{"type": "anthropic_aws", "base_url": "https://bedrock.invalid", "credentials": map[string]any{"apiKey": "fixture-key"}},
		{"type": "doubao", "base_url": "https://ark.invalid/api/v3", "credentials": map[string]any{"apiKeys": []string{"fixture-active", "fixture-disabled"}}, "disabled_api_keys": []any{map[string]any{"key": "fixture-disabled"}}},
		{"type": "zenmux_video", "base_url": "https://zenmux.invalid/api", "credentials": map[string]any{"apiKey": "fixture-key"}, "endpoints": []any{map[string]any{"api_format": "openai/video"}}},
		{"type": "typesafe", "base_url": "https://typesafe.invalid", "credentials": map[string]any{"apiKey": "fixture-key"}},
		{"type": "codex", "base_url": "https://codex.invalid/api#", "credentials": map[string]any{"oauth": map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh"}}, "endpoints": []any{map[string]any{"api_format": "openai/alpha_search", "path": "/lookup"}}},
		{"type": "ollama", "status": "disabled", "base_url": "https://ollama.invalid", "credentials": map[string]any{"apiKey": "fixture-disabled"}, "disabled_api_keys": []any{map[string]any{"key": "fixture-disabled"}}},
	}
	orders := [][]string{{"ollama/chat"}, {"anthropic/messages"}, {"anthropic/messages"}, {"seedance/video"}, {"openai/video", "zenmux/video"}, {"typesafe/systemone"}, {"openai/alpha_search"}, {"ollama/chat"}}
	models := make([]map[string]any, 0, len(channels))
	for i, ch := range channels {
		id := i + 1
		name := ch["type"].(string)
		if id == 8 {
			name = "disabled-ollama"
		}
		ch["id"], ch["name"] = id, name
		ch["supported_models"] = []string{"provider-model", "unassociated-model"}
		ch["settings"] = map[string]any{"modelProtocols": []any{map[string]any{"model": name, "apiFormats": orders[i]}}}
		kind := "chat"
		if id == 4 || id == 5 {
			kind = "video_generation"
		}
		models = append(models, map[string]any{"id": id, "model_id": name, "type": kind, "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": id, "modelId": "provider-model"}}}}})
	}
	raw, err := json.Marshal(map[string]any{"version": "1.4", "channels": channels, "models": models})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAxonHubNativeImportGraphAndReplacement(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := axonHubNativeFixture(t)
	preview, err := PreviewAxonHubV14(db, raw, "native-formats")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocking) != 0 || len(preview.SkippedChannels) != 0 || preview.Routable["channels"] != 8 || preview.Routable["credentials"] != 9 || preview.Routable["routes"] != 7 || preview.Routable["models"] != 7 || preview.Routable["grants"] != 7 {
		t.Fatalf("unexpected native preview: %+v", preview)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 0 {
		t.Fatal("preview mutated native graph")
	}
	counts, err := ImportAxonHubV14(db, raw, "native-formats", false)
	if err != nil {
		t.Fatal(err)
	}
	for kind, want := range preview.Routable {
		if counts[kind] != int64(want) {
			t.Fatalf("preview/import %s: %d/%d", kind, want, counts[kind])
		}
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials WHERE kind=? AND secret='' AND enabled=?`, store.DirectCredentialNone, true) != 2 {
		t.Fatal("local Ollama authentication was not preserved")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_grants g JOIN upstream_credentials c ON c.id=g.credential_id WHERE c.enabled=?`, false) != 0 {
		t.Fatal("disabled credentials acquired native grants")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE name=? AND enabled=?`, "disabled-ollama", false) != 1 {
		t.Fatal("disabled channel was enabled")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials c JOIN upstream_channels ch ON ch.id=c.channel_id WHERE ch.name=? AND c.kind=?`, "disabled-ollama", store.DirectCredentialNone) != 0 {
		t.Fatal("disabled API key became anonymous")
	}
	var routeIDs []int64
	if err := db.Select(&routeIDs, `SELECT id FROM token_routes ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	candidates, err := service.NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), routeIDs)
	if err != nil {
		t.Fatal(err)
	}
	wantOrders := []store.DirectProtocolOrder{{protoOllama}, {protoMessages}, {protoMessages}, {protoSeedanceVideo}, {protoVideo, protoZenmuxVideo}, {protoSystemOne}, {protoAlphaSearch}}
	if len(candidates) != len(wantOrders) {
		t.Fatalf("native candidates=%d want=%d", len(candidates), len(wantOrders))
	}
	for i, candidate := range candidates {
		if candidate.Channel.Direct == nil || !reflect.DeepEqual(candidate.Channel.Direct.ProtocolOrder, wantOrders[i]) {
			t.Fatalf("native route %d lost format order: %+v", i, candidate.Channel.Direct)
		}
	}
	var endpointConfig store.DirectEndpoints
	if err := db.Get(&endpointConfig, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE name=?`), "codex"); err != nil {
		t.Fatal(err)
	}
	if endpointConfig.AlphaSearch == nil || endpointConfig.AlphaSearch.URL != "https://codex.invalid/api/lookup" || endpointConfig.AlphaSearch.Profile != "codex-alpha-search" {
		t.Fatalf("Codex alpha endpoint: %+v", endpointConfig.AlphaSearch)
	}
	before, err := axonHubMappedSources(db, db, "native-formats")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE upstream_grants SET success_count=7`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportAxonHubV14(db, raw, "native-formats", false); err != nil {
		t.Fatal(err)
	}
	after, err := axonHubMappedSources(db, db, "native-formats")
	if err != nil || !reflect.DeepEqual(before, after) || countRows(t, db, `SELECT COUNT(*) FROM upstream_grants WHERE success_count=7`) != 7 {
		t.Fatalf("reimport lost native identity/state: %v", err)
	}
	if _, err := ImportAxonHubV14(db, raw, "native-conflict", false); err == nil {
		t.Fatal("second origin stole native routes")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key=?`, "native-conflict") != 0 || countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 8 {
		t.Fatal("native origin conflict left partial writes")
	}
	reduced := removeAxonHubChannelAndItsModels(t, raw, 1)
	if _, err := ImportAxonHubV14(db, reduced, "native-formats", false); !errors.Is(err, ErrAxonHubReplacementRequired) {
		t.Fatalf("replacement gate=%v", err)
	}
	if _, err := ImportAxonHubV14(db, reduced, "native-formats", true); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 7 || countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials WHERE kind=?`, store.DirectCredentialNone) != 1 || countRows(t, db, `SELECT COUNT(*) FROM token_routes`) != 6 {
		t.Fatal("replacement left stale native entities")
	}
}

func TestAxonHubOllamaDisabledKeysNeverBecomeAnonymous(t *testing.T) {
	for _, provider := range []string{"ollama", "ollama_anthropic"} {
		ch := AxonHubSourceChannel{ID: 1, Name: "disabled keys", Type: provider, BaseURL: "https://ollama.invalid", Credentials: AxonHubSourceCredentials{APIKeys: []string{"fixture-key"}}, DisabledAPIKeys: []AxonHubSourceDisabledKey{{Key: "fixture-key"}}}
		plan, err := CompileAxonHubPlan(&AxonHubSource{Channels: []AxonHubSourceChannel{ch}})
		if err != nil || len(plan.credentials) != 1 || plan.credentials[0].Kind != store.DirectCredentialAPIKey || plan.credentials[0].Enabled || len(plan.grants) != 0 {
			t.Fatalf("%s disabled-key plan=%+v err=%v", provider, plan, err)
		}
	}
}
