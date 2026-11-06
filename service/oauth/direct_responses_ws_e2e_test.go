package oauth_test

import (
	"context"
	"encoding/base64"
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
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestImportedCodexOAuthRefreshBetweenWebsocketTurns(t *testing.T) {
	var refreshes, dials atomic.Int32
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := refreshes.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("refresh_token") == "" || r.Form.Get("client_id") != "source-fixture-client" {
			t.Error("refresh source identity missing")
		}
		w.Header().Set("Content-Type", "application/json")
		idToken := "header." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"fixture-account"}}`)) + ".signature"
		_, _ = fmt.Fprintf(w, `{"access_token":"access-fixture-%d","refresh_token":"rotated-refresh-fixture","expires_in":3600,"id_token":%q}`, n, idToken)
	}))
	t.Cleanup(token.Close)
	restore := oauth.OverrideCodexTokenURLForTest(token.URL)
	t.Cleanup(restore)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := dials.Add(1)
		if r.Header.Get("Authorization") != fmt.Sprintf("Bearer access-fixture-%d", n) {
			t.Error("WS used stale access token")
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		for {
			if _, _, err := conn.Read(r.Context()); err != nil {
				return
			}
			if err := conn.Write(r.Context(), websocket.MessageText, []byte(`{"type":"response.completed","response":{"id":"resp-fixture","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`)); err != nil {
				return
			}
		}
	}))
	t.Cleanup(upstream.Close)
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	raw := []byte(fmt.Sprintf(`{"version":"1.4","channels":[{"id":1,"name":"codex-ws","type":"codex","base_url":%q,"credentials":{"oauth":{"access_token":"expired-fixture","refresh_token":"refresh-fixture","client_id":"source-fixture-client","expires_at":"2020-01-01T00:00:00Z"}},"supported_models":["provider-model"]}],"models":[{"id":1,"model_id":"public","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"provider-model"}}]}}]}`, upstream.URL))
	if _, err := backup.ImportAxonHubV14(db, raw, "codex-ws", false); err != nil {
		t.Fatal(err)
	}
	priorRuntime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(priorRuntime); routing.SetGlobalCache(nil) })
	proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{Router: routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil), LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }, ResolveDirectCredential: func(ctx context.Context, id int64, p *string, force bool) (*oauth.DirectCredentialResult, error) {
		return oauth.ResolveDirectCredential(ctx, db.DB, id, p, force)
	}})
	t.Cleanup(func() { proxyhandler.SetUpstreamConfig(nil) })
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyhandler.HandleResponsesWebsocket(w, r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "key-fixture", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy})))
	}))
	t.Cleanup(gateway.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(gateway.URL, "http")+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	for i := 0; i < 2; i++ {
		if i == 1 {
			var state store.DirectOAuthState
			if err := db.Get(&state, `SELECT oauth_state FROM upstream_credentials`); err != nil {
				t.Fatal(err)
			}
			state.ExpiresAt = 1
			if _, err := db.Exec(`UPDATE upstream_credentials SET oauth_state=?`, state); err != nil {
				t.Fatal(err)
			}
		}
		if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.create","model":"public","input":[{"role":"user","content":"hello"}]}`)); err != nil {
			t.Fatal(err)
		}
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var frame map[string]any
		_ = json.Unmarshal(data, &frame)
		if frame["type"] != "response.completed" {
			t.Fatalf("refresh WS relay failed: %s", data)
		}
	}
	if refreshes.Load() != 2 || dials.Load() != 2 {
		t.Fatalf("refreshes=%d dials=%d", refreshes.Load(), dials.Load())
	}
}
