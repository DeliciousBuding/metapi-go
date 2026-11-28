package proxyhandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestOAuthPoolDownstreamPolicyHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			if dialect == store.DialectSQLite {
				db = sqlitetest.Open(t)
			} else {
				dsn := os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
				var err error
				db, err = store.Open(dialect, dsn, false)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				if err := store.AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
				pgtest.Reset(t, db.DB)
			}
			var first, second atomic.Int64
			var fail atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Header.Get("Authorization") {
				case "Bearer fixture-member-one":
					first.Add(1)
				case "Bearer fixture-member-two":
					second.Add(1)
				default:
					t.Error("unexpected upstream credential")
				}
				if r.Method == http.MethodGet && !fail.Load() {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"data":[{"id":"pool-model"}]}`))
					return
				}
				if fail.Load() {
					http.Error(w, "fixture rejection", 502)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"fixture","model":"pool-model","choices":[{"index":0,"message":{"role":"assistant","content":"receipt"},"finish_reason":"stop"}]}`))
			}))
			t.Cleanup(upstream.Close)
			for _, id := range []int{1, 2} {
				videoExec(t, db, `INSERT INTO sites (id,name,url,platform,status,created_at,updated_at) VALUES (?,?,?,'openai','active','2026-10-17T00:00:00Z','2026-10-17T00:00:00Z')`, id, fmt.Sprintf("Site %d", id), upstream.URL+fmt.Sprintf("/site%d", id))
				credential := []string{"fixture-member-one", "fixture-member-two"}[id-1]
				videoExec(t, db, `INSERT INTO accounts (id,site_id,access_token,api_token,status,created_at,updated_at) VALUES (?,?,'',?,'active','2026-10-17T00:00:00Z','2026-10-17T00:00:00Z')`, id, id, credential)
			}
			videoExec(t, db, `INSERT INTO oauth_route_units (id,site_id,provider,name,strategy,enabled) VALUES (1,1,'fixture','Pool','round_robin',TRUE)`)
			videoExec(t, db, `INSERT INTO oauth_route_unit_members (id,unit_id,account_id,sort_order) VALUES (1,1,1,0),(2,1,2,1)`)
			videoExec(t, db, `INSERT INTO token_routes (id,model_pattern,route_mode,routing_strategy,enabled) VALUES (1,'pool-model','pattern','round_robin',TRUE)`)
			videoExec(t, db, `INSERT INTO route_channels (id,route_id,account_id,oauth_route_unit_id,source_model,enabled) VALUES (1,1,1,1,'pool-model',TRUE)`)
			for _, table := range []string{"token_routes", "oauth_route_units", "oauth_route_unit_members"} {
				videoExec(t, db, `UPDATE `+table+` SET created_at='2026-10-17T00:00:00Z',updated_at='2026-10-17T00:00:00Z'`)
			}
			previousDB, previousRuntime, previousUpstream := store.GetDB(), config.RuntimeSafe(), getUpstreamConfig()
			store.OverrideDB(db)
			config.SetRuntime(&config.RuntimeSettings{})
			t.Cleanup(func() {
				store.OverrideDB(previousDB)
				config.SetRuntime(previousRuntime)
				SetUpstreamConfig(previousUpstream)
				routing.SetGlobalCache(nil)
			})
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
			SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
			r := chi.NewRouter()
			r.Use(auth.ProxyAuth(store.GetDB()))
			r.Route("/v1", RegisterProxyRoutes)
			for _, strategy := range []string{"round_robin", "stick_until_unavailable"} {
				videoExec(t, db, `UPDATE oauth_route_units SET strategy=?`, strategy)
				routing.InvalidateCache()
				for n, tc := range []struct {
					name, column, value string
					want                int
				}{
					{"unrestricted", "allowed_site_ids", `[]`, 1},
					{"site allow", "allowed_site_ids", `[2]`, 2},
					{"site exclude", "excluded_site_ids", `[1]`, 2},
					{"credential allow", "allowed_credential_refs", `[{"kind":"default_api_key","siteId":2,"accountId":2}]`, 2},
					{"credential exclude", "excluded_credential_refs", `[{"kind":"default_api_key","siteId":1,"accountId":1}]`, 2},
					{"no allowed site", "allowed_site_ids", `[3]`, 0},
					{"all sites excluded", "excluded_site_ids", `[1,2]`, 0},
					{"direct-only policy", "access_policy", `{"allowedUpstreamChannelIds":[10]}`, 0},
				} {
					t.Run(strategy+"/"+tc.name, func(t *testing.T) {
						videoExec(t, db, `UPDATE oauth_route_unit_members SET last_selected_at=NULL,last_used_at=NULL`)
						routing.InvalidateCache()
						key := fmt.Sprintf("sk-pool-%s-%d", strategy, n)
						videoExec(t, db, `INSERT INTO downstream_api_keys (name,key,enabled,supported_models,`+tc.column+`) VALUES (?,?,TRUE,'["pool-model"]',?)`, key, key, tc.value)
						var before, after [2]int64
						for i := range before {
							if err := db.Get(&before[i], db.Rebind(`SELECT COALESCE(success_count,0) FROM oauth_route_unit_members WHERE account_id=?`), i+1); err != nil {
								t.Fatal(err)
							}
						}
						a, b := first.Load(), second.Load()
						req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"pool-model","messages":[{"role":"user","content":"hello"}]}`))
						req.Header.Set("Authorization", "Bearer "+key)
						req.Header.Set("Content-Type", "application/json")
						out := httptest.NewRecorder()
						r.ServeHTTP(out, req)
						if tc.want == 0 {
							if out.Code < 400 || first.Load() != a || second.Load() != b {
								t.Fatalf("denied policy dispatched: status=%d first=%d second=%d", out.Code, first.Load()-a, second.Load()-b)
							}
						} else if out.Code != 200 || first.Load()-a != int64(2-tc.want) || second.Load()-b != int64(tc.want-1) {
							t.Fatalf("policy selected wrong member: status=%d first=%d second=%d body=%s", out.Code, first.Load()-a, second.Load()-b, out.Body.String())
						}
						for i := range after {
							if err := db.Get(&after[i], db.Rebind(`SELECT COALESCE(success_count,0) FROM oauth_route_unit_members WHERE account_id=?`), i+1); err != nil {
								t.Fatal(err)
							}
						}
						if after[0]-before[0] != first.Load()-a || after[1]-before[1] != second.Load()-b {
							t.Fatalf("member successes %v -> %v, requests %d/%d", before, after, first.Load()-a, second.Load()-b)
						}
					})
				}
			}
			fail.Store(true)
			rkey := "sk-pool-member-b-failure"
			videoExec(t, db, `INSERT INTO downstream_api_keys(name,key,enabled,supported_models,allowed_site_ids) VALUES(?,?,TRUE,'["pool-model"]','[2]')`, rkey, rkey)
			req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"pool-model","messages":[{"role":"user","content":"hello"}]}`))
			req.Header.Set("Authorization", "Bearer "+rkey)
			out := httptest.NewRecorder()
			r.ServeHTTP(out, req)
			var failures []int64
			if err := db.Select(&failures, `SELECT COALESCE(fail_count,0) FROM oauth_route_unit_members ORDER BY account_id`); err != nil {
				t.Fatal(err)
			}
			if out.Code != 502 || len(failures) != 2 || failures[0] != 0 || failures[1] != 1 {
				t.Fatalf("member failure attribution status=%d counts=%v", out.Code, failures)
			}

			probe := proxy.NewChannelHealthProbeExecutor(db, &config.Config{})
			videoExec(t, db, `UPDATE oauth_route_unit_members SET cooldown_until='2099-01-01T00:00:00Z'`)
			for _, success := range []bool{false, true} {
				fail.Store(!success)
				a, b := first.Load(), second.Load()
				target := proxy.ChannelProbeTarget{ChannelID: 1, AccountID: 2, SiteID: 2, ModelName: "pool-model"}
				res, err := probe.ProbeChannel(context.Background(), target)
				if err != nil || res.AccountID != 2 || res.SiteID != 2 || first.Load() != a || second.Load() != b+1 {
					t.Fatalf("probe identity=%+v hits=%d/%d err=%v", res, first.Load()-a, second.Load()-b, err)
				}
				if success {
					if res.Status != "success" {
						t.Fatal(res)
					}
					err = router.RecordProbeSuccess(context.Background(), 1, res.LatencyMs, &target.ModelName, &res.AccountID)
				} else {
					if res.Status != "failure" {
						t.Fatal(res)
					}
					err = router.RecordProbeFailure(context.Background(), 1, routing.SiteRuntimeFailureContext{Status: &res.HTTPStatus, ModelName: &target.ModelName}, &res.AccountID)
				}
				if err != nil {
					t.Fatal(err)
				}
				var cooldowns []string
				if err = db.Select(&cooldowns, `SELECT COALESCE(cooldown_until,'') FROM oauth_route_unit_members ORDER BY account_id`); err != nil || len(cooldowns) != 2 || cooldowns[0] != "2099-01-01T00:00:00Z" || (cooldowns[1] == "") != success {
					t.Fatalf("probe success=%v cooldowns=%v err=%v", success, cooldowns, err)
				}
			}
		})
	}
}
