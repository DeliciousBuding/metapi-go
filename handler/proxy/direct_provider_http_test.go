package proxyhandler

import (
	"context"
	"encoding/json"
	"github.com/deliciousbuding/metapi-go/proxy"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDirectCodexWireAggregatesForJSONClients(t *testing.T) {
	for _, provider := range []string{"codex", "fenno"} {
		for _, complete := range []bool{true, false} {
			t.Run(provider+map[bool]string{true: "_complete", false: "_truncated"}[complete], func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var body map[string]json.RawMessage
					if json.NewDecoder(r.Body).Decode(&body) != nil {
						t.Error("invalid provider body")
					}
					if string(body["stream"]) != "true" || string(body["store"]) != "false" {
						t.Error("missing Codex stream/store contract")
					}
					if body["max_output_tokens"] != nil || body["metadata"] != nil || body["user"] != nil {
						t.Error("Codex unsupported fields retained")
					}
					if !strings.Contains(string(body["input"]), "encrypted-fixture") || !strings.Contains(string(body["input"]), "call-before") {
						t.Errorf("native reasoning/tool continuation lost: %s", body["input"])
					}
					if r.Header.Get("Authorization") != "Bearer fixture-upstream-key" || r.Header.Get("Session-Id") == "" || r.Header.Get("Accept") != "text/event-stream" {
						t.Error("Codex provider identity not applied")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					frames := directNativeSSE(directWireFixtures[1])
					if !complete {
						frames = frames[:len(frames)-1]
					}
					for _, frame := range frames {
						_, _ = io.WriteString(w, frame)
					}
				}))
				defer upstream.Close()
				installAxonHubEndpointFixture(t, provider, upstream.URL, nil)
				logs := make(chan proxy.ProxyLogEntry, 1)
				getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
				request := `{"model":"client-alias","input":[{"type":"reasoning","encrypted_content":"encrypted-fixture","summary":[]},{"type":"function_call","call_id":"call-before","name":"echo","arguments":"{}"},{"type":"function_call_output","call_id":"call-before","output":"receipt"}],"max_output_tokens":123,"metadata":{"fixture":true},"user":"fixture"}`
				out := httptest.NewRecorder()
				HandleResponses(out, makeProxyReq("POST", "/v1/responses", request), "")
				if complete {
					if out.Code != 200 || !strings.Contains(out.Header().Get("Content-Type"), "application/json") || !json.Valid(out.Body.Bytes()) || !strings.Contains(out.Body.String(), "receipt-tool") {
						t.Fatalf("JSON client did not receive terminal response: %d %s", out.Code, out.Body.String())
					}
				} else if out.Code != 502 || strings.Contains(out.Body.String(), "response.output_text.delta") {
					t.Fatalf("truncated SSE leaked or succeeded: %d %s", out.Code, out.Body.String())
				}
				select {
				case entry := <-logs:
					if complete && (entry.TotalTokens == nil || *entry.TotalTokens != 7 || entry.Status != "success") {
						t.Fatalf("aggregate lost actual usage: %+v", entry)
					}
					if !complete && entry.Status != "failed" {
						t.Fatalf("aggregate failure logged as %s", entry.Status)
					}
				default:
					t.Fatal("aggregate produced no proxy log")
				}
			})
		}
	}
}

func TestDirectCodexStreamingRequiresRealTerminal(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		frames := directNativeSSE(directWireFixtures[1])
		for _, frame := range frames[:len(frames)-1] {
			_, _ = io.WriteString(w, frame)
		}
	}))
	defer upstream.Close()
	installAxonHubEndpointFixture(t, "codex", upstream.URL, nil)
	out := httptest.NewRecorder()
	HandleResponses(out, makeProxyReq("POST", "/v1/responses", `{"model":"client-alias","input":"hi","stream":true}`), "")
	if !strings.Contains(out.Body.String(), "upstream_error") || strings.Contains(out.Body.String(), `"type":"response.completed"`) {
		t.Fatalf("Codex incomplete stream misreported: %s", out.Body.String())
	}
}
