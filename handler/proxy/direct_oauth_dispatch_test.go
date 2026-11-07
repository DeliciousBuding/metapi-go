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
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

// Control only the issuer boundary. Selection, persisted exchange intent,
// publication and the final HTTP/WS dispatch all use their production owners.
func TestDirectOAuthExchangeRechecksDispatchAuthority(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, websocketTurn := range []bool{false, true} {
			for _, change := range []string{"unchanged", "grant_revoked", "model_disabled", "member_protocol", "channel_origin", "credential_origin", "channel_source", "credential_source", "endpoint_changed", "operations_changed"} {
				t.Run(fmt.Sprintf("%s/ws=%t/%s", dialect, websocketTurn, change), func(t *testing.T) {
					var calls, exchanges atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.Header.Get("Authorization") != "Bearer synthetic-dispatch-access" {
							t.Error("published credential not used")
						}
						if websocketTurn {
							conn, err := websocket.Accept(w, r, nil)
							if err != nil {
								return
							}
							defer conn.CloseNow()
							if _, _, err := conn.Read(r.Context()); err == nil {
								_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp-authority","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`))
							}
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
					}))
					t.Cleanup(upstream.Close)
					endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer}}
					protocol := store.DirectProtocolChat
					if websocketTurn {
						endpoints = store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http"), Auth: store.DirectAuthBearer}}
						protocol = store.DirectProtocolResponsesWebsocket
					}
					db := installDirectMediaRuntime(t, dialect, endpoints, protocol, "")
					videoExec(t, db, `UPDATE upstream_channels SET provider='codex'`)
					videoExec(t, db, `UPDATE upstream_credentials SET kind='oauth',oauth_state=?`, store.DirectOAuthState{RefreshToken: "synthetic-dispatch-refresh", ClientID: "synthetic-dispatch-client", ExpiresAt: 1})
					def := oauth.GetProviderDefinition("codex")
					original := def.RefreshAccessToken
					def.RefreshAccessToken = func(ctx context.Context, input oauth.RefreshTokenInput) (*oauth.TokenSet, error) {
						exchanges.Add(1)
						var statement string
						switch change {
						case "grant_revoked":
							statement = `UPDATE upstream_grants SET enabled=FALSE`
						case "model_disabled":
							statement = `UPDATE upstream_models SET enabled=FALSE`
						case "member_protocol":
							statement = `UPDATE upstream_group_items SET protocol_order='[2]'`
						case "channel_origin":
							statement = `UPDATE upstream_channels SET origin_key='replacement-origin'`
						case "credential_origin":
							statement = `UPDATE upstream_credentials SET origin_key='replacement-origin'`
						case "channel_source":
							statement = `UPDATE upstream_channels SET source_id=2`
						case "credential_source":
							statement = `UPDATE upstream_credentials SET source_id=2`
						case "endpoint_changed":
							statement = `UPDATE upstream_channels SET endpoint_config='{}'`
						case "operations_changed":
							statement = `UPDATE upstream_channels SET param_override='{"temperature":0.2}'`
						}
						if statement != "" {
							if _, err := db.ExecContext(ctx, statement); err != nil {
								return nil, err
							}
						}
						return &oauth.TokenSet{AccessToken: "synthetic-dispatch-access", RefreshToken: "synthetic-dispatch-rotated", TokenExpiresAt: time.Now().Add(time.Hour).UnixMilli()}, nil
					}
					t.Cleanup(func() { def.RefreshAccessToken = original })
					var succeeded bool
					if websocketTurn {
						ctx, conn := dialDirectResponsesGateway(t, "synthetic-authority-key")
						writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hello"}]}`)
						frame := readWSFrame(t, ctx, conn)
						succeeded = frame["type"] == "response.completed"
					} else {
						out := httptest.NewRecorder()
						directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
						succeeded = out.Code == http.StatusOK
					}
					wantCalls := int32(0)
					if change == "unchanged" {
						wantCalls = 1
					}
					if exchanges.Load() != 1 || calls.Load() != wantCalls || succeeded != (change == "unchanged") {
						t.Errorf("authority changed during exchange: exchanges=%d dispatches=%d success=%t want dispatches=%d", exchanges.Load(), calls.Load(), succeeded, wantCalls)
					}
				})
			}
		}
	}
}

func TestOAuthPostExchangeRevalidationDoesNotReadmitUser(t *testing.T) {
	for _, change := range []string{"unchanged", "key_revoked", "group_grant_revoked", "key_proxy_changed"} {
		t.Run(change, func(t *testing.T) {
			db, users, user, keys := newBudgetProxyUser(t)
			videoExec(t, db, `UPDATE token_routes SET created_at='2026-11-05T00:00:00Z',updated_at='2026-11-05T00:00:00Z'`)
			if _, err := users.SetUserBudget(t.Context(), user.ID, 1, false); err != nil {
				t.Fatal(err)
			}
			var calls, exchanges atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
			}))
			t.Cleanup(upstream.Close)
			raw := []byte(fmt.Sprintf(`{"version":"1.4","channels":[{"id":1,"type":"qiniu","name":"oauth-budget","base_url":%q,"credentials":{"apiKey":"synthetic-upstream"},"supported_models":["provider"]}],"models":[{"id":1,"model_id":"oauth-budget-model","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"provider"}}]}}]}`, upstream.URL))
			if _, err := backup.ImportAxonHubV14(db, raw, "oauth-budget", false); err != nil {
				t.Fatal(err)
			}
			var routeID int64
			if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='oauth-budget-model'`); err != nil {
				t.Fatal(err)
			}
			if err := users.GrantUserGroupRoute(t.Context(), user.GroupID, routeID); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET provider='codex'`)
			videoExec(t, db, `UPDATE upstream_credentials SET kind='oauth',oauth_state=?`, store.DirectOAuthState{RefreshToken: "synthetic-budget-refresh", ClientID: "synthetic-budget-client", ExpiresAt: 1})
			def := oauth.GetProviderDefinition("codex")
			original := def.RefreshAccessToken
			def.RefreshAccessToken = func(ctx context.Context, input oauth.RefreshTokenInput) (*oauth.TokenSet, error) {
				exchanges.Add(1)
				var err error
				switch change {
				case "key_revoked":
					_, err = db.ExecContext(ctx, `UPDATE downstream_api_keys SET enabled=FALSE`)
				case "group_grant_revoked":
					_, err = db.ExecContext(ctx, `DELETE FROM gateway_user_group_route_grants WHERE route_id=?`, routeID)
				case "key_proxy_changed":
					_, err = db.ExecContext(ctx, `UPDATE downstream_api_keys SET proxy_url='http://unused-proxy.invalid:8080'`)
				}
				if err != nil {
					return nil, err
				}
				return &oauth.TokenSet{AccessToken: "synthetic-budget-access", RefreshToken: "synthetic-budget-rotated", TokenExpiresAt: time.Now().Add(time.Hour).UnixMilli()}, nil
			}
			t.Cleanup(func() { def.RefreshAccessToken = original })
			oldConfig, oldUpstream := config.GetSafe(), getUpstreamConfig()
			config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
			t.Cleanup(func() { config.Set(oldConfig); SetUpstreamConfig(oldUpstream); routing.SetGlobalCache(nil) })
			SetUpstreamConfig(&UpstreamConfig{
				Router:   routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil),
				LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
				ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
					return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
				},
			})
			handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleChatCompletions)), func(context.Context, store.GatewayRequest) error { return nil })
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"oauth-budget-model","messages":[{"role":"user","content":"hello"}],"max_tokens":16}`))
			request.Header.Set("Authorization", "Bearer "+keys[0])
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, request)
			budget, err := users.GetUserBudget(t.Context(), user.ID)
			if err != nil || budget.Reserved != 0 {
				t.Fatalf("post-exchange admission leaked hold: %+v %v", budget, err)
			}
			var holds int
			if err := db.Get(&holds, `SELECT COUNT(*) FROM gateway_user_budget_ledger`); err != nil || holds != 1 {
				t.Fatalf("one ingress produced %d budget admissions: %v", holds, err)
			}
			if exchanges.Load() != 1 {
				t.Fatalf("unexpected exchanges=%d", exchanges.Load())
			}
			if change == "unchanged" {
				if out.Code != 200 || calls.Load() != 1 || budget.Used == 0 {
					t.Fatalf("normal OAuth request lost: status=%d calls=%d used=%g body=%s", out.Code, calls.Load(), budget.Used, out.Body.String())
				}
			} else if out.Code < 400 || calls.Load() != 0 || budget.Used != 0 {
				t.Fatalf("revoked request executed/charged: status=%d calls=%d used=%g", out.Code, calls.Load(), budget.Used)
			}
		})
	}
}
