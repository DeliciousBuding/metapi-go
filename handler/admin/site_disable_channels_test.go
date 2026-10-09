package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestSiteDisableUpdatesWarmChannelListsAndRouting(t *testing.T) {
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(nil) })
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			var mux chi.Router
			if dialect == store.DialectPostgres {
				db, mux = setupTokenRoutesPostgresTest(t)
			} else {
				db, mux = setupTokenRoutesTest(t)
			}
			globalChannelsCache.clear()
			globalChannelsErrorSummaryCache.clear()
			RegisterSitesRoutes(mux, db.DB)
			routeID, accountID, _ := seedChannelsCacheFleet(t, db, 1)
			var siteID int64
			if err := db.Get(&siteID, `SELECT site_id FROM accounts WHERE id=?`, accountID); err != nil {
				t.Fatal(err)
			}
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
			t.Cleanup(func() { routing.SetGlobalCache(nil) })
			if selected, err := router.SelectChannel(context.Background(), "gpt-cache-0", routing.DownstreamRoutingPolicy{}); err != nil || selected == nil {
				t.Fatalf("fixture not routable: %v", err)
			}
			requireCacheState(t, mux, "/api/channels", "miss")
			requireCacheState(t, mux, "/api/channels", "hit")
			doGet(t, mux, "/api/channels/error-summary")
			setStatus := func(status string) {
				req := httptest.NewRequest(http.MethodPut, "/api/sites/"+strconv.FormatInt(siteID, 10), strings.NewReader(`{"status":"`+status+`"}`))
				out := httptest.NewRecorder()
				mux.ServeHTTP(out, req)
				if out.Code != 200 {
					t.Fatalf("site update: %d %s", out.Code, out.Body.String())
				}
			}
			setStatus("disabled")
			out := requireCacheState(t, mux, "/api/channels", "miss")
			var body struct {
				Items []struct {
					Status  string `json:"status"`
					Enabled bool   `json:"enabled"`
				} `json:"items"`
			}
			if err := json.Unmarshal(out.Body.Bytes(), &body); err != nil || len(body.Items) != 1 || body.Items[0].Status != routing.ChannelStatusManuallyDisabled || !body.Items[0].Enabled {
				t.Fatalf("inherited disable/configured state incorrect: %s %v", out.Body.String(), err)
			}
			if selected, err := router.SelectChannel(context.Background(), "gpt-cache-0", routing.DownstreamRoutingPolicy{}); err != nil || selected != nil {
				t.Fatalf("disabled site was still selected: %v %v", selected, err)
			}
			filtered := doGet(t, mux, "/api/channels?status=enabled")
			if !strings.Contains(filtered.Body.String(), `"total":0`) {
				t.Fatalf("enabled filter retained disabled site: %s", filtered.Body.String())
			}
			summary := doGet(t, mux, "/api/channels/error-summary")
			if summary.Header().Get("x-channels-error-summary-cache") != "miss" || !strings.Contains(summary.Body.String(), `"manually_disabled":1`) {
				t.Fatalf("stale summary: %s", summary.Body.String())
			}
			var enabled bool
			if err := db.Get(&enabled, `SELECT enabled FROM token_routes WHERE id=?`, routeID); err != nil || !enabled {
				t.Fatalf("shared route configuration was changed: %v", err)
			}
			setStatus("active")
			if selected, err := router.SelectChannel(context.Background(), "gpt-cache-0", routing.DownstreamRoutingPolicy{}); err != nil || selected == nil {
				t.Fatalf("reenabled site not routable: %v", err)
			}
			deleteReq := httptest.NewRequest(http.MethodDelete, "/api/sites/"+strconv.FormatInt(siteID, 10), nil)
			deleted := httptest.NewRecorder()
			mux.ServeHTTP(deleted, deleteReq)
			if deleted.Code != 200 {
				t.Fatalf("delete failed: %s", deleted.Body.String())
			}
			var remaining int
			if err := db.Get(&remaining, `SELECT COUNT(*) FROM route_channels WHERE route_id=?`, routeID); err != nil || remaining != 0 {
				t.Fatalf("delete left orphaned channels: %d %v", remaining, err)
			}
		})
	}
}
