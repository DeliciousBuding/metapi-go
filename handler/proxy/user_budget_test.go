package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestUserBudgetWebSocketTurnSettlesOnce(t *testing.T) {
	db, users, user, keys := newBudgetProxyUser(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := users.SetUserBudget(ctx, user.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["max_output_tokens"] != float64(4096) {
			t.Errorf("WS output not bounded %+v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_fixture\",\"model\":\"fixture\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":10,\"output_tokens\":20,\"total_tokens\":30}}}\n\n")
	}))
	defer upstream.Close()
	videoExec(t, db, `INSERT INTO sites(id,name,url,platform,status) VALUES(1,'fixture',?,'openai','active')`, upstream.URL)
	videoExec(t, db, `INSERT INTO accounts(id,site_id,access_token,api_token,status) VALUES(1,1,'','synthetic','active')`)
	videoExec(t, db, `INSERT INTO route_channels(route_id,account_id,source_model,enabled) VALUES(1,1,'fixture',TRUE)`)
	for _, table := range []string{"sites", "accounts", "token_routes"} {
		videoExec(t, db, `UPDATE `+table+` SET created_at='now',updated_at='now'`)
	}
	priorCfg, priorUpstream := config.GetSafe(), getUpstreamConfig()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(priorCfg); SetUpstreamConfig(priorUpstream) })
	SetUpstreamConfig(&UpstreamConfig{Router: routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil), LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
	completed := make(chan store.GatewayRequest, 4)
	h := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleResponsesWebsocket)), func(_ context.Context, entry store.GatewayRequest) error { completed <- entry; return nil })
	srv := httptest.NewServer(h)
	defer srv.Close()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/v1/responses", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + keys[0]}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	for turn := range 2 {
		writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"fixture","input":[{"role":"user","content":"hello"}]}`)
		frame := readWSFrame(t, ctx, conn)
		if frame["type"] != "response.completed" {
			t.Fatalf("turn %d %+v", turn, frame)
		}
		select {
		case entry := <-completed:
			if entry.Transport != "websocket" {
				t.Fatalf("not WS turn %+v", entry)
			}
		case <-ctx.Done():
			t.Fatal("turn never settled")
		}
	}
	budget, err := users.GetUserBudget(ctx, user.ID)
	if err != nil || budget.Used != 0.00006 || budget.Reserved != 0 || budget.Frozen {
		t.Fatalf("WS turn accounting %+v %v", budget, err)
	}
	var count int
	if err = db.Get(&count, `SELECT COUNT(*) FROM gateway_user_budget_ledger`); err != nil || count != 2 {
		t.Fatalf("WS ledger rows %d %v", count, err)
	}
}

func TestUserBudgetTextEstimate(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/chat/completions", "/v1/completions", "/v1/messages", "/v1/responses", "/responses", "/v1/embeddings", "/v1beta/models/example:generateContent"} {
		t.Run(path, func(t *testing.T) {
			body := map[string]any{"model": "fixture", "messages": []any{map[string]any{"role": "user", "content": "文本🙂"}}}
			n, err := userBudgetTokenEstimate(path, body)
			if err != nil || n <= 1024 {
				t.Fatalf("estimate %d %v", n, err)
			}
		})
	}
	body := map[string]any{"model": "fixture", "max_tokens": float64(8), "stream": true}
	if _, err := userBudgetTokenEstimate("/v1/chat/completions", body); err != nil {
		t.Fatal(err)
	}
	if body["max_tokens"] != float64(8) {
		t.Fatal("client cap increased")
	}
	if options, ok := body["stream_options"].(map[string]any); !ok || options["include_usage"] != true {
		t.Fatal("stream usage not requested")
	}
	for _, raw := range []string{`{"max_tokens":0}`, `{"max_tokens":-1}`, `{"max_tokens":1.5}`, `{"max_tokens":null}`, `{"n":2}`, `{"image_url":{"url":"opaque"}}`, `{"previous_response_id":"resp_opaque"}`, `{"background":true}`, `{"tools":[{"type":"web_search_preview"}]}`, `{"messages":[{"content":[{"type":"image","source":{"type":"url","url":"https://example.invalid/1"}}]}]}`} {
		var body map[string]any
		if err := json.Unmarshal([]byte(raw), &body); err != nil {
			t.Fatal(err)
		}
		if _, err := userBudgetTokenEstimate("/v1/chat/completions", body); err == nil {
			t.Fatalf("unsafe body allowed: %s", raw)
		}
	}
	for _, path := range []string{"/v1/videos", "/v1/images/generations", "/v1/audio/speech", "/v1/responses/compact", "/v1/files", "/v1/search"} {
		if _, err := userBudgetTokenEstimate(path, map[string]any{}); err == nil {
			t.Fatalf("unsafe path %s", path)
		}
	}
}

func newBudgetProxyUser(t *testing.T) (*store.DB, *store.GatewayUserStore, store.GatewayUser, []string) {
	t.Helper()
	db := setupManagedKeyCostTestDB(t)
	users := store.NewGatewayUserStore(db)
	ctx := context.Background()
	user, err := users.CreateUser(ctx, "budget-user", "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	videoExec(t, db, `INSERT INTO token_routes(id,model_pattern,enabled) VALUES(1,'fixture',TRUE)`)
	if err = users.GrantUserGroupRoute(ctx, user.GroupID, 1); err != nil {
		t.Fatal(err)
	}
	keys := []string{"sk-budget-first", "sk-budget-second"}
	for _, key := range keys {
		id := insertWSAdmissionKey(t, db, key, nil, 0, nil, 0)
		videoExec(t, db, `UPDATE downstream_api_keys SET key_owner_type='user',owner_user_id=? WHERE id=?`, user.ID, id)
	}
	prior := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(prior) })
	return db, users, user, keys
}

func TestUserBudgetHTTPAllKeysAndRequestSettlement(t *testing.T) {
	db, users, user, keys := newBudgetProxyUser(t)
	ctx := context.Background()
	if _, err := users.SetUserBudget(ctx, user.ID, 0.02, false); err != nil {
		t.Fatal(err)
	}
	var incoming map[string]any
	handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &incoming); err != nil {
			t.Fatal(err)
		}
		attempt := beginGatewayAttempt(r.Context())
		tokens := int64(100)
		logProxy(attempt, &UpstreamConfig{LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }}, proxy.ProxyLogEntry{Status: "failed", TotalTokens: &tokens})
		// A repeated callback for the same attempt must not add usage again.
		logProxy(attempt, &UpstreamConfig{LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }}, proxy.ProxyLogEntry{Status: "failed", TotalTokens: &tokens})
		next := beginGatewayAttempt(r.Context())
		tokens = 200
		logProxy(next, &UpstreamConfig{LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }}, proxy.ProxyLogEntry{Status: "success", TotalTokens: &tokens})
		_, _ = io.WriteString(w, `{"ok":true}`)
	})), func(context.Context, store.GatewayRequest) error { return nil })
	for _, key := range keys {
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"fixture","messages":[{"role":"user","content":"hi"}]}`))
		r.Header.Set("Authorization", "Bearer "+key)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status %d %s", w.Code, w.Body.String())
		}
		if incoming["max_completion_tokens"] != float64(4096) {
			t.Fatalf("missing default output ceiling %+v", incoming)
		}
	}
	budget, err := users.GetUserBudget(ctx, user.ID)
	if err != nil || budget.Used != 0.0006 || budget.Reserved != 0 {
		t.Fatalf("request-level across Keys %+v %v", budget, err)
	}
	var count int
	if err = db.Get(&count, `SELECT COUNT(*) FROM gateway_user_budget_ledger`); err != nil || count != 2 {
		t.Fatalf("ledger count %d %v", count, err)
	}
	if _, err = users.SetUserBudget(ctx, user.ID, 0, false); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"fixture"}`))
	r.Header.Set("Authorization", "Bearer "+keys[1])
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 429 {
		t.Fatalf("second Key bypasses total budget %d", w.Code)
	}
}

func TestUserBudgetUnknownUsageAndZeroDispatch(t *testing.T) {
	_, users, user, keys := newBudgetProxyUser(t)
	ctx := context.Background()
	if _, err := users.SetUserBudget(ctx, user.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	for _, dispatched := range []bool{false, true} {
		h := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if dispatched {
				beginGatewayAttempt(r.Context())
			}
			w.WriteHeader(502)
		})), func(context.Context, store.GatewayRequest) error { return nil })
		r := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"fixture"}`))
		r.Header.Set("Authorization", "Bearer "+keys[0])
		h.ServeHTTP(httptest.NewRecorder(), r)
		budget, err := users.GetUserBudget(ctx, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !dispatched && (budget.Reserved != 0 || budget.Frozen) {
			t.Fatalf("zero dispatch did not release %+v", budget)
		}
		if dispatched && (!budget.Frozen || budget.Reserved == 0 || budget.Used != 0) {
			t.Fatalf("unknown dispatch became free %+v", budget)
		}
	}
}

func TestUserBudgetMalformedUsageIsNotFree(t *testing.T) {
	for _, raw := range []string{`{"usage":{"total_tokens":-1}}`, `{"usage":{"prompt_tokens":-1,"completion_tokens":4}}`, `{"usage":{"total_tokens":1.5}}`, `{"usage":{"total_tokens":18446744073709551616}}`, `{"usage":{"total_tokens":"0"}}`} {
		got := ParseUsageFromBody([]byte(raw))
		if !got.invalidForUserBudget {
			t.Fatalf("invalid usage accepted %s %+v", raw, got)
		}
	}
	if got := ParseUsageFromBody([]byte(`{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`)); got.invalidForUserBudget || !got.Found {
		t.Fatalf("explicit zero invalid %+v", got)
	}
	if got := mergeUsagePreferLater(ParsedUsage{invalidForUserBudget: true}, ParsedUsage{Found: true, TotalTokens: 2}); !got.invalidForUserBudget {
		t.Fatal("later valid hides malformed")
	}
	if n, ok := userBudgetPositiveInteger(math.Inf(1)); ok || n > 0 {
		t.Fatal("inf accepted")
	}
}

func TestUserBudgetUnsupportedProtocolNeverDispatches(t *testing.T) {
	_, users, user, keys := newBudgetProxyUser(t)
	ctx := context.Background()
	if _, err := users.SetUserBudget(ctx, user.ID, 0, true); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/videos", "/v1/images/generations", "/v1/audio/speech", "/v1/responses/compact"} {
		h := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unsupported dispatched") })), func(context.Context, store.GatewayRequest) error { return nil })
		r := httptest.NewRequest("POST", path, strings.NewReader(`{"model":"fixture"}`))
		r.Header.Set("Authorization", "Bearer "+keys[0])
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("%s status %d %s", path, w.Code, w.Body.String())
		}
	}
	// System Keys retain their exact old body and do not acquire a user hold.
	key := "sk-budget-system"
	insertWSAdmissionKey(t, store.GetDB(), key, nil, 0, nil, 0)
	h := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if string(raw) != `{"model":"fixture"}` {
			t.Fatalf("system body changed %s", raw)
		}
		fmt.Fprint(w, "ok")
	})), func(context.Context, store.GatewayRequest) error { return nil })
	r := httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"model":"fixture"}`))
	r.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("system behavior %d", w.Code)
	}
}
