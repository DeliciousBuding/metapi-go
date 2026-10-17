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
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Header.Get("Authorization") {
				case "Bearer fixture-member-one":
					first.Add(1)
				case "Bearer fixture-member-two":
					second.Add(1)
				default:
					t.Error("unexpected upstream credential")
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
			r.Use(auth.ProxyAuth())
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
					})
				}
			}
		})
	}
}
