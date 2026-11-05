package proxyhandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectCompactIsNativeAndSeparatelyAuthorized(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			var status atomic.Int32
			status.Store(200)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				if r.URL.Path != "/compact-exact" || !strings.Contains(string(body), `"model":"provider-model"`) ||
					!strings.Contains(string(body), `"encrypted_content":"opaque-fixture"`) || !strings.Contains(string(body), "9007199254740993") {
					t.Errorf("compact request lost native fields: %s %s", r.URL.Path, body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(int(status.Load()))
				if status.Load() != 200 {
					_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"fixture","message":"compact unavailable","native":9007199254740993}}`)
				} else {
					_, _ = io.WriteString(w, `{"id":"cmp-fixture","object":"response.compaction","created_at":9007199254740993,"output":[{"type":"compaction","encrypted_content":"opaque-fixture","extension":9007199254740993}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
				}
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{
				ResponsesCompact: &store.DirectEndpoint{URL: upstream.URL + "/compact-exact", Auth: store.DirectAuthBearer},
			}, store.DirectProtocolResponsesCompact, "")
			cfg := *getUpstreamConfig()
			var logged []proxy.ProxyLogEntry
			cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logged = append(logged, entry); return nil }
			SetUpstreamConfig(&cfg)
			body := `{"model":"client-alias","input":[{"type":"compaction","encrypted_content":"opaque-fixture","opaque":9007199254740993}],"instructions":"keep native"}`
			for _, path := range []string{"/v1/responses", "/v1/chat/completions"} {
				out := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(out, makeProxyReq("POST", path, body))
				if out.Code < 400 || calls.Load() != 0 {
					t.Fatal("compact grant allowed a generation endpoint")
				}
			}
			for _, invalid := range []string{strings.TrimSuffix(body, "}") + `,"stream":true}`, strings.TrimSuffix(body, "}") + `,"stream":"false"}`} {
				out := httptest.NewRecorder()
				HandleResponses(out, makeProxyReq("POST", "/v1/responses/compact", invalid), "/v1/responses/compact")
				if out.Code != 400 || calls.Load() != 0 {
					t.Fatal("streaming compact reached upstream")
				}
			}
			out := httptest.NewRecorder()
			HandleResponses(out, makeProxyReq("POST", "/v1/responses/compact", body), "/v1/responses/compact")
			if out.Code != 200 || calls.Load() != 1 || !json.Valid(out.Body.Bytes()) || strings.Count(out.Body.String(), "9007199254740993") != 2 || !strings.Contains(out.Body.String(), "opaque-fixture") {
				t.Fatalf("compact response changed: %d %s", out.Code, out.Body.String())
			}
			if len(logged) != 1 || logged[0].TotalTokens == nil || *logged[0].TotalTokens != 3 {
				t.Fatalf("compact usage not recorded: %+v", logged)
			}
			status.Store(422)
			out = httptest.NewRecorder()
			HandleResponses(out, makeProxyReq("POST", "/v1/responses/compact", body), "/v1/responses/compact")
			if out.Code != 422 || !strings.Contains(out.Body.String(), `"native":9007199254740993`) {
				t.Fatalf("native compact error changed: %d %s", out.Code, out.Body.String())
			}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_grants SET protocols=?`), store.DirectProtocolResponses); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			before := calls.Load()
			out = httptest.NewRecorder()
			HandleResponses(out, makeProxyReq("POST", "/v1/responses/compact", body), "/v1/responses/compact")
			if out.Code < 400 || calls.Load() != before {
				t.Fatal("HTTP Responses grant authorized compact")
			}
		})
	}
}

func TestCompactOpaqueOutputCanContinueResponsesWithoutPrecisionLoss(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/compact" {
			_, _ = io.WriteString(w, `{"id":"cmp-fixture","object":"response.compaction","output":[{"type":"compaction","encrypted_content":"opaque-fixture","extension":9007199254740993}]}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"encrypted_content":"opaque-fixture"`) || !strings.Contains(string(body), "9007199254740993") {
			t.Errorf("opaque continuation modified: %s", body)
		}
		_, _ = io.WriteString(w, `{"id":"resp-tool","object":"response","status":"completed","output":[{"type":"function_call","call_id":"call-fixture","name":"echo","arguments":"{\"n\":9007199254740993}"}],"usage":{"input_tokens":2,"output_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)
	installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{
		ResponsesCompact: &store.DirectEndpoint{URL: upstream.URL + "/compact", Auth: store.DirectAuthBearer},
		Responses:        &store.DirectEndpoint{URL: upstream.URL + "/responses", Auth: store.DirectAuthBearer},
	}, store.DirectProtocolResponsesCompact|store.DirectProtocolResponses, "")
	cfg := *getUpstreamConfig()
	logs := make(chan proxy.ProxyLogEntry, 2)
	cfg.LogProxy = func(_ context.Context, e proxy.ProxyLogEntry) error { logs <- e; return nil }
	SetUpstreamConfig(&cfg)
	out := httptest.NewRecorder()
	HandleResponses(out, makeProxyReq("POST", "/v1/responses/compact", `{"model":"client-alias","input":[{"role":"user","content":"hello"}]}`), "/v1/responses/compact")
	if out.Code != 200 {
		t.Fatal(out.Body.String())
	}
	entry := <-logs
	if entry.TotalTokens != nil {
		t.Fatal("unknown compact usage fabricated")
	}
	var compact struct{ Output json.RawMessage }
	_ = json.Unmarshal(out.Body.Bytes(), &compact)
	out = httptest.NewRecorder()
	HandleResponses(out, makeProxyReq("POST", "/v1/responses", `{"model":"client-alias","input":`+string(compact.Output)+`}`), "/v1/responses")
	if out.Code != 200 || calls.Load() != 2 || !strings.Contains(out.Body.String(), "call-fixture") {
		t.Fatalf("compact tool continuation failed: %d %s", out.Code, out.Body.String())
	}
}
