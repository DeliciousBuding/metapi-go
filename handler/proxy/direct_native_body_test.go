package proxyhandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectResponsesPreservesNativeReasoningContinuation(t *testing.T) {
	var observed map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &observed); err != nil {
			t.Error(err)
		}
		if r.Header.Get("Authorization") != "Bearer fixture-direct" {
			t.Error("wrong selected credential")
		}
		input := observed["input"].([]any)
		reasoning := input[1].(map[string]any)
		if _, invented := reasoning["content"]; invented {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"message":"native reasoning rejects injected content"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-fixture","model":"provider-model","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"receipt-fixture"}]}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`))
	}))
	defer upstream.Close()
	SetUpstreamConfig(&UpstreamConfig{
		Router: &upstreamTestRouter{selected: routing.SelectedChannel{
			Channel:    store.RouteChannel{ID: -1, Enabled: true},
			TokenValue: "fixture-direct", ActualModel: "provider-model",
			Direct: &store.DirectUpstreamCandidate{BaseURL: upstream.URL, ResponsesPath: "/v1/responses", Protocols: 4},
		}},
		LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
	})
	t.Cleanup(func() { SetUpstreamConfig(nil) })
	raw := `{"model":"client-model","previous_response_id":"resp-before","store":false,"input":[{"role":"user","content":"continue"},{"type":"reasoning","id":"reasoning-before","summary":[{"type":"summary_text","text":"retained summary"}],"encrypted_content":"opaque-fixture"},{"type":"function_call","call_id":"call-before","name":"echo","arguments":"{}"},{"type":"function_call_output","call_id":"call-before","output":"receipt-fixture"}]}`
	out := httptest.NewRecorder()
	HandleResponses(out, makeProxyReq("POST", "/v1/responses", raw), "")
	if out.Code != 200 {
		t.Fatalf("native continuation failed: %d %s", out.Code, out.Body.String())
	}
	var expected map[string]any
	_ = json.Unmarshal([]byte(raw), &expected)
	expected["model"] = "provider-model"
	want, _ := json.Marshal(expected)
	got, _ := json.Marshal(observed)
	if string(got) != string(want) {
		t.Fatalf("native request changed beyond model mapping:\ngot %s\nwant %s", got, want)
	}
}
