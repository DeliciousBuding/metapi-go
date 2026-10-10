package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/google/uuid"
)

func installPlatformHTTPRuntime(t *testing.T, dialect, provider, model string, endpoints store.DirectEndpoints, protocols int, overrides string) (*store.DB, chan proxy.ProxyLogEntry) {
	t.Helper()
	db := installDirectMediaRuntime(t, dialect, endpoints, protocols, overrides)
	if _, err := db.Exec(`UPDATE upstream_channels SET provider=?`, provider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE upstream_models SET name=?`, model); err != nil {
		t.Fatal(err)
	}
	routing.InvalidateCache()
	logs := make(chan proxy.ProxyLogEntry, 4)
	getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
	return db, logs
}

func callPlatformHTTP(t *testing.T, down directWireFixture, stream bool, headers http.Header) *httptest.ResponseRecorder {
	t.Helper()
	path, body := down.path, down.request
	if stream {
		if strings.Contains(path, ":generateContent") {
			path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
		} else {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal([]byte(body), &obj); err != nil {
				t.Fatal(err)
			}
			obj["stream"] = json.RawMessage("true")
			encoded, err := json.Marshal(obj)
			if err != nil {
				t.Fatal(err)
			}
			body = string(encoded)
		}
	}
	r := makeProxyReq(http.MethodPost, path, body)
	r.Header.Set("Authorization", "Bearer fixture-downstream-secret")
	r.Header.Set("X-Api-Key", "fixture-downstream-key")
	r.Header.Set("X-Goog-Api-Key", "fixture-downstream-google-key")
	for name, values := range headers {
		r.Header[name] = append([]string(nil), values...)
	}
	out := httptest.NewRecorder()
	directMediaRouter().ServeHTTP(out, r)
	return out
}

func platformUsage(t *testing.T, logs chan proxy.ProxyLogEntry, status string, input, output int64) proxy.ProxyLogEntry {
	t.Helper()
	select {
	case entry := <-logs:
		if entry.UpstreamChannelID == nil || *entry.UpstreamChannelID <= 0 || entry.UpstreamGrantID == nil || *entry.UpstreamGrantID <= 0 || entry.ChannelID != nil {
			t.Errorf("platform attempt lost direct grant/channel identity: %+v", entry)
		}
		if entry.Status != status || entry.PromptTokens == nil || *entry.PromptTokens != input || entry.CompletionTokens == nil || *entry.CompletionTokens != output || entry.TotalTokens == nil || *entry.TotalTokens != input+output {
			t.Errorf("actual platform status/usage changed: %+v", entry)
		}
		return entry
	default:
		t.Fatal("missing actual platform usage log")
		return proxy.ProxyLogEntry{}
	}
}

func assertPlatformAuth(t *testing.T, r *http.Request, messages bool) {
	t.Helper()
	if messages {
		if r.Header.Get("X-Api-Key") != "fixture-media-key" || r.Header.Get("Authorization") != "" || r.Header.Get("Anthropic-Version") == "" {
			t.Error("Messages wire did not use only the selected credential/version")
		}
	} else if r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("X-Api-Key") != "" {
		t.Error("wire did not use only the selected Bearer credential")
	}
	if r.Header.Get("X-Goog-Api-Key") != "" {
		t.Error("downstream credential leaked")
	}
}

func platformOpenCodeEndpoints(base string) store.DirectEndpoints {
	return store.DirectEndpoints{
		Chat:       &store.DirectEndpoint{URL: base + "/internal/chat", Auth: store.DirectAuthBearer, Profile: "opencode-go", ModelWireURLs: &store.DirectModelWireURLs{Responses: base + "/internal/responses", Messages: base + "/internal/messages"}},
		Responses:  &store.DirectEndpoint{URL: base + "/custom-must-not-be-used/responses", Auth: store.DirectAuthBearer},
		Messages:   &store.DirectEndpoint{URL: base + "/custom-must-not-be-used/messages", Auth: store.DirectAuthAPIKey},
		Embeddings: &store.DirectEndpoint{URL: base + "/custom-must-not-be-used/embeddings", Auth: store.DirectAuthBearer},
	}
}

func TestDirectPlatformOpenCodeLogicalGrantHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, tc := range []struct {
			model    string
			up, down int
			stream   bool
		}{
			{"deepseek-v4-flash", 0, 0, false}, {"deepseek-v4-flash", 0, 2, true},
			{"gpt-wire", 1, 0, false}, {"gpt-wire", 1, 1, true}, {"gpt-wire", 1, 2, true}, {"gpt-wire", 1, 3, true},
			{"minimax-wire", 2, 0, true}, {"minimax-wire", 2, 1, false}, {"minimax-wire", 2, 2, false}, {"minimax-wire", 2, 3, false},
			{"grok-wire", 1, 0, false}, {"qwen3-wire", 2, 0, true}, {"GPT-wire", 0, 0, false}, {"glm-wire", 0, 1, true},
		} {
			t.Run(fmt.Sprintf("%s/%s/to_%d/stream_%t", dialect, tc.model, tc.down, tc.stream), func(t *testing.T) {
				up, down := directWireFixtures[tc.up], directWireFixtures[tc.down]
				wantPath := []string{"/internal/chat", "/internal/responses", "/internal/messages"}[tc.up]
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, _ := io.ReadAll(r.Body)
					var obj map[string]json.RawMessage
					_ = json.Unmarshal(body, &obj)
					if r.URL.Path != wantPath || string(obj["model"]) != fmt.Sprintf("%q", tc.model) {
						t.Errorf("logical grant chose wrong mapped model/wire: %s %s", r.URL.Path, body)
					}
					if (tc.up == 1) != (obj["input"] != nil) || tc.up != 1 && obj["messages"] == nil {
						t.Errorf("body protocol disagrees with selected wire: %s", body)
					}
					if strings.HasPrefix(tc.model, "deepseek") && !bytes.Contains(body, []byte(`"thinking":{"type":"enabled"}`)) {
						t.Errorf("DeepSeek body profile missing: %s", body)
					}
					if r.Header.Get("X-Opencode-Session") != "client-current" {
						t.Error("OpenCode session precedence lost")
					}
					assertPlatformAuth(t, r, tc.up == 2)
					if tc.stream {
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range directNativeSSE(up) {
							_, _ = io.WriteString(w, frame)
						}
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, up.response)
					}
				}))
				defer server.Close()
				db, logs := installPlatformHTTPRuntime(t, dialect, "opencode_go", tc.model, platformOpenCodeEndpoints(server.URL), store.DirectProtocolChat, "{}")
				headers := http.Header{"X-Opencode-Session": {"client-current"}, "X-Session-Id": {"client-id"}, "X-Session-Affinity": {"client-legacy"}}
				out := callPlatformHTTP(t, down, tc.stream, headers)
				if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "receipt-tool") || !strings.Contains(out.Body.String(), down.toolMarker) {
					t.Fatalf("real OpenCode dispatch failed: status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
				}
				platformUsage(t, logs, "success", 5, 2)
				var granted int
				if err := db.Get(&granted, `SELECT protocols FROM upstream_grants`); err != nil || granted != store.DirectProtocolChat {
					t.Fatalf("execution broadened logical grant: %d %v", granted, err)
				}
				denied := callPlatformHTTP(t, directWireFixture{path: "/v1/embeddings", request: `{"model":"client-alias","input":"must-not-run"}`}, false, nil)
				if denied.Code == 200 || calls.Load() != 1 {
					t.Fatalf("logical Chat grant enabled unrelated media: status=%d calls=%d", denied.Code, calls.Load())
				}
			})
		}
	}
}

func TestDirectPlatformOpenCodeCustomFormatsAndHeaderOverrideHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, tc := range []struct {
			provider, model string
			index, bit      int
		}{
			{"opencode_go", "gpt-force-custom-chat", 0, store.DirectProtocolChat},
			{"opencode_go", "minimax-force-custom-responses", 1, store.DirectProtocolResponses},
			{"opencode_go_anthropic", "gpt-force-custom-messages", 2, store.DirectProtocolMessages},
			{"opencode_go", "gpt-force-custom-media", 4, store.DirectProtocolEmbeddings},
		} {
			t.Run(fmt.Sprintf("%s/%s/%d", dialect, tc.provider, tc.index), func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					body, _ := io.ReadAll(r.Body)
					if r.URL.Path != "/exact/custom" || r.URL.RawQuery != "" || r.Header.Get("X-Opencode-Session") != "channel-session" || !strings.Contains(string(body), tc.model) {
						t.Errorf("custom format/path/session changed: %s %s", r.URL, body)
					}
					assertPlatformAuth(t, r, tc.index == 2)
					w.Header().Set("Content-Type", "application/json")
					if tc.index == 4 {
						_, _ = io.WriteString(w, `{"object":"list","data":[{"embedding":[0.25]}],"usage":{"prompt_tokens":3,"completion_tokens":0,"total_tokens":3}}`)
					} else {
						_, _ = io.WriteString(w, directWireFixtures[tc.index].response)
					}
				}))
				defer server.Close()
				endpoint := &store.DirectEndpoint{URL: server.URL + "/exact/custom", Auth: store.DirectAuthBearer}
				var endpoints store.DirectEndpoints
				var down directWireFixture
				switch tc.index {
				case 0:
					endpoints.Chat = endpoint
					down = directWireFixtures[0]
				case 1:
					endpoints.Responses = endpoint
					down = directWireFixtures[1]
				case 2:
					endpoint.Auth = store.DirectAuthAPIKey
					endpoints.Messages = endpoint
					down = directWireFixtures[2]
				case 4:
					endpoints.Embeddings = endpoint
					down = directWireFixture{path: "/v1/embeddings", request: `{"model":"client-alias","input":"embed this"}`}
				}
				db, logs := installPlatformHTTPRuntime(t, dialect, tc.provider, tc.model, endpoints, tc.bit, "{}")
				if _, err := db.Exec(`UPDATE upstream_channels SET custom_header=?`, `[{"header_key":"x-opencode-session","header_value":"channel-session"},{"header_key":"Authorization","header_value":"Bearer unselected"},{"header_key":"X-Api-Key","header_value":"unselected"}]`); err != nil {
					t.Fatal(err)
				}
				routing.InvalidateCache()
				out := callPlatformHTTP(t, down, false, http.Header{"X-Opencode-Session": {"client-session"}})
				if out.Code != 200 || calls.Load() != 1 {
					t.Fatalf("custom dispatch status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
				}
				if tc.index == 4 {
					platformUsage(t, logs, "success", 3, 0)
				} else {
					platformUsage(t, logs, "success", 5, 2)
				}
			})
		}
	}
}

func TestDirectPlatformOpenCodeRetrySessionHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			sessions := make(chan string, 2)
			keys := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sessions <- r.Header.Get("X-Opencode-Session")
				keys <- r.Header.Get("Authorization")
				w.Header().Set("Content-Type", "application/json")
				if calls.Add(1) == 1 {
					w.WriteHeader(503)
					_, _ = io.WriteString(w, `{"error":{"message":"fixture unavailable"}}`)
					return
				}
				_, _ = io.WriteString(w, directWireFixtures[1].response)
			}))
			defer server.Close()
			db, logs := installPlatformHTTPRuntime(t, dialect, "opencode_go", "gpt-retry", platformOpenCodeEndpoints(server.URL), store.DirectProtocolChat, "{}")
			for _, sql := range []string{
				`INSERT INTO upstream_credentials(origin_key,channel_id,source_id,name,secret,enabled,kind) SELECT origin_key,channel_id,source_id+100,'retry-credential','fixture-retry-key',enabled,kind FROM upstream_credentials`,
				`INSERT INTO upstream_grants(origin_key,source_id,model_id,credential_id,protocols,enabled) SELECT origin_key,source_id+100,model_id,(SELECT MAX(id) FROM upstream_credentials),protocols,enabled FROM upstream_grants`,
				`INSERT INTO upstream_group_items(origin_key,group_id,source_id,grant_id,priority,weight,protocol_order) SELECT origin_key,group_id,source_id+100,(SELECT MAX(id) FROM upstream_grants),priority+1,weight,protocol_order FROM upstream_group_items`,
			} {
				if _, err := db.Exec(sql); err != nil {
					t.Fatal(err)
				}
			}
			old := config.GetSafe()
			config.Set(&config.Config{ProxyMaxChannelAttempts: 2})
			t.Cleanup(func() { config.Set(old) })
			routing.InvalidateCache()
			out := callPlatformHTTP(t, directWireFixtures[0], false, nil)
			if out.Code != 200 || calls.Load() != 2 {
				t.Fatalf("retry did not run two real grants: status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
			first, second := <-sessions, <-sessions
			if _, err := uuid.Parse(first); err != nil || first != second {
				t.Fatalf("retry session changed: first=%q second=%q parse=%v", first, second, err)
			}
			key1, key2 := <-keys, <-keys
			if key1 == key2 || (key1 != "Bearer fixture-media-key" && key1 != "Bearer fixture-retry-key") || (key2 != "Bearer fixture-media-key" && key2 != "Bearer fixture-retry-key") {
				t.Fatal("retry did not use each selected credential")
			}
			failed := <-logs
			if failed.Status != "failed" || failed.HTTPStatus != 503 {
				t.Errorf("retry failure was not logged: %+v", failed)
			}
			platformUsage(t, logs, "success", 5, 2)
		})
	}
}

func platformBailianFrames() []string {
	frames := [][]byte{
		bailianTestTool(0, `{"n":42}`, "call-platform"),
		bailianTestChunk(`[{"index":0,"delta":{"content":"tool explanation"}}]`),
		bailianTestTool(0, "{}", ""),
		bailianTestChunk(`[{"index":0,"delta":{},"finish_reason":"tool_calls"}]`),
		bailianTestFrame(`{"id":"bailian-id","object":"chat.completion.chunk","model":"qwen-test","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":23,"total_tokens":34}}`),
		bailianTestFrame(`[DONE]`),
	}
	out := make([]string, len(frames))
	for i, frame := range frames {
		out[i] = string(frame)
	}
	return out
}

func TestDirectPlatformBailianAndClineHTTPMatrix(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, provider := range []string{"bailian", "cline"} {
			for index, fixture := range directWireFixtures {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/to_%s/stream_%t", dialect, provider, fixture.provider, stream), func(t *testing.T) {
						down := fixture
						if index == 2 && provider == "cline" {
							// Tool responses with hidden reasoning need a stable native
							// conversation identity for the existing replay callbacks.
							// This exercises those production callbacks rather than
							// stripping reasoning or mocking SaveReasoning in the test.
							down.request = strings.TrimSuffix(down.request, "}") + `,"metadata":{"user_id":"cline-platform-conversation"}}`
						}
						if index == 0 && provider == "bailian" {
							down.request = `{"model":"client-alias","messages":[{"role":"user","content":"receipt-history"},{"role":"assistant","tool_calls":[{"id":"one","type":"function","function":{"name":"echo","arguments":"{}"}}]},{"role":"assistant","tool_calls":[{"id":"two","type":"function","function":{"name":"echo","arguments":"{}"}}]},{"role":"tool","tool_call_id":"one","content":"one"},{"role":"tool","tool_call_id":"two","content":"two"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}]} `
						} else if index == 0 {
							down.request = `{"model":"client-alias","messages":[{"role":"user","content":""},{"role":"assistant","content":null,"reasoning_content":"retained-reason","tool_calls":[{"id":"before","type":"function","function":{"name":"echo","arguments":"{}"}}]},{"role":"tool","tool_call_id":"before","content":"receipt-history"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}]} `
						}
						var calls atomic.Int32
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls.Add(1)
							body, _ := io.ReadAll(r.Body)
							if r.URL.Path != "/exact/platform/chat" || !bytes.Contains(body, []byte(`"model":"provider-model"`)) {
								t.Errorf("bad platform target/model: %s %s", r.URL, body)
							}
							assertPlatformAuth(t, r, false)
							if provider == "bailian" {
								if !bytes.Contains(body, []byte(`"enable_thinking":false`)) || bytes.Contains(body, []byte(`"reasoning_effort"`)) {
									t.Errorf("Bailian request profile missing: %s", body)
								}
								if index == 0 && bytes.Count(body, []byte(`"role":"assistant"`)) != 1 {
									t.Errorf("Bailian consecutive tool calls not merged: %s", body)
								}
							} else {
								if r.Header.Get("X-Client-Type") != "cline-cli" {
									t.Error("Cline identity header missing")
								}
								if index == 0 && (!bytes.Contains(body, []byte(`"content":[{"type":"text","text":""}]`)) || !bytes.Contains(body, []byte(`"reasoning":"retained-reason"`)) || bytes.Contains(body, []byte(`"reasoning_content"`))) {
									t.Errorf("Cline request was not normalized: %s", body)
								}
							}
							w.Header().Set("ETag", "wrapped-provider-body")
							if stream {
								w.Header().Set("Content-Type", "text/event-stream")
								if provider == "bailian" {
									for _, frame := range platformBailianFrames() {
										_, _ = io.WriteString(w, frame)
									}
								} else {
									_, _ = io.WriteString(w, clineTestStream())
								}
							} else {
								w.Header().Set("Content-Type", "application/json")
								if provider == "bailian" {
									_, _ = io.WriteString(w, directWireFixtures[0].response)
								} else {
									_, _ = io.WriteString(w, `{"success":true,"data":`+clineTestJSON+`}`)
								}
							}
						}))
						defer server.Close()
						overrides := "{}"
						if provider == "bailian" {
							overrides = `{"reasoning_effort":"none"}`
						}
						_, logs := installPlatformHTTPRuntime(t, dialect, provider, "provider-model", store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: server.URL + "/exact/platform/chat", Auth: store.DirectAuthBearer, Profile: provider}}, store.DirectProtocolChat, overrides)
						out := callPlatformHTTP(t, down, stream, nil)
						if out.Code != 200 || calls.Load() != 1 || strings.Contains(out.Body.String(), `"type":"error"`) {
							t.Fatalf("platform relay failed: status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
						}
						if provider == "bailian" && stream {
							if !strings.Contains(out.Body.String(), "namespace__lookup") || !strings.Contains(out.Body.String(), "tool explanation") || strings.Contains(out.Body.String(), `"arguments":"{}"`) {
								t.Fatalf("Bailian filter not executed: %s", out.Body.String())
							}
							platformUsage(t, logs, "success", 11, 23)
						} else if provider == "cline" {
							if strings.Contains(out.Body.String(), `"success":true`) || !strings.Contains(out.Body.String(), "lookup") || !strings.Contains(out.Body.String(), "9007199254740993") {
								t.Fatalf("Cline envelope/semantic conversion failed: %s", out.Body.String())
							}
							platformUsage(t, logs, "success", 12, 7)
						} else {
							platformUsage(t, logs, "success", 5, 2)
						}
					})
				}
			}
		}
	}
}

func TestDirectPlatformStreamFailureKeepsUsageHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, provider := range []string{"bailian", "cline", "opencode_go"} {
			for _, failure := range []string{"truncated", "upstream-error", "idle"} {
				t.Run(dialect+"/"+provider+"/"+failure, func(t *testing.T) {
					frames := platformBailianFrames()
					prompt, completion := int64(11), int64(23)
					if provider == "cline" {
						frames = []string{strings.TrimSuffix(clineTestStream(), "data: [DONE]\n\n"), "data: [DONE]\n\n"}
						prompt, completion = 12, 7
					}
					if provider == "opencode_go" {
						frames = directNativeSSE(directWireFixtures[1])
						prompt, completion = 5, 2
						frames[0] = strings.Replace(frames[0], `"output":[]`, `"output":[],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}`, 1)
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range frames[:len(frames)-1] {
							_, _ = io.WriteString(w, frame)
							w.(http.Flusher).Flush()
						}
						switch failure {
						case "upstream-error":
							_, _ = io.WriteString(w, "event: error\ndata: {\"error\":{\"message\":\"fixture failed after usage\"}}\n\n")
						case "idle":
							<-r.Context().Done()
						}
					}))
					defer server.Close()
					endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: server.URL + "/platform", Auth: store.DirectAuthBearer, Profile: provider}}
					model := "provider-model"
					if provider == "opencode_go" {
						endpoints = platformOpenCodeEndpoints(server.URL)
						model = "gpt-failure"
					}
					_, logs := installPlatformHTTPRuntime(t, dialect, provider, model, endpoints, store.DirectProtocolChat, "{}")
					old := config.GetSafe()
					config.Set(&config.Config{ProxyMaxChannelAttempts: 1, ProxyStreamIdleTimeoutSec: 1})
					t.Cleanup(func() { config.Set(old) })
					out := callPlatformHTTP(t, directWireFixtures[0], true, nil)
					entry := platformUsage(t, logs, "failed", prompt, completion)
					if !strings.Contains(out.Body.String(), "upstream_error") || entry.HTTPStatus == 200 {
						t.Errorf("failed stream looked successful: log=%+v body=%s", entry, out.Body.String())
					}
					if failure == "idle" && (entry.HTTPStatus != 408 || !strings.Contains(out.Body.String(), "idle timeout")) {
						t.Errorf("idle failure was misclassified: log=%+v body=%s", entry, out.Body.String())
					}
				})
			}
		}
	}
}
