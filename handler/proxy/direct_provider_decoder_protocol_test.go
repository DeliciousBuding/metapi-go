package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderDecoderNativeJSONProtocolMatrix(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, protocol := range []string{"responses", "messages", "gemini", "responsesCompact"} {
				t.Run(protocol, func(t *testing.T) {
					native, path, request := `{"id":"decoder-response","object":"response","status":"completed","model":"provider-model","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"vendor receipt"}]}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9},"extension":9007199254740993}`, "/v1/responses", `{"model":"client-alias","input":"hello"}`
					if protocol == "messages" {
						native, path, request = `{"id":"decoder-message","type":"message","role":"assistant","model":"provider-model","content":[{"type":"text","text":"vendor receipt"}],"stop_reason":"end_turn","usage":{"input_tokens":7,"output_tokens":2},"extension":9007199254740993}`, "/v1/messages", `{"model":"client-alias","max_tokens":16,"messages":[{"role":"user","content":"hello"}]}`
					} else if protocol == "gemini" {
						native, path, request = `{"modelVersion":"provider-model","candidates":[{"content":{"role":"model","parts":[{"text":"vendor receipt"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":2,"totalTokenCount":9},"extension":9007199254740993}`, "/v1beta/models/client-alias:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`
					} else if protocol == "responsesCompact" {
						native, path, request = `{"id":"decoder-compact","object":"response.compaction","output":[{"type":"compaction","encrypted_content":"opaque-vendor-receipt"}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9},"extension":9007199254740993}`, "/v1/responses/compact", `{"model":"client-alias","input":[{"role":"user","content":"hello"}]}`
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]any{"vendor": map[string]any{"native": native, "meter": map[string]int{"in": 7, "out": 2, "total": 9}, "model": "provider-model"}})
					}))
					defer upstream.Close()
					var endpoints store.DirectEndpoints
					ep := &store.DirectEndpoint{URL: upstream.URL + "/generate", Auth: store.DirectAuthBearer}
					bit := store.DirectProtocolResponses
					switch protocol {
					case "responses":
						endpoints.Responses = ep
					case "messages":
						endpoints.Messages, bit = ep, store.DirectProtocolMessages
					case "gemini":
						endpoints.Gemini, bit = ep, store.DirectProtocolGemini
					case "responsesCompact":
						endpoints.ResponsesCompact, bit = ep, store.DirectProtocolResponsesCompact
					}
					db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", endpoints, bit, "")
					source := strings.ReplaceAll(providerDecoderSource, `"chat"`, fmt.Sprintf("%q", protocol))
					source = strings.ReplaceAll(source, `observations:{chat:`, "observations:{"+protocol+":")
					if _, err := channelplugins.Save(t.Context(), db.DB, "decoder", source, true, 0); err != nil {
						t.Fatal(err)
					}
					videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "decoder", Protocols: []string{protocol}}})
					getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
					w := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(w, makeProxyReq("POST", path, request))
					if w.Code != 200 || !strings.Contains(w.Body.String(), "9007199254740993") {
						t.Fatalf("native decoder failed: %s %d %s", protocol, w.Code, w.Body.String())
					}
					if protocol != "responsesCompact" && !strings.Contains(w.Body.String(), "vendor receipt") {
						t.Fatal("decoded content was lost")
					}
					entry := platformUsage(t, logs, "success", 7, 2)
					if entry.UpstreamReportedModel == nil || *entry.UpstreamReportedModel != "provider-model" {
						t.Fatalf("observed model was lost: %+v", entry)
					}
				})
			}
		})
	}
}
