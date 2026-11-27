package proxyhandler_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/handler/admin"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestAttemptTelemetryDetailHTTP(t *testing.T) {
	const text = `private-content-probe`
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, protocol := range []string{"chat", "responses", "messages", "gemini", "interrupted", "diagnostic"} {
			t.Run(dialect+"/"+protocol, func(t *testing.T) {
				var endpoints store.DirectEndpoints
				ep := &store.DirectEndpoint{Auth: store.DirectAuthBearer}
				bit := store.DirectProtocolChat
				path, body := "/v1/responses", `{"model":"client-alias","stream":true,"input":"hello"}`
				var frames []string
				switch protocol {
				case "chat", "interrupted":
					endpoints.Chat = ep
					frames = []string{`{"id":"c","choices":[{"index":0,"delta":{"content":"` + text + `"}}]}`}
					if protocol == "chat" {
						frames = append(frames, `{"id":"c","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_tokens_details":{"cache_write_tokens":4,"cache_write_tokens_details":{"ttl_12h_tokens":3}}}}`, "[DONE]")
					}
				case "responses":
					endpoints.Responses, bit = ep, store.DirectProtocolResponses
					frames = []string{`{"type":"response.output_item.added","item":{"type":"reasoning"}}`, `{"type":"response.output_text.delta","delta":"` + text + `"}`, `{"type":"response.output_text.delta","delta":"` + text + `"}`, `{"type":"response.output_item.done","output_index":1,"item":{"id":"private-tool-id","type":"file_search_call","queries":["private-query-probe"]}}`, `{"type":"response.completed","response":{"id":"r","status":"completed","usage":{"input_tokens":8,"output_tokens":2}}}`}
				case "messages":
					endpoints.Messages, bit = ep, store.DirectProtocolMessages
					path, body = "/v1/messages", `{"model":"client-alias","max_tokens":20,"stream":true,"messages":[{"role":"user","content":"hello"}]}`
					frames = []string{`{"type":"message_start","message":{"id":"m","role":"assistant","usage":{"input_tokens":2,"cache_creation_input_tokens":5,"cache_creation":{"ephemeral_5m_input_tokens":3,"ephemeral_1h_input_tokens":2}}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + text + `"}}`, `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2,"server_tool_use":{"web_search_requests":1}}}`, `{"type":"message_stop"}`}
				case "gemini", "diagnostic":
					endpoints.Gemini, bit = ep, store.DirectProtocolGemini
					path, body = "/v1beta/models/client-alias:streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`
					frames = []string{`{"candidates":[{"content":{"parts":[{"text":"` + text + `"}]},"finishReason":"STOP","groundingMetadata":{"webSearchQueries":["private-query-probe"]}}],"usageMetadata":{"promptTokenCount":10,"cachedContentTokenCount":4,"candidatesTokenCount":2}}`}
				}
				if protocol == "diagnostic" {
					path = "/v1/chat/completions"
					body = `{"model":"client-alias","stream":true,"messages":[{"role":"user","content":"hello"}]}`
				}
				db, rec := proxyhandler.RunTelemetryFixture(t, dialect, endpoints, ep, bit, path, body, frames)
				var id int64
				var gatewayID string
				if err := db.QueryRow(`SELECT id,gateway_request_id FROM proxy_logs`).Scan(&id, &gatewayID); err != nil {
					t.Fatalf("%v, HTTP %d %s", err, rec.Code, rec.Body.String())
				}
				api := chi.NewRouter()
				admin.RegisterStatsRoutes(api, db.DB)
				for _, detailPath := range []string{fmt.Sprintf("/api/stats/proxy-logs/%d", id), "/api/stats/requests/" + gatewayID} {
					out := httptest.NewRecorder()
					api.ServeHTTP(out, httptest.NewRequest("GET", detailPath, nil))
					var row map[string]any
					if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &row) != nil {
						t.Fatal(out.Code, out.Body.String())
					}
					if attempts, ok := row["attempts"].([]any); ok {
						if len(attempts) != 1 {
							t.Fatal(row)
						}
						row = attempts[0].(map[string]any)
					}
					timing, ok := row["streamTiming"].(map[string]any)
					if !ok || timing["firstTextMs"] == nil || timing["headerMs"] == nil {
						t.Fatalf("missing upstream timing: %+v", row)
					}
					for _, key := range []string{"firstReasoningMs", "decodeTps", "maxContentGapMs"} {
						if _, ok := timing[key]; !ok {
							t.Fatal(key, timing)
						}
					}
					if protocol == "interrupted" || protocol == "diagnostic" {
						if timing["decodeStatus"] != "interrupted" || row["status"] != "failed" {
							t.Fatal(row)
						}
					} else if row["status"] != "success" {
						t.Fatal(row)
					}
					if protocol == "chat" || protocol == "interrupted" {
						conv, ok := row["conversion"].(map[string]any)
						if !ok || conv["owner"] != "relaykit" || conv["sourceProtocol"] != "responses" || conv["upstreamProtocol"] != "chat" || conv["quality"] == nil || len(conv["requestSteps"].([]any)) == 0 || len(conv["responseSteps"].([]any)) == 0 {
							t.Fatal(row)
						}
					}
					if protocol == "diagnostic" {
						conv := row["conversion"].(map[string]any)
						ds := conv["diagnostics"].([]any)
						if len(ds) == 0 {
							t.Fatal(conv)
						}
						for _, value := range ds {
							d := value.(map[string]any)
							if len(d) != 4 || d["phase"] != "stream" || d["severity"] == nil || d["code"] == nil || d["fieldClass"] == nil {
								t.Fatal(d)
							}
						}
					} else if protocol != "chat" && protocol != "interrupted" && row["conversion"] != nil {
						t.Fatal(row["conversion"])
					}
					if protocol == "responses" && (timing["decodeStatus"] != "measured" || timing["decodeTps"] == nil || timing["firstReasoningMs"] == nil || timing["maxContentGapMs"] == nil) {
						t.Fatal(timing)
					}
					billing := row["billingDetails"]
					if protocol != "interrupted" {
						details := billing.(map[string]any)
						usage := details["usage"].(map[string]any)
						switch protocol {
						case "chat":
							if usage["cacheCreation12hTokens"] != float64(3) {
								t.Fatal(details)
							}
						case "messages":
							if usage["cacheCreation1hTokens"] != float64(2) || usage["cacheCreation5mTokens"] != float64(3) {
								t.Fatal(details)
							}
						case "gemini":
							if usage["cacheReadTokens"] != float64(4) {
								t.Fatal(details)
							}
						}
						if protocol != "chat" {
							charges := details["breakdown"].(map[string]any)["toolSurcharges"].([]any)
							want := map[string]string{"responses": "file_search", "diagnostic": "grounding", "gemini": "grounding", "messages": "web_search"}[protocol]
							if len(charges) == 0 || charges[0].(map[string]any)["tool"] != want || charges[0].(map[string]any)["quantity"] != float64(1) {
								t.Fatal(details)
							}
						}
					}
					for _, probe := range []string{text, "private-query-probe", "private-tool-id", "conversionDetails"} {
						if strings.Contains(out.Body.String(), probe) {
							t.Fatalf("leaked %s", probe)
						}
					}
				}
			})
		}
	}
}
