package proxyhandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// The Messages tool schema is valid without tools[].type. It must not be
// replayed unchanged at Chat Completions after a timeout or protocol hint.
func TestMessagesToolsFallbackTranslatesWireProtocol(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "protocol_hint_json"
		if stream {
			name = "first_byte_timeout_stream"
		}
		t.Run(name, func(t *testing.T) {
			prevCfg, prevRT := config.GetSafe(), config.RuntimeSafe()
			config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
			config.UpdateRuntime(func(r *config.RuntimeSettings) {
				r.DisableCrossProtocolFallback = false
				r.ProxyFirstByteTimeoutSec = 1
			})
			t.Cleanup(func() { config.Set(prevCfg); config.SetRuntime(prevRT) })

			var mu sync.Mutex
			var paths []string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				tools, _ := body["tools"].([]any)
				if len(tools) != 1 {
					t.Errorf("tools = %#v", body["tools"])
					w.WriteHeader(400)
					return
				}
				tool, _ := tools[0].(map[string]any)
				switch r.URL.Path {
				case "/v1/messages":
					if tool["name"] != "Read" || tool["input_schema"] == nil || tool["type"] != nil {
						t.Errorf("native tool was rewritten: %#v", tool)
					}
					if stream {
						<-r.Context().Done()
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"error":{"message":"unsupported endpoint, please use /v1/chat/completions"}}`)
				case "/v1/chat/completions":
					function, _ := tool["function"].(map[string]any)
					if tool["type"] != "function" || function["name"] != "Read" || function["parameters"] == nil {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						_, _ = io.WriteString(w, `{"error":{"message":"field Tools[0].Type invalid, should be set"}}`)
						return
					}
					if body["tool_choice"] != "auto" {
						t.Errorf("Chat tool_choice = %#v", body["tool_choice"])
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+`{"id":"chat-tool-1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call-read-1","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"receipt.txt\"}"}}]},"finish_reason":null}]}`+"\n\n")
						_, _ = io.WriteString(w, "data: "+`{"id":"chat-tool-1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`+"\n\ndata: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"chat-tool-1","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-read-1","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"receipt.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`)
				default:
					t.Errorf("unexpected fallback path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(upstream.Close)
			router := &upstreamTestRouter{selected: routing.SelectedChannel{
				Channel: store.RouteChannel{ID: 42, Enabled: true}, Account: store.Account{ID: 7, Status: "active"},
				Site:       store.Site{ID: 3, URL: upstream.URL, Platform: "new-api", Status: "active"},
				TokenValue: "fixture-token", ActualModel: "test-model",
			}}
			var logs []proxy.ProxyLogEntry
			SetUpstreamConfig(&UpstreamConfig{Router: router, Executor: proxy.NewRuntimeExecutor(5 * time.Second),
				LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil },
			})
			t.Cleanup(func() { SetUpstreamConfig(nil) })
			body := `{"model":"test-model","max_tokens":64,"messages":[{"role":"user","content":"Read receipt.txt"}],"tools":[{"name":"Read","description":"Read a local fixture","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}],"tool_choice":{"type":"auto"}}`
			if stream {
				body = strings.TrimSuffix(body, "}") + `,"stream":true}`
			}
			rec := httptest.NewRecorder()
			HandleClaudeMessages(rec, makeProxyReq(http.MethodPost, "/v1/messages", body))
			if rec.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
			}
			mu.Lock()
			gotPaths := append([]string(nil), paths...)
			mu.Unlock()
			if !reflect.DeepEqual(gotPaths, []string{"/v1/messages", "/v1/chat/completions"}) {
				t.Fatalf("paths = %v", gotPaths)
			}
			if stream {
				for _, marker := range []string{"event: message_start", "event: content_block_start", "input_json_delta", "call-read-1", `"stop_reason":"tool_use"`, "event: message_stop"} {
					if !strings.Contains(rec.Body.String(), marker) {
						t.Errorf("missing %q in %s", marker, rec.Body.String())
					}
				}
				if strings.Contains(rec.Body.String(), "chat.completion") || strings.Contains(rec.Body.String(), "[DONE]") {
					t.Errorf("Chat wire leaked to Messages client: %s", rec.Body.String())
				}
			} else {
				var response map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if response["type"] != "message" || response["stop_reason"] != "tool_use" {
					t.Fatalf("not a Messages response: %s", rec.Body.String())
				}
				blocks, _ := response["content"].([]any)
				if len(blocks) != 1 {
					t.Fatalf("content = %#v", response["content"])
				}
				tool := blocks[0].(map[string]any)
				if tool["type"] != "tool_use" || tool["id"] != "call-read-1" || tool["name"] != "Read" {
					t.Fatalf("tool identity = %#v", tool)
				}
				if !reflect.DeepEqual(tool["input"], map[string]any{"file_path": "receipt.txt"}) {
					t.Fatalf("tool input = %#v", tool["input"])
				}
			}
			if len(router.failures) != 0 || len(router.successes) != 1 || len(logs) != 1 || logs[0].Status != "success" {
				t.Fatalf("router failures=%+v successes=%+v logs=%+v", router.failures, router.successes, logs)
			}
			if logs[0].TotalTokens == nil || *logs[0].TotalTokens != 13 {
				t.Fatalf("upstream usage lost: %+v", logs[0])
			}
		})
	}
}

// Preferred-channel selection still owns authorization. This stub rejects a
// current policy exclusion so a replay hint cannot accidentally bypass it.
type messagesBridgeTestRouter struct {
	*upstreamTestRouter
	preferred      routing.SelectedChannel
	preferredCalls int
}

func (r *messagesBridgeTestRouter) SelectPreferredChannel(_ context.Context, _ string, id int64, policy routing.DownstreamRoutingPolicy, excluded []int64) (*routing.SelectedChannel, error) {
	r.preferredCalls++
	r.policies = append(r.policies, policy)
	for _, siteID := range policy.ExcludedSiteIDs {
		if siteID == r.preferred.Site.ID {
			return nil, nil
		}
	}
	for _, channelID := range excluded {
		if channelID == id {
			return nil, nil
		}
	}
	if id != r.preferred.Channel.ID {
		return nil, nil
	}
	return &r.preferred, nil
}

// A hidden-reasoning tool continuation must use its original authorized channel,
// even when ordinary weighted selection would choose a different one.
func TestMessagesAdaptiveToolContinuationKeepsBridgeAndHiddenReasoning(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			prevCfg, prevRT := config.GetSafe(), config.RuntimeSafe()
			config.Set(&config.Config{ProxyMaxChannelAttempts: 2})
			config.UpdateRuntime(func(r *config.RuntimeSettings) { r.DisableCrossProtocolFallback = false })
			previousCache := messagesBridgeReplayCache
			messagesBridgeReplayCache = newMessagesReplayCache()
			t.Cleanup(func() { config.Set(prevCfg); config.SetRuntime(prevRT); messagesBridgeReplayCache = previousCache })
			var mu sync.Mutex
			var paths []string
			chatCalls := 0
			var publicToolID string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/messages" {
					w.WriteHeader(404)
					_, _ = io.WriteString(w, `{"error":{"message":"please use /v1/chat/completions"}}`)
					return
				}
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected endpoint %s", r.URL.Path)
					w.WriteHeader(400)
					return
				}
				var request map[string]any
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if request["reasoning_effort"] != "high" {
					t.Errorf("adaptive thinking was disabled: %#v", request["reasoning_effort"])
				}
				chatCalls++
				if chatCalls == 1 {
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+`{"id":"chat-replay-1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"hidden-fixture-planning","tool_calls":[{"index":0,"id":"call-replay-1","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"receipt.txt\"}"}}]},"finish_reason":null}]}`+"\n\ndata: "+`{"id":"chat-replay-1","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`+"\n\ndata: [DONE]\n\n")
					} else {
						_, _ = io.WriteString(w, `{"id":"chat-replay-1","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"hidden-fixture-planning","tool_calls":[{"id":"call-replay-1","type":"function","function":{"name":"Read","arguments":"{\"file_path\":\"receipt.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}`)
					}
					return
				}
				history, _ := request["messages"].([]any)
				if len(history) != 3 {
					t.Errorf("history length=%d", len(history))
					w.WriteHeader(400)
					return
				}
				assistant, _ := history[1].(map[string]any)
				tool, _ := history[2].(map[string]any)
				if assistant["reasoning_content"] != "hidden-fixture-planning" {
					t.Error("tool continuation lost hidden reasoning")
				}
				calls, _ := assistant["tool_calls"].([]any)
				if len(calls) != 1 || calls[0].(map[string]any)["id"] != publicToolID {
					t.Error("assistant tool identity changed")
				}
				if tool["role"] != "tool" || tool["tool_call_id"] != publicToolID || tool["content"] != "receipt-fixture" {
					t.Errorf("invalid tool result: %#v", tool)
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: "+`{"id":"chat-replay-2","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"receipt-fixture","reasoning_content":"hidden-fixture-final"},"finish_reason":null}]}`+"\n\ndata: "+`{"id":"chat-replay-2","object":"chat.completion.chunk","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":14}}`+"\n\ndata: [DONE]\n\n")
				} else {
					_, _ = io.WriteString(w, `{"id":"chat-replay-2","object":"chat.completion","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"receipt-fixture","reasoning_content":"hidden-fixture-final"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":14}}`)
				}
			}))
			t.Cleanup(upstream.Close)
			original := routing.SelectedChannel{
				Channel: store.RouteChannel{ID: 142, Enabled: true}, Account: store.Account{ID: 107, Status: "active"},
				Site: store.Site{ID: 103, URL: upstream.URL, Platform: "new-api", Status: "active"}, TokenValue: "fixture-upstream", ActualModel: "test-model",
			}
			router := &messagesBridgeTestRouter{upstreamTestRouter: &upstreamTestRouter{selected: original}, preferred: original}
			var logs []proxy.ProxyLogEntry
			SetUpstreamConfig(&UpstreamConfig{Router: router, Executor: proxy.NewRuntimeExecutor(5 * time.Second), LogProxy: func(_ context.Context, e proxy.ProxyLogEntry) error { logs = append(logs, e); return nil }})
			t.Cleanup(func() { SetUpstreamConfig(nil) })
			first := `{"model":"test-model","max_tokens":64,"metadata":{"user_id":"fixture-adaptive-tool-conversation"},"thinking":{"type":"adaptive","display":"omitted"},"output_config":{"effort":"high"},"context_management":{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]},"messages":[{"role":"user","content":"Read receipt.txt"}],"tools":[{"name":"Read","input_schema":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}]}`
			if stream {
				first = strings.TrimSuffix(first, "}") + `,"stream":true}`
			}
			firstRec := httptest.NewRecorder()
			HandleClaudeMessages(firstRec, makeProxyReq("POST", "/v1/messages", first))
			if firstRec.Code != 200 || strings.Contains(firstRec.Body.String(), "hidden-fixture") {
				t.Fatalf("first tool response status=%d; reasoning leaked=%v", firstRec.Code, strings.Contains(firstRec.Body.String(), "hidden-fixture"))
			}
			blocks := messagesBridgeResponseTool(t, firstRec, stream)
			publicToolID, _ = blocks[0].(map[string]any)["id"].(string)
			if !strings.HasPrefix(publicToolID, "toolu_mcb_") || len(publicToolID) != len("toolu_mcb_")+32 {
				t.Fatalf("not an opaque bridge-owned tool ID: %q", publicToolID)
			}
			var next map[string]any
			if err := json.Unmarshal([]byte(first), &next); err != nil {
				t.Fatal(err)
			}
			next["messages"] = append(next["messages"].([]any), map[string]any{"role": "assistant", "content": blocks}, map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": publicToolID, "content": "receipt-fixture"}}})
			wire, err := json.Marshal(next)
			if err != nil {
				t.Fatal(err)
			}
			// A normal second selection now points elsewhere. Replay must still
			// enter SelectPreferredChannel and use the original channel.
			router.selected.Channel.ID = 242
			router.selected.TokenValue = "fixture-unrelated-upstream"
			secondRec := httptest.NewRecorder()
			HandleClaudeMessages(secondRec, makeProxyReq("POST", "/v1/messages", string(wire)))
			if secondRec.Code != 200 || !strings.Contains(secondRec.Body.String(), "receipt-fixture") || strings.Contains(secondRec.Body.String(), "hidden-fixture") {
				t.Fatalf("second tool response status=%d; receipt=%v; reasoning leaked=%v", secondRec.Code, strings.Contains(secondRec.Body.String(), "receipt-fixture"), strings.Contains(secondRec.Body.String(), "hidden-fixture"))
			}
			mu.Lock()
			got := append([]string(nil), paths...)
			mu.Unlock()
			if !reflect.DeepEqual(got, []string{"/v1/messages", "/v1/chat/completions", "/v1/chat/completions"}) {
				t.Fatalf("tool continuation replayed a different protocol: %v", got)
			}
			if router.preferredCalls != 1 || len(logs) != 2 || len(router.failures) != 0 || logs[0].TotalTokens == nil || *logs[0].TotalTokens != 13 || logs[1].TotalTokens == nil || *logs[1].TotalTokens != 14 {
				t.Fatalf("tool-roundtrip billing/failure mismatch: %+v failures=%+v preferred=%d", logs, router.failures, router.preferredCalls)
			}
			if stream && (strings.Count(secondRec.Body.String(), "event: message_stop") != 1 || strings.Contains(secondRec.Body.String(), "[DONE]")) {
				t.Fatal("invalid Messages SSE terminal")
			}

			for _, failure := range []string{"policy-revoked", "forced-conflict", "credential-changed", "model-changed", "fallback-disabled", "unsupported-conversion", "session-changed", "cache-missing", "cache-expired"} {
				t.Run(failure, func(t *testing.T) {
					req := makeProxyReq("POST", "/v1/messages", string(wire))
					ctx, errResult := PrepareCtx(req, SurfConfig{})
					if errResult != nil {
						t.Fatalf("prepare continuation: %+v", errResult)
					}
					ctx.DownstreamPath = "/v1/messages"
					wantCode := http.StatusBadRequest
					switch failure {
					case "policy-revoked":
						ctx.Policy.ExcludedSiteIDs = []int64{original.Site.ID}
						wantCode = http.StatusServiceUnavailable
					case "forced-conflict":
						id := int64(242)
						ctx.ForcedChannelID = &id
					case "credential-changed":
						router.preferred.TokenValue = "fixture-rotated"
						t.Cleanup(func() { router.preferred = original })
					case "model-changed":
						router.preferred.ActualModel = "changed-model"
						t.Cleanup(func() { router.preferred = original })
					case "fallback-disabled":
						config.UpdateRuntime(func(r *config.RuntimeSettings) { r.DisableCrossProtocolFallback = true })
						t.Cleanup(func() {
							config.UpdateRuntime(func(r *config.RuntimeSettings) { r.DisableCrossProtocolFallback = false })
						})
					case "unsupported-conversion":
						ctx.Body["stop_sequences"] = []any{"stop"}
						ctx.RawBody, _ = json.Marshal(ctx.Body)
					case "session-changed":
						ctx.Body["metadata"] = map[string]any{"user_id": "different-session"}
						ctx.RawBody, _ = json.Marshal(ctx.Body)
					case "cache-missing":
						saved := messagesBridgeReplayCache
						messagesBridgeReplayCache = newMessagesReplayCache()
						t.Cleanup(func() { messagesBridgeReplayCache = saved })
					case "cache-expired":
						messagesBridgeReplayCache.now = func() time.Time { return time.Now().Add(messagesReplayTTL + time.Second) }
					}
					rec := httptest.NewRecorder()
					dispatchUpstream(rec, req, ctx)
					if rec.Code != wantCode {
						t.Errorf("HTTP %d, want %d: %s", rec.Code, wantCode, rec.Body.String())
					}
					mu.Lock()
					count := len(paths)
					mu.Unlock()
					if count != 3 {
						t.Fatalf("unsafe continuation reached an upstream: %d requests", count)
					}
					if failure == "policy-revoked" && !reflect.DeepEqual(router.policies[len(router.policies)-1].ExcludedSiteIDs, []int64{original.Site.ID}) {
						t.Error("preferred selection lost current downstream policy")
					}
				})
			}
		})
	}
}

// Reassemble the single fixture tool from the actual downstream wire; using
// the upstream's original ID would miss the opaque-ID continuation contract.
func messagesBridgeResponseTool(t *testing.T, rec *httptest.ResponseRecorder, stream bool) []any {
	t.Helper()
	if !stream {
		var response map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		blocks, _ := response["content"].([]any)
		if response["stop_reason"] != "tool_use" || len(blocks) != 1 {
			t.Fatalf("invalid tool response: %s", rec.Body.String())
		}
		return blocks
	}
	var block map[string]any
	var arguments strings.Builder
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatal(err)
		}
		if event["type"] == "content_block_start" {
			if block != nil {
				t.Fatal("fixture returned more than one block")
			}
			block, _ = event["content_block"].(map[string]any)
		}
		if event["type"] == "content_block_delta" {
			delta, _ := event["delta"].(map[string]any)
			part, _ := delta["partial_json"].(string)
			arguments.WriteString(part)
		}
	}
	if block == nil || block["type"] != "tool_use" || strings.Count(rec.Body.String(), "event: message_stop") != 1 {
		t.Fatalf("invalid tool stream: %s", rec.Body.String())
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(arguments.String()), &input); err != nil {
		t.Fatal(err)
	}
	block["input"] = input
	return []any{block}
}
