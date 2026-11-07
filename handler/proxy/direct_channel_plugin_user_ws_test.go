package proxyhandler

import (
	"context"
	"encoding/json"
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
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestWebsocketPluginsRespectUserAdmissionAndRawSettlement(t *testing.T) {
	for _, mode := range []string{"headers-only", "expanded-vendor", "hosted-tool", "event-failure", "provider-headers-only", "provider-expanded-vendor", "provider-hosted-tool", "provider-event-failure", "provider-decoded-headers-only", "provider-decoded-event-failure", "provider-decoded-decode-failure", "provider-decoded-forged-usage", "provider-decoded-original-error"} {
		t.Run(mode, func(t *testing.T) {
			provider := strings.HasPrefix(mode, "provider-")
			baseMode := strings.TrimPrefix(mode, "provider-")
			decode := strings.HasPrefix(baseMode, "decoded-")
			baseMode = strings.TrimPrefix(baseMode, "decoded-")
			db, users, user, keys := newBudgetProxyUser(t)
			videoExec(t, db, `UPDATE token_routes SET created_at='now',updated_at='now'`)
			if _, err := users.SetUserBudget(t.Context(), user.ID, 1, false); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-User-Plugin") != "on" || r.Header.Get("Authorization") != "Bearer synthetic-user-plugin" {
					t.Error("plugin header/credential missing")
				}
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				if _, _, err := conn.Read(r.Context()); err != nil {
					return
				}
				calls.Add(1)
				payload := []byte(`{"type":"response.completed","response":{"id":"resp-user-plugin","status":"completed","output":[],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}},"extension":9007199254740993}`)
				if decode {
					vendor := map[string]any{"native": string(payload), "meter": map[string]int{"input": 7, "output": 2, "total": 9}}
					if baseMode == "original-error" {
						vendor["error"] = "synthetic-private-provider-error"
					}
					payload, _ = json.Marshal(map[string]any{"vendor": vendor})
				}
				_ = conn.Write(r.Context(), websocket.MessageText, payload)
			}))
			t.Cleanup(upstream.Close)
			raw := []byte(fmt.Sprintf(`{"version":"1.4","channels":[{"id":1,"type":"qiniu","name":"user-ws-plugin","base_url":%q,"credentials":{"apiKey":"synthetic-user-plugin"},"supported_models":["provider"]}],"models":[{"id":1,"model_id":"user-ws-model","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"provider"}}]}}]}`, upstream.URL))
			if _, err := backup.ImportAxonHubV14(db, raw, "user-ws-plugin", false); err != nil {
				t.Fatal(err)
			}
			var routeID int64
			if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='user-ws-model'`); err != nil {
				t.Fatal(err)
			}
			if err := users.GrantUserGroupRoute(t.Context(), user.GroupID, routeID); err != nil {
				t.Fatal(err)
			}
			endpoints := store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http"), Auth: store.DirectAuthBearer}}
			videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, endpoints)
			videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, store.DirectProtocolResponsesWebsocket)
			videoExec(t, db, `UPDATE upstream_group_items SET protocol_order='[]'`)
			operation := ""
			if baseMode == "expanded-vendor" {
				operation = `body:[{op:"set",path:"vendor",value:"` + strings.Repeat("x", 4096) + `"}],`
			}
			if baseMode == "hosted-tool" {
				operation = `body:[{op:"set",path:"tools",value:'[{"type":"web_search"}]'}],`
			}
			source := `export const meta={apiVersion:1,key:"user-ws",name:"User WS",version:"1.0.0",protocols:["responsesWebsocket"],hooks:["request","event"]};export function request(){return {` + operation + `headers:[{op:"set",path:"X-User-Plugin",value:"on"}]};}export function event(){` + func() string {
				if baseMode == "event-failure" {
					return `throw new Error("synthetic-private-hook");`
				}
				return `return {};`
			}() + `}`
			if provider {
				mutation := ""
				if baseMode == "expanded-vendor" {
					mutation = `const payload=JSON.parse(i.body);payload.vendor="` + strings.Repeat("x", 4096) + `";i.body=JSON.stringify(payload);`
				}
				if baseMode == "hosted-tool" {
					mutation = `const payload=JSON.parse(i.body);payload.tools=[{type:"web_search"}];i.body=JSON.stringify(payload);`
				}
				event := `return {body:i.body};`
				if baseMode == "event-failure" {
					event = `throw new Error("synthetic-private-hook");`
				}
				source = `export const meta={apiVersion:2,kind:"provider",key:"user-ws",name:"User WS",version:"1.0.0",protocols:["responsesWebsocket"],credentialKinds:["api_key"],hooks:["request","event"]};export function request(i){` + mutation + `return {body:i.body,headers:{"X-User-Plugin":"on"}};}export function event(i){` + event + `}`
				if decode {
					source = strings.Replace(source, `hooks:["request","event"]`, `hooks:["request","event","decodeEvent"],observations:{responsesWebsocket:{event:{usagePaths:{input_tokens:"/vendor/meter/input",output_tokens:"/vendor/meter/output",total_tokens:"/vendor/meter/total"},errorPath:"/vendor/error"}}}`, 1)
					decoder := `return {body:JSON.parse(i.body).vendor.native};`
					if baseMode == "decode-failure" {
						decoder = `throw new Error("synthetic-private-decoder");`
					} else if baseMode == "forged-usage" {
						decoder = `return {body:JSON.parse(i.body).vendor.native.replace('"total_tokens":9','"total_tokens":90')};`
					}
					source += `export function decodeEvent(i){` + decoder + `}`
				}
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "user-ws", source, true, 0); err != nil {
				t.Fatal(err)
			}
			binding := store.DirectRequestConfig{Plugins: []store.DirectPluginBinding{{Key: "user-ws", Protocols: []string{"responsesWebsocket"}}}}
			if provider {
				binding = store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "user-ws", Protocols: []string{"responsesWebsocket"}}}
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, binding)
			prior := getUpstreamConfig()
			t.Cleanup(func() { SetUpstreamConfig(prior); routing.SetGlobalCache(nil) })
			SetUpstreamConfig(&UpstreamConfig{Router: routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil), ChannelPlugins: channelplugins.New(db.DB), LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }, ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
				return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
			}})
			completed := make(chan store.GatewayRequest, 2)
			gateway := httptest.NewServer(requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleResponsesWebsocket)), func(_ context.Context, entry store.GatewayRequest) error { completed <- entry; return nil }))
			t.Cleanup(gateway.Close)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/responses", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + keys[0]}}})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.CloseNow()
			writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"user-ws-model","input":[{"role":"user","content":"hello"}],"max_output_tokens":16}`)
			frame := readWSFrame(t, ctx, conn)
			select {
			case <-completed:
			case <-ctx.Done():
				t.Fatal("plugin WS turn did not settle")
			}
			wantCalls := int32(1)
			if baseMode == "expanded-vendor" || baseMode == "hosted-tool" {
				wantCalls = 0
			}
			if calls.Load() != wantCalls || (frame["type"] == "response.completed") != (baseMode == "headers-only") {
				t.Fatalf("plugin bypassed admission/failure: mode=%s calls=%d frame=%v", mode, calls.Load(), frame)
			}
			if baseMode != "headers-only" {
				wantStatus := float64(403)
				if baseMode == "event-failure" || baseMode == "decode-failure" || baseMode == "forged-usage" || baseMode == "original-error" {
					wantStatus = 502
				}
				if frame["status"] != wantStatus {
					t.Fatalf("wrong plugin rejection status: %v want %g", frame, wantStatus)
				}
			}
			budget, err := users.GetUserBudget(t.Context(), user.ID)
			wantUsed := float64(wantCalls) * 0.000009
			if err != nil || budget.Reserved != 0 || budget.Used != wantUsed {
				t.Fatalf("raw usage or hold changed: %+v err=%v wantUsed=%g", budget, err, wantUsed)
			}
			var holds int
			if err := db.Get(&holds, `SELECT COUNT(*) FROM gateway_user_budget_ledger`); err != nil || holds != 1 {
				t.Fatalf("one turn admitted %d times: %v", holds, err)
			}
		})
	}
}
