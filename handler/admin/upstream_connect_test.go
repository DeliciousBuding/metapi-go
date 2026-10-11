package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

const connectPath = catalogPrefix + "/connect"

func TestUpstreamConnectHTTPAndRelay(t *testing.T) {
	previous := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	defer config.Set(previous)
	previousRuntime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{RoutingFallbackUnitCost: 1})
	defer config.SetRuntime(previousRuntime)
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			mux := catalogMux(db)
			var discoveries, relays atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-connect-key" {
					t.Error("wrong credential reached provider")
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/models":
					discoveries.Add(1)
					fmt.Fprint(w, `{"data":[{"id":"fixture-connect-model"}]}`)
				case "/v1/chat/completions":
					relays.Add(1)
					var body struct {
						Model string `json:"model"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "fixture-connect-model" {
						t.Error("wrong model reached provider")
					}
					fmt.Fprint(w, `{"id":"fixture","model":"fixture-connect-model","choices":[{"message":{"role":"assistant","content":"connected receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
				default:
					t.Errorf("unexpected wire path %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			response := catalogCall(t, mux, "POST", connectPath, map[string]any{"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture-connect-key"}, 201)
			if response["ownership"] != "native" || response["enabled"] != true || response["modelCount"] != float64(1) || response["routeCount"] != float64(1) || response["discovery"].(map[string]any)["status"] != "discovered" {
				t.Fatalf("unexpected response %+v", response)
			}
			if len(response) != 7 {
				t.Fatalf("unexpected response fields %+v", response)
			}
			encoded, _ := json.Marshal(response)
			if strings.Contains(string(encoded), "fixture-connect-key") {
				t.Fatal("key in response")
			}
			var mask int
			if err := db.Get(&mask, `SELECT protocols FROM upstream_grants`); err != nil {
				t.Fatal(err)
			}
			if mask != service.UpstreamPresetEndpointPaths("new-api-connection", "new-api").ProtocolMask() {
				t.Fatal("preset protocols were lost")
			}
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
			defer routing.SetGlobalCache(nil)
			for _, protocol := range []int{store.DirectProtocolChat, store.DirectProtocolResponses, store.DirectProtocolMessages, store.DirectProtocolGemini} {
				policy := routing.EmptyDownstreamRoutingPolicy
				policy.RequiredUpstreamProtocol = protocol
				selected, err := router.SelectChannel(t.Context(), "fixture-connect-model", policy)
				if err != nil || selected == nil {
					t.Fatalf("protocol %d unavailable: %v", protocol, err)
				}
			}
			proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
			defer proxyhandler.SetUpstreamConfig(nil)
			down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-downstream", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
				proxyhandler.HandleChatCompletions(w, r)
			}))
			defer down.Close()
			relay, err := http.Post(down.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"fixture-connect-model","messages":[{"role":"user","content":"hello"}]}`))
			if err != nil {
				t.Fatal(err)
			}
			body, _ := io.ReadAll(relay.Body)
			relay.Body.Close()
			if relay.StatusCode != 200 || !strings.Contains(string(body), "connected receipt") || discoveries.Load() != 1 || relays.Load() != 1 {
				t.Fatalf("relay status=%d discoveries=%d relays=%d", relay.StatusCode, discoveries.Load(), relays.Load())
			}
		})
	}
}

func TestUpstreamConnectHTTPValidationAndDiscoveryStatus(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			mux := catalogMux(db)
			var requests atomic.Int32
			var status atomic.Int32
			status.Store(404)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(int(status.Load()))
				fmt.Fprint(w, "fixture-key-never-echo")
			}))
			defer server.Close()
			for _, body := range []any{
				`{"presetId":"unknown","apiKey":"fixture-key-never-echo"}`,
				map[string]any{"presetId": "new-api-connection", "baseUrl": server.URL},
				map[string]any{"presetId": "new-api-connection", "baseUrl": "http://169.254.169.254", "apiKey": "fixture-key-never-echo"},
				map[string]any{"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture\r\nkey"},
				map[string]any{"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture-key-never-echo", "channelProxy": "http://169.254.169.254"},
				`{"presetId":"new-api-connection","presetId":"unknown"}`,
				`{"presetId":"new-api-connection","apiKey":null}`,
				`{"presetId":"new-api-connection","endpointConfig":{}}`,
			} {
				out := catalogRequest(mux, "POST", connectPath, body)
				if out.Code != 400 || strings.Contains(out.Body.String(), "fixture-key-never-echo") {
					t.Fatalf("invalid/unsafe input response %d %s", out.Code, out.Body.String())
				}
			}
			if requests.Load() != 0 {
				t.Fatal("invalid requests reached upstream")
			}
			for _, rejected := range []int32{401, 403} {
				status.Store(rejected)
				out := catalogRequest(mux, "POST", connectPath, map[string]any{"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture-key-never-echo"})
				if out.Code != 400 || strings.Contains(out.Body.String(), "fixture-key-never-echo") {
					t.Fatalf("unsafe authentication response %d %s", out.Code, out.Body.String())
				}
			}
			var channels int
			if err := db.Get(&channels, `SELECT COUNT(*) FROM upstream_channels`); err != nil || channels != 0 {
				t.Fatal("failed connection persisted")
			}
			status.Store(404)
			empty := catalogCall(t, mux, "POST", connectPath, map[string]any{"presetId": "new-api-connection", "baseUrl": server.URL, "apiKey": "fixture-key-never-echo"}, 201)
			if empty["discovery"].(map[string]any)["status"] != "empty" || empty["modelCount"] != float64(0) {
				t.Fatal("empty discovery hidden")
			}
			for _, preset := range service.ListUpstreamPresets() {
				if preset.CredentialMode == "oauth" {
					out := catalogRequest(mux, "POST", connectPath, map[string]any{"presetId": preset.ID, "baseUrl": server.URL, "apiKey": "fixture-key-never-echo"})
					if out.Code != 400 || !strings.Contains(out.Body.String(), "OAuth") {
						t.Fatalf("OAuth accepted ordinary key: %s", out.Body.String())
					}
				}
			}
			preset := service.GetUpstreamPreset("longcat")
			if preset == nil || len(preset.RecommendedModels) == 0 {
				t.Fatal("expected LongCat recommendations")
			}
			fallback := catalogCall(t, mux, "POST", connectPath, map[string]any{"presetId": preset.ID, "baseUrl": server.URL, "apiKey": "fixture-key-never-echo"}, 201)
			if fallback["discovery"].(map[string]any)["status"] != "preset" || fallback["modelCount"] == float64(0) {
				t.Fatal("preset discovery hidden")
			}
		})
	}
}

func TestUpstreamConnectAnonymousHTTP(t *testing.T) {
	db := catalogTestDB(t, store.DialectSQLite, ":memory:")
	mux := catalogMux(db)
	var keyed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantAuth := ""
		if keyed.Load() {
			wantAuth = "Bearer fixture-ollama-key"
		}
		if r.URL.Path != "/api/tags" || r.Header.Get("Authorization") != wantAuth {
			t.Error("wrong anonymous discovery contract")
		}
		fmt.Fprint(w, `{"models":[{"name":"local-model"}]}`)
	}))
	defer server.Close()
	var presetID string
	for _, preset := range service.ListUpstreamPresets() {
		if preset.Provider == "ollama" && preset.CredentialMode == "optional" {
			presetID = preset.ID
			break
		}
	}
	if presetID == "" {
		t.Fatal("missing anonymous Ollama preset")
	}
	out := catalogCall(t, mux, "POST", connectPath, map[string]any{"presetId": presetID, "baseUrl": server.URL}, 201)
	if out["modelCount"] != float64(1) {
		t.Fatal("anonymous models missing")
	}
	var secret, kind string
	if err := db.QueryRow(`SELECT secret,kind FROM upstream_credentials`).Scan(&secret, &kind); err != nil {
		t.Fatal(err)
	}
	if secret != "" || kind != store.DirectCredentialNone {
		t.Fatal("anonymous credential stored as a key")
	}
	keyed.Store(true)
	keyedResult := catalogCall(t, mux, "POST", connectPath, map[string]any{"presetId": presetID, "baseUrl": server.URL, "apiKey": "fixture-ollama-key"}, 201)
	var endpoints store.DirectEndpoints
	if err := db.Get(&endpoints, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE id=?`), catalogID(keyedResult)); err != nil {
		t.Fatal(err)
	}
	for _, entry := range endpoints.Entries() {
		if entry.Endpoint != nil && entry.Endpoint.Auth != store.DirectAuthBearer {
			t.Fatal("Ollama key was ignored by an endpoint")
		}
	}
}
