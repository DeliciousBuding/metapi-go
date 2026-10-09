package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

var _ routing.ChannelSelectorDB = (*service.ProxyRoutingStore)(nil)
var _ routing.ChannelLoadSnapshotProvider = proxyLoadProvider{}
var _ proxy.TokenRouterInterface = (*routing.TokenRouter)(nil)

func TestConfigureProxyUpstreamWiresRealSQLiteRouter(t *testing.T) {
	_ = store.CloseDatabase()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("upstream path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer upstream-token" {
			t.Fatalf("Authorization = %q, want upstream token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_real_route","choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	cfg := testProxyConfig(t)
	t.Cleanup(func() {
		proxyhandler.SetUpstreamConfig(nil)
		scheduler.SetActiveChannelIDsProvider(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ShutdownProxyLogBatchWriter(ctx)
		_ = store.CloseDatabase()
	})
	config.Set(cfg)
	if err := store.EnsureRuntimeDatabase(cfg, config.RuntimeSafe()); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	db := store.GetDB()
	channelID := seedProxyRoute(t, db, upstream.URL, "gpt-real", "upstream-token")

	if err := ConfigureProxyUpstream(cfg); err != nil {
		t.Fatalf("ConfigureProxyUpstream: %v", err)
	}

	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.ProxyAuth())
		proxyhandler.RegisterProxyRoutes(r)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-real","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer downstream-token")
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "chatcmpl_real_route") {
		t.Fatalf("body = %q, want upstream response", body)
	}

	var lastSelected string
	if err := db.QueryRow(`SELECT last_selected_at FROM route_channels WHERE id = ?`, channelID).Scan(&lastSelected); err != nil {
		t.Fatalf("read last_selected_at: %v", err)
	}
	if strings.TrimSpace(lastSelected) == "" {
		t.Fatal("last_selected_at was not updated by real router")
	}
}

func TestOctopusV5DirectGrantTraversesProxyRouter(t *testing.T) {
	_ = store.CloseDatabase()
	var upstreamCalls atomic.Int32
	var proxyCalls atomic.Int32
	var upstreamSeenMu sync.Mutex
	var upstreamSeen []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		auth := r.Header.Get("Authorization")
		upstreamSeenMu.Lock()
		upstreamSeen = append(upstreamSeen, auth)
		upstreamSeenMu.Unlock()
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("upstream path = %q", r.URL.Path)
		}
		if auth == "Bearer octopus-secret-1" {
			if got := r.Header.Get("X-Octopus-Test"); got != "channel-one" {
				t.Errorf("channel one custom header = %q", got)
			}
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"retry fixture"}`))
			return
		}
		if auth != "Bearer octopus-secret-2" {
			t.Errorf("Authorization = %q", auth)
		}
		if got := r.Header.Get("X-Octopus-Test"); got != "channel-two" {
			t.Errorf("channel two custom header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"direct-chat","choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	proxySpy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		http.Error(w, "proxy=false should ignore this residual proxy", http.StatusBadGateway)
	}))
	t.Cleanup(proxySpy.Close)
	cfg := testProxyConfig(t)
	config.Set(cfg)
	if err := store.EnsureRuntimeDatabase(cfg, config.RuntimeSafe()); err != nil {
		t.Fatal(err)
	}
	db := store.GetDB()
	fixture := fmt.Sprintf(`{"version":5,"exported_at":"2026-10-06T00:00:00Z","channels":[{"id":11,"name":"direct one","dialect":"generic","enabled":true,"base_url":%q,"openai_chat_completion_path":"/v1/chat/completions","openai_response_path":"/v1/responses","anthropic_message_path":"/v1/messages","proxy":false,"channel_proxy":"","custom_header":[{"header_key":"X-Octopus-Test","header_value":"channel-one"}],"param_override":"","match_regex":""},{"id":12,"name":"direct two","dialect":"generic","enabled":true,"base_url":%q,"openai_chat_completion_path":"/v1/chat/completions","openai_response_path":"/v1/responses","anthropic_message_path":"/v1/messages","proxy":false,"channel_proxy":"","custom_header":[{"header_key":"X-Octopus-Test","header_value":"channel-two"}],"param_override":"","match_regex":""}],"channel_keys":[{"id":13,"channel_id":11,"name":"primary","key":"octopus-secret-1","enabled":true},{"id":14,"channel_id":12,"name":"backup","key":"octopus-secret-2","enabled":true}],"channel_models":[{"id":15,"channel_id":11,"name":"provider-model"},{"id":16,"channel_id":12,"name":"provider-model"}],"channel_grants":[{"id":17,"channel_model_id":15,"channel_key_id":13,"protocols":2},{"id":18,"channel_model_id":16,"channel_key_id":14,"protocols":2}],"groups":[{"id":21,"name":"octopus-chat","mode":"failover","active_item_id":31,"relay_config":{}}],"group_items":[{"id":31,"group_id":21,"channel_grant_id":17,"priority":0,"weight":1},{"id":32,"group_id":21,"channel_grant_id":18,"priority":1,"weight":1}],"api_keys":[],"llm_infos":[],"settings":[],"stats_total":[],"stats_daily":[],"stats_hourly":[],"stats_api_key":[]}`, upstream.URL, upstream.URL)
	fixture = strings.Replace(fixture, `"channel_proxy":""`, `"channel_proxy":`+fmt.Sprintf("%q", proxySpy.URL), 1)
	if _, err := backup.ImportOctopusV5(db, []byte(fixture), "direct-e2e"); err != nil {
		t.Fatalf("import fixture: %v", err)
	}
	var routeID, grantOneID, grantTwoID int64
	for _, entry := range []struct {
		query string
		dest  *int64
	}{
		{`SELECT id FROM token_routes WHERE model_pattern = 'octopus-chat'`, &routeID},
		{`SELECT id FROM upstream_grants WHERE source_id = 17`, &grantOneID},
		{`SELECT id FROM upstream_grants WHERE source_id = 18`, &grantTwoID},
	} {
		if err := db.Get(entry.dest, entry.query); err != nil {
			t.Fatalf("load imported route/grant ID: %v", err)
		}
	}
	selector := routing.NewTokenRouter(service.NewProxyRoutingStore(db), cfg, nil, nil)
	basePolicy := routing.EmptyDownstreamRoutingPolicy
	basePolicy.RequiredUpstreamProtocol = routing.UpstreamProtocolChat
	withRoute := func(id int64) routing.DownstreamRoutingPolicy {
		policy := basePolicy
		policy.AllowedRouteIDs = []int64{id}
		return policy
	}
	withAllowedGrant := func(id int64) routing.DownstreamRoutingPolicy {
		policy := basePolicy
		policy.AllowedCredentialRefs = []routing.CredentialRef{{Kind: "direct_grant", GrantID: id}}
		return policy
	}
	withExcludedGrant := func(id int64) routing.DownstreamRoutingPolicy {
		policy := basePolicy
		policy.ExcludedCredentialRefs = []routing.CredentialRef{{Kind: "direct_grant", GrantID: id}}
		return policy
	}
	withSiteRestriction := func(id int64) routing.DownstreamRoutingPolicy {
		policy := basePolicy
		policy.AllowedSiteIDs = []int64{id}
		return policy
	}
	for _, tc := range []struct {
		name     string
		policy   routing.DownstreamRoutingPolicy
		wantID   int64
		wantNone bool
	}{
		{name: "unrestricted", policy: basePolicy, wantID: grantOneID},
		{name: "allowed route", policy: withRoute(routeID), wantID: grantOneID},
		{name: "route outside allow-list", policy: withRoute(routeID + 1), wantNone: true},
		{name: "allowed direct grant", policy: withAllowedGrant(grantOneID), wantID: grantOneID},
		{name: "different allowed grant", policy: withAllowedGrant(grantTwoID), wantID: grantTwoID},
		{name: "excluded direct grant falls through", policy: withExcludedGrant(grantOneID), wantID: grantTwoID},
		{name: "direct grant has no site identity", policy: withSiteRestriction(1), wantNone: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := selector.SelectChannel(t.Context(), "octopus-chat", tc.policy)
			if err != nil {
				t.Fatalf("SelectChannel: %v", err)
			}
			if tc.wantNone {
				if selected != nil {
					t.Fatalf("selected grant %d, want no eligible route", selected.Direct.GrantID)
				}
				return
			}
			if selected == nil || selected.Direct == nil || selected.Direct.GrantID != tc.wantID {
				t.Fatalf("selected=%+v, want direct grant %d", selected, tc.wantID)
			}
		})
	}
	for _, tc := range []struct {
		table    string
		sourceID int64
	}{
		{"upstream_channels", 11},
		{"upstream_credentials", 13},
		{"upstream_models", 15},
		{"upstream_grants", 17},
		{"upstream_groups", 21},
	} {
		t.Run("disabled "+tc.table, func(t *testing.T) {
			if _, err := db.Exec(fmt.Sprintf(`UPDATE %s SET enabled = ? WHERE origin_key = ? AND source_id = ?`, tc.table), false, "direct-e2e", tc.sourceID); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			selected, err := selector.SelectChannel(t.Context(), "octopus-chat", withAllowedGrant(grantOneID))
			if err != nil || selected != nil {
				t.Fatalf("disabled %s remained selectable: selected=%+v err=%v", tc.table, selected, err)
			}
			if _, err := db.Exec(fmt.Sprintf(`UPDATE %s SET enabled = ? WHERE origin_key = ? AND source_id = ?`, tc.table), true, "direct-e2e", tc.sourceID); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
		})
	}
	if err := ConfigureProxyUpstream(cfg); err != nil {
		t.Fatalf("ConfigureProxyUpstream: %v", err)
	}
	t.Cleanup(func() {
		proxyhandler.SetUpstreamConfig(nil)
		scheduler.SetActiveChannelIDsProvider(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ShutdownProxyLogBatchWriter(ctx)
		_ = store.CloseDatabase()
	})
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.ProxyAuth())
		proxyhandler.RegisterProxyRoutes(r)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"octopus-chat","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer downstream-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "direct-chat") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var logs, withAccount, withNativeChannel, directRefs int
	if err := db.Get(&logs, `SELECT COUNT(*) FROM proxy_logs`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&withAccount, `SELECT COUNT(*) FROM proxy_logs WHERE account_id IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&withNativeChannel, `SELECT COUNT(*) FROM proxy_logs WHERE channel_id IS NOT NULL`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&directRefs, `SELECT COUNT(*) FROM proxy_logs WHERE upstream_channel_id > 0 AND upstream_grant_id > 0`); err != nil {
		t.Fatal(err)
	}
	if logs != 2 || withAccount != 0 || withNativeChannel != 0 || directRefs != 2 {
		t.Fatalf("proxy logs=%d native channel refs=%d account refs=%d typed direct refs=%d; want two attempts with typed upstream refs only", logs, withNativeChannel, withAccount, directRefs)
	}
	negative := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"octopus-chat","input":"hello"}`))
	negative.Header.Set("Content-Type", "application/json")
	negative.Header.Set("Authorization", "Bearer downstream-token")
	negativeRec := httptest.NewRecorder()
	r.ServeHTTP(negativeRec, negative)
	if negativeRec.Code == http.StatusOK || upstreamCalls.Load() != 2 {
		t.Fatalf("unauthorized Responses protocol was forwarded: status=%d upstream calls=%d", negativeRec.Code, upstreamCalls.Load())
	}
	var activeItemID int64
	if err := db.Get(&activeItemID, `SELECT target_id FROM external_source_ids WHERE origin_key = ? AND entity_type = 'group_items' AND source_id = 32`, "direct-e2e"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE upstream_groups SET mode = 'manual', active_item_id = ? WHERE origin_key = ? AND source_id = ?`, activeItemID, "direct-e2e", 21); err != nil {
		t.Fatal(err)
	}
	routing.InvalidateCache()
	manualReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"octopus-chat","messages":[{"role":"user","content":"manual selection"}]}`))
	manualReq.Header.Set("Content-Type", "application/json")
	manualReq.Header.Set("Authorization", "Bearer downstream-token")
	manualRec := httptest.NewRecorder()
	r.ServeHTTP(manualRec, manualReq)
	if manualRec.Code != http.StatusOK || upstreamCalls.Load() != 3 {
		t.Fatalf("manual active item dispatch status=%d calls=%d body=%s", manualRec.Code, upstreamCalls.Load(), manualRec.Body.String())
	}
	upstreamSeenMu.Lock()
	defer upstreamSeenMu.Unlock()
	if len(upstreamSeen) != 3 || upstreamSeen[0] != "Bearer octopus-secret-1" || upstreamSeen[1] != "Bearer octopus-secret-2" || upstreamSeen[2] != "Bearer octopus-secret-2" {
		t.Fatalf("same-URL failover/manual credentials = %v; want failover first→second and manual active second", upstreamSeen)
	}
	if proxyCalls.Load() != 0 {
		t.Fatalf("proxy=false residual channel_proxy received %d upstream request(s); want zero", proxyCalls.Load())
	}
}

func TestOctopusClientHeaderTemplateExpandsOnlySafeHeaders(t *testing.T) {
	_ = store.CloseDatabase()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Octopus-Trace"); got != "trace-client-42" {
			t.Errorf("expanded client header = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-all-protocols" {
			t.Errorf("authorization was not the imported upstream secret: %q", got)
		}
		if got := r.Header.Get("Host"); got == "attacker.example" {
			t.Errorf("host override was forwarded: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"direct-template","choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)
	cfg := testProxyConfig(t)
	config.Set(cfg)
	if err := store.EnsureRuntimeDatabase(cfg, config.RuntimeSafe()); err != nil {
		t.Fatal(err)
	}
	db := store.GetDB()
	t.Cleanup(func() {
		proxyhandler.SetUpstreamConfig(nil)
		scheduler.SetActiveChannelIDsProvider(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ShutdownProxyLogBatchWriter(ctx)
		_ = store.CloseDatabase()
	})
	payload := strings.Replace(octopusDirectProtocolsFixture(upstream.URL), `"custom_header":[]`, `"custom_header":[{"header_key":"X-Octopus-Trace","header_value":"{client_header:X-Request-ID}"},{"header_key":"Host","header_value":"attacker.example"}]`, 1)
	if _, err := backup.ImportOctopusV5(db, []byte(payload), "client-header-template"); err != nil {
		t.Fatalf("import safe template fixture: %v", err)
	}
	if err := ConfigureProxyUpstream(cfg); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.ProxyAuth())
		proxyhandler.RegisterProxyRoutes(r)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"all-protocol-model","messages":[{"role":"user","content":"test"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer downstream-token")
	req.Header.Set("X-Request-ID", "trace-client-42")
	req.Header.Set("Host", "client.example")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "direct-template") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOctopusV5DirectGrantDispatchesResponsesAndMessages(t *testing.T) {
	_ = store.CloseDatabase()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/route-responses":
			if r.Header.Get("Authorization") != "Bearer sk-all-protocols" || r.Header.Get("x-api-key") != "" {
				t.Errorf("Responses credential headers: authorization=%q x-api-key=%q", r.Header.Get("Authorization"), r.Header.Get("x-api-key"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"resp_direct","object":"response","status":"completed","model":"provider-model","output":[]}`))
		case "/v1/route-messages":
			if r.Header.Get("x-api-key") != "sk-all-protocols" || r.Header.Get("Authorization") != "" {
				t.Errorf("Messages credential headers: authorization=%q x-api-key=%q", r.Header.Get("Authorization"), r.Header.Get("x-api-key"))
			}
			if got := r.Header.Get("anthropic-version"); got == "" {
				t.Error("Messages request missing anthropic-version")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"msg_direct","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"provider-model","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
		default:
			t.Errorf("unexpected upstream path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	cfg := testProxyConfig(t)
	if dsn := os.Getenv("OCTOPUS_DIRECT_PG_DSN"); dsn != "" {
		rt := config.RuntimeSafe()
		rt.DbType = store.DialectPostgres
		rt.DbUrl = dsn
		config.SetRuntime(rt)
	}
	config.Set(cfg)
	if err := store.EnsureRuntimeDatabase(cfg, config.RuntimeSafe()); err != nil {
		t.Fatal(err)
	}
	db := store.GetDB()
	t.Cleanup(func() {
		proxyhandler.SetUpstreamConfig(nil)
		scheduler.SetActiveChannelIDsProvider(nil)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ShutdownProxyLogBatchWriter(ctx)
		_ = store.CloseDatabase()
	})
	if _, err := backup.ImportOctopusV5(db, []byte(octopusDirectProtocolsFixture(upstream.URL)), "direct-protocols-e2e"); err != nil {
		t.Fatalf("import fixture: %v", err)
	}
	if err := ConfigureProxyUpstream(cfg); err != nil {
		t.Fatalf("ConfigureProxyUpstream: %v", err)
	}
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(auth.ProxyAuth())
		proxyhandler.RegisterProxyRoutes(r)
	})
	for _, tc := range []struct {
		path string
		body string
		want string
	}{
		{"/v1/responses", `{"model":"all-protocol-model","input":"hello"}`, "resp_direct"},
		{"/v1/messages", `{"model":"all-protocol-model","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`, "msg_direct"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer downstream-token")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("status=%d body=%s; want response marker %q", rec.Code, rec.Body.String(), tc.want)
			}
		})
	}
}

func octopusDirectProtocolsFixture(baseURL string) string {
	return fmt.Sprintf(`{"version":5,"exported_at":"2026-10-06T00:00:00Z","channels":[{"id":1,"name":"all protocols","dialect":"generic","enabled":true,"base_url":%q,"openai_chat_completion_path":"/v1/route-chat","openai_response_path":"/v1/route-responses","anthropic_message_path":"/v1/route-messages","proxy":false,"channel_proxy":"","custom_header":[],"param_override":"","match_regex":""}],"channel_keys":[{"id":2,"channel_id":1,"name":"primary","key":"sk-all-protocols","enabled":true}],"channel_models":[{"id":3,"channel_id":1,"name":"provider-model"}],"channel_grants":[{"id":4,"channel_model_id":3,"channel_key_id":2,"protocols":14}],"groups":[{"id":5,"name":"all-protocol-model","mode":"failover","active_item_id":6,"relay_config":{}}],"group_items":[{"id":6,"group_id":5,"channel_grant_id":4,"priority":0,"weight":1}],"api_keys":[],"llm_infos":[],"settings":[],"stats_total":[],"stats_daily":[],"stats_hourly":[],"stats_api_key":[]}`, baseURL)
}

func testProxyConfig(t *testing.T) *config.Config {
	t.Helper()
	dataDir := t.TempDir()
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:                        "admin-token",
		ProxyToken:                       "downstream-token",
		DbType:                           store.DialectSQLite,
		DbUrl:                            filepath.Join(dataDir, "metapi.db"),
		ProxyFirstByteTimeoutSec:         90,
		RoutingFallbackUnitCost:          1,
		TokenRouterFailureCooldownMaxSec: 3600,
		RoutingWeights: config.RoutingWeights{
			BaseWeightFactor: 1,
			ValueScoreFactor: 1,
			CostWeight:       1,
			BalanceWeight:    1,
			UsageWeight:      1,
		},
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	cfg := &config.Config{
		DataDir:                 dataDir,
		RequestBodyLimit:        1 << 20,
		ProxyMaxChannelAttempts: 3,
		TokenRouterCacheTtlMs:   60_000,
	}
	config.Set(cfg)
	return cfg
}

func seedProxyRoute(t *testing.T, db *store.DB, upstreamURL, model, token string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"upstream-site", upstreamURL, "anyrouter", "active", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("site LastInsertId: %v", err)
	}
	res, err = db.Exec(`INSERT INTO accounts (site_id, access_token, api_token, status, balance, quota, value_score, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, token, token, "active", 10.0, 100.0, 1.0, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("account LastInsertId: %v", err)
	}
	res, err = db.Exec(`INSERT INTO token_routes (model_pattern, route_mode, routing_strategy, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		model, "pattern", "weighted", true, now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	routeID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("route LastInsertId: %v", err)
	}
	res, err = db.Exec(`INSERT INTO route_channels (route_id, account_id, source_model, priority, weight, enabled) VALUES (?, ?, ?, ?, ?, ?)`,
		routeID, accountID, model, 0, 10, true)
	if err != nil {
		t.Fatalf("insert channel: %v", err)
	}
	channelID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("channel LastInsertId: %v", err)
	}
	return channelID
}

func TestProxyRoutingStoreSelectsSeededChannel(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("Open SQLite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	channelID := seedProxyRoute(t, db, "https://example.invalid", "gpt-seeded", "seed-token")
	router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), testProxyConfig(t), nil, nil)

	selected, err := router.SelectChannel(context.Background(), "gpt-seeded", routing.EmptyDownstreamRoutingPolicy)
	if err != nil {
		t.Fatalf("SelectChannel: %v", err)
	}
	if selected == nil {
		t.Fatal("SelectChannel returned nil")
	}
	if selected.Channel.ID != channelID || selected.TokenValue != "seed-token" || selected.Site.URL != "https://example.invalid" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestConfigureProxyUpstreamWiresActiveChannelIDsProvider(t *testing.T) {
	_ = store.CloseDatabase()
	scheduler.SetActiveChannelIDsProvider(nil)

	// Unset path / failed reconfigure clears the provider.
	if err := ConfigureProxyUpstream(testProxyConfig(t)); err == nil {
		t.Fatal("expected ConfigureProxyUpstream to fail without DB")
	}
	if got := scheduler.GetActiveChannelIDsFromProvider(); got != nil {
		t.Fatalf("provider should be cleared when DB missing, got %v", got)
	}

	cfg := testProxyConfig(t)
	// Register after TempDir so CloseDatabase runs before TempDir cleanup (Windows file lock).
	t.Cleanup(func() {
		proxyhandler.SetUpstreamConfig(nil)
		scheduler.SetActiveChannelIDsProvider(nil)
		_ = store.CloseDatabase()
	})
	config.Set(cfg)
	if err := store.EnsureRuntimeDatabase(cfg, config.RuntimeSafe()); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	if err := ConfigureProxyUpstream(cfg); err != nil {
		t.Fatalf("ConfigureProxyUpstream: %v", err)
	}

	// Provider is registered and returns a non-nil empty slice when no leases exist.
	ids := scheduler.GetActiveChannelIDsFromProvider()
	if ids == nil {
		t.Fatal("expected non-nil provider after ConfigureProxyUpstream")
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty active IDs with no leases, got %v", ids)
	}
}
