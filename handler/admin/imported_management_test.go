package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestImportedManagementContract(t *testing.T) {
	previousConfig := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	defer config.Set(previousConfig)
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == store.DialectPostgres {
				dsn = os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
			}
			db, err := store.Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if dialect == store.DialectPostgres {
				pgtest.Reset(t, db.DB)
			}
			if err = store.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			if _, err = backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "management-test"); err != nil {
				t.Fatal(err)
			}
			mux := chi.NewRouter()
			RegisterImportedUpstreamRoutes(mux, db.DB)
			request := func(method, path, body string) *httptest.ResponseRecorder {
				out := httptest.NewRecorder()
				mux.ServeHTTP(out, httptest.NewRequest(method, path, strings.NewReader(body)))
				return out
			}
			var channelID, memberID, grantID, credentialID, modelID, groupID, routeID int64
			if err = db.QueryRow(`SELECT c.id,i.id,g.id,k.id,m.id,i.group_id,rg.route_id FROM upstream_channels c JOIN upstream_models m ON m.channel_id=c.id JOIN upstream_grants g ON g.model_id=m.id JOIN upstream_credentials k ON k.id=g.credential_id JOIN upstream_group_items i ON i.grant_id=g.id JOIN upstream_route_groups rg ON rg.group_id=i.group_id ORDER BY c.id LIMIT 1`).Scan(&channelID, &memberID, &grantID, &credentialID, &modelID, &groupID, &routeID); err != nil {
				t.Fatal(err)
			}
			path := fmt.Sprintf("/api/imported-upstreams/%d", channelID)
			memberPath := fmt.Sprintf("/api/imported-upstreams/members/%d", memberID)
			credentialPath := fmt.Sprintf("/api/imported-upstreams/credentials/%d", credentialID)
			invalidUserBase, _ := json.Marshal(map[string]string{"baseUrl": (&url.URL{Scheme: "https", Host: "example.com", User: url.UserPassword("user", "pass")}).String()})
			for _, invalid := range []string{`{}`, `{"enabled":null}`, `{"enabled":"false"}`, `{"enabled":true,"enabled":false}`, `{"enabled":false} {}`, `{"provider":"unknown"}`, `{"name":""}`, `{"baseUrl":"http://169.254.169.254"}`, string(invalidUserBase), `{"openaiChatCompletionPath":"/%2e%2e/private"}`, `{"endpointConfig":{"chat":{"url":"https://example.com/chat","auth":"unknown"}}}`, `{"endpointConfig":{"chat":{"url":"https://example.com/chat","auth":"bearer","profile":"codex"}}}`, `{"endpointConfig":{"chat":{"url":"https://example.com/chat","auth":"bearer","modelPath":null}}}`, `{"paramOverride":"[]"}`, `{"customHeaders":"[{\"header_key\":\"Authorization\",\"header_value\":\"bad\"}]"}`, `{"name":"must-not-save","baseUrl":"bad"}`} {
				if out := request("PATCH", path, invalid); out.Code != 400 {
					t.Fatalf("accepted invalid input %s: %d %s", invalid, out.Code, out.Body.String())
				}
			}
			if out := request("PATCH", path, `{"name":"Primary backup"}`); out.Code != 409 {
				t.Fatalf("name conflict=%d %s", out.Code, out.Body.String())
			}
			var name string
			if err = db.Get(&name, db.Rebind(`SELECT name FROM upstream_channels WHERE id=?`), channelID); err != nil || name != "Primary" {
				t.Fatalf("invalid patch partially saved: %s %v", name, err)
			}
			for _, invalid := range []string{`{"weight":0}`, `{"priority":1.5}`, `{"protocolOrder":[4]}`, `{"protocolOrder":[2,2]}`, `{"protocolOrder":[1]}`, `{"protocolOrder":[null]}`, `{"grantId":1}`} {
				if out := request("PATCH", memberPath, invalid); out.Code != 400 {
					t.Fatalf("accepted invalid member patch %s: %d", invalid, out.Code)
				}
			}
			for _, valid := range []string{`{"priority":-5,"weight":3,"protocolOrder":[2]}`, `{"protocolOrder":[]}`} {
				if out := request("PATCH", memberPath, valid); out.Code != 200 {
					t.Fatalf("member patch: %d %s", out.Code, out.Body.String())
				}
			}
			for _, invalidProfile := range []string{
				`{"endpointConfig":{"chat":{"url":"https://example.com/chat","auth":"bearer","profile":"unknown"}}}`,
				`{"endpointConfig":{"chat":{"url":"https://example.com/chat","auth":"x-api-key","profile":"zai"}}}`,
			} {
				if out := request("PATCH", path, invalidProfile); out.Code != 400 {
					t.Fatalf("invalid endpoint profile accepted: %d", out.Code)
				}
			}
			if out := request("PATCH", credentialPath, `{"name":"renamed"}`); out.Code != 200 {
				t.Fatalf("credential rename: %s", out.Body.String())
			}
			for _, p := range []string{"/api/imported-upstreams/999999", "/api/imported-upstreams/999999/request-config", "/api/imported-upstreams/999999/credentials"} {
				if out := request("GET", p, ""); out.Code != 404 {
					t.Fatalf("missing resource %s=%d", p, out.Code)
				}
			}
			if out := request("PATCH", "/api/imported-upstreams/credentials/999999", `{"name":"missing"}`); out.Code != 404 {
				t.Fatalf("missing credential=%d", out.Code)
			}
			var received string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				received = r.URL.Path
				if r.Header.Get("Authorization") != "Bearer fixture-key-octopus-1" || r.Header.Get("X-Private-Config") != "confidential-config" || !strings.Contains(string(body), `"temperature":0.25`) {
					t.Error("saved request configuration did not reach upstream")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"fixture","choices":[{"message":{"role":"assistant","content":"receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			}))
			defer upstream.Close()
			previous := config.RuntimeSafe()
			config.SetRuntime(&config.RuntimeSettings{RoutingFallbackUnitCost: 1})
			defer config.SetRuntime(previous)
			defer routing.SetGlobalCache(nil)
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
			policy := routing.EmptyDownstreamRoutingPolicy
			policy.RequiredUpstreamProtocol = 2
			if selected, e := router.SelectChannel(t.Context(), "client-model", policy); e != nil || selected == nil {
				t.Fatalf("warm router: %v", e)
			}
			patch, _ := json.Marshal(map[string]any{"name": "Edited", "baseUrl": upstream.URL, "endpointConfig": store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/edited-chat", Auth: "bearer"}}, "customHeaders": `[{"header_key":"X-Private-Config","header_value":"confidential-config"}]`, "paramOverride": `{"temperature":0.25}`, "channelProxy": (&url.URL{Scheme: "http", Host: "proxy.example:8080", User: url.UserPassword("proxy-user", "proxy-password")}).String()})
			if out := request("PATCH", path, string(patch)); out.Code != 200 {
				t.Fatalf("config patch: %d %s", out.Code, out.Body.String())
			}
			for _, p := range []string{"/api/imported-upstreams", path} {
				out := request("GET", p, "")
				if out.Code != 200 {
					t.Fatalf("get=%d %s", out.Code, out.Body.String())
				}
				for _, secret := range []string{"fixture-key-octopus", "confidential-config", "proxy-password", "temperature"} {
					if strings.Contains(out.Body.String(), secret) {
						t.Fatalf("ordinary read leaked %s", secret)
					}
				}
			}
			if out := request("GET", path+"/request-config", ""); out.Code != 200 || out.Header().Get("Cache-Control") != "no-store" || !strings.Contains(out.Body.String(), "confidential-config") || strings.Contains(out.Body.String(), "fixture-key-octopus") {
				t.Fatalf("explicit config read: %d %s", out.Code, out.Body.String())
			}
			if out := request("PATCH", path, `{"channelProxy":""}`); out.Code != 200 {
				t.Fatal(out.Body.String())
			}
			if out := request("PATCH", path, `{"endpointConfig":{}}`); out.Code != 400 {
				t.Fatalf("configured endpoints silently fell back to legacy: %d", out.Code)
			}
			proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
			defer proxyhandler.SetUpstreamConfig(nil)
			downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-downstream", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
				proxyhandler.HandleChatCompletions(w, r)
			}))
			defer downstream.Close()
			resp, err := http.Post(downstream.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"client-model","messages":[{"role":"user","content":"hi"}]}`))
			if err != nil {
				t.Fatal(err)
			}
			result, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 || received != "/edited-chat" || !strings.Contains(string(result), "receipt") {
				t.Fatalf("live router used stale config: %d %s path=%s", resp.StatusCode, result, received)
			}
			for _, profile := range []string{"deepseek", "zai"} {
				profilePatch, _ := json.Marshal(map[string]any{"endpointConfig": store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/edited-chat", Auth: "bearer", Profile: profile}}})
				if out := request("PATCH", path, string(profilePatch)); out.Code != 200 {
					t.Fatalf("valid Chat profile %s rejected: %s", profile, out.Body.String())
				}
			}
			if out := request("PATCH", path, `{"endpointConfig":{"responses":{"url":"https://example.com/responses","auth":"bearer","profile":"zai"}}}`); out.Code != 400 {
				t.Fatal("Chat profile accepted on Responses")
			}
			if _, err = db.Exec(db.Rebind(`UPDATE upstream_grants SET protocols=6 WHERE id=?`), grantID); err != nil {
				t.Fatal(err)
			}
			if out := request("PATCH", memberPath, `{"protocolOrder":[2]}`); out.Code != 200 {
				t.Fatal(out.Body.String())
			}
			responsePolicy := routing.EmptyDownstreamRoutingPolicy
			responsePolicy.RequiredUpstreamProtocol = 4
			if selected, e := router.SelectChannel(t.Context(), "client-model", responsePolicy); e != nil || selected != nil {
				t.Fatalf("member restriction did not gate cached router: selected=%v err=%v", selected, e)
			}
			if out := request("PATCH", memberPath, `{"protocolOrder":[]}`); out.Code != 200 {
				t.Fatal(out.Body.String())
			}
			if selected, e := router.SelectChannel(t.Context(), "client-model", responsePolicy); e != nil || selected == nil || selected.Direct.GrantID != grantID {
				t.Fatalf("explicit inheritance was not restored: %v %v", selected, e)
			}
			if _, err = db.Exec(db.Rebind(`UPDATE upstream_grants SET protocols=2 WHERE id=?`), grantID); err != nil {
				t.Fatal(err)
			}
			for _, entry := range []struct {
				table string
				id    int64
				field string
			}{{"upstream_channels", channelID, "channelEnabled"}, {"upstream_models", modelID, "modelEnabled"}, {"upstream_credentials", credentialID, "credentialEnabled"}, {"upstream_grants", grantID, "grantEnabled"}, {"upstream_groups", groupID, "groupEnabled"}, {"token_routes", routeID, "routeEnabled"}} {
				if _, err = db.Exec(db.Rebind(`UPDATE `+entry.table+` SET enabled=? WHERE id=?`), false, entry.id); err != nil {
					t.Fatal(err)
				}
				var inventory struct {
					Members []map[string]any `json:"members"`
				}
				out := request("GET", "/api/imported-upstreams", "")
				if json.Unmarshal(out.Body.Bytes(), &inventory) != nil {
					t.Fatal(out.Body.String())
				}
				for _, member := range inventory.Members {
					if int64(member["id"].(float64)) == memberID && (member[entry.field] != false || member["effectiveEnabled"] != false || int64(member["grantId"].(float64)) != grantID) {
						t.Fatalf("false availability: %v", member)
					}
				}
				if _, err = db.Exec(db.Rebind(`UPDATE `+entry.table+` SET enabled=? WHERE id=?`), true, entry.id); err != nil {
					t.Fatal(err)
				}
			}
			var sharedGroup, sharedRoute int64
			if err = db.QueryRowx(`INSERT INTO upstream_groups (origin_key,source_id,name,mode,active_item_id,relay_config,enabled) SELECT origin_key,10000,'Shared grant','failover',0,'{}',enabled FROM upstream_groups ORDER BY id LIMIT 1 RETURNING id`).Scan(&sharedGroup); err != nil {
				t.Fatal(err)
			}
			if err = db.QueryRowx(`INSERT INTO token_routes (model_pattern,route_mode,routing_strategy,enabled) SELECT 'shared-client','pattern','weighted',enabled FROM token_routes ORDER BY id LIMIT 1 RETURNING id`).Scan(&sharedRoute); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(db.Rebind(`INSERT INTO upstream_route_groups (route_id,group_id) VALUES (?,?)`), sharedRoute, sharedGroup); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(db.Rebind(`INSERT INTO upstream_group_items (origin_key,source_id,group_id,grant_id,priority,weight) SELECT origin_key,10000,?,?,0,1 FROM upstream_group_items WHERE id=?`), sharedGroup, grantID, memberID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(db.Rebind(`UPDATE upstream_grants SET cooldown_until='2099-01-01T00:00:00Z',cooldown_reason_code='test',fail_count=7 WHERE id=?`), grantID); err != nil {
				t.Fatal(err)
			}
			if out := request("POST", memberPath+"/cooldown/clear", ""); out.Code != 200 || !strings.Contains(out.Body.String(), `"grantId"`) {
				t.Fatalf("clear=%d %s", out.Code, out.Body.String())
			}
			var cooldown *string
			var failures int
			if err = db.QueryRowx(db.Rebind(`SELECT cooldown_until,fail_count FROM upstream_grants WHERE id=?`), grantID).Scan(&cooldown, &failures); err != nil || cooldown != nil || failures != 7 {
				t.Fatalf("clear changed history: %v %d %v", cooldown, failures, err)
			}
			var memberships, cooled int
			if err = db.QueryRowx(db.Rebind(`SELECT COUNT(*),COUNT(g.cooldown_until) FROM upstream_group_items i JOIN upstream_grants g ON g.id=i.grant_id WHERE g.id=?`), grantID).Scan(&memberships, &cooled); err != nil || memberships != 2 || cooled != 0 {
				t.Fatalf("shared clear=%d memberships/%d cooled err=%v", memberships, cooled, err)
			}
			if _, err = backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "management-test"); err != nil {
				t.Fatal(err)
			}
			if err = db.Get(&name, db.Rebind(`SELECT name FROM upstream_channels WHERE id=?`), channelID); err != nil || name != "Primary" {
				t.Fatalf("reimport did not restore source config: %s %v", name, err)
			}
		})
	}
}

func TestImportedManagementStorageFailureIsNotNotFound(t *testing.T) {
	db := setupBackupTestDB(t)
	mux := chi.NewRouter()
	RegisterImportedUpstreamRoutes(mux, db.DB)
	db.Close()
	for _, tc := range []struct{ method, path, body string }{{"GET", "/api/imported-upstreams/1/credentials", ""}, {"PATCH", "/api/imported-upstreams/credentials/1", `{"name":"example"}`}, {"GET", "/api/imported-upstreams/1", ""}} {
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if out.Code != 500 {
			t.Fatalf("storage failure=%d %s", out.Code, out.Body.String())
		}
	}
}
