package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestUpstreamConnectSelectedProtocolsShareConnection(t *testing.T) {
	previous := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(previous) })
	previousRuntime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{RoutingFallbackUnitCost: 1})
	t.Cleanup(func() { config.SetRuntime(previousRuntime) })
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, presetID := range []string{"custom", "sub2api", "vllm", "sglang", "groq"} {
			t.Run(dialect+"/"+presetID, func(t *testing.T) {
				db := catalogTestDB(t, dialect, ":memory:")
				advanced := presetID == "groq"
				messagesPath, responsesPath := "/v1/messages", "/v1/responses"
				if advanced {
					messagesPath, responsesPath = "/custom/anthropic", "/custom/replies"
				}
				var calls [3]atomic.Int32
				upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if advanced && r.Header.Get("X-Tenant") != "fixture-private-tenant" {
						t.Error("advanced request headers did not reach discovery or relay")
					}
					wantAPIKey := (presetID == "custom" || advanced) && r.URL.Path == messagesPath
					if wantAPIKey {
						if r.Header.Get("X-Api-Key") != "fixture-multi-key" || r.Header.Get("Authorization") != "" {
							t.Error("custom Messages credential did not retain its native header")
						}
					} else if r.Header.Get("Authorization") != "Bearer fixture-multi-key" {
						t.Error("provider requires Bearer authentication, including Messages")
					}
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/v1/models" {
						fmt.Fprint(w, `{"data":[{"id":"fixture-multi-model"}]}`)
						return
					}
					var body map[string]any
					if json.NewDecoder(r.Body).Decode(&body) != nil || body["model"] != "fixture-multi-model" {
						t.Error("invalid native request model")
					}
					if advanced && body["temperature"] != 0.25 {
						t.Error("advanced parameter override did not reach the selected protocol")
					}
					switch r.URL.Path {
					case "/v1/chat/completions":
						calls[0].Add(1)
						if body["messages"] == nil || body["fixture_native"] != "chat" {
							t.Error("Chat request was not preserved")
						}
						fmt.Fprint(w, `{"id":"chat-fixture","model":"fixture-multi-model","choices":[{"message":{"role":"assistant","content":"chat receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
					case responsesPath:
						calls[1].Add(1)
						if body["input"] == nil || body["fixture_native"] != "responses" {
							t.Error("Responses request was not preserved")
						}
						fmt.Fprint(w, `{"id":"resp_fixture","object":"response","model":"fixture-multi-model","status":"completed","output":[{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"responses receipt","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
					case messagesPath:
						calls[2].Add(1)
						if body["system"] != "native system" || body["fixture_native"] != "messages" || r.Header.Get("Anthropic-Version") != "2023-06-01" {
							t.Error("Messages request or version header was not preserved")
						}
						fmt.Fprint(w, `{"id":"msg_fixture","type":"message","role":"assistant","model":"fixture-multi-model","content":[{"type":"text","text":"messages receipt"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":1}}`)
					default:
						t.Errorf("unselected endpoint reached: %s", r.URL.Path)
						http.NotFound(w, r)
					}
				}))
				t.Cleanup(upstreamServer.Close)
				input := map[string]any{
					"presetId": presetID, "baseUrl": upstreamServer.URL, "apiKey": "fixture-multi-key", "protocols": []string{"messages", "chat", "responses"},
				}
				if advanced {
					defaults := service.UpstreamPresetEndpointPaths(presetID, "openai")
					if defaults.Messages != nil || defaults.Responses != nil {
						t.Fatal("advanced fixture must exercise protocols absent from the template")
					}
					delete(input, "protocols")
					input["endpointConfig"] = store.DirectEndpoints{
						Chat:      &store.DirectEndpoint{URL: upstreamServer.URL + "/v1/chat/completions", Auth: store.DirectAuthBearer},
						Responses: &store.DirectEndpoint{URL: upstreamServer.URL + responsesPath, Auth: store.DirectAuthBearer},
						Messages:  &store.DirectEndpoint{URL: upstreamServer.URL + messagesPath, Auth: store.DirectAuthAPIKey},
					}
					input["customHeaders"] = `[{"header_key":"X-Tenant","header_value":"fixture-private-tenant"}]`
					input["paramOverride"] = `{"temperature":0.25,"model":"must-not-replace-selected-model"}`
				}
				out := catalogCall(t, catalogMux(db), http.MethodPost, connectPath, input, http.StatusCreated)
				if out["modelCount"] != float64(1) || out["routeCount"] != float64(1) {
					t.Fatalf("one shared model route expected: %+v", out)
				}
				if advanced {
					detail := catalogRequest(catalogMux(db), http.MethodGet, fmt.Sprintf("%s/%d", catalogPrefix, catalogID(out)), nil)
					if detail.Code != http.StatusOK || strings.Contains(detail.Body.String(), "fixture-private-tenant") || strings.Contains(detail.Body.String(), "must-not-replace-selected-model") {
						t.Fatal("advanced settings leaked into the channel detail response")
					}
				}
				var endpoints store.DirectEndpoints
				if err := db.Get(&endpoints, `SELECT endpoint_config FROM upstream_channels`); err != nil {
					t.Fatal(err)
				}
				var mask, credentials int
				if err := db.Get(&mask, `SELECT protocols FROM upstream_grants`); err != nil {
					t.Fatal(err)
				}
				if err := db.Get(&credentials, `SELECT count(*) FROM upstream_credentials`); err != nil {
					t.Fatal(err)
				}
				if mask != 14 || endpoints.ProtocolMask() != mask || credentials != 1 {
					t.Fatalf("selected protocol authorization changed: mask=%d endpoints=%d credentials=%d", mask, endpoints.ProtocolMask(), credentials)
				}
				router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
				t.Cleanup(func() { routing.SetGlobalCache(nil) })
				proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
				t.Cleanup(func() { proxyhandler.SetUpstreamConfig(nil) })
				for _, request := range []struct {
					path, body, receipt string
					handler             http.HandlerFunc
				}{
					{"/v1/chat/completions", `{"model":"fixture-multi-model","messages":[{"role":"user","content":"hi"}],"fixture_native":"chat"}`, "chat receipt", proxyhandler.HandleChatCompletions},
					{"/v1/responses", `{"model":"fixture-multi-model","input":"hi","fixture_native":"responses"}`, "responses receipt", func(w http.ResponseWriter, r *http.Request) { proxyhandler.HandleResponses(w, r, "/v1/responses") }},
					{"/v1/messages", `{"model":"fixture-multi-model","system":"native system","messages":[{"role":"user","content":"hi"}],"max_tokens":128,"fixture_native":"messages"}`, "messages receipt", proxyhandler.HandleClaudeMessages},
				} {
					r := httptest.NewRequest(http.MethodPost, request.path, strings.NewReader(request.body))
					r.Header.Set("Content-Type", "application/json")
					r.Header.Set("Anthropic-Version", "2023-06-01")
					r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-downstream", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
					w := httptest.NewRecorder()
					request.handler(w, r)
					if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), request.receipt) {
						t.Fatalf("native relay %s status=%d body=%s", request.path, w.Code, w.Body.String())
					}
				}
				for i := range calls {
					if calls[i].Load() != 1 {
						t.Fatalf("protocol %d was converted or never forwarded n=%d", i, calls[i].Load())
					}
				}
			})
		}
	}
}

func TestUpstreamConnectRejectsInvalidProtocolSelection(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				fmt.Fprint(w, `{"data":[]}`)
			}))
			defer server.Close()
			for _, selection := range []any{[]string{}, []string{"chat", "chat"}, []string{"unknown"}, []string{"chat", "messages"}, []string{" chat"}, nil, "chat", []int{2}} {
				out := catalogRequest(catalogMux(db), http.MethodPost, connectPath, map[string]any{
					"presetId": "groq", "baseUrl": server.URL, "apiKey": "fixture-secret-no-echo", "protocols": selection,
				})
				if out.Code != http.StatusBadRequest || strings.Contains(out.Body.String(), "fixture-secret-no-echo") {
					t.Fatalf("invalid selection accepted or reflected: %d %s", out.Code, out.Body.String())
				}
			}
			var count int
			if err := db.Get(&count, `SELECT count(*) FROM upstream_channels`); err != nil || count != 0 || requests.Load() != 0 {
				t.Fatalf("invalid selection made an outbound request or wrote data: n=%d requests=%d err=%v", count, requests.Load(), err)
			}
		})
	}
}

func TestUpstreamConnectRejectsInvalidAdvancedConfig(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				fmt.Fprint(w, `{"data":[]}`)
			}))
			defer server.Close()
			for _, endpoints := range []any{
				nil,
				map[string]any{},
				map[string]any{"unknown": map[string]any{"url": server.URL, "auth": "bearer"}},
				map[string]any{"responses": map[string]any{"url": "http://169.254.169.254/v1/responses", "auth": "bearer"}},
				map[string]any{"messages": map[string]any{"url": server.URL, "auth": "none"}},
				map[string]any{"messages": map[string]any{"url": server.URL, "auth": "bearer", "profile": "codex"}},
				map[string]any{"responses": map[string]any{"url": server.URL, "auth": "bearer", "profile": "codex"}},
			} {
				out := catalogRequest(catalogMux(db), http.MethodPost, connectPath, map[string]any{
					"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture-advanced-key", "endpointConfig": endpoints,
				})
				if out.Code != http.StatusBadRequest || strings.Contains(out.Body.String(), "fixture-advanced-key") {
					t.Fatalf("invalid advanced config accepted or reflected: %d %s", out.Code, out.Body.String())
				}
			}
			both := catalogRequest(catalogMux(db), http.MethodPost, connectPath, map[string]any{
				"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture-advanced-key", "protocols": []string{"chat"},
				"endpointConfig": map[string]any{"chat": map[string]any{"url": server.URL + "/v1/chat/completions", "auth": "bearer"}},
			})
			if both.Code != http.StatusBadRequest {
				t.Fatalf("ambiguous template/custom configuration accepted: %d", both.Code)
			}
			for _, field := range []struct{ name, value string }{
				{"customHeaders", `[{"header_key":"Authorization","header_value":"Bearer fixture-private"}]`},
				{"customHeaders", `[{"header_key":"X-Tenant","header_value":"bad\r\nheader"}]`},
				{"customHeaders", `{"invalid":"shape"}`},
				{"paramOverride", `{"duplicate":1,"duplicate":2}`},
				{"paramOverride", `[]`},
			} {
				input := map[string]any{"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture-advanced-key", field.name: field.value}
				out := catalogRequest(catalogMux(db), http.MethodPost, connectPath, input)
				if out.Code != http.StatusBadRequest || strings.Contains(out.Body.String(), "fixture-private") {
					t.Fatalf("invalid %s accepted or reflected: %d %s", field.name, out.Code, out.Body.String())
				}
			}
			var count int
			if err := db.Get(&count, `SELECT count(*) FROM upstream_channels`); err != nil || count != 0 || requests.Load() != 0 {
				t.Fatalf("invalid custom configuration escaped validation: channels=%d requests=%d err=%v", count, requests.Load(), err)
			}
		})
	}
}

func TestUpstreamConnectAdvancedOllamaPreservesAuthentication(t *testing.T) {
	db := catalogTestDB(t, store.DialectSQLite, ":memory:")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" || r.Header.Get("Authorization") != "" {
			t.Error("advanced anonymous catalog was rewritten by the preset credential default")
		}
		fmt.Fprint(w, `{"models":[{"name":"fixture-mixed-auth"}]}`)
	}))
	defer server.Close()
	endpoints := store.DirectEndpoints{
		Ollama:   &store.DirectEndpoint{URL: server.URL + "/api/chat", Auth: store.DirectAuthBearer, Profile: "ollama"},
		Messages: &store.DirectEndpoint{URL: server.URL + "/v1/messages", Auth: store.DirectAuthNone, Profile: "ollama-messages"},
	}
	result := catalogCall(t, catalogMux(db), http.MethodPost, connectPath, map[string]any{
		"presetId": "ollama-native", "baseUrl": server.URL, "apiKey": "fixture-optional-key", "endpointConfig": endpoints,
	}, http.StatusCreated)
	var stored store.DirectEndpoints
	if err := db.Get(&stored, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE id=?`), catalogID(result)); err != nil {
		t.Fatal(err)
	}
	if stored.Ollama.Auth != store.DirectAuthBearer || stored.Messages.Auth != store.DirectAuthNone {
		t.Fatal("advanced endpoint authentication was replaced by template defaults")
	}
}
