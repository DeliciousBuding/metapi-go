package proxyhandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectResponsesWSCurrentPoliciesOverrideAffinity(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, change := range []string{"key", "condition", "stream", "queued stream"} {
			t.Run(dialect+"/"+change, func(t *testing.T) {
				var calls, dials, httpCalls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/http" {
						httpCalls.Add(1)
						http.Error(w, "unexpected HTTP fallback", http.StatusBadGateway)
						return
					}
					conn, err := websocket.Accept(w, r, nil)
					if err != nil {
						return
					}
					defer conn.CloseNow()
					dials.Add(1)
					for {
						if _, _, err := conn.Read(r.Context()); err != nil {
							return
						}
						calls.Add(1)
						if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.completed","response":{"id":"policy-turn","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`)); err != nil {
							return
						}
					}
				}))
				t.Cleanup(upstream.Close)
				db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{
					Responses:          &store.DirectEndpoint{URL: upstream.URL + "/http", Auth: store.DirectAuthBearer},
					ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http") + "/ws", Auth: store.DirectAuthBearer},
				}, store.DirectProtocolResponses|store.DirectProtocolResponsesWebsocket, "")
				previousDB := store.GetDB()
				store.OverrideDB(db)
				t.Cleanup(func() { store.OverrideDB(previousDB) })
				for _, key := range []string{"policy-sticky", "policy-independent"} {
					strategy := "round_robin"
					if key == "policy-independent" {
						strategy = "weighted"
					}
					policy := fmt.Sprintf(`{"routingStrategy":%q,"traceStickyMode":"prefer_previous_channel"}`, strategy)
					videoExec(t, db, `INSERT INTO downstream_api_keys(name,key,enabled,supported_models,access_policy) VALUES(?,?,TRUE,'["client-alias"]',?)`, key, key, policy)
				}
				manager := channellimit.NewManager()
				limits := store.DirectRateLimit{Config: channellimit.Config{RPM: 10, MaxConcurrent: 1}}
				videoExec(t, db, `UPDATE upstream_channels SET rate_limit=?`, limits)
				var channelID int64
				if err := db.Get(&channelID, `SELECT id FROM upstream_channels`); err != nil {
					t.Fatal(err)
				}
				routing.InvalidateCache()
				cfg := *getUpstreamConfig()
				cfg.ChannelLimits = manager
				cfg.Router.(*routing.TokenRouter).SetChannelLimits(manager)
				logs := make(chan proxy.ProxyLogEntry, 4)
				cfg.LogProxy = func(_ context.Context, e proxy.ProxyLogEntry) error { logs <- e; return nil }
				SetUpstreamConfig(&cfg)
				gateway := httptest.NewServer(auth.ProxyAuth()(http.HandlerFunc(HandleResponsesWebsocket)))
				t.Cleanup(gateway.Close)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				dial := func(key string) *websocket.Conn {
					conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/responses", &websocket.DialOptions{HTTPHeader: http.Header{
						"Authorization": {"Bearer " + key}, "AH-Trace-Id": {"same-explicit-trace"},
					}})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = conn.CloseNow() })
					return conn
				}
				turn := func(conn *websocket.Conn, want string) {
					writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hello"}]}`)
					if frame := readWSFrame(t, ctx, conn); frame["type"] != want {
						t.Fatalf("turn got %+v, want %s", frame, want)
					}
				}
				setStreamCondition := func(stream bool) {
					var conditions store.DirectAssociationConditions
					if err := conditions.Scan(fmt.Sprintf(`[{"priority":1,"protocolOrder":[%d,%d],"condition":{"conditions":[{"field":"stream","operator":"eq","value":%t}]}}]`, store.DirectProtocolResponses, store.DirectProtocolResponsesWebsocket, stream)); err != nil {
						t.Fatal(err)
					}
					videoExec(t, db, `UPDATE upstream_group_items SET association_conditions=?`, conditions)
				}
				if change == "condition" {
					setStreamCondition(true)
					routing.InvalidateCache()
				}
				windowStarted := time.Now().Unix() / 60
				first := dial("policy-sticky")
				turn(first, "response.completed")
				turn(first, "response.completed")
				other := dial("policy-independent")
				turn(other, "response.completed")
				if dials.Load() != 2 || calls.Load() != 3 {
					t.Fatalf("Key/session identity leaked: dials=%d calls=%d", dials.Load(), calls.Load())
				}
				before := waitDirectChannelIdle(t, manager, channelID, limits.Config)
				beforeMinute := time.Now().Unix() / 60
				if windowStarted == beforeMinute && (before.Requests != 3 || before.Tokens != 9) {
					t.Fatalf("successful turns were not accounted once: %+v", before)
				}
				var held *channellimit.Permit
				if change == "queued stream" {
					var err error
					held, err = manager.Acquire(ctx, channelID, limits.Config)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(held.Release)
					writeWSMessage(t, ctx, first, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"queued"}]}`)
					for manager.Snapshot(channelID, limits.Config).Waiting != 1 {
						select {
						case <-ctx.Done():
							t.Fatal("native WS turn never entered channel queue")
						case <-time.After(time.Millisecond):
						}
					}
				}
				switch change {
				case "key":
					videoExec(t, db, `UPDATE downstream_api_keys SET access_policy=? WHERE key=?`, `{"allowedUpstreamChannelIds":[],"routingStrategy":"round_robin","traceStickyMode":"prefer_previous_channel"}`, "policy-sticky")
				case "condition":
					setStreamCondition(false)
				case "stream", "queued stream":
					videoExec(t, db, `UPDATE upstream_channels SET stream_policy='forbid'`)
				}
				// The open socket and a warm route cache must not extend a grant.
				if held != nil {
					held.Release()
					if frame := readWSFrame(t, ctx, first); frame["type"] != "error" {
						t.Fatalf("queued native WS ignored tightened policy: %+v", frame)
					}
				} else {
					turn(first, "error")
				}
				routing.InvalidateCache()
				turn(first, "error")
				if calls.Load() != 3 || dials.Load() != 2 || httpCalls.Load() != 0 {
					t.Fatalf("tightened policy dispatched/replayed: calls=%d dials=%d http=%d", calls.Load(), dials.Load(), httpCalls.Load())
				}
				for range 3 {
					select {
					case entry := <-logs:
						if entry.Status != "success" || entry.TotalTokens == nil || *entry.TotalTokens != 3 {
							t.Fatalf("wrong turn accounting: %+v", entry)
						}
					case <-ctx.Done():
						t.Fatal("missing upstream attempt log")
					}
				}
				if len(logs) != 0 {
					t.Fatal("zero-dispatch rejection recorded an upstream attempt")
				}
				snap := waitDirectChannelIdle(t, manager, channelID, limits.Config)
				// These counters intentionally expire at a natural minute boundary.
				// No generation after the baseline may add RPM or usage in either window.
				wantRequests, wantTokens := before.Requests, before.Tokens
				if time.Now().Unix()/60 != beforeMinute {
					wantRequests, wantTokens = 0, 0
				}
				if snap.Requests != wantRequests || snap.Tokens != wantTokens {
					t.Fatalf("policy rejection leaked/billed admission: %+v", snap)
				}
			})
		}
	}
}

func TestDirectCompactNeverAggregatesRequiredStream(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				t.Error("required-stream compact reached upstream")
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{ResponsesCompact: &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolResponsesCompact, "")
			videoExec(t, db, `UPDATE upstream_channels SET stream_policy='require'`)
			routing.InvalidateCache()
			out := httptest.NewRecorder()
			HandleResponses(out, makeProxyReq(http.MethodPost, "/v1/responses/compact", `{"model":"client-alias","input":"hello"}`), "/v1/responses/compact")
			if out.Code < 400 || calls.Load() != 0 {
				t.Fatalf("compact used text aggregation: status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}

func TestDirectResponsesWSUserBudgetKeepsNativeBounds(t *testing.T) {
	db, users, user, keys := newBudgetProxyUser(t)
	if _, err := users.SetUserBudget(t.Context(), user.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_, body, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		calls.Add(1)
		if !strings.Contains(string(body), `"max_output_tokens":16`) || strings.Contains(string(body), `"metadata"`) {
			t.Errorf("native WS budget/operations changed: %s", body)
		}
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.completed","response":{"id":"budget-direct","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`))
	}))
	t.Cleanup(upstream.Close)
	payload := fmt.Sprintf(`{"version":"1.4","channels":[{"id":1,"type":"qiniu","name":"budget-direct","base_url":%q,"credentials":{"apiKey":"synthetic-direct"},"supported_models":["provider"],"settings":{"bodyOverrideOperations":[{"op":"delete","path":"metadata"}]}}],"models":[{"id":1,"model_id":"budget-direct","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"provider"}}]}}]}`, upstream.URL)
	if _, err := backup.ImportAxonHubV14(db, []byte(payload), "budget-direct", false); err != nil {
		t.Fatal(err)
	}
	videoExec(t, db, `UPDATE token_routes SET created_at='2026-11-06T00:00:00Z',updated_at='2026-11-06T00:00:00Z'`)
	endpoints := store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http"), Auth: store.DirectAuthBearer}}
	videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, endpoints)
	videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, store.DirectProtocolResponsesWebsocket)
	videoExec(t, db, `UPDATE upstream_group_items SET protocol_order='[]'`)
	var routeID int64
	if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='budget-direct'`); err != nil {
		t.Fatal(err)
	}
	if err := users.GrantUserGroupRoute(t.Context(), user.GroupID, routeID); err != nil {
		t.Fatal(err)
	}
	priorConfig, priorUpstream := config.GetSafe(), getUpstreamConfig()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(priorConfig); SetUpstreamConfig(priorUpstream); routing.SetGlobalCache(nil) })
	SetUpstreamConfig(&UpstreamConfig{
		Router:   routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil),
		LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
		ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
			return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
		},
	})
	requests := make(chan store.GatewayRequest, 4)
	handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleResponsesWebsocket)), func(_ context.Context, e store.GatewayRequest) error { requests <- e; return nil })
	gateway := httptest.NewServer(handler)
	t.Cleanup(gateway.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/responses", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + keys[0]}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"budget-direct","input":[{"role":"user","content":"hello"}],"metadata":{"remove":"me"},"max_output_tokens":16}`)
	if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
		t.Fatalf("bounded native WS refused: %+v", frame)
	}
	select {
	case <-requests:
	case <-ctx.Done():
		t.Fatal("native WS turn did not settle")
	}
	writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"budget-direct","previous_response_id":"budget-direct","input":[{"role":"user","content":"opaque"}]}`)
	if frame := readWSFrame(t, ctx, conn); frame["type"] != "error" || frame["status"] != float64(http.StatusForbidden) {
		t.Fatalf("opaque continuation expanded user budget: %+v", frame)
	}
	select {
	case <-requests:
	case <-ctx.Done():
		t.Fatal("refused WS turn did not settle")
	}
	budget, err := users.GetUserBudget(ctx, user.ID)
	if err != nil || budget.Reserved != 0 || budget.Used <= 0 || calls.Load() != 1 {
		t.Fatalf("native WS budget/dispatch mismatch: %+v calls=%d err=%v", budget, calls.Load(), err)
	}
	var ledgerRows int
	if err := db.Get(&ledgerRows, `SELECT COUNT(*) FROM gateway_user_budget_ledger`); err != nil || ledgerRows != 1 {
		t.Fatalf("native WS settled more than once: rows=%d err=%v", ledgerRows, err)
	}
}
