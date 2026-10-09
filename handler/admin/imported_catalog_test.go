package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

const catalogPrefix = "/api/imported-upstreams"

func catalogTestDB(t *testing.T, dialect, sqliteDSN string) *store.DB {
	t.Helper()
	dsn := sqliteDSN
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
	t.Cleanup(func() { db.Close() })
	if dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	if err = store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}
func catalogMux(db *store.DB) *chi.Mux {
	r := chi.NewRouter()
	RegisterImportedUpstreamRoutes(r, db.DB)
	return r
}
func catalogRequest(r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var raw []byte
	if s, ok := body.(string); ok {
		raw = []byte(s)
	} else if body != nil {
		raw, _ = json.Marshal(body)
	}
	out := httptest.NewRecorder()
	r.ServeHTTP(out, httptest.NewRequest(method, path, strings.NewReader(string(raw))))
	return out
}
func catalogCall(t *testing.T, r http.Handler, method, path string, body any, status int) map[string]any {
	t.Helper()
	value, err := catalogTry(r, method, path, body, status)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func catalogTry(r http.Handler, method, path string, body any, status int) (map[string]any, error) {
	out := catalogRequest(r, method, path, body)
	if out.Code != status {
		return nil, fmt.Errorf("%s %s: got %d want %d: %s", method, path, out.Code, status, out.Body.String())
	}
	var value map[string]any
	if err := json.Unmarshal(out.Body.Bytes(), &value); err != nil {
		return nil, err
	}
	return value, nil
}
func catalogID(value map[string]any) int64 { return int64(value["id"].(float64)) }
func catalogChannelInput(name, url string) map[string]any {
	return map[string]any{"name": name, "provider": "generic", "baseUrl": url, "enabled": true, "endpointConfig": map[string]any{"chat": map[string]any{"url": url + "/chat", "auth": "bearer"}, "responses": map[string]any{"url": url + "/responses", "auth": "bearer"}}}
}
func catalogCreateModel(t *testing.T, r http.Handler, channelID int64, name string) int64 {
	t.Helper()
	out := catalogCall(t, r, "POST", fmt.Sprintf(catalogPrefix+"/%d/models", channelID), map[string]any{"name": name}, 201)
	return catalogID(out["items"].([]any)[0].(map[string]any))
}
func catalogCounts(t *testing.T, db *store.DB) []int {
	t.Helper()
	out := []int{}
	for _, table := range []string{"upstream_groups", "upstream_group_items", "token_routes", "upstream_route_groups"} {
		var count int
		if err := db.Get(&count, `SELECT COUNT(*) FROM `+table); err != nil {
			t.Fatal(err)
		}
		out = append(out, count)
	}
	return out
}

func TestUpstreamCatalogContract(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			mux := catalogMux(db)
			channel := catalogID(catalogCall(t, mux, "POST", catalogPrefix, catalogChannelInput("Native", "https://example.com"), 201))
			base := fmt.Sprintf(catalogPrefix+"/%d", channel)
			for _, body := range []any{`{}`, `{"name":"x","provider":"x","baseUrl":"https://example.com","endpointConfig":{}}`, map[string]any{"name": "x", "provider": "x", "baseUrl": "http://169.254.169.254", "endpointConfig": map[string]any{"chat": map[string]string{"url": "https://example.com/chat", "auth": "bearer"}}}, `{"name":"x","provider":"generic","baseUrl":"https://example.com","endpointConfig":{"chat":{"url":"https://example.com/chat","auth":"bearer","profile":"codex"}}}`} {
				catalogCall(t, mux, "POST", catalogPrefix, body, 400)
			}
			catalogCall(t, mux, "POST", catalogPrefix, catalogChannelInput("Native", "https://example.com"), 409)
			credential := catalogID(catalogCall(t, mux, "POST", base+"/credentials", map[string]any{"name": "key", "apiKey": "fixture-secret", "enabled": true}, 201))
			credential2 := catalogID(catalogCall(t, mux, "POST", base+"/credentials", map[string]any{"name": "spare", "apiKey": "fixture-spare", "enabled": true}, 201))
			for _, body := range []string{`{"name":"bad"}`, `{"name":"bad","apiKey":""}`, `{"name":"bad","apiKey":"{}"}`, `{"name":"bad","apiKey":"x","oauth":{"accessToken":"x"}}`, `{"name":"bad","oauth":{"accessToken":"x"}}`, `{"name":"bad","apiKey":null}`, `{"name":"bad","apiKey":"x","apiKey":"y"}`, `{"name":"bad","unknown":true}`} {
				catalogCall(t, mux, "POST", base+"/credentials", body, 400)
			}
			catalogCall(t, mux, "POST", base+"/credentials", `{"name":"key","apiKey":"another"}`, 409)
			model := catalogCreateModel(t, mux, channel, "actual-model")
			for _, body := range []string{`{"names":[]}`, `{"names":["x","x"]}`, `{"name":"x","names":["y"]}`, `{"name":"","names":["y"]}`, `{"names":["x",null]}`, `{"name":" "}`} {
				catalogCall(t, mux, "POST", base+"/models", body, 400)
			}
			catalogCall(t, mux, "POST", base+"/models", `{"names":["must-rollback","actual-model"]}`, 409)
			var count int
			db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM upstream_models WHERE name=?`), "must-rollback")
			if count != 0 {
				t.Fatal("batch partially committed")
			}
			grantInput := map[string]any{"modelId": model, "credentialId": credential, "protocols": []int{2, 4}}
			grant := catalogID(catalogCall(t, mux, "POST", catalogPrefix+"/grants", grantInput, 201))
			grant2 := catalogID(catalogCall(t, mux, "POST", catalogPrefix+"/grants", map[string]any{"modelId": model, "credentialId": credential2, "protocols": []int{2, 4}}, 201))
			catalogCall(t, mux, "POST", catalogPrefix+"/grants", grantInput, 409)
			for _, protocols := range [][]int{{}, {2, 2}, {1}, {8}} {
				catalogCall(t, mux, "POST", catalogPrefix+"/grants", map[string]any{"modelId": model, "credentialId": credential, "protocols": protocols}, 400)
			}
			otherChannel := catalogID(catalogCall(t, mux, "POST", catalogPrefix, catalogChannelInput("Other", "https://example.org"), 201))
			otherModel := catalogCreateModel(t, mux, otherChannel, "foreign")
			catalogCall(t, mux, "POST", catalogPrefix+"/grants", map[string]any{"modelId": otherModel, "credentialId": credential, "protocols": []int{2}}, 400)
			for _, path := range []string{catalogPrefix + "/999999/models", catalogPrefix + "/999999/credentials"} {
				catalogCall(t, mux, "GET", path, nil, 404)
			}
			catalogCall(t, mux, "POST", catalogPrefix+"/999999/models", `{"name":"missing"}`, 404)
			catalogCall(t, mux, "POST", catalogPrefix+"/999999/credentials", `{"name":"missing","apiKey":"x"}`, 404)
			groupInput := map[string]any{"name": "Public group", "enabled": true, "mode": "manual", "activeGrantId": grant, "route": map[string]any{"modelPattern": "public-alias", "displayName": "Public alias", "routingStrategy": "stable_first"}, "members": []any{map[string]any{"grantId": grant, "protocolOrder": []int{2}}, map[string]any{"grantId": grant2, "protocolOrder": []int{4, 2}}}}
			group := catalogCall(t, mux, "POST", catalogPrefix+"/groups", groupInput, 201)
			groupID := catalogID(group)
			memberID := catalogID(group["members"].([]any)[0].(map[string]any))
			member2 := catalogID(group["members"].([]any)[1].(map[string]any))
			routeID := int64(group["routeId"].(float64))
			if int64(group["activeMemberId"].(float64)) != memberID {
				t.Fatal("active grant not translated to local member")
			}
			groupsPath := fmt.Sprintf(catalogPrefix+"/groups/%d", groupID)
			catalogCall(t, mux, "POST", groupsPath+"/members", map[string]any{"grantId": grant}, 409)
			catalogCall(t, mux, "POST", groupsPath+"/members", map[string]any{"grantId": grant, "weight": 0}, 400)
			catalogCall(t, mux, "PATCH", groupsPath, map[string]any{"activeMemberId": member2}, 200)
			catalogCall(t, mux, "PATCH", groupsPath, `{"activeMemberId":0}`, 400)
			catalogCall(t, mux, "PATCH", groupsPath, `{"mode":"unknown"}`, 400)
			before := fmt.Sprint(catalogCounts(t, db))
			for _, body := range []any{
				map[string]any{"name": "duplicate-public", "route": map[string]any{"modelPattern": "public-alias"}},
				map[string]any{"name": "duplicate-display", "route": map[string]any{"modelPattern": "another-alias", "displayName": "Public alias"}},
			} {
				catalogCall(t, mux, "POST", catalogPrefix+"/groups", body, 409)
			}
			for _, body := range []any{
				`{"name":"Bad","mode":"manual","enabled":true,"route":{"modelPattern":"bad"}}`,
				`{"name":"Bad","route":{"modelPattern":"*"}}`,
				`{"name":"Bad","route":{"modelPattern":"bad","routingStrategy":"made-up"}}`,
				map[string]any{"name": "Bad", "activeGrantId": 999999, "route": map[string]any{"modelPattern": "bad"}, "members": []any{map[string]any{"grantId": grant}}},
				map[string]any{"name": "Bad", "route": map[string]any{"modelPattern": "bad"}, "members": []any{map[string]any{"grantId": grant, "protocolOrder": []int{8}}}},
			} {
				catalogCall(t, mux, "POST", catalogPrefix+"/groups", body, 400)
			}
			catalogCall(t, mux, "POST", catalogPrefix+"/groups", map[string]any{"name": "Bad", "route": map[string]any{"modelPattern": "bad"}, "members": []any{map[string]any{"grantId": grant}, map[string]any{"grantId": 999999}}}, 404)
			if after := fmt.Sprint(catalogCounts(t, db)); after != before {
				t.Fatalf("failed group left graph fragments: %s -> %s", before, after)
			}
			another := catalogCall(t, mux, "POST", catalogPrefix+"/groups", map[string]any{"name": "Another group", "route": map[string]any{"modelPattern": "another-public"}}, 201)
			newMember := catalogCall(t, mux, "POST", fmt.Sprintf(catalogPrefix+"/groups/%d/members", catalogID(another)), map[string]any{"grantId": grant}, 201)
			catalogCall(t, mux, "PATCH", groupsPath, map[string]any{"activeMemberId": catalogID(newMember)}, 400)
			shrink := catalogCall(t, mux, "PATCH", fmt.Sprintf(catalogPrefix+"/grants/%d", grant2), `{"protocols":[2]}`, 409)
			if ids := shrink["conflictingMemberIds"].([]any); len(ids) != 1 || int64(ids[0].(float64)) != member2 {
				t.Fatal("missing conflicting member identity")
			}
			catalogCall(t, mux, "PATCH", fmt.Sprintf(catalogPrefix+"/members/%d", member2), `{"protocolOrder":[]}`, 200)
			catalogCall(t, mux, "PATCH", fmt.Sprintf(catalogPrefix+"/grants/%d", grant2), `{"protocols":[2]}`, 200)
			changed := catalogCall(t, mux, "PATCH", fmt.Sprintf(catalogPrefix+"/models/%d", model), `{"name":"renamed-actual"}`, 200)
			if !strings.Contains(fmt.Sprint(changed["affectedRouteIds"]), fmt.Sprint(routeID)) {
				t.Fatal("rename omitted affected routes")
			}
			models := catalogCall(t, mux, "GET", base+"/models", nil, 200)["items"].([]any)
			g := models[0].(map[string]any)["grants"].([]any)[0].(map[string]any)
			if g["ownership"] != "native" || g["credentialName"] != "key" || g["memberCount"].(float64) != 2 || g["protocols"].(float64) != 6 {
				t.Fatalf("grant projection=%v", g)
			}
			for _, path := range []string{base + "/credentials", base + "/models", catalogPrefix + "/groups"} {
				out := catalogRequest(mux, "GET", path, nil)
				if out.Code != 200 || strings.Contains(out.Body.String(), "fixture-secret") {
					t.Fatalf("projection/secret leak: %s", out.Body.String())
				}
			}
			if _, err := backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "catalog-import"); err != nil {
				t.Fatal(err)
			}
			var importedID int64
			if err := db.Get(&importedID, db.Rebind(`SELECT id FROM upstream_channels WHERE origin_key=? ORDER BY id LIMIT 1`), "catalog-import"); err != nil {
				t.Fatal(err)
			}
			importedBase := fmt.Sprintf(catalogPrefix+"/%d", importedID)
			localModel := catalogCreateModel(t, mux, importedID, "local-child")
			localCredential := catalogID(catalogCall(t, mux, "POST", importedBase+"/credentials", `{"name":"local-child","apiKey":"fixture-local"}`, 201))
			var existingModelName, existingCredentialName string
			if err := db.Get(&existingModelName, db.Rebind(`SELECT name FROM upstream_models WHERE channel_id=? AND origin_key=? ORDER BY id LIMIT 1`), importedID, "catalog-import"); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&existingCredentialName, db.Rebind(`SELECT name FROM upstream_credentials WHERE channel_id=? AND origin_key=? ORDER BY id LIMIT 1`), importedID, "catalog-import"); err != nil {
				t.Fatal(err)
			}
			catalogCall(t, mux, "POST", importedBase+"/models", map[string]any{"name": existingModelName}, 409)
			catalogCall(t, mux, "POST", importedBase+"/credentials", map[string]any{"name": existingCredentialName, "apiKey": "fixture-conflict"}, 409)
			for table, id := range map[string]int64{"upstream_models": localModel, "upstream_credentials": localCredential} {
				var origin string
				db.Get(&origin, db.Rebind(`SELECT origin_key FROM `+table+` WHERE id=?`), id)
				if origin != upstream.NativeOrigin {
					t.Fatal("child inherited imported ownership")
				}
			}
			var nativeMappings int
			db.Get(&nativeMappings, db.Rebind(`SELECT COUNT(*) FROM external_source_ids WHERE origin_key=?`), upstream.NativeOrigin)
			if nativeMappings != 0 {
				t.Fatal("native rows fabricated external mappings")
			}
			var importedModel int64
			var source int64
			db.QueryRowx(db.Rebind(`SELECT id,source_id FROM upstream_models WHERE origin_key=? ORDER BY id LIMIT 1`), "catalog-import").Scan(&importedModel, &source)
			catalogCall(t, mux, "PATCH", fmt.Sprintf(catalogPrefix+"/models/%d", importedModel), `{"enabled":false}`, 200)
			var afterOrigin string
			var afterSource int64
			db.QueryRowx(db.Rebind(`SELECT origin_key,source_id FROM upstream_models WHERE id=?`), importedModel).Scan(&afterOrigin, &afterSource)
			if afterOrigin != "catalog-import" || afterSource != source {
				t.Fatal("editing imported model changed ownership")
			}
			for _, table := range []string{"channels", "credentials", "models", "grants", "groups", "group_items"} {
				var bad int
				if err := db.Get(&bad, db.Rebind(`SELECT COUNT(*) FROM upstream_`+table+` WHERE origin_key=? AND source_id<>-id`), upstream.NativeOrigin); err != nil || bad != 0 {
					t.Fatalf("invalid native identity in %s: %d %v", table, bad, err)
				}
			}
		})
	}
}

func TestUpstreamCatalogOAuthCreation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			mux := catalogMux(db)
			input := map[string]any{"name": "OAuth", "provider": "codex", "baseUrl": "https://example.com", "endpointConfig": map[string]any{"responses": map[string]any{"url": "https://example.com/responses", "auth": "bearer", "profile": "codex"}}}
			channel := catalogCall(t, mux, "POST", catalogPrefix, input, 201)
			if channel["enabled"] != false {
				t.Fatal("channel must default disabled")
			}
			path := fmt.Sprintf(catalogPrefix+"/%d/credentials", catalogID(channel))
			created := catalogCall(t, mux, "POST", path, `{"name":"oauth","oauth":{"accessToken":"fixture-access","refreshToken":"fixture-refresh","idToken":"fixture-id","accountId":"fixture-account","expiresAt":9999999999999}}`, 201)
			if created["kind"] != "oauth" || created["canRefresh"] != true || created["ownership"] != "native" || created["enabled"] != false {
				t.Fatalf("OAuth summary=%v", created)
			}
			for _, secret := range []string{"fixture-access", "fixture-refresh", "fixture-id", "fixture-account"} {
				if strings.Contains(fmt.Sprint(created), secret) {
					t.Fatal("creation response leaked OAuth material")
				}
			}
			for _, body := range []string{`{"name":"bad","oauth":{"accessToken":" "}}`, `{"name":"bad","oauth":{"accessToken":"x","expiresAt":-1}}`, `{"name":"bad","oauth":{"accessToken":"x","unknown":true}}`} {
				catalogCall(t, mux, "POST", path, body, 400)
			}
			out := catalogRequest(mux, "GET", path, nil)
			if out.Code != 200 || strings.Contains(out.Body.String(), "fixture-") {
				t.Fatal("list leaked OAuth state")
			}
			catalogCall(t, mux, "PATCH", fmt.Sprintf(catalogPrefix+"/credentials/%d", catalogID(created)), `{"apiKey":"replacement"}`, 200)
			var kind string
			var state store.DirectOAuthState
			if err := db.QueryRowx(db.Rebind(`SELECT kind,oauth_state FROM upstream_credentials WHERE id=?`), catalogID(created)).Scan(&kind, &state); err != nil {
				t.Fatal(err)
			}
			if kind != store.DirectCredentialAPIKey || state.RefreshToken != "" {
				t.Fatal("replacement did not clear OAuth state")
			}
		})
	}
}

func TestUpstreamCatalogRealHTTP(t *testing.T) {
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
			type receipt struct{ Model, Key string }
			receipts := make(chan receipt, 8)
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				receipts <- receipt{body.Model, r.Header.Get("Authorization")}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"id":"fixture","choices":[{"message":{"role":"assistant","content":"receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			}))
			defer up.Close()
			channel := catalogID(catalogCall(t, mux, "POST", catalogPrefix, catalogChannelInput("Native HTTP", up.URL), 201))
			base := fmt.Sprintf(catalogPrefix+"/%d", channel)
			model := catalogCreateModel(t, mux, channel, "actual-wire")
			grants := []int64{}
			for _, name := range []string{"first", "second"} {
				credential := catalogID(catalogCall(t, mux, "POST", base+"/credentials", map[string]any{"name": name, "enabled": true, "apiKey": name}, 201))
				grant := catalogID(catalogCall(t, mux, "POST", catalogPrefix+"/grants", map[string]any{"modelId": model, "credentialId": credential, "protocols": []int{2}}, 201))
				grants = append(grants, grant)
			}
			group := catalogCall(t, mux, "POST", catalogPrefix+"/groups", map[string]any{"name": "HTTP group", "mode": "manual", "enabled": true, "activeGrantId": grants[0], "route": map[string]any{"modelPattern": "public-http"}, "members": []any{map[string]any{"grantId": grants[0]}, map[string]any{"grantId": grants[1]}}}, 201)
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
			defer routing.SetGlobalCache(nil)
			proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
			defer proxyhandler.SetUpstreamConfig(nil)
			down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
				proxyhandler.HandleChatCompletions(w, r)
			}))
			defer down.Close()
			relay := func(want receipt) {
				t.Helper()
				resp, err := http.Post(down.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"public-http","messages":[{"role":"user","content":"hi"}]}`))
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode != 200 || !strings.Contains(string(body), "receipt") {
					t.Fatalf("relay=%d %s", resp.StatusCode, body)
				}
				if got := <-receipts; got != want {
					t.Fatalf("wire got %+v want %+v", got, want)
				}
			}
			relay(receipt{"actual-wire", "Bearer first"})
			policy := routing.EmptyDownstreamRoutingPolicy
			policy.RequiredUpstreamProtocol = 2
			warm := func() {
				t.Helper()
				if selected, err := router.SelectChannel(t.Context(), "public-http", policy); err != nil || selected == nil {
					t.Fatalf("warm routing cache: %v %v", selected, err)
				}
			}
			warm()
			member2 := catalogID(group["members"].([]any)[1].(map[string]any))
			groupPath := fmt.Sprintf(catalogPrefix+"/groups/%d", catalogID(group))
			catalogCall(t, mux, "PATCH", groupPath, map[string]any{"activeMemberId": member2}, 200)
			relay(receipt{"actual-wire", "Bearer second"})
			warm()
			catalogCall(t, mux, "PATCH", fmt.Sprintf(catalogPrefix+"/models/%d", model), `{"name":"renamed-wire"}`, 200)
			relay(receipt{"renamed-wire", "Bearer second"})
			warm()
			catalogCall(t, mux, "PATCH", groupPath, `{"enabled":false}`, 200)
			selected, err := router.SelectChannel(t.Context(), "public-http", policy)
			if err != nil || selected != nil {
				t.Fatalf("disabled group remained in cache: %v %v", selected, err)
			}
		})
	}
}

func TestUpstreamCatalogConcurrentIdentity(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "catalog.db")
			db := catalogTestDB(t, dialect, dsn)
			const workers = 8
			connections := make([]*store.DB, workers)
			for i := range workers {
				connections[i] = db
				if dialect == store.DialectSQLite {
					conn, err := store.Open(dialect, dsn, false)
					if err != nil {
						t.Fatal(err)
					}
					defer conn.Close()
					connections[i] = conn
				}
			}
			start := make(chan struct{})
			failures := make(chan error, workers)
			var wg sync.WaitGroup
			for i := range workers {
				wg.Go(func() {
					<-start
					mux := catalogMux(connections[i])
					name := fmt.Sprintf("parallel-%d", i)
					channelResponse, err := catalogTry(mux, "POST", catalogPrefix, catalogChannelInput(name, "https://example.com"), 201)
					if err != nil {
						failures <- err
						return
					}
					channel := catalogID(channelResponse)
					base := fmt.Sprintf(catalogPrefix+"/%d", channel)
					credentialResponse, err := catalogTry(mux, "POST", base+"/credentials", map[string]any{"name": name, "apiKey": "fixture"}, 201)
					if err != nil {
						failures <- err
						return
					}
					credential := catalogID(credentialResponse)
					modelResponse, err := catalogTry(mux, "POST", base+"/models", map[string]any{"name": name}, 201)
					if err != nil {
						failures <- err
						return
					}
					model := catalogID(modelResponse["items"].([]any)[0].(map[string]any))
					grantResponse, err := catalogTry(mux, "POST", catalogPrefix+"/grants", map[string]any{"modelId": model, "credentialId": credential, "protocols": []int{2}}, 201)
					if err != nil {
						failures <- err
						return
					}
					grant := catalogID(grantResponse)
					if _, err = catalogTry(mux, "POST", catalogPrefix+"/groups", map[string]any{"name": name, "route": map[string]any{"modelPattern": name}, "members": []any{map[string]any{"grantId": grant}}}, 201); err != nil {
						failures <- err
					}
				})
			}
			close(start)
			wg.Wait()
			close(failures)
			for err := range failures {
				t.Error(err)
			}
			for _, table := range []string{"channels", "credentials", "models", "grants", "groups", "group_items"} {
				var count, bad int
				if err := db.Get(&count, `SELECT COUNT(*) FROM upstream_`+table); err != nil {
					t.Fatal(err)
				}
				if err := db.Get(&bad, db.Rebind(`SELECT COUNT(*) FROM upstream_`+table+` WHERE origin_key<>? OR source_id<>-id OR source_id=0`), upstream.NativeOrigin); err != nil {
					t.Fatal(err)
				}
				if count != workers || bad != 0 {
					t.Fatalf("%s count=%d bad identities=%d", table, count, bad)
				}
			}
		})
	}
}
