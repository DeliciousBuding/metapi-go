package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestRouteDecisionEndpointContextAcrossAllQueries(t *testing.T) {
	for _, tc := range []struct{ path, payload string }{
		{"/api/routes/decision?model=coding&endpoint=/v1/responses", ""},
		{"/api/routes/decision/batch", `{"models":["coding"],"endpoint":"/v1/responses"}`},
		{"/api/routes/decision/by-route/batch", `{"items":[{"routeId":1,"model":"coding"}],"endpoint":"/v1/responses"}`},
		{"/api/routes/decision/route-wide/batch", `{"routeIds":[1],"endpoint":"/v1/responses"}`},
	} {
		t.Run(tc.path, func(t *testing.T) {
			fake := &fakeDecisionRouter{}
			_, mux := setupTokenRoutesDecisionTest(t, TokenRoutesDeps{Router: fake})
			method := http.MethodPost
			if tc.payload == "" {
				method = http.MethodGet
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(method, tc.path, strings.NewReader(tc.payload)))
			if rec.Code != 200 || len(fake.policies) != 1 {
				t.Fatalf("query: %d %s", rec.Code, rec.Body.String())
			}
			p := fake.policies[0]
			if p.RequiredUpstreamProtocol != store.DirectProtocolResponses || !p.AllowUpstreamProtocolConversion || p.AccessPolicy != nil {
				t.Fatalf("wrong administrator protocol context: %+v", p)
			}
		})
	}
	for _, endpoint := range []string{"/v1/unknown", "/v1/../chat/completions", "/v1/responses?token=fixture", "https://fixture.example/v1/responses", "/v1/%2e%2e/responses"} {
		fake := &fakeDecisionRouter{}
		_, mux := setupTokenRoutesDecisionTest(t, TokenRoutesDeps{Router: fake})
		rec := doGet(t, mux, "/api/routes/decision?model=coding&endpoint="+url.QueryEscape(endpoint))
		if rec.Code != 400 || len(fake.policies) != 0 {
			t.Fatalf("invalid endpoint accepted: %q status=%d", endpoint, rec.Code)
		}
	}
}

func TestRouteDecisionDirectPreviewReadOnlyAndSecretSafe(t *testing.T) {
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
			if err := store.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			var upstreamCalls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				upstreamCalls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer upstream.Close()
			fixture := strings.ReplaceAll(backupsvcFixtureV5, "https://upstream.example/v1", upstream.URL+"/v1")
			if _, err := backup.ImportOctopusV5(db, []byte(fixture), "explanation-test"); err != nil {
				t.Fatal(err)
			}
			endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/v1/chat/completions", Auth: store.DirectAuthBearer}}
			if _, err := db.Exec(`UPDATE upstream_channels SET endpoint_config=?`, endpoints); err != nil {
				t.Fatal(err)
			}
			previous := config.RuntimeSafe()
			config.SetRuntime(&config.RuntimeSettings{RoutingFallbackUnitCost: 1})
			defer config.SetRuntime(previous)
			defer routing.SetGlobalCache(nil)
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
			mux := chi.NewRouter()
			RegisterTokenRoutesWithDeps(mux, db.DB, TokenRoutesDeps{Router: router})
			// Compare the persisted selection/cooldown/configuration and log rows;
			// preview must not consume/rotate a candidate or save a snapshot.
			capture := func() string {
				state := map[string][]map[string]any{}
				for _, table := range []string{"upstream_grants", "upstream_group_items", "token_routes", "proxy_logs"} {
					rows, err := db.Queryx("SELECT * FROM " + table + " ORDER BY id")
					if err != nil {
						t.Fatal(err)
					}
					for rows.Next() {
						row := map[string]any{}
						if err := rows.MapScan(row); err != nil {
							t.Fatal(err)
						}
						state[table] = append(state[table], row)
					}
					if err := rows.Err(); err != nil {
						t.Fatal(err)
					}
					rows.Close()
				}
				encoded, err := json.Marshal(state)
				if err != nil {
					t.Fatal(err)
				}
				return string(encoded)
			}
			before := capture()
			for _, tc := range []struct{ endpoint, status string }{
				{"", "notEvaluated"},
				{"/v1/chat/completions", "eligible"},
				{"/v1/responses", "eligible"},
				{"/v1/messages/count_tokens", "blocked"},
				{"/v1/embeddings", "blocked"},
			} {
				rec := doGet(t, mux, "/api/routes/decision?model=client-model&endpoint="+url.QueryEscape(tc.endpoint))
				if rec.Code != 200 {
					t.Fatalf("preview: %d %s", rec.Code, rec.Body.String())
				}
				var body struct {
					Decision routing.RouteDecisionExplanation `json:"decision"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				d := body.Decision
				if !d.Matched || len(d.Candidates) != 2 || d.CheckedAt == "" || len(d.NotEvaluated) < 4 {
					t.Fatalf("incomplete preview: %s", rec.Body.String())
				}
				for _, c := range d.Candidates {
					if c.Eligibility != tc.status || c.CandidateRef.Kind != "direct" || c.CandidateRef.ID <= 0 || c.CandidateRef.GrantID <= 0 || c.CandidateRef.GroupID <= 0 || c.ActualModel != "provider-model" || c.ProbabilityEvaluated {
						t.Fatalf("endpoint %q: incorrect direct candidate: %+v", tc.endpoint, c)
					}
				}
				for _, private := range []string{"fixture-key-octopus", "baseUrl", upstream.URL, "customHeader", "account-0"} {
					if strings.Contains(rec.Body.String(), private) {
						t.Fatalf("private field escaped: %s", private)
					}
				}
			}
			if before != capture() || upstreamCalls.Load() != 0 {
				t.Fatal("route preview changed persistent state or contacted upstream")
			}
			// A preview with missing context never makes execution permissive.
			selected, err := router.SelectChannel(t.Context(), "client-model", routing.EmptyDownstreamRoutingPolicy)
			if err != nil || selected != nil {
				t.Fatalf("execution missing protocol must deny: %v", err)
			}
		})
	}
}
