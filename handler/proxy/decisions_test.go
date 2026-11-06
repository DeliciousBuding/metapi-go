package proxyhandler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDecisionsNativeJSONAndIndependentGrant(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			var reply atomic.Value
			reply.Store(`{"model":"provider-model","answers":[{"choice":9007199254740993}],"vendor":{"receipt":true},"usage":{"input_tokens":7}}`)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				if r.URL.Path != "/exact-decisions" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("x-api-key") != "" ||
					!strings.Contains(string(body), `"model":"provider-model"`) || !strings.Contains(string(body), "9007199254740993") || !strings.Contains(string(body), `"quality":"high"`) {
					t.Errorf("Decisions wire lost native body/auth/model/override: %s %s", r.URL.Path, body)
				}
				w.Header().Set("Content-Type", "text/plain")
				_, _ = io.WriteString(w, reply.Load().(string))
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{
				Decisions: &store.DirectEndpoint{URL: upstream.URL + "/exact-decisions", Auth: store.DirectAuthBearer},
			}, store.DirectProtocolDecisions, `{"quality":"high"}`)
			config.SetRuntime(&config.RuntimeSettings{ProxyEmptyContentFailEnabled: true})
			cfg := *getUpstreamConfig()
			var logs []proxy.ProxyLogEntry
			cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }
			SetUpstreamConfig(&cfg)
			for _, input := range []string{
				`{"model":"client-alias","input":"text","questions":[{}],"stream":true}`,
				`{"model":"client-alias","input":"text","questions":[{}],"stream":"false"}`,
				`{"model":"client-alias","input":null,"questions":[{}]}`,
				`{"model":"client-alias","input":"","questions":[{}]}`,
				`{"model":"client-alias","input":[],"questions":[{}]}`,
				`{"model":"client-alias","input":{},"questions":[{}]}`,
				`{"model":"client-alias","input":[{"type":"function_call"}],"questions":[{}]}`,
				`{"model":"client-alias","input":[{"type":"message","content":[]}],"questions":[{}]}`,
				`{"model":"client-alias","input":[{"type":"input_image","image_url":false}],"questions":[{}]}`,
				`{"model":"client-alias","input":"text","questions":[]}`,
				`{"model":"client-alias","input":"text","questions":{}}`,
			} {
				out := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/decisions", input))
				if out.Code != 400 || calls.Load() != 0 {
					t.Fatalf("invalid Decisions request dispatched: %d %s", out.Code, out.Body.String())
				}
			}
			body := `{"model":"client-alias","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"fixture"},{"type":"input_image","image_url":"https://example.invalid/synthetic.png"}]}],"questions":[{"extension":9007199254740993}],"vendor":{"id":9007199254740993},"stream":null}`
			for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/systemone", "/v1/alpha/search", "/v1/responses/compact"} {
				out := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(out, makeProxyReq("POST", path, body))
				if out.Code < 400 || calls.Load() != 0 {
					t.Fatalf("Decisions grant authorized %s", path)
				}
			}
			out := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/decisions", body))
			if out.Code != 200 || calls.Load() != 1 || out.Header().Get("Content-Type") != "application/json" || out.Body.String() != reply.Load().(string) {
				t.Fatalf("native Decisions response changed: %d %s", out.Code, out.Body.String())
			}
			if len(logs) != 1 || logs[0].PromptTokens == nil || *logs[0].PromptTokens != 7 || logs[0].CompletionTokens == nil || *logs[0].CompletionTokens != 0 || logs[0].TotalTokens == nil || *logs[0].TotalTokens != 7 || logs[0].DownstreamPath == nil || *logs[0].DownstreamPath != "/v1/decisions" {
				t.Fatal("native Decisions input-only usage/path was not recorded")
			}
			reply.Store(`{"answers":[{"opaque":9007199254740993}],"usage":null}`)
			out = httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/decisions", body))
			if out.Code != 200 || len(logs) != 2 || logs[1].TotalTokens != nil {
				t.Fatal("missing Decisions usage became fabricated zero")
			}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_grants SET protocols=?`), store.DirectProtocolResponses); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			out = httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/decisions", body))
			if out.Code < 400 || calls.Load() != 2 {
				t.Fatal("Responses grant authorized Decisions")
			}
		})
	}
}

func TestDecisionsInvalidResponseIsNotReplayed(t *testing.T) {
	for _, reply := range []string{`data: {"answers":[]}`, `{"answers":{}}`, `{"model":123,"answers":[]}`, `{"answers":[],"usage":{"input_tokens":"seven"}}`, `[]`} {
		t.Run(reply, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, reply)
			}))
			t.Cleanup(upstream.Close)
			installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Decisions: &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolDecisions, "")
			out := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/decisions", `{"model":"client-alias","input":"text","questions":[{}]}`))
			if out.Code != 502 || calls.Load() != 1 {
				t.Fatalf("bad native response accepted/replayed: %d calls=%d", out.Code, calls.Load())
			}
		})
	}
}

func TestDecisionsCancellationReachesUpstream(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	t.Cleanup(upstream.Close)
	installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Decisions: &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolDecisions, "")
	req := makeProxyReq("POST", "/v1/decisions", `{"model":"client-alias","input":"text","questions":[{}]}`)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() { defer close(done); directMediaRouter().ServeHTTP(httptest.NewRecorder(), req) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request never reached upstream")
	}
	cancel()
	for _, event := range []<-chan struct{}{canceled, done} {
		select {
		case <-event:
		case <-time.After(3 * time.Second):
			t.Fatal("Decisions cancellation was not propagated")
		}
	}
}

func TestDecisionsDoesNotBorrowUserTextBudget(t *testing.T) {
	if _, err := userBudgetTokenEstimate("/v1/decisions", map[string]any{"model": "fixture", "input": "text", "questions": []any{map[string]any{}}}); err == nil {
		t.Fatal("Decisions silently acquired generation token-budget admission")
	}
}
