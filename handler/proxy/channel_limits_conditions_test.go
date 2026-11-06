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

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/internal/routecondition"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

type queuedAssociationRouter struct {
	*routing.TokenRouter
	before chan *routecondition.Request
	after  chan *routecondition.Request
}

func (r *queuedAssociationRouter) SelectChannel(ctx context.Context, model string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	r.before <- policy.ResolveAssociationRequest()
	return r.TokenRouter.SelectChannel(ctx, model, policy)
}

func (r *queuedAssociationRouter) RevalidateQueuedChannel(ctx context.Context, selected *routing.SelectedChannel, model string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	var descriptor *routecondition.Request
	if policy.ResolveAssociationRequest != nil {
		descriptor = policy.ResolveAssociationRequest()
	}
	r.after <- descriptor
	return r.TokenRouter.RevalidateQueuedChannel(ctx, selected, model, policy)
}

func TestQueuedConditionalOperationsKeepRequestSnapshot(t *testing.T) {
	for _, change := range []string{"unchanged", "conditionDenied", "conditionProtocol", "conditionProtocolNarrowing", "keyPolicy", "freshEndpointAndOperations", "requiredAggregate", "requireBecameNative", "streamBecameForbidden", "embeddingsBecameRequired"} {
		t.Run(change, func(t *testing.T) {
			db, users, user, keys := newBudgetProxyUser(t)
			videoExec(t, db, `UPDATE token_routes SET created_at='2026-11-05T00:00:00Z',updated_at='2026-11-05T00:00:00Z'`)
			if _, err := users.SetUserBudget(t.Context(), user.ID, 1, false); err != nil {
				t.Fatal(err)
			}
			var calls, freshCalls atomic.Int32
			respond := func(w http.ResponseWriter, r *http.Request, header string) {
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil || body["metadata"] != nil ||
					body["model"] != "provider" || r.Header.Get("X-Imported") != header || r.Header.Get("X-Tier") != "" {
					t.Errorf("fresh wire configuration or scope lost: body=%v imported=%q tier=%q", body, r.Header.Get("X-Imported"), r.Header.Get("X-Tier"))
				}
				w.Header().Set("Content-Type", "application/json")
				if change == "requiredAggregate" || change == "requireBecameNative" {
					wantStream := change == "requiredAggregate"
					if (body["stream"] == true) != wantStream {
						t.Error("queued request used stale stream policy")
					}
					if wantStream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprint(w, streamPolicyFrame("", `{"id":"c","choices":[{"index":0,"delta":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)+streamPolicyFrame("", "[DONE]"))
						return
					}
				}
				if r.URL.Path == "/v1/responses" {
					_, _ = fmt.Fprint(w, `{"id":"response-fixture","object":"response","model":"provider","created_at":1700000000,"status":"completed","output":[{"id":"message-fixture","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
					return
				}
				_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				respond(w, r, "original")
			}))
			defer upstream.Close()
			fresh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				freshCalls.Add(1)
				header := "fresh"
				if change == "conditionProtocolNarrowing" {
					header = "original"
				}
				respond(w, r, header)
			}))
			defer fresh.Close()
			condition := `{"type":"group","conditions":[{"field":"request_header.X-Tier","operator":"eq","value":"gold"},{"field":"daily_time","operator":"within","value":"00:00-00:01"}]}`
			raw := []byte(fmt.Sprintf(`{"version":"1.4","channels":[{"id":1,"type":"qiniu","name":"conditional-limited","base_url":%q,"credentials":{"apiKey":"synthetic-limited"},"supported_models":["provider"],"settings":{"rateLimit":{"rpm":10,"maxConcurrent":1,"queueSize":2},"bodyOverrideOperations":[{"op":"delete","path":"metadata"}],"headerOverrideOperations":[{"op":"set","path":"X-Imported","value":"original"}]}}],"models":[{"id":1,"model_id":"conditional-limited","type":"chat","settings":{"associations":[{"type":"channel_model","when":{"enabled":true,"condition":%s},"channelModel":{"channelId":1,"modelId":"provider"}}]}}]}`, upstream.URL, condition))
			if _, err := backup.ImportAxonHubV14(db, raw, "queued-conditional", false); err != nil {
				t.Fatal(err)
			}
			if change == "requiredAggregate" || change == "requireBecameNative" {
				videoExec(t, db, `UPDATE upstream_channels SET stream_policy='require'`)
			}
			if change == "embeddingsBecameRequired" {
				endpoints := store.DirectEndpoints{Embeddings: &store.DirectEndpoint{URL: upstream.URL + "/v1/embeddings", Auth: store.DirectAuthBearer}}
				videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?,request_config='{}'`, endpoints)
				videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, store.DirectProtocolEmbeddings)
				var rules store.DirectAssociationConditions
				if err := db.Get(&rules, `SELECT association_conditions FROM upstream_group_items WHERE origin_key='queued-conditional'`); err != nil {
					t.Fatal(err)
				}
				rules[0].ProtocolOrder = store.DirectProtocolOrder{store.DirectProtocolEmbeddings}
				videoExec(t, db, `UPDATE upstream_group_items SET protocol_order=?,association_conditions=? WHERE origin_key='queued-conditional'`, rules[0].ProtocolOrder, rules)
			}
			if change == "conditionProtocolNarrowing" {
				var endpoints store.DirectEndpoints
				if err := db.Get(&endpoints, `SELECT endpoint_config FROM upstream_channels`); err != nil {
					t.Fatal(err)
				}
				endpoints.Responses = &store.DirectEndpoint{URL: fresh.URL + "/v1/responses", Auth: store.DirectAuthBearer}
				videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, endpoints)
				videoExec(t, db, `UPDATE upstream_grants SET protocols=6`)
				videoExec(t, db, `UPDATE upstream_group_items SET protocol_order='[2,4]' WHERE origin_key='queued-conditional'`)
			}
			var routeID, channelID int64
			if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='conditional-limited'`); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&channelID, `SELECT id FROM upstream_channels`); err != nil {
				t.Fatal(err)
			}
			if err := users.GrantUserGroupRoute(t.Context(), user.GroupID, routeID); err != nil {
				t.Fatal(err)
			}
			manager := channellimit.NewManager()
			limits := channellimit.Config{RPM: 10, MaxConcurrent: 1, QueueSize: 2}
			held, err := manager.Acquire(t.Context(), channelID, limits)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Release()
			priorConfig, priorUpstream := config.GetSafe(), getUpstreamConfig()
			config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
			t.Cleanup(func() { config.Set(priorConfig); SetUpstreamConfig(priorUpstream); routing.SetGlobalCache(nil) })
			tokenRouter := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
			tokenRouter.SetChannelLimits(manager)
			router := &queuedAssociationRouter{TokenRouter: tokenRouter, before: make(chan *routecondition.Request, 1), after: make(chan *routecondition.Request, 1)}
			SetUpstreamConfig(&UpstreamConfig{
				Router: router, ChannelLimits: manager,
				LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
				ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
					return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
				},
			})
			surface := http.HandlerFunc(HandleChatCompletions)
			if change == "embeddingsBecameRequired" {
				surface = HandleEmbeddings
			}
			handler := requestLifecycleMiddleware(auth.ProxyAuth()(surface), func(context.Context, store.GatewayRequest) error { return nil })
			body := `{"model":"conditional-limited","messages":[{"role":"user","content":"hello"}],"metadata":{"marker":"remove"},"max_tokens":16}`
			path := "/v1/chat/completions"
			if change == "streamBecameForbidden" {
				body = strings.Replace(body, `"max_tokens":16`, `"max_tokens":16,"stream":true`, 1)
			}
			if change == "embeddingsBecameRequired" {
				path, body = "/v1/embeddings", `{"model":"conditional-limited","input":"hello"}`
			}
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+keys[0])
			request.Header.Set("X-Tier", "gold")
			fixed := time.Date(2026, 1, 1, 0, 0, 30, 0, time.UTC)
			resolve := lazyAssociationRequest(path, []byte(body), request.Header, change == "streamBecameForbidden", fixed)
			request = request.WithContext(context.WithValue(request.Context(), associationRequestContextKey{}, resolve))
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				out := httptest.NewRecorder()
				handler.ServeHTTP(out, request)
				done <- out
			}()
			var original *routecondition.Request
			select {
			case original = <-router.before:
			case out := <-done:
				t.Fatalf("request never reached selection: %d %s", out.Code, out.Body.String())
			case <-time.After(3 * time.Second):
				t.Fatal("request did not reach selection")
			}
			deadline := time.Now().Add(3 * time.Second)
			for manager.Snapshot(channelID, limits).Waiting != 1 {
				select {
				case out := <-done:
					t.Fatalf("request never queued: %d %s", out.Code, out.Body.String())
				default:
				}
				if time.Now().After(deadline) {
					t.Fatal("timed out awaiting conditional queue")
				}
				time.Sleep(time.Millisecond)
			}
			want := 200
			switch change {
			case "requireBecameNative":
				videoExec(t, db, `UPDATE upstream_channels SET stream_policy='forbid'`)
			case "streamBecameForbidden":
				videoExec(t, db, `UPDATE upstream_channels SET stream_policy='forbid'`)
				want = 503
			case "embeddingsBecameRequired":
				videoExec(t, db, `UPDATE upstream_channels SET stream_policy='require'`)
				want = 503
			case "conditionDenied", "conditionProtocol", "conditionProtocolNarrowing":
				var rules store.DirectAssociationConditions
				if err := db.Get(&rules, `SELECT association_conditions FROM upstream_group_items WHERE origin_key='queued-conditional'`); err != nil {
					t.Fatal(err)
				}
				if change == "conditionDenied" {
					rules[0].Condition.Conditions[0].Value = json.RawMessage(`"silver"`)
				} else {
					rules[0].ProtocolOrder = store.DirectProtocolOrder{store.DirectProtocolResponses}
					if change == "conditionProtocol" {
						videoExec(t, db, `UPDATE upstream_group_items SET protocol_order='[2]' WHERE origin_key='queued-conditional'`)
					}
				}
				videoExec(t, db, `UPDATE upstream_group_items SET association_conditions=? WHERE origin_key='queued-conditional'`, rules)
				want = 503
				if change == "conditionProtocolNarrowing" {
					want = 200
				}
			case "keyPolicy":
				videoExec(t, db, `UPDATE downstream_api_keys SET supported_models='["different"]',allowed_route_ids='[]' WHERE key=?`, keys[0])
				want = 503
			case "freshEndpointAndOperations":
				var endpoints store.DirectEndpoints
				if err := db.Get(&endpoints, `SELECT endpoint_config FROM upstream_channels`); err != nil {
					t.Fatal(err)
				}
				endpoints.Chat.URL = fresh.URL + "/v1/chat/completions"
				operations := store.DirectRequestConfig{Config: requestoverride.Config{
					Body:    []requestoverride.Operation{{Op: "delete", Path: "metadata"}},
					Headers: []requestoverride.Operation{{Op: "set", Path: "X-Imported", Value: "fresh"}},
				}}
				videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?,request_config=?`, endpoints, operations)
			}
			held.Release()
			out := <-done
			after := <-router.after
			if original == nil || after != original || !after.Now.Equal(fixed) || after.Headers["X-Tier"] != "gold" {
				t.Fatalf("queued turn facts replaced: before=%+v after=%+v", original, after)
			}
			wantCalls, wantFresh := int32(0), int32(0)
			if want == 200 {
				wantCalls = 1
			}
			if change == "freshEndpointAndOperations" || change == "conditionProtocolNarrowing" {
				wantCalls, wantFresh = 0, 1
			}
			if out.Code != want || calls.Load() != wantCalls || freshCalls.Load() != wantFresh {
				t.Fatalf("queued dispatch status=%d want=%d calls=%d fresh=%d body=%s", out.Code, want, calls.Load(), freshCalls.Load(), out.Body.String())
			}
			budget, err := users.GetUserBudget(t.Context(), user.ID)
			if err != nil || budget.Reserved != 0 || want != 200 && budget.Used != 0 {
				t.Fatalf("queued request leaked or billed rejected hold: %+v %v", budget, err)
			}
			snapshot := manager.Snapshot(channelID, limits)
			if snapshot.Active != 0 || snapshot.Waiting != 0 || snapshot.Requests != int64(wantCalls+wantFresh) {
				t.Fatalf("queued request leaked permit or charged RPM: %+v", snapshot)
			}
			if want == 200 && (snapshot.Tokens != 3 || snapshot.UnknownUsage != 0) {
				t.Fatalf("queued aggregate lost or counted usage more than once: %+v", snapshot)
			}
		})
	}
}
