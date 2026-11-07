package proxyhandler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderAnonymousMessagesUsesCurrentCredentialAndOriginalUsage(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				for _, key := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "Cookie", "Proxy-Authorization"} {
					if r.Header.Get(key) != "" {
						t.Errorf("anonymous driver sent %s", key)
					}
				}
				if r.Header.Get("X-Provider-Anonymous") != "on" {
					t.Error("provider request hook was not executed")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"msg-anonymous","type":"message","role":"assistant","model":"provider-model","content":[{"type":"text","text":"anonymous receipt"}],"stop_reason":"end_turn","usage":{"input_tokens":7,"output_tokens":2}}`)
			}))
			defer server.Close()
			endpoints := store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: server.URL + "/v1/messages", Auth: store.DirectAuthNone, Profile: "ollama-messages"}}
			db, logs := installPlatformHTTPRuntime(t, dialect, "ollama", "provider-model", endpoints, store.DirectProtocolMessages, "")
			videoExec(t, db, `UPDATE upstream_credentials SET kind='none',secret=''`)
			const source = `export const meta={apiVersion:2,kind:"provider",key:"anonymous",name:"Anonymous",version:"1.0.0",protocols:["messages"],credentialKinds:["none"],hooks:["request","response"]};export function request(i){return {method:"POST",body:i.body,headers:{"X-Provider-Anonymous":"on"}};}export function response(i){return {body:i.body};}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "anonymous", source, true, 0); err != nil {
				t.Fatalf("anonymous driver cannot be installed: %v", err)
			}
			binding := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "anonymous", Protocols: []string{"messages"}}}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, binding)
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			w := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/messages", `{"model":"client-alias","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`))
			if w.Code != 200 || !strings.Contains(w.Body.String(), "anonymous receipt") || calls.Load() != 1 {
				t.Fatalf("anonymous driver did not dispatch: %d calls=%d %s", w.Code, calls.Load(), w.Body.String())
			}
			select {
			case log := <-logs:
				if log.Status != "success" || log.TotalTokens == nil || *log.TotalTokens != 9 {
					t.Fatalf("anonymous usage was not observed: %+v", log)
				}
			case <-time.After(time.Second):
				t.Fatal("no anonymous attempt log")
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "anonymous", source, false, 1); err != nil {
				t.Fatal(err)
			}
			w = httptest.NewRecorder()
			directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/messages", `{"model":"client-alias","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`))
			if w.Code != 503 || calls.Load() != 1 {
				t.Fatalf("disabled driver was bypassed: %d calls=%d %s", w.Code, calls.Load(), w.Body.String())
			}
		})
	}
}
