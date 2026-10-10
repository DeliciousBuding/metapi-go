package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func directVideoFixture(t *testing.T, dialect, upstreamURL string) (*store.DB, *httptest.Server, func()) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "video.db")
	if dialect == store.DialectPostgres {
		dsn = os.Getenv("PG_TEST_DSN")
		if dsn == "" {
			t.Skip("PG_TEST_DSN not set")
		}
		adminDB, err := store.Open(dialect, dsn, false)
		if err != nil {
			t.Fatal(err)
		}
		schema := fmt.Sprintf("video_test_%d", time.Now().UnixNano())
		if _, err := adminDB.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE"); _ = adminDB.Close() })
		if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			q := u.Query()
			q.Set("search_path", schema)
			u.RawQuery = q.Encode()
			dsn = u.String()
		} else {
			dsn += " search_path=" + schema
		}
	}
	db, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		"version": "1.4", "channels": []any{map[string]any{
			"id": 1, "type": "openai", "name": "video fixture", "base_url": upstreamURL,
			"credentials": map[string]any{"apiKey": "fixture-upstream-key"}, "supported_models": []string{"provider-model"},
		}},
		"models": []any{map[string]any{"id": 1, "model_id": "client-alias", "status": "enabled",
			"settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "provider-model"}}}},
		}},
	})
	if _, err := backup.ImportAxonHubV14(db, raw, "video-fixture", false); err != nil {
		t.Fatal(err)
	}
	endpoint := store.DirectEndpoints{Video: &store.DirectEndpoint{URL: upstreamURL + "/tenant/videos", Auth: store.DirectAuthBearer}}
	videoExec(t, db, `UPDATE upstream_channels SET endpoint_config = ?`, endpoint)
	videoExec(t, db, `UPDATE upstream_grants SET protocols = ?`, routing.UpstreamProtocolVideo)
	videoExec(t, db, `UPDATE upstream_group_items SET protocol_order = '[]'`)
	for _, key := range []string{"client-one", "client-two"} {
		videoExec(t, db, `INSERT INTO downstream_api_keys (name, key, enabled, supported_models) VALUES (?, ?, ?, '["client-alias"]')`, key, key, true)
	}
	previousDB, previousRuntime, previousUpstream := store.GetDB(), config.RuntimeSafe(), getUpstreamConfig()
	previousConfig := config.Get()
	cfg := *previousConfig
	cfg.ProxyVideoTaskRetentionDays = 7
	config.Set(&cfg)
	t.Cleanup(func() { config.Set(previousConfig) })
	store.OverrideDB(db)
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() {
		SetUpstreamConfig(previousUpstream)
		config.SetRuntime(previousRuntime)
		store.OverrideDB(previousDB)
	})
	wire := func(current *store.DB) {
		router := routing.NewTokenRouter(service.NewProxyRoutingStore(current), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
		SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
			ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
				return oauth.ResolveDirectCredential(ctx, current.DB, id, proxyURL, force)
			}})
	}
	wire(db)
	r := chi.NewRouter()
	r.Use(auth.ProxyAuth())
	r.Route("/v1", RegisterProxyRoutes)
	server := httptest.NewServer(r)
	t.Cleanup(server.Close)
	// Reopen a separate connection and rebuild the router, as another worker or
	// restarted process would; no task is restored from the process-local cache.
	reopen := func() {
		peer, err := store.Open(dialect, dsn, false)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = peer.Close() })
		store.OverrideDB(peer)
		wire(peer)
		videoTaskStoreMu.Lock()
		clear(videoTaskStore)
		videoTaskStoreMu.Unlock()
	}
	return db, server, reopen
}

func videoExec(t *testing.T, db *store.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func videoHTTP(t *testing.T, server *httptest.Server, method, path, body, key string) (int, http.Header, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, data
}

func createdVideoID(t *testing.T, server *httptest.Server) string {
	t.Helper()
	status, _, data := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"a quiet forest","seconds":"4"}`, "client-one")
	var body map[string]any
	if status != 200 || json.Unmarshal(data, &body) != nil {
		t.Fatalf("create status=%d body=%s", status, data)
	}
	id, _ := body["id"].(string)
	if strings.Contains(string(data), `"opaque"`) && !strings.Contains(string(data), "9007199254740993") {
		t.Fatal("video response metadata lost numeric precision")
	}
	if !strings.HasPrefix(id, directVideoIDPrefix) {
		t.Fatalf("missing durable public ID: %s", data)
	}
	return id
}

func TestDirectVideoTaskLifecycleHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls, deleteCalls atomic.Int32
			var rotated atomic.Bool
			const upstreamID = "provider/id ?#%"
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				wantKey := "fixture-upstream-key"
				if rotated.Load() {
					wantKey = "rotated-upstream-key"
				}
				if r.Header.Get("Authorization") != "Bearer "+wantKey {
					t.Error("credential resolver did not use current upstream key")
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == "POST" && r.URL.Path == "/tenant/videos":
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["model"] != "provider-model" || body["prompt"] != "a quiet forest" {
						t.Errorf("create payload: %v", body)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": upstreamID, "object": "video", "status": "queued", "opaque": json.RawMessage(`{"number":9007199254740993}`)})
				case r.Method == "GET" && r.URL.EscapedPath() == "/tenant/videos/"+url.PathEscape(upstreamID)+"/content":
					if r.URL.Query().Get("variant") != "thumbnail" {
						t.Error("content variant was lost")
					}
					w.Header().Set("Content-Type", "image/webp")
					_, _ = w.Write([]byte{0, 1, 2, 255, 0, 17})
				case r.Method == "POST" && r.URL.EscapedPath() == "/tenant/videos/"+url.PathEscape(upstreamID)+"/remix":
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					if len(body) != 1 || body["prompt"] != "make it snow" {
						t.Errorf("remix must preserve prompt-only body: %v", body)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "remix-provider", "object": "video", "status": "queued", "remixed_from_video_id": upstreamID})
				case r.Method == "GET" && r.URL.EscapedPath() == "/tenant/videos/"+url.PathEscape(upstreamID):
					_ = json.NewEncoder(w).Encode(map[string]any{"id": upstreamID, "status": "completed"})
				case r.Method == "GET" && r.URL.Path == "/tenant/videos/remix-provider":
					_ = json.NewEncoder(w).Encode(map[string]any{"id": "remix-provider", "status": "completed", "remixed_from_video_id": upstreamID})
				case r.Method == "DELETE" && r.URL.EscapedPath() == "/tenant/videos/"+url.PathEscape(upstreamID):
					if deleteCalls.Add(1) == 1 {
						w.WriteHeader(400)
						_, _ = io.WriteString(w, `{"error":{"message":"retry later"}}`)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": upstreamID, "deleted": true})
				default:
					t.Errorf("wrong upstream task URL %s %s", r.Method, r.URL.String())
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			db, server, reopen := directVideoFixture(t, dialect, upstream.URL)
			id := createdVideoID(t, server)
			var token, siteURL, identity string
			if err := db.QueryRow(`SELECT token_value, site_url, direct_identity FROM proxy_video_tasks WHERE public_id = ?`, id).Scan(&token, &siteURL, &identity); err != nil {
				t.Fatal(err)
			}
			if token != "" || siteURL != "" || strings.Contains(identity, "fixture-upstream-key") || strings.Contains(identity, "client-one") {
				t.Fatal("secret persisted in task row")
			}
			for _, path := range []string{"/v1/videos/" + id, "/v1/videos/" + id + "/content", "/v1/videos/" + id + "/remix"} {
				method := "GET"
				if strings.HasSuffix(path, "remix") {
					method = "POST"
				}
				before := calls.Load()
				status, _, _ := videoHTTP(t, server, method, path, `{"prompt":"steal"}`, "client-two")
				if status != 404 || calls.Load() != before {
					t.Fatal("foreign client reached task")
				}
			}
			reopen()
			videoExec(t, db, `UPDATE upstream_credentials SET secret = ?`, "rotated-upstream-key")
			rotated.Store(true)
			status, _, data := videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
			if status != 200 || !strings.Contains(string(data), id) {
				t.Fatalf("cold/rotated poll %d %s", status, data)
			}
			status, headers, data := videoHTTP(t, server, "GET", "/v1/videos/"+id+"/content?variant=thumbnail", "", "client-one")
			if status != 200 || headers.Get("Content-Type") != "image/webp" || string(data) != string([]byte{0, 1, 2, 255, 0, 17}) {
				t.Fatalf("content corrupted: %d %v %v", status, headers, data)
			}
			status, _, data = videoHTTP(t, server, "POST", "/v1/videos/"+id+"/remix", `{"prompt":"make it snow"}`, "client-one")
			var remix map[string]any
			if status != 200 || json.Unmarshal(data, &remix) != nil || remix["remixed_from_video_id"] != id || remix["id"] == id {
				t.Fatalf("remix %d %s", status, data)
			}
			reopen()
			status, _, data = videoHTTP(t, server, "GET", "/v1/videos/"+remix["id"].(string), "", "client-one")
			if status != 200 || !strings.Contains(string(data), id) || strings.Contains(string(data), upstreamID) {
				t.Fatalf("remix poll %d %s", status, data)
			}
			status, _, _ = videoHTTP(t, server, "DELETE", "/v1/videos/"+id, "", "client-one")
			if status != 400 {
				t.Fatalf("failed upstream delete status %d", status)
			}
			if _, err := loadDirectVideoTask(db, id); err != nil {
				t.Fatal("failed delete lost task")
			}
			// Existing grant health policy cools down upstream errors. Advance the
			// fixture past that cooldown before retrying the preserved task.
			videoExec(t, db, `UPDATE upstream_grants SET cooldown_until = NULL`)
			routing.InvalidateCache()
			status, _, data = videoHTTP(t, server, "DELETE", "/v1/videos/"+id, "", "client-one")
			if status != 200 || !strings.Contains(string(data), id) {
				t.Fatalf("delete %d %s", status, data)
			}
			before := calls.Load()
			status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
			if status != 404 || calls.Load() != before {
				t.Fatal("deleted task survived in another worker")
			}
		})
	}
}

func TestDirectVideoTaskRevocationHTTP(t *testing.T) {
	mutations := map[string]string{
		"route renamed":         `UPDATE token_routes SET model_pattern = 'different-model', display_name = 'different-model'`,
		"key disabled":          `UPDATE downstream_api_keys SET enabled = FALSE WHERE key = 'client-one'`,
		"key deleted":           `DELETE FROM downstream_api_keys WHERE key = 'client-one'`,
		"model policy":          `UPDATE downstream_api_keys SET supported_models = '["other"]' WHERE key = 'client-one'`,
		"channel policy":        `UPDATE downstream_api_keys SET access_policy = '{"allowedUpstreamChannelIds":[]}' WHERE key = 'client-one'`,
		"route disabled":        `UPDATE token_routes SET enabled = FALSE`,
		"group disabled":        `UPDATE upstream_groups SET enabled = FALSE`,
		"channel disabled":      `UPDATE upstream_channels SET enabled = FALSE`,
		"grant disabled":        `UPDATE upstream_grants SET enabled = FALSE`,
		"credential disabled":   `UPDATE upstream_credentials SET enabled = FALSE`,
		"model disabled":        `UPDATE upstream_models SET enabled = FALSE`,
		"item deleted":          `DELETE FROM upstream_group_items`,
		"route group deleted":   `DELETE FROM upstream_route_groups`,
		"grant deleted":         `DELETE FROM upstream_grants`,
		"credential deleted":    `DELETE FROM upstream_credentials`,
		"model changed":         `UPDATE upstream_models SET name = 'different'`,
		"protocol revoked":      `UPDATE upstream_grants SET protocols = 2`,
		"item protocol revoked": `UPDATE upstream_group_items SET protocol_order = '[2]'`,
		"endpoint changed":      `UPDATE upstream_channels SET endpoint_config = '{"video":{"url":"http://127.0.0.1:1/foreign/videos","auth":"bearer"}}'`,
		"task deleted":          `DELETE FROM proxy_video_tasks`,
		"task expired":          `UPDATE proxy_video_tasks SET created_at = '2000-01-01T00:00:00Z'`,
	}
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for name, query := range mutations {
				t.Run(name, func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"provider-video","status":"queued"}`)
					}))
					defer upstream.Close()
					db, server, _ := directVideoFixture(t, dialect, upstream.URL)
					id := createdVideoID(t, server)
					task, err := loadDirectVideoTask(db, id)
					if err != nil {
						t.Fatal(err)
					}
					policy := routingPolicyFromAuth(auth.AuthorizeDownstreamToken("client-one", config.Runtime()).Policy)
					policy.RequiredUpstreamProtocol = routing.UpstreamProtocolVideo
					selected, err := getUpstreamConfig().Router.SelectPreferredChannel(context.Background(), "client-alias", -task.Identity.ItemID, policy, nil)
					if err != nil || selected == nil {
						t.Fatalf("cannot prime route cache: %v", err)
					}
					videoExec(t, db, query)
					// Do not invalidate routing cache: revocation must be seen immediately.
					status, _, data := videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
					if status < 400 || calls.Load() != 1 {
						t.Fatalf("revocation bypass: %d calls=%d body=%s", status, calls.Load(), data)
					}
				})
			}
		})
	}
}

func TestDirectVideoTaskPersistenceFailureHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, failure := range []string{"insert", "missing id", "upstream failure"} {
				t.Run(failure, func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						switch failure {
						case "missing id":
							_, _ = io.WriteString(w, `{"status":"queued"}`)
						case "upstream failure":
							w.WriteHeader(400)
							_, _ = io.WriteString(w, `{"error":{"message":"fixture failure"}}`)
						default:
							_, _ = io.WriteString(w, `{"id":"upstream-created","status":"queued"}`)
						}
					}))
					defer upstream.Close()
					db, server, _ := directVideoFixture(t, dialect, upstream.URL)
					table := "proxy_video_tasks"
					if failure == "insert" {
						videoExec(t, db, `ALTER TABLE proxy_video_tasks RENAME TO unavailable_video_tasks`)
						table = "unavailable_video_tasks"
					}
					status, _, data := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"a forest"}`, "client-one")
					if status < 400 || calls.Load() != 1 || strings.Contains(string(data), directVideoIDPrefix) {
						t.Fatalf("failed create was retried or published: %d calls=%d body=%s", status, calls.Load(), data)
					}
					var rows int
					if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&rows); err != nil || rows != 0 {
						t.Fatalf("failed create left mappings: %d %v", rows, err)
					}
				})
			}
		})
	}
}

func TestDirectVideoOwnerIdentity(t *testing.T) {
	id := int64(7)
	pac := &auth.ProxyAuthContext{Source: "managed", KeyID: &id, Token: "old-key"}
	want := directVideoOwner(pac)
	pac.Token = "rotated-key"
	if directVideoOwner(pac) != want {
		t.Fatal("managed key rotation changed task owner")
	}
	id++
	if directVideoOwner(pac) == want {
		t.Fatal("different managed key has the same task owner")
	}
	pac = &auth.ProxyAuthContext{Source: "global", Token: "global-one"}
	want = directVideoOwner(pac)
	pac.Token = "global-two"
	if directVideoOwner(pac) == want {
		t.Fatal("different global tokens have the same task owner")
	}
}

func TestDirectVideoDeleteRequiresDurableStorage(t *testing.T) {
	previous := store.GetDB()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(previous) })
	ctx := &Ctx{videoTask: &directVideoTask{PublicID: directVideoIDPrefix + "missing"}}
	selected := &routing.SelectedChannel{Direct: &store.DirectUpstreamCandidate{}}
	if _, err := processVideoTaskResponse(ctx, selected, http.MethodDelete, "/v1/videos/upstream", nil); err == nil {
		t.Fatal("unavailable task storage was reported as a durable deletion")
	}
}

func TestDirectVideoDeleteRejectsNullResponse(t *testing.T) {
	db, _, _ := directVideoFixture(t, store.DialectSQLite, "http://127.0.0.1:1")
	const id = "video_direct_null_response"
	videoExec(t, db, `INSERT INTO proxy_video_tasks (public_id,upstream_video_id,site_url,token_value) VALUES (?, 'upstream', '', '')`, id)
	ctx := &Ctx{videoTask: &directVideoTask{PublicID: id}}
	selected := &routing.SelectedChannel{Direct: &store.DirectUpstreamCandidate{}}
	if _, err := processVideoTaskResponse(ctx, selected, http.MethodDelete, "/v1/videos/upstream", []byte("null")); err == nil {
		t.Fatal("invalid upstream response accepted")
	}
}

func TestDirectVideoTaskSchemaUpgrade(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db, _, _ := directVideoFixture(t, dialect, "http://127.0.0.1:1")
			videoExec(t, db, `ALTER TABLE proxy_video_tasks DROP COLUMN direct_identity`)
			videoExec(t, db, `DELETE FROM schema_migrations WHERE version = 'direct_video_task_identity_v1'`)
			videoExec(t, db, `INSERT INTO proxy_video_tasks (public_id,upstream_video_id,site_url,token_value) VALUES ('legacy-video','upstream-video','https://example.com','legacy-value')`)
			if err := store.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			var identity *string
			var token string
			if err := db.QueryRow(`SELECT direct_identity,token_value FROM proxy_video_tasks WHERE public_id='legacy-video'`).Scan(&identity, &token); err != nil {
				t.Fatal(err)
			}
			if identity != nil || token != "legacy-value" {
				t.Fatal("upgrade changed native video task")
			}
			if err := store.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDirectVideoTaskUnmappedAndKeyRotationHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"provider-video","status":"queued"}`)
			}))
			defer upstream.Close()
			db, server, _ := directVideoFixture(t, dialect, upstream.URL)
			id := createdVideoID(t, server)
			for _, unknown := range []string{"provider-video", directVideoIDPrefix + "missing"} {
				for _, method := range []string{"GET", "DELETE"} {
					status, _, _ := videoHTTP(t, server, method, "/v1/videos/"+unknown, `{"model":"client-alias"}`, "client-one")
					if status < 400 || calls.Load() != 1 {
						t.Fatal("unmapped task ID reached Direct upstream")
					}
				}
			}
			status, _, _ := videoHTTP(t, server, "DELETE", "/v1/videos/"+id, "", "client-two")
			if status != 404 || calls.Load() != 1 {
				t.Fatal("foreign client deleted task")
			}
			videoExec(t, db, `UPDATE downstream_api_keys SET key = 'client-one-rotated' WHERE key = 'client-one'`)
			status, _, data := videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one-rotated")
			if status != 200 || calls.Load() != 2 || !strings.Contains(string(data), id) {
				t.Fatalf("same managed key rotation lost task: %d %s", status, data)
			}
		})
	}
}

func TestDirectVideoTaskExplicitGroupRevocationHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, mutation := range []string{"disabled", "unlinked"} {
				t.Run(mutation, func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"id":"provider-video","status":"queued"}`)
					}))
					defer upstream.Close()
					db, server, _ := directVideoFixture(t, dialect, upstream.URL)
					var source, group int64
					if err := db.QueryRow(`SELECT id FROM token_routes WHERE model_pattern='client-alias'`).Scan(&source); err != nil {
						t.Fatal(err)
					}
					if err := db.QueryRow(`INSERT INTO token_routes (model_pattern,display_name,route_mode,enabled,created_at,updated_at) VALUES ('group-alias','group-alias','explicit_group',TRUE,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z') RETURNING id`).Scan(&group); err != nil {
						t.Fatal(err)
					}
					videoExec(t, db, `INSERT INTO route_group_sources (group_route_id,source_route_id) VALUES (?,?)`, group, source)
					videoExec(t, db, `UPDATE downstream_api_keys SET supported_models='["group-alias"]'`)
					status, _, data := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"group-alias","prompt":"forest"}`, "client-one")
					var created struct {
						ID string `json:"id"`
					}
					if status != 200 || json.Unmarshal(data, &created) != nil || !strings.HasPrefix(created.ID, directVideoIDPrefix) {
						t.Fatalf("group create %d %s", status, data)
					}
					task, err := loadDirectVideoTask(db, created.ID)
					if err != nil {
						t.Fatal(err)
					}
					policy := routingPolicyFromAuth(auth.AuthorizeDownstreamToken("client-one", config.Runtime()).Policy)
					policy.RequiredUpstreamProtocol = routing.UpstreamProtocolVideo
					if selected, err := getUpstreamConfig().Router.SelectPreferredChannel(context.Background(), "group-alias", -task.Identity.ItemID, policy, nil); err != nil || selected == nil {
						t.Fatalf("cannot prime group route: %v", err)
					}
					if mutation == "disabled" {
						videoExec(t, db, `UPDATE token_routes SET enabled=FALSE WHERE id=?`, group)
					} else {
						videoExec(t, db, `DELETE FROM route_group_sources WHERE group_route_id=?`, group)
					}
					status, _, data = videoHTTP(t, server, "GET", "/v1/videos/"+created.ID, "", "client-one")
					if status < 400 || calls.Load() != 1 {
						t.Fatalf("revoked group escaped warm cache: %d calls=%d %s", status, calls.Load(), data)
					}
				})
			}
		})
	}
}
