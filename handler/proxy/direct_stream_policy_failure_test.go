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

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectRequiredStreamOllamaAggregation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var input map[string]json.RawMessage
				if json.NewDecoder(r.Body).Decode(&input) != nil || string(input["stream"]) != "true" {
					t.Error("Ollama require did not stream")
				}
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = fmt.Fprintln(w, `{"model":"provider-model","done":false,"message":{"role":"assistant","content":"hel","thinking":"reason"}}`)
				_, _ = fmt.Fprintln(w, `{"model":"provider-model","done":false,"message":{"content":"lo","tool_calls":[{"function":{"name":"echo","arguments":{"n":9007199254740993}}}]}}`)
				_, _ = fmt.Fprintln(w, `{"model":"provider-model","done":true,"done_reason":"stop","prompt_eval_count":2,"eval_count":3,"total_duration":9007199254740993}`)
			}))
			t.Cleanup(server.Close)
			db := installOllamaHTTPRuntime(t, dialect, server.URL, true)
			if _, err := db.Exec(`UPDATE upstream_channels SET stream_policy='require'`); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			out := httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
			if out.Code != 200 || !json.Valid(out.Body.Bytes()) || !strings.Contains(out.Body.String(), `"content":"hello"`) || !strings.Contains(out.Body.String(), `"reasoning_content":"reason"`) || strings.Count(out.Body.String(), "9007199254740993") != 2 {
				t.Fatalf("Ollama aggregate: %d %s", out.Code, out.Body.String())
			}
		})
	}
}

func TestDirectRequiredStreamFailureNeverCommitsPartialJSON(t *testing.T) {
	for _, mode := range []string{"missing-terminal", "explicit-error", "byte-limit", "cancel", "idle"} {
		t.Run(mode, func(t *testing.T) {
			previous := config.GetSafe()
			cfg := *config.Get()
			cfg.ProxyMaxChannelAttempts = 1
			cfg.ProxyDirectAccountMaxConcurrency = 1
			if mode == "byte-limit" {
				cfg.ProxyMaxStreamResponseBytes = 48
			}
			if mode == "idle" {
				cfg.ProxyStreamIdleTimeoutSec = 1
			}
			config.Set(&cfg)
			t.Cleanup(func() { config.Set(previous) })
			var calls atomic.Int32
			canceled := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				defer close(canceled)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, streamPolicyFrame("", `{"id":"c","choices":[{"index":0,"delta":{"role":"assistant","content":"private-partial"},"finish_reason":null}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
				w.(http.Flusher).Flush()
				switch mode {
				case "explicit-error":
					_, _ = fmt.Fprint(w, streamPolicyFrame("error", `{"error":{"message":"failed"}}`))
				case "cancel", "idle":
					<-r.Context().Done()
				}
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			if _, err := db.Exec(`UPDATE upstream_channels SET stream_policy='require'`); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			limiter := proxy.NewSiteConcurrencyLimiter()
			getUpstreamConfig().DirectAccountLimiter = limiter
			manager := channellimit.NewManager()
			limits := channellimit.Config{RPM: 10, TPM: 100, MaxConcurrent: 1}
			getUpstreamConfig().ChannelLimits = manager
			getUpstreamConfig().Router.(*routing.TokenRouter).SetChannelLimits(manager)
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET rate_limit=?`), store.DirectRateLimit{Config: limits}); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			var channelID int64
			if err := db.Get(&channelID, `SELECT id FROM upstream_channels LIMIT 1`); err != nil {
				t.Fatal(err)
			}
			var credentialID int64
			if err := db.Get(&credentialID, `SELECT id FROM upstream_credentials LIMIT 1`); err != nil {
				t.Fatal(err)
			}
			var logs []proxy.ProxyLogEntry
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }
			req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`)
			if mode == "cancel" {
				ctx, cancel := context.WithTimeout(req.Context(), 50*time.Millisecond)
				defer cancel()
				req = req.WithContext(ctx)
			}
			out := httptest.NewRecorder()
			windowStarted := time.Now().Unix() / 60
			HandleChatCompletions(out, req)
			if out.Code < 400 || strings.Contains(out.Body.String(), "private-partial") || strings.Contains(out.Body.String(), "data:") || calls.Load() != 1 || len(logs) != 1 || logs[0].Status == "success" {
				t.Fatalf("partial success status=%d calls=%d logs=%d body=%s", out.Code, calls.Load(), len(logs), out.Body.String())
			}
			if limiter.ActiveCount(credentialID) != 0 {
				t.Fatal("aggregate failure leaked credential slot")
			}
			snapshot := manager.Snapshot(channelID, limits)
			windowFinished := time.Now().Unix() / 60
			if snapshot.Active != 0 || snapshot.Waiting != 0 || snapshot.Requests > 1 || snapshot.UnknownUsage > 1 || snapshot.Tokens > 3 {
				t.Fatalf("aggregate failure leaked or double-counted channel state: %+v windows=%d/%d", snapshot, windowStarted, windowFinished)
			}
			if windowStarted == windowFinished && snapshot.Requests != 1 {
				t.Fatalf("aggregate failure lost the current-minute request: %+v", snapshot)
			}
			// Natural-minute counters can roll over during the idle timeout.
			// The attempt log must retain its reported usage in either window.
			if mode != "byte-limit" && (logs[0].TotalTokens == nil || *logs[0].TotalTokens != 3 || windowStarted == windowFinished && snapshot.Tokens != 3) {
				t.Fatalf("aggregate failure lost reported usage: %+v", snapshot)
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("stream cancellation did not close transport")
			}
		})
	}
}

func TestDirectRequiredStreamRetriesBeforeCommitting(t *testing.T) {
	previous := config.GetSafe()
	cfg := *config.Get()
	cfg.ProxyMaxChannelAttempts = 2
	config.Set(&cfg)
	t.Cleanup(func() { config.Set(previous) })
	var calls [2]atomic.Int32
	var servers [2]*httptest.Server
	for i := range servers {
		index := i
		servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls[index].Add(1)
			var body map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			if string(body["stream"]) != "true" {
				t.Error("retry lost required stream")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			content, finish := "discarded-partial", "null"
			if index == 1 {
				content, finish = "final-answer", `"stop"`
			}
			_, _ = fmt.Fprint(w, streamPolicyFrame("", fmt.Sprintf(`{"id":"c","choices":[{"index":0,"delta":{"role":"assistant","content":%q},"finish_reason":%s}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, content, finish)))
			if index == 1 {
				_, _ = fmt.Fprint(w, streamPolicyFrame("", "[DONE]"))
			}
		}))
		t.Cleanup(servers[i].Close)
	}
	db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: servers[0].URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
	payload := fmt.Sprintf(`{"version":"1.4","channels":[{"id":1,"type":"qiniu","name":"First","base_url":%q,"credentials":{"apiKey":"synthetic-one"},"supported_models":["provider-model"],"policies":{"stream":"require"}},{"id":2,"type":"qiniu","name":"Second","base_url":%q,"credentials":{"apiKey":"synthetic-two"},"supported_models":["provider-model"],"policies":{"stream":"require"}}],"models":[{"id":1,"model_id":"client-alias","settings":{"associations":[{"type":"channel_model","priority":1,"channelModel":{"channelId":1,"modelId":"provider-model"}},{"type":"channel_model","priority":2,"channelModel":{"channelId":2,"modelId":"provider-model"}}]}}]}`, servers[0].URL, servers[1].URL)
	if _, err := backup.ImportAxonHubV14(db, []byte(payload), "media-fixture", false); err != nil {
		t.Fatal(err)
	}
	var logs []proxy.ProxyLogEntry
	getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }
	out := httptest.NewRecorder()
	HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
	if out.Code != 200 || calls[0].Load() != 1 || calls[1].Load() != 1 || len(logs) != 2 || logs[0].Status == "success" || logs[1].Status != "success" || strings.Contains(out.Body.String(), "discarded-partial") || !strings.Contains(out.Body.String(), "final-answer") {
		t.Fatalf("retry status=%d attempts=%d/%d logs=%d body=%s", out.Code, calls[0].Load(), calls[1].Load(), len(logs), out.Body.String())
	}
	for _, entry := range logs {
		if entry.TotalTokens == nil || *entry.TotalTokens != 2 {
			t.Fatal("attempt usage lost or counted twice")
		}
	}
}

func TestDirectRequireDoesNotAggregateEmbeddings(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	t.Cleanup(upstream.Close)
	db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Embeddings: &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolEmbeddings, "")
	if _, err := db.Exec(`UPDATE upstream_channels SET stream_policy='require'`); err != nil {
		t.Fatal(err)
	}
	routing.InvalidateCache()
	out := httptest.NewRecorder()
	directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/embeddings", `{"model":"client-alias","input":"hello"}`))
	if out.Code < 400 || calls.Load() != 0 {
		t.Fatalf("embeddings misaggregated: status=%d calls=%d", out.Code, calls.Load())
	}
}

func TestDirectRequiredStreamPreservesNativeLimitedTerminal(t *testing.T) {
	for _, tc := range []struct {
		protocol, path, request, frame, marker string
		bit                                    int
		endpoints                              func(string) store.DirectEndpoints
	}{
		{"responses", "/v1/responses", `{"model":"client-alias","input":"hello"}`, streamPolicyFrame("response.incomplete", `{"type":"response.incomplete","response":{"id":"r","model":"provider-model","status":"incomplete","output":[],"incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":2,"output_tokens":3}}}`), `"reason":"max_output_tokens"`, store.DirectProtocolResponses, func(url string) store.DirectEndpoints {
			return store.DirectEndpoints{Responses: &store.DirectEndpoint{URL: url, Auth: store.DirectAuthBearer}}
		}},
		{"gemini", "/v1beta/models/client-alias:generateContent", `{"contents":[{"parts":[{"text":"hello"}]}]}`, streamPolicyFrame("", `{"promptFeedback":{"blockReason":"SAFETY"},"modelVersion":"provider-model","usageMetadata":{"promptTokenCount":2}}`), `"blockReason":"SAFETY"`, store.DirectProtocolGemini, func(url string) store.DirectEndpoints {
			return store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: url, Auth: store.DirectAuthGoogle}}
		}},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, tc.frame)
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, store.DialectSQLite, tc.endpoints(upstream.URL), tc.bit, "")
			if _, err := db.Exec(`UPDATE upstream_channels SET stream_policy='require'`); err != nil {
				t.Fatal(err)
			}
			routing.InvalidateCache()
			out := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", tc.path, tc.request))
			if out.Code != 200 || !json.Valid(out.Body.Bytes()) || !strings.Contains(out.Body.String(), tc.marker) {
				t.Fatalf("native limited terminal became gateway error: %d %s", out.Code, out.Body.String())
			}
		})
	}
}
