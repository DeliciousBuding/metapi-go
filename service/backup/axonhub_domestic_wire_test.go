package backup

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubDomesticWireProfiles(t *testing.T) {
	for _, provider := range []string{"moonshot", "longcat", "openrouter", "cerebras", "nanogpt"} {
		for _, custom := range []bool{false, true} {
			for _, path := range []string{"", "/custom"} {
				if !custom && path != "" {
					continue
				}
				t.Run(provider+"/custom="+map[bool]string{true: "true", false: "false"}[custom]+path, func(t *testing.T) {
					ch := AxonHubSourceChannel{ID: 1, Type: provider, BaseURL: "https://fixture.invalid/api", Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
					if custom {
						ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "openai/chat_completions", Path: path}}
					}
					compiled, reasons, _ := compileAxonHubChannel(ch)
					if compiled == nil || len(reasons) != 0 {
						t.Fatalf("reasons=%v", reasons)
					}
					profile, target := provider, "https://fixture.invalid/api/v1/chat/completions"
					if !custom && (provider == "openrouter" || provider == "cerebras") {
						target = "https://fixture.invalid/api/chat/completions"
					}
					if custom {
						profile = ""
						if path != "" {
							target = "https://fixture.invalid/api" + path
						}
					}
					if compiled.Endpoints.Chat.Profile != profile || compiled.Endpoints.Chat.URL != target || compiled.Endpoints.Chat.Auth != store.DirectAuthBearer {
						t.Fatalf("endpoint=%+v want profile=%q url=%q", compiled.Endpoints.Chat, profile, target)
					}
				})
			}
		}
	}
}

func TestAxonHubDomesticWireURLModes(t *testing.T) {
	for _, tc := range []struct {
		provider, marker, want, reason string
		custom                         bool
	}{
		{"moonshot", "#", "https://fixture.invalid/api/chat/completions", "", false},
		{"moonshot", "##", "", "endpoint_url_mode_unsupported", false},
		{"moonshot", "##", "https://fixture.invalid/api/", "", true},
		{"longcat", "#", "https://fixture.invalid/api/chat/completions", "", false},
		{"longcat", "##", "https://fixture.invalid/api/", "", false},
		{"nanogpt", "#", "https://fixture.invalid/api/chat/completions", "", false},
		{"nanogpt", "##", "https://fixture.invalid/api/", "", false},
		{"openrouter", "#", "", "endpoint_url_mode_unsupported", false},
		{"openrouter", "##", "", "endpoint_url_mode_unsupported", false},
		{"openrouter", "##", "https://fixture.invalid/api/", "", true},
		{"cerebras", "#", "", "endpoint_url_mode_unsupported", false},
		{"cerebras", "##", "", "endpoint_url_mode_unsupported", false},
		{"cerebras", "##", "https://fixture.invalid/api/", "", true},
	} {
		t.Run(tc.provider+tc.marker+map[bool]string{true: "custom", false: "default"}[tc.custom], func(t *testing.T) {
			ch := AxonHubSourceChannel{Type: tc.provider, BaseURL: "https://fixture.invalid/api/" + tc.marker, Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
			if tc.custom {
				ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "openai/chat_completions", Path: "/ignored"}}
			}
			compiled, reasons, _ := compileAxonHubChannel(ch)
			if tc.reason != "" {
				if compiled != nil || !reflect.DeepEqual(reasons, []string{tc.reason}) {
					t.Fatalf("compiled=%+v reasons=%v", compiled, reasons)
				}
				return
			}
			if compiled == nil || len(reasons) != 0 || compiled.Endpoints.Chat.URL != tc.want {
				t.Fatalf("compiled=%+v reasons=%v", compiled, reasons)
			}
		})
	}
}

func TestAxonHubCerebrasPrimaryDefaultDoesNotSupplyCustomBase(t *testing.T) {
	ch := AxonHubSourceChannel{Type: "cerebras", Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
	compiled, reasons, _ := compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 || compiled.Endpoints.Chat.URL != "https://api.cerebras.ai/v1/chat/completions" {
		t.Fatalf("primary constructor default missing: %+v %v", compiled, reasons)
	}
	ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "openai/chat_completions", Path: "/custom"}}
	compiled, reasons, _ = compileAxonHubChannel(ch)
	if compiled != nil || !reflect.DeepEqual(reasons, []string{"source_custom_endpoint_requires_base_url"}) {
		t.Fatalf("primary default leaked into custom constructor: %+v %v", compiled, reasons)
	}
	ch.Endpoints[0].BaseURL = "https://custom.invalid/api"
	compiled, reasons, _ = compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 || compiled.Endpoints.Chat.Profile != "" || compiled.Endpoints.Chat.URL != "https://custom.invalid/api/custom" {
		t.Fatalf("explicit custom failed: %+v %v", compiled, reasons)
	}
	ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "gemini/contents"}}
	compiled, reasons, _ = compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 || compiled.Endpoints.Gemini.URL != "https://generativelanguage.googleapis.com/v1beta/models" || ch.Endpoints[0].BaseURL != "" {
		t.Fatalf("Gemini constructor default or source immutability lost: %+v %v", compiled, reasons)
	}
}

func TestAxonHubDomesticFormatsRemainIndependent(t *testing.T) {
	for _, tc := range []struct {
		provider string
		custom   bool
		auth     string
	}{
		{"moonshot_anthropic", false, store.DirectAuthAPIKey}, {"moonshot_coding", false, store.DirectAuthAPIKey},
		{"longcat_anthropic", false, store.DirectAuthBearer}, {"longcat_anthropic", true, store.DirectAuthAPIKey},
	} {
		ch := AxonHubSourceChannel{Type: tc.provider, BaseURL: "https://fixture.invalid/api", Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
		if tc.custom {
			ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "anthropic/messages"}}
		}
		compiled, reasons, _ := compileAxonHubChannel(ch)
		if compiled == nil || len(reasons) != 0 || compiled.Endpoints.Messages.Auth != tc.auth || compiled.Endpoints.Messages.Profile != "" {
			t.Fatalf("Messages profile/auth changed: %+v %v", compiled, reasons)
		}
	}
	ch := AxonHubSourceChannel{Type: "nanogpt_responses", BaseURL: "https://nano.invalid/exact/##", Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
	compiled, reasons, _ := compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 || compiled.Protocols != protoResponses || compiled.Endpoints.Responses.Profile != "" || compiled.Endpoints.Responses.URL != "https://nano.invalid/exact/" {
		t.Fatalf("NanoGPT Responses acquired Chat normalization: %+v %v", compiled, reasons)
	}
	ch = AxonHubSourceChannel{Type: "nanogpt", BaseURL: "https://nano.invalid/api", Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
	compiled, reasons, _ = compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 {
		t.Fatal(reasons)
	}
	for _, entry := range compiled.Endpoints.Entries() {
		if entry.Endpoint == nil || entry.Protocol == protoChat {
			continue
		}
		if entry.Endpoint.Profile != "" || !strings.HasPrefix(entry.Endpoint.URL, "https://nano.invalid/api/v1/") {
			t.Fatalf("NanoGPT media lost generic constructor: %+v", entry)
		}
	}
	ch.Type = "openrouter"
	compiled, reasons, _ = compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 || compiled.Protocols&(protoImageGeneration|protoImageEdit) != 0 {
		t.Fatalf("OpenRouter gained undeclared image grants: %+v %v", compiled, reasons)
	}
	ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "openai/image_generation", Path: "/generate"}, {APIFormat: "openai/image_edit", Path: "/edit"}}
	compiled, reasons, _ = compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 || compiled.Endpoints.ImageGeneration.Profile != "" || compiled.Endpoints.ImageGeneration.URL != "https://nano.invalid/api/generate" || compiled.Endpoints.ImageEdit.Profile != "" || compiled.Endpoints.ImageEdit.URL != "https://nano.invalid/api/edit" {
		t.Fatalf("OpenRouter custom image falsely acquired wrapper: %+v %v", compiled, reasons)
	}
}

func TestAxonHubDomesticWireImportGraphAndReimport(t *testing.T) {
	for _, provider := range []string{"moonshot", "longcat", "openrouter", "cerebras", "nanogpt"} {
		t.Run(provider, func(t *testing.T) {
			db := openAxonHubTestDB(t)
			channel := map[string]any{"id": 1, "name": "fixture", "type": provider, "base_url": "https://fixture.invalid/api", "credentials": map[string]any{"apiKeys": []string{"fixture-active", "fixture-disabled"}}, "disabled_api_keys": []any{map[string]any{"key": "fixture-disabled"}}, "supported_models": []string{"actual-model"}, "endpoints": []any{map[string]any{"api_format": "openai/responses", "path": "/custom-responses"}}, "settings": map[string]any{"modelProtocols": []any{map[string]any{"model": "native-alias", "apiFormats": []string{"openai/chat_completions"}}, map[string]any{"model": "custom-alias", "apiFormats": []string{"openai/responses"}}}}}
			models := []any{}
			for i, name := range []string{"native-alias", "custom-alias"} {
				models = append(models, map[string]any{"id": i + 1, "model_id": name, "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "actual-model"}}}}})
			}
			payload := map[string]any{"version": "1.4", "channels": []any{channel}, "models": models}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			preview, err := PreviewAxonHubV14(db, raw, "domestic-wire")
			if err != nil || len(preview.Blocking) != 0 || preview.Routable["credentials"] != 2 || preview.Routable["grants"] != 1 || preview.Routable["routes"] != 2 {
				t.Fatalf("preview=%+v err=%v", preview, err)
			}
			if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 0 {
				t.Fatal("preview mutated graph")
			}
			if _, err := ImportAxonHubV14(db, raw, "domestic-wire", false); err != nil {
				t.Fatal(err)
			}
			var ids []int64
			if err := db.Select(&ids, `SELECT id FROM token_routes ORDER BY id`); err != nil {
				t.Fatal(err)
			}
			candidates, err := service.NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), ids)
			if err != nil || len(candidates) != 2 {
				t.Fatalf("candidates=%d err=%v", len(candidates), err)
			}
			for i, order := range []store.DirectProtocolOrder{{protoChat}, {protoResponses}} {
				if !reflect.DeepEqual(candidates[i].Channel.Direct.ProtocolOrder, order) || candidates[i].Channel.Direct.Endpoints.Chat.Profile != provider || candidates[i].Channel.Direct.Endpoints.Responses.Profile != "" {
					t.Fatalf("route scope/profile changed: %+v", candidates[i].Channel.Direct)
				}
			}
			if countRows(t, db, `SELECT COUNT(*) FROM upstream_grants g JOIN upstream_credentials c ON c.id=g.credential_id WHERE c.enabled=?`, false) != 0 {
				t.Fatal("disabled credential acquired grant")
			}
			before, err := axonHubMappedSources(db, db, "domestic-wire")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ImportAxonHubV14(db, raw, "domestic-wire", false); err != nil {
				t.Fatal(err)
			}
			after, err := axonHubMappedSources(db, db, "domestic-wire")
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("reimport changed identity: %v", err)
			}
			channel["endpoints"] = append(channel["endpoints"].([]any), map[string]any{"api_format": "openai/chat_completions", "path": "/custom-chat"})
			raw, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ImportAxonHubV14(db, raw, "domestic-wire", false); err != nil {
				t.Fatal(err)
			}
			var endpoints store.DirectEndpoints
			if err := db.Get(&endpoints, `SELECT endpoint_config FROM upstream_channels`); err != nil {
				t.Fatal(err)
			}
			if endpoints.Chat.Profile != "" || endpoints.Chat.URL != "https://fixture.invalid/api/custom-chat" {
				t.Fatalf("custom overwrite retained default profile: %+v", endpoints.Chat)
			}
			after, err = axonHubMappedSources(db, db, "domestic-wire")
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("endpoint edit changed identity: %v", err)
			}
		})
	}
}
