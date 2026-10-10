package backup

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubProviderWireProfilesAndURLModes(t *testing.T) {
	for _, tc := range []struct {
		name, provider, base, custom, wantURL, profile string
		internal                                       bool
	}{
		{"Bailian", "bailian", "https://bailian.invalid/compatible-mode/v1", "", "https://bailian.invalid/compatible-mode/v1/chat/completions", "bailian", false},
		{"Bailian marker", "bailian", "https://bailian.invalid/api#", "", "https://bailian.invalid/api/chat/completions", "bailian", false},
		{"Bailian raw", "bailian", "https://bailian.invalid/raw##", "", "https://bailian.invalid/raw", "bailian", false},
		{"Bailian raw trailing slash", "bailian", "https://bailian.invalid/raw/##", "", "https://bailian.invalid/raw/", "bailian", false},
		{"Bailian custom", "bailian", "https://bailian.invalid/api", "/custom", "https://bailian.invalid/api/custom", "", false},
		{"Cline", "cline", "https://cline.invalid/api", "", "https://cline.invalid/api/v1/chat/completions", "cline", false},
		{"Cline custom", "cline", "https://cline.invalid/api", "/custom", "https://cline.invalid/api/custom", "cline", false},
		{"Cline custom raw", "cline", "https://cline.invalid/raw##", "/ignored", "https://cline.invalid/raw", "cline", false},
		{"Cline custom raw trailing slash", "cline", "https://cline.invalid/raw/##", "/ignored", "https://cline.invalid/raw/", "cline", false},
		{"OpenCode", "opencode_go", "https://opencode.invalid/go", "", "https://opencode.invalid/go/v1/chat/completions", "opencode-go", true},
		{"OpenCode marker", "opencode_go", "https://opencode.invalid/go#", "", "https://opencode.invalid/go/chat/completions", "opencode-go", true},
		{"OpenCode custom", "opencode_go", "https://opencode.invalid/go", "/custom", "https://opencode.invalid/go/custom", "", false},
		{"OpenCode custom raw", "opencode_go", "https://opencode.invalid/raw##", "/ignored", "https://opencode.invalid/raw", "", false},
		{"OpenCode custom raw trailing slash", "opencode_go", "https://opencode.invalid/raw/##", "/ignored", "https://opencode.invalid/raw/", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := AxonHubSourceChannel{ID: 1, Type: tc.provider, BaseURL: tc.base, Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
			if tc.custom != "" {
				ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "openai/chat_completions", Path: tc.custom}}
			}
			compiled, reasons, _ := compileAxonHubChannel(ch)
			if compiled == nil || len(reasons) != 0 {
				t.Fatalf("compile=%v reasons=%v", compiled, reasons)
			}
			ep := compiled.Endpoints.Chat
			if ep == nil || ep.URL != tc.wantURL || ep.Profile != tc.profile || ep.Auth != store.DirectAuthBearer || (ep.ModelWireURLs != nil) != tc.internal {
				t.Fatalf("endpoint=%+v", ep)
			}
			if tc.internal {
				prefix := tc.wantURL[:len(tc.wantURL)-len("/chat/completions")]
				if ep.ModelWireURLs.Responses != prefix+"/responses" || ep.ModelWireURLs.Messages != prefix+"/messages" || compiled.Protocols != protoChat || compiled.Endpoints.Responses != nil || compiled.Endpoints.Messages != nil {
					t.Fatalf("internal addresses became independent grants: %+v", compiled)
				}
			}
		})
	}
	for _, provider := range []string{"bailian_responses", "bailian_anthropic"} {
		ch := AxonHubSourceChannel{Type: provider, BaseURL: "https://bailian.invalid/api", Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
		compiled, reasons, _ := compileAxonHubChannel(ch)
		if compiled == nil || len(reasons) != 0 {
			t.Fatal(reasons)
		}
		for _, ep := range compiled.Endpoints.Entries() {
			if ep.Endpoint != nil && ep.Endpoint.Profile != "" {
				t.Fatal("Bailian non-Chat profile was changed")
			}
		}
	}
	ch := AxonHubSourceChannel{Type: "opencode_go", BaseURL: "https://opencode.invalid/raw##", Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
	if compiled, reasons, _ := compileAxonHubChannel(ch); compiled != nil || !reflect.DeepEqual(reasons, []string{"endpoint_url_mode_unsupported"}) {
		t.Fatalf("invalid raw native family was invented: %+v %v", compiled, reasons)
	}
}

func TestAxonHubOpenCodeAnthropicFallbackAndCustomReplacement(t *testing.T) {
	ch := AxonHubSourceChannel{Type: "opencode_go_anthropic", BaseURL: "https://opencode.invalid/go#", Credentials: AxonHubSourceCredentials{APIKey: "fixture-key"}}
	compiled, reasons, _ := compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 || compiled.Protocols != protoMessages || compiled.Endpoints.Messages == nil || compiled.Endpoints.Messages.URL != "https://opencode.invalid/go/messages" || compiled.Endpoints.Messages.Auth != store.DirectAuthAPIKey {
		t.Fatalf("legacy primary outbound was lost: %+v %v", compiled, reasons)
	}
	ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "openai/chat_completions", Path: "/fixed"}}
	compiled, reasons, _ = compileAxonHubChannel(ch)
	if compiled == nil || len(reasons) != 0 || compiled.Protocols != protoChat || compiled.Endpoints.Messages != nil || compiled.Endpoints.Chat.Profile != "" || compiled.Endpoints.Chat.ModelWireURLs != nil {
		t.Fatalf("custom fixed endpoint retained a default fallback: %+v %v", compiled, reasons)
	}
}

func TestAxonHubInternalWireURLValidation(t *testing.T) {
	for i := 0; i < 3; i++ {
		ep := &store.DirectEndpoint{URL: "https://opencode.invalid/v1/chat/completions", Profile: "opencode-go", ModelWireURLs: &store.DirectModelWireURLs{Responses: "https://opencode.invalid/v1/responses", Messages: "https://opencode.invalid/v1/messages"}}
		withUserInfo := (&url.URL{Scheme: "https", Host: "fixture.invalid", Path: "/api", User: url.UserPassword("fixture-user", "fixture-password")}).String()
		for _, invalid := range []string{"http://169.254.169.254/latest/meta-data", "https://fixture.invalid/path?secret=1", withUserInfo} {
			fields := ep.URLFields()
			original := *fields[i]
			*fields[i] = invalid
			if validateAxonHubResolvedEndpoint(ep) == "" {
				t.Fatalf("URL field %d escaped importer validation", i)
			}
			*fields[i] = original
		}
	}
}

func TestAxonHubOpenCodeImportPreservesInternalAddressesAndRoutes(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := []byte(`{"version":"1.4","channels":[{"id":1,"name":"OpenCode Go fixture","type":"opencode_go","base_url":"https://opencode.invalid/go#","credentials":{"apiKeys":["fixture-active","fixture-disabled"]},"disabled_api_keys":[{"key":"fixture-disabled"}],"supported_models":["gpt-actual"],"endpoints":[{"api_format":"openai/responses","base_url":"https://custom.invalid/api","path":"/fixed-responses"}],"settings":{"modelProtocols":[{"model":"dynamic-model","apiFormats":["openai/chat_completions"]},{"model":"fixed-model","apiFormats":["openai/responses"]}]}}],"models":[{"id":1,"model_id":"dynamic-model","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"gpt-actual"}}]}},{"id":2,"model_id":"fixed-model","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"gpt-actual"}}]}}]}`)
	preview, err := PreviewAxonHubV14(db, raw, "provider-wire")
	if err != nil || len(preview.Blocking) != 0 || preview.Routable["channels"] != 1 || preview.Routable["credentials"] != 2 || preview.Routable["grants"] != 1 || preview.Routable["routes"] != 2 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 0 {
		t.Fatal("preview mutated graph")
	}
	if _, err := ImportAxonHubV14(db, raw, "provider-wire", false); err != nil {
		t.Fatal(err)
	}
	var endpoints store.DirectEndpoints
	if err := db.Get(&endpoints, `SELECT endpoint_config FROM upstream_channels`); err != nil {
		t.Fatal(err)
	}
	if endpoints.Chat.ModelWireURLs == nil || endpoints.Chat.ModelWireURLs.Responses != "https://opencode.invalid/go/responses" || endpoints.Responses.URL != "https://custom.invalid/api/fixed-responses" || endpoints.Messages != nil {
		t.Fatalf("custom response rewrote internal transport: %+v", endpoints)
	}
	var routeIDs []int64
	if err := db.Select(&routeIDs, `SELECT id FROM token_routes ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	candidates, err := service.NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), routeIDs)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("candidates=%d err=%v", len(candidates), err)
	}
	for i, order := range []store.DirectProtocolOrder{{protoChat}, {protoResponses}} {
		if !reflect.DeepEqual(candidates[i].Channel.Direct.ProtocolOrder, order) || candidates[i].Channel.Direct.Protocols != protoChat|protoResponses {
			t.Fatalf("route protocol ownership changed: %+v", candidates[i].Channel.Direct)
		}
	}
	before, err := axonHubMappedSources(db, db, "provider-wire")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportAxonHubV14(db, raw, "provider-wire", false); err != nil {
		t.Fatal(err)
	}
	after, err := axonHubMappedSources(db, db, "provider-wire")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("reimport changed source identity: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	channel := payload["channels"].([]any)[0].(map[string]any)
	channel["endpoints"] = append(channel["endpoints"].([]any), map[string]any{"api_format": "openai/chat_completions", "path": "/fixed-chat"})
	raw, err = json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportAxonHubV14(db, raw, "provider-wire", false); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&endpoints, `SELECT endpoint_config FROM upstream_channels`); err != nil {
		t.Fatal(err)
	}
	if endpoints.Chat.Profile != "" || endpoints.Chat.ModelWireURLs != nil || endpoints.Chat.URL != "https://opencode.invalid/go/fixed-chat" {
		t.Fatalf("fixed Chat retained dynamic routing: %+v", endpoints.Chat)
	}
	after, err = axonHubMappedSources(db, db, "provider-wire")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("endpoint update changed source identity: %v", err)
	}
}
