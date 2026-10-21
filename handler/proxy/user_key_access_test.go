package proxyhandler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestUserKeyHTTPCurrentGroupAndHistoricalOwnership(t *testing.T) {
	db := setupManagedKeyCostTestDB(t)
	previousRuntime, previousConfig, previousUpstream := config.RuntimeSafe(), config.GetSafe(), getUpstreamConfig()
	config.SetRuntime(&config.RuntimeSettings{})
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() {
		config.SetRuntime(previousRuntime)
		config.Set(previousConfig)
		SetUpstreamConfig(previousUpstream)
	})
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fixture","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"user-key-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
	}))
	defer upstream.Close()
	videoExec(t, db, `INSERT INTO sites (id,name,url,platform,status) VALUES (1,'fixture',?,'openai','active')`, upstream.URL)
	videoExec(t, db, `INSERT INTO accounts (id,site_id,access_token,api_token,status) VALUES (1,1,'','fixture-upstream','active')`)
	videoExec(t, db, `INSERT INTO token_routes (id,model_pattern,route_mode,routing_strategy,enabled) VALUES (1,'gpt-4o','pattern','weighted',TRUE),(2,'hidden-model','pattern','weighted',TRUE),(3,'group-alias','explicit_group','weighted',TRUE)`)
	videoExec(t, db, `UPDATE token_routes SET display_name='group-alias' WHERE id=3`)
	videoExec(t, db, `INSERT INTO route_channels (route_id,account_id,source_model,enabled) VALUES (1,1,'gpt-4o',TRUE),(2,1,'hidden-model',TRUE)`)
	videoExec(t, db, `INSERT INTO route_group_sources (group_route_id,source_route_id) VALUES (3,1)`)
	for _, table := range []string{"sites", "accounts", "token_routes"} {
		videoExec(t, db, `UPDATE `+table+` SET created_at='2026-10-21T00:00:00Z',updated_at='2026-10-21T00:00:00Z'`)
	}
	users := store.NewGatewayUserStore(db)
	ctx := context.Background()
	groupA, err := users.CreateUserGroup(ctx, "a", "A", true)
	if err != nil {
		t.Fatal(err)
	}
	groupB, err := users.CreateUserGroup(ctx, "b", "B", true)
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.CreateUser(ctx, "person", "Person", groupA.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.SetUserBudget(ctx, user.ID, 10, false); err != nil {
		t.Fatal(err)
	}
	for group, route := range map[int64]int64{groupA.ID: 1, groupB.ID: 3} {
		if err := users.GrantUserGroupRoute(ctx, group, route); err != nil {
			t.Fatal(err)
		}
	}
	keyID := insertWSAdmissionKey(t, db, "sk-user-http-fixture", nil, 0, nil, 0)
	videoExec(t, db, `UPDATE downstream_api_keys SET key_owner_type='user',owner_user_id=? WHERE id=?`, user.ID, keyID)
	actualRouter := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
	SetUpstreamConfig(&UpstreamConfig{Router: actualRouter})
	// Use the real synchronous persistence owners, not assertions on policy DTOs.
	getUpstreamConfig().LogProxy = defaultLogProxyWriter
	handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			HandleModels(w, r)
			return
		}
		HandleChatCompletions(w, r)
	})), func(ctx context.Context, entry store.GatewayRequest) error {
		return store.InsertGatewayRequest(ctx, db, entry)
	})
	call := func(model string, want int) *httptest.ResponseRecorder {
		t.Helper()
		method, path, body := "POST", "/v1/chat/completions", fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}]}`, model)
		if model == "" {
			method, path, body = "GET", "/v1/models", ""
		}
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer sk-user-http-fixture")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("model=%q: status=%d body=%s want=%d", model, w.Code, w.Body.String(), want)
		}
		return w
	}
	listing := call("", 200).Body.String()
	if !strings.Contains(listing, "gpt-4o") || strings.Contains(listing, "hidden-model") || strings.Contains(listing, "group-alias") {
		t.Fatalf("group A catalog: %s", listing)
	}
	call("gpt-4o", 200)
	before := calls.Load()
	call("hidden-model", 503)
	call("group-alias", 503)
	if calls.Load() != before {
		t.Fatal("unauthorized route dispatched")
	}
	if err := users.UpdateUser(ctx, user.ID, "Renamed", groupB.ID, true); err != nil {
		t.Fatal(err)
	}
	listing = call("", 200).Body.String()
	if strings.Contains(listing, "gpt-4o") || !strings.Contains(listing, "group-alias") {
		t.Fatalf("stale or shared model cache: %s", listing)
	}
	call("gpt-4o", 503)      // composite access does not grant its source entrypoint
	call("group-alias", 200) // source itself need not be granted
	videoExec(t, db, `UPDATE downstream_api_keys SET supported_models='["gpt-*"]',allowed_route_ids='[3]' WHERE id=?`, keyID)
	call("group-alias", 403) // a Key's model rule must narrow, not OR with its route list
	videoExec(t, db, `UPDATE downstream_api_keys SET supported_models='["*"]',allowed_route_ids='[1]' WHERE id=?`, keyID)
	call("group-alias", 503) // nor may a Key widen its user's group
	videoExec(t, db, `UPDATE downstream_api_keys SET allowed_route_ids='[]' WHERE id=?`, keyID)
	if err := users.UpdateUser(ctx, user.ID, "Renamed", groupB.ID, false); err != nil {
		t.Fatal(err)
	}
	call("group-alias", 403)
	if err := users.UpdateUser(ctx, user.ID, "Renamed", groupB.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := users.RevokeUserGroupRoute(ctx, groupB.ID, 3); err != nil {
		t.Fatal(err)
	}
	call("group-alias", 403) // empty group is deny-all even with '*' on the Key
	var records []store.GatewayRequest
	if err := db.Select(&records, `SELECT * FROM gateway_requests WHERE terminal_status='completed' AND model_requested IS NOT NULL ORDER BY started_at`); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("successful request records: %+v", records)
	}
	for i, record := range records {
		wantGroup := groupA.ID
		if i == 1 {
			wantGroup = groupB.ID
		}
		if record.OwnerUserID == nil || *record.OwnerUserID != user.ID || record.UserGroupID == nil || *record.UserGroupID != wantGroup {
			t.Fatalf("history changed ownership: %+v", record)
		}
		var log store.ProxyLog
		if err := db.Get(&log, `SELECT * FROM proxy_logs WHERE gateway_request_id=?`, record.ID); err != nil {
			t.Fatal(err)
		}
		if log.OwnerUserID == nil || *log.OwnerUserID != user.ID || log.UserGroupID == nil || *log.UserGroupID != wantGroup {
			t.Fatalf("attempt ownership: %+v", log)
		}
	}
}

func TestUserKeyWebSocketReloadsUserAndGroup(t *testing.T) {
	db := setupManagedKeyCostTestDB(t)
	users := store.NewGatewayUserStore(db)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	user, err := users.CreateUser(ctx, "ws-person", "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	videoExec(t, db, `INSERT INTO token_routes (id,model_pattern,enabled) VALUES (1,'gpt-fixture',TRUE)`)
	if err := users.GrantUserGroupRoute(ctx, user.GroupID, 1); err != nil {
		t.Fatal(err)
	}
	keyID := insertWSAdmissionKey(t, db, "sk-user-ws-fixture", nil, 0, nil, 0)
	videoExec(t, db, `UPDATE downstream_api_keys SET key_owner_type='user',owner_user_id=? WHERE id=?`, user.ID, keyID)
	conn := dialResponsesWS(t, "sk-user-ws-fixture")
	writeWSMessage(t, ctx, conn, wsPrewarmTurn)
	for _, want := range []string{"response.created", "response.completed"} {
		if frame := readWSFrame(t, ctx, conn); frame["type"] != want {
			t.Fatalf("turn: %v", frame)
		}
	}
	if err := users.UpdateUserGroup(ctx, user.GroupID, "Disabled", false); err != nil {
		t.Fatal(err)
	}
	writeWSMessage(t, ctx, conn, wsPrewarmTurn)
	frame := readWSFrame(t, ctx, conn)
	if frame["type"] != "error" || frame["status"] != float64(403) {
		t.Fatalf("group disabled survived open WS: %v", frame)
	}
	if count := readWSRequestCount(t, db, keyID); count != 1 {
		t.Fatalf("rejected turn consumed quota: %d", count)
	}
}
