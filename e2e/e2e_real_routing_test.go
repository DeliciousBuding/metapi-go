package e2e

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/admin"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// Unlike the protocol-isolation tests, this uses ConfigureProxyUpstream's real
// TokenRouter and SQL routing store. No selected channel is pre-programmed.
func TestConfiguredRoutesReachUpstreamThroughRealRouter(t *testing.T) {
	for _, discoverable := range []bool{true, false} {
		name := "manual-with-discovery-disabled"
		if discoverable {
			name = "automatic-from-discovery"
		}
		t.Run(name, func(t *testing.T) {
			cfg := makeTestConfig()
			cfg.DataDir = t.TempDir()
			cfg.AccountCredentialSecret = "real-router-fixture-secret"
			cfg.RequestBodyLimit = 20 * 1024 * 1024
			cfg.ProxyLogAsync = false
			rt := makeTestRuntime()
			rt.AuthToken = "route-admin-fixture"
			rt.ProxyToken = "sk-route-proxy-fixture"
			config.Set(cfg)
			config.SetRuntime(rt)
			if err := store.EnsureRuntimeDatabase(cfg, rt); err != nil {
				t.Fatal(err)
			}
			db := store.GetDB()
			if err := app.ConfigureProxyUpstream(cfg); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				proxyhandler.SetUpstreamConfig(nil)
				_ = store.CloseDatabase()
				_ = app.ConfigureProxyUpstream(cfg) // no DB clears the runtime router hooks
				config.Set(makeTestConfig())
				config.SetRuntime(makeTestRuntime())
			})
			var random [16]byte
			if _, err := rand.Read(random[:]); err != nil {
				t.Fatal(err)
			}
			receipt := hex.EncodeToString(random[:])
			model := "route-lifecycle-model"
			var discoveryCalls, relayCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Header.Get("Authorization") != "Bearer sk-upstream-route-fixture" {
					t.Error("upstream received wrong relay credential")
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				if req.URL.Path == "/v1/models" {
					discoveryCalls.Add(1)
					if !discoverable {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					writeJSONHelper(w, 200, map[string]any{"data": []map[string]string{{"id": model}}})
					return
				}
				if req.Method != http.MethodPost || req.URL.Path != "/v1/chat/completions" {
					http.NotFound(w, req)
					return
				}
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if body["model"] != model {
					t.Errorf("upstream model=%v, want %s", body["model"], model)
				}
				relayCalls.Add(1)
				writeJSONHelper(w, 200, map[string]any{
					"id": "chatcmpl-real-router-fixture", "object": "chat.completion", "model": model,
					"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": receipt}, "finish_reason": "stop"}},
					"usage":   map[string]int{"prompt_tokens": 7, "completion_tokens": 4, "total_tokens": 11},
				})
			}))
			t.Cleanup(upstream.Close)
			r := chi.NewRouter()
			r.Group(func(r chi.Router) {
				r.Use(auth.AdminAuth(nil))
				admin.RegisterSitesRoutes(r, db.DB)
				admin.RegisterAccountsRoutes(r, db.DB, cfg)
				admin.RegisterTokenRoutesWithDeps(r, db.DB, admin.TokenRoutesDeps{})
				admin.RegisterSettingsRoutes(r, db.DB, cfg)
			})
			r.Route("/v1", func(r chi.Router) { r.Use(auth.ProxyAuth()); proxyhandler.RegisterProxyRoutes(r) })
			callAdmin := func(method, path string, body any) map[string]any {
				t.Helper()
				data, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(method, path, strings.NewReader(string(data)))
				request.Header.Set("Authorization", "Bearer "+rt.AuthToken)
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				r.ServeHTTP(response, request)
				if response.Code < 200 || response.Code >= 300 {
					t.Fatalf("admin %s %s: %d %s", method, path, response.Code, response.Body.String())
				}
				var result map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			relay := func() *httptest.ResponseRecorder {
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"return the fixture receipt"}]}`, model)))
				request.Header.Set("Authorization", "Bearer "+rt.ProxyToken)
				request.Header.Set("Content-Type", "application/json")
				response := httptest.NewRecorder()
				r.ServeHTTP(response, request)
				return response
			}
			site := callAdmin(http.MethodPost, "/api/sites", map[string]any{"name": "real routing fixture", "url": upstream.URL, "platform": "openai"})
			accountInput := map[string]any{"siteId": site["id"], "accessToken": "sk-upstream-route-fixture", "credentialMode": "apikey"}
			if !discoverable {
				// The existing API-key form exposes an explicit "Skip model
				// fetch" choice. Never silently convert a failed verification
				// into a verified credential just to make this flow pass.
				rejected := doAdminPost(t, r, "/api/accounts", rt.AuthToken, accountInput)
				if rejected.Code != http.StatusBadRequest {
					t.Fatalf("blocked verification unexpectedly accepted: %d", rejected.Code)
				}
				accountInput["skipModelFetch"] = true
			}
			account := callAdmin(http.MethodPost, "/api/accounts", accountInput)
			if !discoverable {
				result, ok := account["modelRefresh"].(map[string]any)
				if !ok || result["skipped"] != true {
					t.Fatalf("explicit no-probe import not reported: %v", account)
				}
			}
			if discoveryCalls.Load() == 0 {
				t.Fatal("account creation bypassed actual model discovery")
			}
			before := relay()
			if before.Code != http.StatusServiceUnavailable || relayCalls.Load() != 0 {
				t.Fatalf("unconfigured route falsely relayed: %d %s", before.Code, before.Body.String())
			}
			if discoverable {
				callAdmin(http.MethodPut, "/api/settings/runtime", map[string]any{"autoCreateModelRoutes": true})
				result := callAdmin(http.MethodPost, "/api/routes/rebuild", map[string]any{"refreshModels": false, "wait": true})
				if result["routesCreated"] != float64(1) {
					t.Fatalf("automatic route not actually created: %v", result)
				}
			} else {
				route := callAdmin(http.MethodPost, "/api/routes", map[string]any{"modelPattern": model, "enabled": true})
				result := callAdmin(http.MethodPost, fmt.Sprintf("/api/routes/%.0f/channels/batch", route["id"]), map[string]any{"channels": []map[string]any{{"accountId": account["id"]}}})
				if result["created"] != float64(1) {
					t.Fatalf("manual channel not bound: %v", result)
				}
				callAdmin(http.MethodPost, "/api/routes/rebuild", map[string]any{"refreshModels": false, "wait": true})
			}
			response := relay()
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), receipt) || relayCalls.Load() != 1 {
				t.Fatalf("configured real router failed forwarding: status=%d calls=%d body=%s", response.Code, relayCalls.Load(), response.Body.String())
			}
			var successfulLogs int
			if err := db.Get(&successfulLogs, "SELECT COUNT(*) FROM proxy_logs WHERE account_id = ? AND status = 'success'", int64(account["id"].(float64))); err != nil {
				t.Fatal(err)
			}
			if successfulLogs != 1 {
				t.Fatalf("real dispatch did not persist its success: %d", successfulLogs)
			}
		})
	}
}
