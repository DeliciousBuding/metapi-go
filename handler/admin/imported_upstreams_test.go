package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestImportedUpstreamsAvailabilityAndSecretBoundary(t *testing.T) {
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
			if _, err = backupsvc.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "test-availability"); err != nil {
				t.Fatal(err)
			}
			mux := chi.NewRouter()
			RegisterImportedUpstreamRoutes(mux, db.DB)
			rec := doGet(t, mux, "/api/imported-upstreams")
			if rec.Code != 200 || strings.Contains(rec.Body.String(), "fixture-key-octopus") || strings.Contains(rec.Body.String(), "customHeader") {
				t.Fatalf("unsafe inventory: %s", rec.Body.String())
			}
			var body struct {
				Items []struct {
					ID      int64 `json:"id"`
					Enabled bool  `json:"enabled"`
				}
				Members []map[string]any `json:"members"`
			}
			if err = json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Items) != 2 || !body.Items[0].Enabled || len(body.Members) == 0 {
				t.Fatalf("incomplete inventory: %s", rec.Body.String())
			}
			prev := config.RuntimeSafe()
			config.SetRuntime(&config.RuntimeSettings{RoutingFallbackUnitCost: 1})
			defer config.SetRuntime(prev)
			defer routing.SetGlobalCache(nil)
			tokenRouter := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
			policy := routing.EmptyDownstreamRoutingPolicy
			policy.RequiredUpstreamProtocol = routing.UpstreamProtocolChat
			selected, err := tokenRouter.SelectChannel(t.Context(), "client-model", policy)
			if err != nil || selected == nil || selected.Direct == nil {
				t.Fatalf("initial selection: %+v %v", selected, err)
			}
			disabledID := selected.Direct.ChannelID
			path := "/api/imported-upstreams/" + strconv.FormatInt(disabledID, 10)
			patch := func(payload string) *httptest.ResponseRecorder {
				out := httptest.NewRecorder()
				mux.ServeHTTP(out, httptest.NewRequest(http.MethodPatch, path, strings.NewReader(payload)))
				return out
			}
			for _, invalid := range []string{`{}`, `{"enabled":null}`, `{"enabled":"false"}`, `{"enabled":false,"secret":"unsafe"}`, `{"enabled":false} {}`} {
				if result := patch(invalid); result.Code != 400 {
					t.Fatalf("invalid update accepted: %s status=%d", invalid, result.Code)
				}
			}
			if result := patch(`{"enabled":false}`); result.Code != 200 {
				t.Fatalf("disable: %s", result.Body.String())
			}
			selected, err = tokenRouter.SelectChannel(t.Context(), "client-model", policy)
			if err != nil || selected == nil || selected.Direct == nil || selected.Direct.ChannelID == disabledID {
				t.Fatalf("disabled channel persisted in hot route cache: %+v %v", selected, err)
			}
			if result := patch(`{"enabled":true}`); result.Code != 200 {
				t.Fatalf("enable: %s", result.Body.String())
			}
			primary, err := tokenRouter.SelectChannel(t.Context(), "client-model", policy)
			if err != nil || primary == nil || primary.Direct == nil {
				t.Fatalf("re-enabled selection failed: %v", err)
			}
			status := http.StatusServiceUnavailable
			if err := tokenRouter.RecordFailure(t.Context(), primary.Channel.ID, routing.SiteRuntimeFailureContext{Status: &status}, nil); err != nil {
				t.Fatal(err)
			}
			// A new router must read persisted cooldown, not a process-local flag.
			restarted := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
			sibling, err := restarted.SelectChannel(t.Context(), "client-model", policy)
			if err != nil || sibling == nil || sibling.Direct == nil || sibling.Direct.GrantID == primary.Direct.GrantID {
				t.Fatalf("failed imported grant was immediately retried: %+v %v", sibling, err)
			}
			if err := restarted.RecordSuccess(t.Context(), primary.Channel.ID, 250, 0.001, nil, nil); err != nil {
				t.Fatal(err)
			}
			var state struct {
				Failures  int64   `db:"fail_count"`
				Successes int64   `db:"success_count"`
				Latency   int64   `db:"total_latency_ms"`
				Cooldown  *string `db:"cooldown_until"`
			}
			if err := db.Get(&state, "SELECT fail_count,success_count,total_latency_ms,cooldown_until FROM upstream_grants WHERE id=?", primary.Direct.GrantID); err != nil {
				t.Fatal(err)
			}
			if state.Successes != 1 || state.Latency != 250 || state.Cooldown != nil {
				t.Fatalf("direct grant recovery not persisted: %+v", state)
			}
		})
	}
}
