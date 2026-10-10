package proxyhandler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

// These are native client histories, not converter-generated requests. They
// replay the call-fixture returned by the first HTTP round with its tool result.
var openCodeContinuationRequests = []string{
	`{"model":"client-alias","messages":[{"role":"user","content":"receipt-history"},{"role":"assistant","reasoning_content":"retained-thinking","content":null,"tool_calls":[{"id":"call-fixture","type":"function","function":{"name":"echo","arguments":"{\"value\":\"receipt-tool\"}"}}]},{"role":"tool","tool_call_id":"call-fixture","content":"receipt-result"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}]}`,
	`{"model":"client-alias","input":[{"role":"user","content":"receipt-history"},{"type":"function_call","call_id":"call-fixture","name":"echo","arguments":"{\"value\":\"receipt-tool\"}"},{"type":"function_call_output","call_id":"call-fixture","output":"receipt-result"}],"tools":[{"type":"function","name":"echo","parameters":{"type":"object"}}]}`,
	`{"model":"client-alias","max_tokens":64,"messages":[{"role":"user","content":"receipt-history"},{"role":"assistant","content":[{"type":"tool_use","id":"call-fixture","name":"echo","input":{"value":"receipt-tool"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-fixture","content":"receipt-result"}]}],"tools":[{"name":"echo","input_schema":{"type":"object"}}]}`,
	`{"contents":[{"role":"user","parts":[{"text":"receipt-history"}]},{"role":"model","parts":[{"functionCall":{"id":"call-fixture","name":"echo","args":{"value":"receipt-tool"}}}]},{"role":"user","parts":[{"functionResponse":{"id":"call-fixture","name":"echo","response":{"result":"receipt-result"}}}]}],"tools":[{"functionDeclarations":[{"name":"echo","parameters":{"type":"object"}}]}]}`,
}

func TestDirectOpenCodeToolContinuationHTTP(t *testing.T) {
	for _, family := range []struct {
		model string
		index int
	}{
		{"deepseek-v4-flash", 0}, {"gpt-5.6-luna", 1}, {"minimax-m3", 2},
	} {
		for index, down := range directWireFixtures {
			// The existing Gemini -> Chat tool-result intermediate carries name,
			// which Responses and Messages deliberately reject. Covered below.
			if index == 3 && family.index != 0 {
				continue
			}
			t.Run(family.model+"/"+down.provider, func(t *testing.T) {
				up := directWireFixtures[family.index]
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					turn := calls.Add(1)
					body, _ := io.ReadAll(r.Body)
					if r.Header.Get("X-Opencode-Session") != "conversation-fixture" {
						t.Error("tool round lost conversation affinity")
					}
					if turn == 2 {
						for _, marker := range []string{"receipt-history", "call-fixture", "echo", "receipt-tool", "receipt-result"} {
							if !strings.Contains(string(body), marker) {
								t.Errorf("tool history lost %s: %s", marker, body)
							}
						}
						if family.index == 0 && index == 0 && !strings.Contains(string(body), `"reasoning_content":"retained-thinking"`) {
							t.Errorf("native DeepSeek reasoning was discarded: %s", body)
						}
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, up.response)
				}))
				defer upstream.Close()
				for turn := 0; turn < 2; turn++ {
					if turn == 1 {
						down.request = openCodeContinuationRequests[index]
						if index == 0 && family.index != 0 {
							down.request = strings.Replace(down.request, `"reasoning_content":"retained-thinking",`, "", 1)
						}
					}
					out, entry := runOpenCodeWireAttempt(t, upstream.URL+"/custom/exact", family.model, down, false)
					if out.Code != 200 || entry.Status != "success" || !strings.Contains(out.Body.String(), "call-fixture") {
						t.Fatalf("turn %d failed: status=%d log=%s body=%s", turn, out.Code, entry.Status, out.Body.String())
					}
				}
				if calls.Load() != 2 {
					t.Errorf("tool round caused extra sends: %d", calls.Load())
				}
			})
		}
	}
}

func TestDirectOpenCodeUnrepresentableContinuationRejected(t *testing.T) {
	for _, tc := range []struct {
		request, downstream, upstream, field string
	}{
		{openCodeContinuationRequests[3], directWireFixtures[3].path, "/v1/responses", `"name"`},
		{openCodeContinuationRequests[3], directWireFixtures[3].path, "/v1/messages", ".name"},
		{openCodeContinuationRequests[0], "/v1/chat/completions", "/v1/messages", ".reasoning_content"},
	} {
		t.Run(tc.downstream+"_to_"+tc.upstream, func(t *testing.T) {
			if !json.Valid([]byte(tc.request)) {
				t.Fatal("invalid native fixture")
			}
			_, err := directConvertRequest([]byte(tc.request), tc.downstream, tc.upstream, "upstream-model", false, messages.Options{})
			if err == nil || !strings.Contains(err.Error(), tc.field) || !strings.Contains(err.Error(), "not supported") && !strings.Contains(err.Error(), "unsupported") {
				t.Fatalf("unrepresentable history was not explicitly rejected: %v", err)
			}
		})
	}
}

func TestDirectOpenCodeConvertedTruncatedWireKeepsFailureAndUsage(t *testing.T) {
	for _, family := range []struct {
		model string
		index int
	}{
		{"deepseek-v4-flash", 0}, {"gpt-5.6-luna", 1}, {"minimax-m3", 2},
	} {
		for _, down := range directWireFixtures {
			// Native passthrough completion policy is owned by the shared stream
			// dispatcher. Here we verify the selected cross-protocol bridge.
			if down.provider == directWireFixtures[family.index].provider {
				continue
			}
			t.Run(family.model+"/"+down.provider, func(t *testing.T) {
				frames := directNativeSSE(directWireFixtures[family.index])
				frames = frames[:len(frames)-1]
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					for _, frame := range frames {
						_, _ = io.WriteString(w, frame)
					}
				}))
				defer upstream.Close()
				out, entry := runOpenCodeWireAttempt(t, upstream.URL+"/custom/exact", family.model, down, true)
				if entry.Status != "failed" || !strings.Contains(out.Body.String(), "upstream_error") && !strings.Contains(out.Body.String(), "api_error") {
					t.Fatalf("truncated wire became successful: log=%s body=%s", entry.Status, out.Body.String())
				}
				if family.index != 1 && (entry.TotalTokens == nil || *entry.TotalTokens != 7) {
					t.Errorf("truncated wire lost observed usage: %+v", entry)
				}
			})
		}
	}
}
