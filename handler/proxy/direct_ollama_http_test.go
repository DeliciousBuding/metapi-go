package proxyhandler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func installOllamaHTTPRuntime(t *testing.T, dialect, upstream string, noAuth bool) *store.DB {
	t.Helper()
	authMode := store.DirectAuthBearer
	if noAuth {
		authMode = store.DirectAuthNone
	}
	db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{Ollama: &store.DirectEndpoint{URL: upstream + "/custom/chat", Auth: authMode, Profile: "ollama"}}, store.DirectProtocolOllama, "{}")
	if _, err := db.Exec(`UPDATE upstream_channels SET provider='ollama'`); err != nil {
		t.Fatal(err)
	}
	if noAuth {
		if _, err := db.Exec(`UPDATE upstream_credentials SET kind='none',secret=''`); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

// Each case traverses the stored channel/grant graph, selector, dispatcher,
// native Ollama transport and downstream protocol bridge over real HTTP.
func TestDirectOllamaHTTPProtocolMatrix(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, down := range directWireFixtures {
			for _, stream := range []bool{false, true} {
				for _, noAuth := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/stream_%t/none_%t", dialect, down.provider, stream, noAuth), func(t *testing.T) {
						var calls atomic.Int32
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							calls.Add(1)
							var body map[string]json.RawMessage
							if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
								t.Error(err)
								return
							}
							if r.URL.Path != "/custom/chat" || string(body["model"]) != `"provider-model"` || string(body["stream"]) != fmt.Sprint(stream) {
								t.Errorf("wrong native route/model/stream: %s %s", r.URL.Path, body)
							}
							if !strings.Contains(string(body["messages"]), "receipt-history") || !strings.Contains(string(body["tools"]), "echo") {
								t.Errorf("lost input semantics: %s", body)
							}
							wantAuth := "Bearer fixture-media-key"
							if noAuth {
								wantAuth = ""
							}
							if r.Header.Get("Authorization") != wantAuth {
								t.Errorf("wrong native auth: expected none=%t", noAuth)
							}
							for _, key := range []string{"X-API-Key", "X-Goog-API-Key"} {
								if r.Header.Get(key) != "" {
									t.Errorf("leaked %s", key)
								}
							}
							if stream {
								w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
								for _, frame := range []string{
									`{"model":"provider-model","done":false,"message":{"content":"receipt-text"}}`,
									`{"model":"provider-model","done":false,"message":{"tool_calls":[{"function":{"name":"echo","arguments":{"value":"receipt-tool"}}}]}}`,
									`{"model":"provider-model","done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":2,"total_duration":9007199254740993}`,
								} {
									_, _ = io.WriteString(w, frame+"\n")
									w.(http.Flusher).Flush()
								}
							} else {
								w.Header().Set("Content-Type", "application/json")
								_, _ = io.WriteString(w, `{"model":"provider-model","done":true,"done_reason":"stop","message":{"role":"assistant","content":"receipt-text","tool_calls":[{"function":{"name":"echo","arguments":{"value":"receipt-tool"}}}]},"prompt_eval_count":5,"eval_count":2,"total_duration":9007199254740993}`)
							}
						}))
						defer upstream.Close()
						installOllamaHTTPRuntime(t, dialect, upstream.URL, noAuth)
						logs := make(chan proxy.ProxyLogEntry, 2)
						getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
						downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
							down.handler(w, r)
						}))
						defer downstream.Close()
						request, path := down.request, down.path
						if stream {
							if down.provider == "gemini" {
								path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
							} else {
								var obj map[string]json.RawMessage
								if err := json.Unmarshal([]byte(request), &obj); err != nil {
									t.Fatal(err)
								}
								obj["stream"] = json.RawMessage("true")
								raw, _ := json.Marshal(obj)
								request = string(raw)
							}
						}
						req, err := http.NewRequest(http.MethodPost, downstream.URL+path, strings.NewReader(request))
						if err != nil {
							t.Fatal(err)
						}
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("Authorization", "Bearer fixture-client-secret")
						req.Header.Set("X-API-Key", "fixture-client-key")
						req.Header.Set("X-Goog-API-Key", "fixture-client-google-key")
						resp, err := http.DefaultClient.Do(req)
						if err != nil {
							t.Fatal(err)
						}
						output, err := io.ReadAll(resp.Body)
						_ = resp.Body.Close()
						if err != nil {
							t.Fatal(err)
						}
						if resp.StatusCode != 200 || calls.Load() != 1 {
							t.Fatalf("status=%d calls=%d body=%s", resp.StatusCode, calls.Load(), output)
						}
						for _, marker := range []string{"receipt-text", "receipt-tool", down.toolMarker} {
							if !strings.Contains(string(output), marker) {
								t.Errorf("missing %s: %s", marker, output)
							}
						}
						if strings.Contains(string(output), "upstream_error") || strings.Contains(string(output), `"type":"error"`) {
							t.Errorf("conversion failed: %s", output)
						}
						select {
						case entry := <-logs:
							if entry.Status != "success" || entry.PromptTokens == nil || *entry.PromptTokens != 5 || entry.CompletionTokens == nil || *entry.CompletionTokens != 2 || entry.TotalTokens == nil || *entry.TotalTokens != 7 {
								t.Errorf("wrong native usage/status: %+v", entry)
							}
						case <-time.After(time.Second):
							t.Error("no proxy log")
						}
					})
				}
			}
		}
	}
}

func TestDirectOllamaHTTPDoesNotSucceedWithoutDone(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream_%t", stream), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if stream {
					w.Header().Set("Content-Type", "application/x-ndjson")
				}
				_, _ = io.WriteString(w, `{"model":"provider-model","done":false,"message":{"content":"partial"},"prompt_eval_count":5,"eval_count":2}`+"\n")
			}))
			defer upstream.Close()
			installOllamaHTTPRuntime(t, store.DialectSQLite, upstream.URL, false)
			logs := make(chan proxy.ProxyLogEntry, 2)
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
			out := httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hi"}],"stream":%t}`, stream)))
			if (!stream && out.Code != 502) || !strings.Contains(out.Body.String(), "upstream_error") || strings.Contains(out.Body.String(), `"finish_reason":"stop"`) {
				t.Fatalf("incomplete response succeeded: %d %s", out.Code, out.Body.String())
			}
			select {
			case entry := <-logs:
				if entry.Status != "failed" {
					t.Errorf("truncated status=%s", entry.Status)
				}
			case <-time.After(time.Second):
				t.Error("no failed proxy log")
			}
		})
	}
}

func TestDirectOllamaHTTPMessagesThinkingReplay(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, stable := range []bool{false, true} {
			for _, noAuth := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream_%t/stable_%t/none_%t", stream, stable, noAuth), func(t *testing.T) {
					var calls atomic.Int32
					observed := make(chan map[string]json.RawMessage, 2)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var body map[string]json.RawMessage
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
							return
						}
						observed <- body
						turn := calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						if stream {
							w.Header().Set("Content-Type", "application/x-ndjson")
						}
						if turn == 1 {
							_, _ = io.WriteString(w, `{"model":"provider-model","done":true,"message":{"content":"","thinking":"private planning","tool_calls":[{"function":{"name":"echo","arguments":{}}}]},"prompt_eval_count":5,"eval_count":2}`+"\n")
						} else {
							_, _ = io.WriteString(w, `{"model":"provider-model","done":true,"message":{"content":"receipt"},"prompt_eval_count":8,"eval_count":3}`+"\n")
						}
					}))
					defer upstream.Close()
					installOllamaHTTPRuntime(t, store.DialectSQLite, upstream.URL, noAuth)
					var ctx Ctx
					if err := json.Unmarshal([]byte(`{"model":"client-alias","max_tokens":64,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"call echo"}],"tools":[{"name":"echo","input_schema":{"type":"object"}}]}`), &ctx.Body); err != nil {
						t.Fatal(err)
					}
					if stable {
						ctx.Body["metadata"] = map[string]any{"user_id": "ollama-replay-fixture"}
					}
					ctx.Body["stream"] = stream
					messagesReplayRefresh(t, &ctx)
					first := httptest.NewRecorder()
					HandleClaudeMessages(first, makeProxyReq(http.MethodPost, "/v1/messages", string(ctx.RawBody)))
					if !stable {
						if calls.Load() != 1 || (!stream && first.Code != 502) || (stream && !strings.Contains(first.Body.String(), `"type":"error"`)) || strings.Contains(first.Body.String(), `"stop_reason":"tool_use"`) || strings.Contains(first.Body.String(), "private planning") {
							t.Fatalf("unsafe thinking response: status=%d calls=%d %s", first.Code, calls.Load(), first.Body.String())
						}
						return
					}
					if first.Code != 200 || strings.Contains(first.Body.String(), `"type":"error"`) {
						t.Fatalf("first thinking turn failed: %d %s", first.Code, first.Body.String())
					}
					ids := messagesReplayAppendResponse(t, &ctx, directReplayToolResponse(t, first.Body.String(), stream))
					if len(ids) != 1 || !strings.HasPrefix(ids[0], messagesBridgeToolIDPrefix) {
						t.Fatalf("unowned tool ids: %v", ids)
					}
					<-observed
					messagesReplayRefresh(t, &ctx)
					second := httptest.NewRecorder()
					HandleClaudeMessages(second, makeProxyReq(http.MethodPost, "/v1/messages", string(ctx.RawBody)))
					if second.Code != 200 || calls.Load() != 2 || !strings.Contains(second.Body.String(), "receipt") {
						t.Fatalf("thinking continuation failed: %d calls=%d %s", second.Code, calls.Load(), second.Body.String())
					}
					body := <-observed
					if !strings.Contains(string(body["messages"]), `"thinking":"private planning"`) || !strings.Contains(string(body["messages"]), `"tool_name":"echo"`) {
						t.Fatalf("native continuation lost thinking/tool: %s", body)
					}
				})
			}
		}
	}
}

func TestDirectOllamaHTTPAnonymousRequiresFreshCredential(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mutation := range []string{"missing resolver", "disabled credential", "changed provider"} {
			t.Run(dialect+"/"+mutation, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					_, _ = io.WriteString(w, `{"model":"provider-model","done":true,"message":{"content":"should not arrive"}}`)
				}))
				defer upstream.Close()
				db := installOllamaHTTPRuntime(t, dialect, upstream.URL, true)
				cfg := getUpstreamConfig()
				original := cfg.ResolveDirectCredential
				if mutation == "missing resolver" {
					cfg.ResolveDirectCredential = nil
				} else {
					cfg.ResolveDirectCredential = func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
						var err error
						if mutation == "disabled credential" {
							_, err = db.Exec(db.Rebind(`UPDATE upstream_credentials SET enabled=?`), false)
						} else {
							_, err = db.Exec(`UPDATE upstream_channels SET provider='openai'`)
						}
						if err != nil {
							return nil, err
						}
						return original(ctx, id, proxyURL, force)
					}
				}
				out := httptest.NewRecorder()
				HandleChatCompletions(out, makeProxyReq(http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hi"}]}`))
				if out.Code != http.StatusServiceUnavailable || calls.Load() != 0 {
					t.Fatalf("anonymous auth bypass: status=%d calls=%d %s", out.Code, calls.Load(), out.Body.String())
				}
			})
		}
	}
}

func TestDirectOllamaHTTPFirstFrameBeforeDone(t *testing.T) {
	for _, down := range directWireFixtures {
		t.Run(down.provider, func(t *testing.T) {
			finished := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(finished) }) }
			defer release()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(w, `{"model":"provider-model","done":false,"message":{"content":"receipt-first"}}`+"\n")
				w.(http.Flusher).Flush()
				select {
				case <-finished:
				case <-r.Context().Done():
					return
				}
				_, _ = io.WriteString(w, `{"model":"provider-model","done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":2}`+"\n")
			}))
			defer upstream.Close()
			installOllamaHTTPRuntime(t, store.DialectSQLite, upstream.URL, false)
			downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
				down.handler(w, r)
			}))
			defer downstream.Close()
			var payload map[string]json.RawMessage
			if err := json.Unmarshal([]byte(down.request), &payload); err != nil {
				t.Fatal(err)
			}
			path := down.path
			if down.provider == "gemini" {
				path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
			} else {
				payload["stream"] = json.RawMessage("true")
			}
			raw, _ := json.Marshal(payload)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, downstream.URL+path, strings.NewReader(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			reader := bufio.NewReader(resp.Body)
			var prefix strings.Builder
			for !strings.Contains(prefix.String(), "receipt-first") {
				line, err := reader.ReadString('\n')
				prefix.WriteString(line)
				if err != nil {
					t.Fatalf("first content not delivered before native done: %s %v", prefix.String(), err)
				}
			}
			if resp.StatusCode != 200 {
				t.Fatalf("status=%d %s", resp.StatusCode, prefix.String())
			}
			release()
			tail, err := io.ReadAll(reader)
			if err != nil || strings.Contains(string(tail), `"type":"error"`) || strings.Contains(string(tail), "upstream_error") {
				t.Fatalf("terminal failed: %s %v", tail, err)
			}
		})
	}
}
