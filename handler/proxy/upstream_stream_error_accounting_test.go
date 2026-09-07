package proxyhandler

import (
	"context"
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

// Exercise the production handlers with the optional content heuristics off.
// A spare channel and retry budget make any retry after SSE commitment visible.
func TestDispatchStreamExplicitErrorAccounting(t *testing.T) {
	prevCfg, prevRt := config.GetSafe(), config.RuntimeSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 3})
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() {
		config.Set(prevCfg)
		config.SetRuntime(prevRt)
	})

	protocols := []struct {
		name, path, model, request string
		handle                     http.HandlerFunc
		output, failure, done      string
		hasUsage                   bool
	}{
		{
			name: "chat", path: "/v1/chat/completions", model: "gpt-4o",
			request: `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`,
			handle:  HandleChatCompletions,
			output: `data: {"choices":[{"delta":{"content":"The word error, including insufficient_quota, is ordinary output."}}]}` + "\n\n" +
				`data: {"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}` + "\n\n",
			failure: `data: {"type":"error","error":{"message":"insufficient_quota","type":"invalid_request_error"}}` + "\n\n",
			done:    "data: [DONE]\n\n", hasUsage: true,
		},
		{
			name: "messages", path: "/v1/messages", model: "claude-sonnet-test",
			request: `{"model":"claude-sonnet-test","stream":true,"max_tokens":32,"messages":[{"role":"user","content":"hi"}]}`,
			handle:  HandleClaudeMessages,
			output: "event: message_start\n" + `data: {"type":"message_start","message":{"usage":{"input_tokens":2,"output_tokens":0}}}` + "\n\n" +
				"event: content_block_delta\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"The word error is ordinary output."}}` + "\n\n" +
				"event: message_delta\n" + `data: {"type":"message_delta","usage":{"output_tokens":3}}` + "\n\n",
			failure: "event: error\n" + `data: {"type":"error","error":{"type":"overloaded_error","message":"fixture stream failure"}}` + "\n\n",
			done:    "event: message_stop\n" + `data: {"type":"message_stop"}` + "\n\n", hasUsage: true,
		},
		{
			name: "responses", path: "/v1/responses", model: "gpt-4o",
			request: `{"model":"gpt-4o","stream":true,"input":"hi"}`,
			handle:  func(w http.ResponseWriter, r *http.Request) { HandleResponses(w, r, "/v1/responses") },
			output:  "event: response.output_text.delta\n" + `data: {"type":"response.output_text.delta","delta":"The word error is ordinary output."}` + "\n\n",
			failure: "event: response.failed\n" + `data: {"type":"response.failed","response":{"status":"failed","error":{"code":"server_error","message":"fixture stream failure"}}}` + "\n\n",
			done:    "event: response.completed\n" + `data: {"type":"response.completed","response":{"status":"completed","error":null}}` + "\n\n",
		},
	}
	for _, p := range protocols {
		for _, scenario := range []struct {
			name             string
			withOutput, fail bool
		}{
			{name: "error_only", fail: true},
			{name: "output_then_error", withOutput: true, fail: true},
			{name: "healthy_output_mentions_error", withOutput: true},
		} {
			t.Run(p.name+"/"+scenario.name, func(t *testing.T) {
				var frames []string
				if scenario.withOutput {
					frames = append(frames, p.output)
				}
				if scenario.fail {
					frames = append(frames, p.failure)
				} else {
					frames = append(frames, p.done)
				}
				// A trailing [DONE] must not erase an earlier explicit error.
				if scenario.fail && p.name == "chat" {
					frames = append(frames, p.done)
				}
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					for _, frame := range frames {
						_, _ = w.Write([]byte(frame))
						w.(http.Flusher).Flush()
					}
				}))
				t.Cleanup(upstream.Close)

				selected := routing.SelectedChannel{
					Channel:    store.RouteChannel{ID: 42, Enabled: true},
					Account:    store.Account{ID: 7, Status: "active"},
					Site:       store.Site{ID: 3, URL: upstream.URL, Status: "active"},
					TokenValue: "upstream-token", ActualModel: p.model,
				}
				next := selected
				next.Channel.ID = 43
				router := &upstreamTestRouter{selected: selected, next: &next}
				var logs []proxy.ProxyLogEntry
				SetUpstreamConfig(&UpstreamConfig{
					Router: router, Executor: proxy.NewRuntimeExecutor(10 * time.Second),
					LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error {
						logs = append(logs, entry)
						return nil
					},
				})
				t.Cleanup(func() { SetUpstreamConfig(nil) })

				rec := httptest.NewRecorder()
				p.handle(rec, makeProxyReq(http.MethodPost, p.path, p.request))
				if calls.Load() != 1 || len(router.policies) != 1 {
					t.Fatalf("committed stream retried: upstream calls=%d, channel selections=%d", calls.Load(), len(router.policies))
				}
				if rec.Code != http.StatusOK || !rec.Flushed || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
					t.Fatalf("committed SSE response changed: status=%d, flushed=%v, headers=%v", rec.Code, rec.Flushed, rec.Header())
				}
				if got, want := rec.Body.String(), strings.Join(frames, ""); got != want {
					t.Fatalf("SSE was rewritten or appended to:\n got %q\nwant %q", got, want)
				}
				if len(logs) != 1 || logs[0].RetryCount != 0 {
					t.Fatalf("proxy logs = %+v, want one terminal attempt without retry", logs)
				}
				entry := logs[0]
				if scenario.fail {
					assertFailureSurface(t, router, &logs, http.StatusBadGateway, "Upstream returned an error event")
					if entry.Status != "failed" {
						t.Fatalf("proxy log status = %q, want failed", entry.Status)
					}
				} else if len(router.failures) != 0 || len(router.successes) != 1 || entry.Status != "success" || entry.HTTPStatus != http.StatusOK {
					t.Fatalf("healthy stream was penalized: failures=%+v successes=%+v log=%+v", router.failures, router.successes, entry)
				}
				wantUsage := [3]int64{}
				wantSource := usageSourceUnknown
				if scenario.withOutput && p.hasUsage {
					wantUsage = [3]int64{2, 3, 5}
					wantSource = usageSourceUpstream
				}
				if entry.PromptTokens == nil || entry.CompletionTokens == nil || entry.TotalTokens == nil {
					t.Fatalf("usage fields missing: %+v", entry)
				}
				if got := [3]int64{*entry.PromptTokens, *entry.CompletionTokens, *entry.TotalTokens}; got != wantUsage || entry.UsageSource != wantSource {
					t.Fatalf("usage = %v (%s), want %v (%s)", got, entry.UsageSource, wantUsage, wantSource)
				}
			})
		}
	}
}

func TestStreamExplicitErrorRecognitionAndVerdict(t *testing.T) {
	prevRt := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	for _, tc := range []struct {
		name, frame string
		wantError   bool
	}{
		{"named_error_without_data", "event: error\n\n", true},
		{"named_response_failed", "event: response.failed\ndata: {}\n\n", true},
		{"payload_error_type", `data: {"type":"error","message":"fixture failure"}` + "\n\n", true},
		{"payload_response_failed", `data: {"type":"response.failed","response":{"error":{"code":"server_error"}}}` + "\n\n", true},
		{"error_object", `data: {"error":{"message":"fixture failure"}}` + "\n\n", true},
		{"error_string", `data: {"error":"fixture failure"}` + "\n\n", true},
		{"explicit_error_with_null_payload", `data: {"type":"error","error":null}` + "\n\n", true},
		{"null_error_is_not_failure", `data: {"error":null,"choices":[{"delta":{"content":"hello"}}]}` + "\n\n", false},
		{"quoted_error_in_output", `data: {"choices":[{"delta":{"content":"{\"error\":\"ordinary text\"}"}}]}` + "\n\n", false},
		{"plain_text_mentions_error", "data: error is an ordinary word\n\n", false},
		{"completed_response", "event: response.completed\n" + `data: {"type":"response.completed","response":{"error":null,"status":"completed"}}` + "\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := newIncrementalSseAnalyzer()
			// Split within field names, JSON and SSE boundaries, not just events.
			for start := 0; start < len(tc.frame); start += 7 {
				analyzer.Push([]byte(tc.frame[start:min(start+7, len(tc.frame))]))
			}
			result := analyzer.Result()
			if result.HasErrorEvent != tc.wantError {
				t.Errorf("HasErrorEvent = %v, want %v", result.HasErrorEvent, tc.wantError)
			}
			verdict := judgeStreamContent(http.StatusOK, result, 1, false)
			if verdict.Failed != tc.wantError {
				t.Fatalf("verdict = %+v, want failure=%v with default content settings", verdict, tc.wantError)
			}
			if tc.wantError && (verdict.Code != "upstream_error_event" || verdict.Status != http.StatusBadGateway || verdict.Reason != "Upstream returned an error event") {
				t.Fatalf("explicit error verdict must be stable and sanitized, got %+v", verdict)
			}
		})
	}
}

// Downstream cancellation alone is not an upstream failure, but it must not
// discard an explicit upstream error already parsed before the write failed.
func TestHandleStreamExplicitErrorBeforeClientDisconnect(t *testing.T) {
	prevRt := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	prefix := `data: {"choices":[{"delta":{"content":"partial answer"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}` + "\n\n"
	failure := `data: {"type":"error","error":{"message":"fixture stream failure"}}` + "\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       &chunkedReader{chunks: []string{prefix, failure}},
	}
	writer := &disconnectAfterNWriter{ResponseRecorder: httptest.NewRecorder(), n: 2}
	usage, end, verdict := handleStreamUpstream(writer, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil), resp, 1)
	if end != streamEndedClientDisconnect {
		t.Fatalf("stream end = %v, want client disconnect", end)
	}
	if verdict == nil || !verdict.Failed || verdict.Code != "upstream_error_event" {
		t.Fatalf("observed upstream error was discarded on downstream disconnect: %+v", verdict)
	}
	if !usage.Found || usage.PromptTokens != 2 || usage.CompletionTokens != 3 || usage.TotalTokens != 5 {
		t.Fatalf("partial upstream usage lost: %+v", usage)
	}
	if writer.Body.String() != prefix {
		t.Fatalf("already committed SSE was rewritten: %q", writer.Body.String())
	}
}
