package proxyhandler

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

const routerChatFixtureJSON = `{"id":"chat_fixture","object":"chat.completion","created":9007199254740993,"model":"fixture/model","provider":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"Read both.","reasoning":"alias","reasoning_details":[{"type":"reasoning.text","text":"Think ","format":"unknown","index":0},{"type":"reasoning.summary","summary":"then act.","index":1}],"tool_calls":[{"id":"call_9","type":"function","function":{"name":"read","arguments":"{\"offset\":9007199254740993}"}},{"id":"call_2","type":"function","function":{"name":"read","arguments":"{\"offset\":2}"}}]},"finish_reason":"tool_calls","native_finish_reason":"TOOL"}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"cost":0.0000000000000000000003,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":4}}}`

var routerChatFixturePaths = []string{"/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1beta/models/fixture:generateContent"}

func routerChatFrame(json string) string { return "data: " + json + "\n\n" }

func routerChatFixtureStream() string {
	return ": heartbeat\r\n\r\n" +
		routerChatFrame(`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture/model","choices":[{"index":0,"delta":{"role":"assistant","reasoning":"alias","reasoning_details":[{"type":"reasoning.text","text":"Think ","index":0}]}}]}`) +
		routerChatFrame(`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture/model","choices":[{"index":0,"delta":{"reasoning_details":[{"type":"reasoning.summary","summary":"then act.","index":1}],"content":"Read both.","tool_calls":[{"index":9,"id":"call_9","type":"function","function":{"name":"read","arguments":"{\"offset\":"}},{"index":2,"id":"call_2","type":"function","function":{"name":"read","arguments":"{\"offset\":2}"}}]}}]}`) +
		routerChatFrame(`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture/model","choices":[{"index":0,"delta":{"tool_calls":[{"index":9,"function":{"arguments":"9007199254740993}"}}]},"finish_reason":"tool_calls"}]}`) +
		routerChatFrame(`{"id":"chat_fixture","object":"chat.completion.chunk","model":"fixture/model","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"cost":0.0000000000000000000003,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":4}}}`) +
		"data: [DONE]\n\n"
}

func TestDirectRouterChatRequest(t *testing.T) {
	body := []byte(`{"model":"fixture/model","store":true,"seed":9007199254740993,"provider":{"order":["fixture"]},"messages":[{"role":"user","content":""},{"role":"assistant","reasoning_content":"Think.","content":null,"reasoning_details":[{"type":"reasoning.encrypted","data":"opaque"}],"tool_calls":[{"id":"call_9","function":{"name":"read","arguments":"{\"n\":9007199254740993}"}}]}]}`)
	for _, profile := range []string{"openrouter", "cerebras"} {
		out, err := prepareDirectRouterChatRequest(body, profile)
		if err != nil {
			t.Fatal(err)
		}
		for _, preserved := range []string{`"content":""`, `"reasoning":"Think."`, `"seed":9007199254740993`, `"data":"opaque"`, `"order":["fixture"]`, `"id":"call_9"`} {
			if !bytes.Contains(out, []byte(preserved)) {
				t.Errorf("%s request lost %s: %s", profile, preserved, out)
			}
		}
		if bytes.Contains(out, []byte("reasoning_content")) || bytes.Contains(out, []byte(`"store"`)) != (profile == "openrouter") {
			t.Errorf("%s request alias/store mismatch: %s", profile, out)
		}
	}
	if !bytes.Contains(body, []byte("reasoning_content")) || !bytes.Contains(body, []byte(`"store":true`)) {
		t.Fatal("mutated original request")
	}
	for _, bad := range []string{`null`, `{}`, `{"model":"m","messages":[null]}`, `{"model":"m","messages":[{"reasoning_content":[]}]}`, `{"model":"m","messages":[{"reasoning":"a","reasoning_content":"b"}]}`} {
		if _, err := prepareDirectRouterChatRequest([]byte(bad), "cerebras"); err == nil {
			t.Errorf("accepted invalid request: %s", bad)
		}
	}
	if _, err := prepareDirectRouterChatRequest(body, "openai"); err == nil {
		t.Fatal("applied provider adapter to generic profile")
	}
}

func TestDirectRouterChatJSONProjection(t *testing.T) {
	normalized, err := normalizeDirectRouterChatJSON([]byte(routerChatFixtureJSON), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, preserved := range []string{`"reasoning_content":"Think then act."`, `"reasoning":"alias"`, `"reasoning_details"`, `9007199254740993`, `0.0000000000000000000003`, `"provider":"fixture"`} {
		if !bytes.Contains(normalized, []byte(preserved)) {
			t.Errorf("native response lost %s: %s", preserved, normalized)
		}
	}
	projected, err := projectDirectRouterChatResponse(normalized, false)
	if err != nil || bytes.Contains(projected, []byte(`"reasoning":`)) || bytes.Contains(projected, []byte(`"reasoning_details":`)) || !bytes.Contains(projected, []byte(`"reasoning_content":"Think then act."`)) {
		t.Fatalf("projection kept duplicate aliases or lost text: %s (%v)", projected, err)
	}
	for _, path := range routerChatFixturePaths {
		t.Run(path, func(t *testing.T) {
			var savedReasoning string
			options := messages.Options{SaveReasoning: func(ids []string, reasoning string) error {
				if strings.Join(ids, ",") != "call_9,call_2" {
					t.Errorf("tool order changed: %v", ids)
				}
				savedReasoning = reasoning
				return nil
			}}
			out := normalized
			if path != "/v1/chat/completions" {
				out, err = directConvertResponse(projected, path, "/v1/chat/completions", "fixture/model", options)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(out, []byte("9007199254740993")) || !bytes.Contains(out, []byte("call_9")) || !bytes.Contains(out, []byte("call_2")) {
				t.Fatalf("tool arguments/identity lost: %s", out)
			}
			if path == "/v1/messages" {
				if savedReasoning != "Think then act." || bytes.Contains(out, []byte(`"thinking"`)) || bytes.Contains(out, []byte("Think then act.")) {
					t.Fatalf("Messages replay lost or invented signed thinking: %s / %q", out, savedReasoning)
				}
			} else if !bytes.Contains(out, []byte("Think then act.")) {
				t.Fatalf("plain reasoning lost: %s", out)
			}
		})
	}
}

func TestDirectRouterChatUsagePrecisionAndExtensions(t *testing.T) {
	body := []byte(`{"id":"precise","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok","provider_metadata":{"token":9007199254740993}},"finish_reason":"stop"}],"usage":{"prompt_tokens":9007199254740993,"completion_tokens":2,"total_tokens":9007199254740995,"cost":0.12345678901234567890123456789}}`)
	out, err := normalizeDirectRouterChatJSON(body, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{`"prompt_tokens":9007199254740993`, `"total_tokens":9007199254740995`, `"cost":0.12345678901234567890123456789`, `"provider_metadata":{"token":9007199254740993}`} {
		if !bytes.Contains(out, []byte(literal)) {
			t.Fatalf("native JSON changed %s: %s", literal, out)
		}
	}
	projected, err := projectDirectRouterChatResponse(out, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range routerChatFixturePaths[1:] {
		if out, err := directConvertResponse(projected, path, "/v1/chat/completions", "fixture", messages.Options{}); err == nil {
			t.Fatalf("%s silently dropped unknown message extension: %s", path, out)
		}
	}
}

// Test-only wiring mirrors root's source filter -> projection -> existing
// downstream bridge. Production codec/lifecycle/accounting remain shared.
type routerChatTestProjection struct{ next protocolEventStream }

func (p *routerChatTestProjection) TransformEvent(frame []byte) ([]byte, error) {
	event := parseSseBlock(string(frame))
	if event != nil && event.Data != "" && event.Data != "[DONE]" {
		body, err := projectDirectRouterChatResponse([]byte(event.Data), true)
		if err != nil {
			return nil, err
		}
		frame = []byte(routerChatFrame(string(body)))
	}
	return p.next.TransformEvent(frame)
}

func (p *routerChatTestProjection) Finish() ([]byte, error) { return p.next.Finish() }

func routerChatTestStream(path string, options messages.Options) protocolEventStream {
	stream := newDirectRouterChatStream()
	if path != "/v1/chat/completions" {
		stream = &chainedProtocolStream{first: stream, second: &routerChatTestProjection{next: directResponseStream(path, "/v1/chat/completions", "fixture/model", options)}}
	}
	return stream
}

func routerChatTestReader(t *testing.T, resp *http.Response, stream protocolEventStream, limit int64, idleTimeout time.Duration) (*messagesChatBody, *streamIdleBody) {
	t.Helper()
	decoded := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
	if !decoded.Readable {
		t.Fatal("fixture encoding is unreadable")
	}
	source := resp.Body
	if decoded.Reader != nil {
		source = struct {
			io.Reader
			io.Closer
		}{decoded.Reader, resp.Body}
	}
	idle := &streamIdleBody{ReadCloser: source}
	idle.guard = newStreamIdleGuard(idleTimeout, idle.closeUnderlying)
	return newProtocolBridgeBody(idle, stream, limit), idle
}

func routerChatHTTPFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-Client-Type") != "" || !bytes.Contains(body, []byte(`"reasoning":"Think."`)) {
			t.Errorf("incorrect request wiring: %s", body)
		}
		if r.URL.Path == "/cerebras/v1/chat/completions" && bytes.Contains(body, []byte(`"store"`)) {
			t.Error("Cerebras received unsupported store")
		}
		payload := routerChatFixtureJSON
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("stream") == "true" {
			payload = routerChatFixtureStream()
			w.Header().Set("Content-Type", "text/event-stream")
		}
		if r.URL.Query().Get("images") == "true" {
			payload = routerChatImageJSON
			if r.URL.Query().Get("stream") == "true" {
				payload = routerChatImageStream()
			}
		}
		w.Header().Set("Content-Encoding", "gzip")
		writer := gzip.NewWriter(w)
		for start := 0; start < len(payload); start += 13 {
			_, _ = io.WriteString(writer, payload[start:min(start+13, len(payload))])
			_ = writer.Flush()
			w.(http.Flusher).Flush()
		}
		_ = writer.Close()
	}))
}

func TestDirectRouterChatHTTPFixture(t *testing.T) {
	server := routerChatHTTPFixtureServer(t)
	defer server.Close()
	for _, profile := range []string{"openrouter", "cerebras"} {
		for _, stream := range []bool{false, true} {
			for _, path := range routerChatFixturePaths {
				t.Run(profile+path+map[bool]string{true: "/stream", false: "/JSON"}[stream], func(t *testing.T) {
					body, err := prepareDirectRouterChatRequest([]byte(`{"model":"fixture/model","store":true,"messages":[{"role":"assistant","content":"hi","reasoning_content":"Think."}]}`), profile)
					if err != nil {
						t.Fatal(err)
					}
					target := server.URL + "/" + profile + "/v1/chat/completions"
					if stream {
						target += "?stream=true"
					}
					req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, target, bytes.NewReader(body))
					req.Header.Set("Authorization", "Bearer fixture-token")
					req.Header.Set("Accept-Encoding", "gzip")
					resp, err := server.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					var out []byte
					var savedReasoning string
					options := messages.Options{SaveReasoning: func(ids []string, reasoning string) error { savedReasoning = reasoning; return nil }}
					if stream {
						reader, _ := routerChatTestReader(t, resp, routerChatTestStream(path, options), 4<<20, time.Second)
						defer reader.Close()
						out, err = io.ReadAll(reader)
						usage := reader.original.Result().Usage
						if usage.PromptTokens != 12 || usage.CompletionTokens != 7 || usage.TotalTokens != 19 {
							t.Errorf("raw billable usage lost: %+v", usage)
						}
					} else {
						out, err = io.ReadAll(resp.Body)
						if err == nil {
							decoded := normalizeBufferedUpstreamBody(resp, out, upstreamBodyIdent{})
							out, err = normalizeDirectRouterChatJSON(decoded.bytes, decoded.readable)
						}
						if err == nil && path != "/v1/chat/completions" {
							out, err = projectDirectRouterChatResponse(out, false)
							if err == nil {
								out, err = directConvertResponse(out, path, "/v1/chat/completions", "fixture/model", options)
							}
						}
					}
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Contains(out, []byte("9007199254740993")) || !bytes.Contains(out, []byte("call_9")) || !bytes.Contains(out, []byte("call_2")) || bytes.Index(out, []byte("call_9")) > bytes.Index(out, []byte("call_2")) {
						t.Fatalf("tools or their arrival order changed: %s", out)
					}
					if path == "/v1/messages" && savedReasoning != "Think then act." {
						t.Fatalf("Messages reasoning replay lost: %q", savedReasoning)
					}
					if stream {
						terminal := "[DONE]"
						switch path {
						case "/v1/responses":
							terminal = "response.completed"
						case "/v1/messages":
							terminal = "message_stop"
						case "/v1beta/models/fixture:generateContent":
							terminal = `"finishReason":"STOP"`
						}
						if !bytes.Contains(out, []byte(terminal)) {
							t.Fatalf("missing terminal %s: %s", terminal, out)
						}
					}
				})
			}
		}
	}
}

func TestDirectRouterChatHTTPImages(t *testing.T) {
	server := routerChatHTTPFixtureServer(t)
	defer server.Close()
	for _, stream := range []bool{false, true} {
		for _, path := range routerChatFixturePaths {
			t.Run(path+map[bool]string{true: "/stream", false: "/JSON"}[stream], func(t *testing.T) {
				body, err := prepareDirectRouterChatRequest([]byte(`{"model":"fixture/model","messages":[{"role":"assistant","content":"hi","reasoning_content":"Think."}]}`), "openrouter")
				if err != nil {
					t.Fatal(err)
				}
				target := server.URL + "/openrouter/v1/chat/completions?images=true"
				if stream {
					target += "&stream=true"
				}
				req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, target, bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer fixture-token")
				req.Header.Set("Accept-Encoding", "gzip")
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				var out []byte
				if stream {
					reader, _ := routerChatTestReader(t, resp, routerChatTestStream(path, messages.Options{}), 4<<20, time.Second)
					defer reader.Close()
					out, err = io.ReadAll(reader)
					if path == "/v1/chat/completions" || strings.Contains(path, "generateContent") {
						usage := reader.original.Result().Usage
						if usage.PromptTokens != 2 || usage.CompletionTokens != 3 || usage.TotalTokens != 5 {
							t.Errorf("image usage lost: %+v", usage)
						}
					}
				} else {
					out, err = io.ReadAll(resp.Body)
					if err == nil {
						decoded := normalizeBufferedUpstreamBody(resp, out, upstreamBodyIdent{})
						out, err = normalizeDirectRouterChatJSON(decoded.bytes, decoded.readable)
					}
					if err == nil && path != "/v1/chat/completions" {
						out, err = projectDirectRouterChatResponse(out, false)
						if err == nil {
							out, err = directConvertResponse(out, path, "/v1/chat/completions", "fixture/model", messages.Options{})
						}
					}
				}
				if path == "/v1/messages" || path == "/v1/responses" {
					if err == nil {
						t.Fatalf("unrepresentable image output silently succeeded: %s", out)
					}
					return
				}
				if err != nil || !bytes.Contains(out, []byte("AA==")) || !bytes.Contains(out, []byte("AQ==")) || bytes.Index(out, []byte("AA==")) > bytes.Index(out, []byte("AQ==")) {
					t.Fatalf("image HTTP pipeline lost output/order: %s (%v)", out, err)
				}
				if strings.Contains(path, "generateContent") && bytes.Count(out, []byte(`"inlineData"`)) != 2 {
					t.Fatalf("Gemini HTTP output lost image parts: %s", out)
				}
			})
		}
	}
}

func TestDirectRouterChatOpaqueReasoning(t *testing.T) {
	for _, detail := range []string{
		`{"type":"reasoning.encrypted","data":"cipher","text":"not plaintext"}`,
		`{"type":"future","text":"unknown semantics"}`,
		`{"type":"reasoning.text","text":"signed text","signature":"opaque"}`,
	} {
		body := `{"id":"chat","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok","reasoning_details":[` + detail + `]},"finish_reason":"stop"}]}`
		native, err := normalizeDirectRouterChatJSON([]byte(body), true)
		if err != nil || !bytes.Contains(native, []byte(detail)) {
			t.Fatalf("native opaque reasoning lost: %s (%v)", native, err)
		}
		if strings.Contains(detail, `"type":"future"`) || strings.Contains(detail, `"type":"reasoning.encrypted"`) {
			if bytes.Contains(native, []byte("reasoning_content")) {
				t.Fatalf("opaque detail was converted into plain text: %s", native)
			}
		}
		if _, err := projectDirectRouterChatResponse(native, false); err == nil {
			t.Fatalf("cross projection accepted opaque/signed reasoning: %s", native)
		}
	}
}

const routerChatImageJSON = `{"id":"chat_image","object":"chat.completion","model":"fixture/model","choices":[{"index":0,"message":{"role":"assistant","content":"Two images.","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="},"index":7},{"type":"image_url","image_url":{"url":"data:image/png;base64,AQ=="},"index":2}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"completion_tokens_details":{"image_tokens":2}}}`

func routerChatImageStream() string {
	return routerChatFrame(`{"id":"chat_image","model":"fixture/model","choices":[{"index":0,"delta":{"role":"assistant","content":"Two images.","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="},"index":7}]}}]}`) +
		routerChatFrame(`{"id":"chat_image","model":"fixture/model","choices":[{"index":0,"delta":{"images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AQ=="},"index":2}]},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`) +
		"data: [DONE]\n\n"
}

func TestDirectRouterChatNativeImages(t *testing.T) {
	native, err := normalizeDirectRouterChatJSON([]byte(routerChatImageJSON), true)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := routerChatObject(native)
	var choices []map[string]json.RawMessage
	_ = json.Unmarshal(obj["choices"], &choices)
	msg, _ := routerChatObject(choices[0]["message"])
	var content, images []map[string]json.RawMessage
	_ = json.Unmarshal(msg["content"], &content)
	_ = json.Unmarshal(msg["images"], &images)
	if len(content) != 3 || len(images) != 2 || string(images[0]["index"]) != "7" || string(images[1]["index"]) != "2" || string(content[0]["text"]) != `"Two images."` || !bytes.Contains(content[1]["image_url"], []byte("AA==")) || !bytes.Contains(content[2]["image_url"], []byte("AQ==")) {
		t.Fatalf("images/text order or native indices lost: %s", native)
	}
	projected, err := projectDirectRouterChatResponse(native, false)
	if err != nil || bytes.Contains(projected, []byte(`"images":`)) || bytes.Count(projected, []byte(`"image_url":`)) != 2 {
		t.Fatalf("image projection duplicated/lost content: %s (%v)", projected, err)
	}
	for _, path := range []string{"/v1/messages", "/v1/responses"} {
		if out, err := directConvertResponse(projected, path, "/v1/chat/completions", "fixture/model", messages.Options{}); err == nil {
			t.Fatalf("%s silently accepted unrepresentable output images: %s", path, out)
		}
	}
	reader := newProtocolBridgeBody(io.NopCloser(strings.NewReader(routerChatImageStream())), newDirectRouterChatStream(), 1<<20)
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if err != nil || bytes.Count(out, []byte(`"images":`)) != 2 || bytes.Count(out, []byte(`"content":[`)) != 2 || bytes.Index(out, []byte("AA==")) > bytes.Index(out, []byte("AQ==")) {
		t.Fatalf("cross-chunk native images lost: %s (%v)", out, err)
	}
}

func TestDirectRouterChatGeminiImages(t *testing.T) {
	native, err := normalizeDirectRouterChatJSON([]byte(routerChatImageJSON), true)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectDirectRouterChatResponse(native, false)
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1beta/models/fixture:generateContent"
	out, err := directConvertResponse(projected, path, "/v1/chat/completions", "fixture/model", messages.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`"inlineData"`, `"data":"AA=="`, `"data":"AQ=="`, "Two images."} {
		if !bytes.Contains(out, []byte(marker)) {
			t.Errorf("Gemini JSON image conversion lost %s: %s", marker, out)
		}
	}
	reader := newProtocolBridgeBody(io.NopCloser(strings.NewReader(routerChatImageStream())), routerChatTestStream(path, messages.Options{}), 1<<20)
	defer reader.Close()
	out, err = io.ReadAll(reader)
	if err != nil || bytes.Count(out, []byte(`"inlineData"`)) != 2 || bytes.Index(out, []byte("AA==")) > bytes.Index(out, []byte("AQ==")) || !bytes.Contains(out, []byte(`"finishReason":"STOP"`)) {
		t.Fatalf("Gemini lost/reordered image chunks: %s (%v)", out, err)
	}
}

func TestDirectRouterChatLargeNativeImageFrame(t *testing.T) {
	// Image-bearing Chat profiles need a larger bounded frame budget at the
	// integration layer. The source filter must not truncate base64 at the text
	// analysis cap or mistake it for a transport fragment to concatenate.
	encoded := strings.Repeat("A", 2<<20)
	input := routerChatFrame(`{"id":"large","model":"fixture","choices":[{"index":0,"delta":{"role":"assistant","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,`+encoded+`"},"index":0}]},"finish_reason":"stop"}]}`) +
		routerChatFrame(`{"id":"large","model":"fixture","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`) + "data: [DONE]\n\n"
	reader := newProtocolBridgeBody(io.NopCloser(strings.NewReader(input)), newDirectRouterChatStream(), 8<<20)
	reader.frameLimit = 3 << 20
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if err != nil || bytes.Count(out, []byte(encoded)) != 2 || !bytes.HasSuffix(out, []byte("data: [DONE]\n\n")) {
		t.Fatalf("large image was truncated: bytes=%d error=%v", len(out), err)
	}
	if usage := reader.original.Result().Usage; usage.TotalTokens != 5 {
		t.Fatalf("large image hid later usage: %+v", usage)
	}
}

func TestDirectRouterChatGeminiUsagePrecision(t *testing.T) {
	body := []byte(`{"id":"precise","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9007199254740993,"completion_tokens":2,"total_tokens":9007199254740995}}`)
	normalized, err := normalizeDirectRouterChatJSON(body, true)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectDirectRouterChatResponse(normalized, false)
	if err != nil {
		t.Fatal(err)
	}
	out, err := directConvertResponse(projected, "/v1beta/models/fixture:generateContent", "/v1/chat/completions", "fixture", messages.Options{})
	if err != nil || !bytes.Contains(out, []byte(`"promptTokenCount":9007199254740993`)) || !bytes.Contains(out, []byte(`"totalTokenCount":9007199254740995`)) {
		t.Fatalf("Gemini rounded router usage counters: %s (%v)", out, err)
	}
}

func TestDirectRouterChatReasoningFragments(t *testing.T) {
	input := routerChatFrame(`{"id":"chat_r","model":"fixture/model","choices":[{"index":0,"delta":{"role":"assistant","reasoning_details":[{"type":"reasoning.text","index":0,"text":"Think "},{"type":"reasoning.encrypted","index":1,"data":"part1"}]}}]}`) +
		routerChatFrame(`{"id":"chat_r","model":"fixture/model","choices":[{"index":0,"delta":{"reasoning_details":[{"index":0,"text":"then act."},{"type":"reasoning.encrypted","index":1,"data":"part2"}],"content":"ok"},"finish_reason":"stop"}]}`) + "data: [DONE]\n\n"
	reader := newProtocolBridgeBody(io.NopCloser(strings.NewReader(input)), newDirectRouterChatStream(), 1<<20)
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if err != nil || !bytes.Contains(out, []byte(`"reasoning_content":"Think "`)) || !bytes.Contains(out, []byte(`"reasoning_content":"then act."`)) || !bytes.Contains(out, []byte(`"data":"part1"`)) || !bytes.Contains(out, []byte(`"data":"part2"`)) {
		t.Fatalf("reasoning fragments were rewritten or lost: %s (%v)", out, err)
	}
	for _, path := range routerChatFixturePaths[1:] {
		reader := newProtocolBridgeBody(io.NopCloser(strings.NewReader(input)), routerChatTestStream(path, messages.Options{}), 1<<20)
		out, err := io.ReadAll(reader)
		_ = reader.Close()
		if err == nil || bytes.Contains(out, []byte("part1")) || bytes.Contains(out, []byte("part2")) {
			t.Fatalf("%s converted opaque reasoning into another protocol: %s (%v)", path, out, err)
		}
	}
}

func TestDirectRouterChatFailuresAndPartialUsage(t *testing.T) {
	valid := routerChatFixtureStream()
	partial := routerChatFrame(`{"id":"chat_fixture","model":"fixture/model","choices":[{"index":0,"delta":{"content":"partial"}}],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":14}}`)
	for name, input := range map[string]string{
		"missing DONE":       strings.TrimSuffix(valid, "data: [DONE]\n\n"),
		"partial then error": partial + routerChatFrame(`{"error":{"message":"fixture failed"}}`),
		"error with usage":   partial + routerChatFrame(`{"error":{"message":"fixture failed"},"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}}`),
		"unfinished choice":  partial + "data: [DONE]\n\n",
		"bare DONE":          "data: [DONE]\n\n",
		"late error":         valid + "event: error\n\n",
		"duplicate DONE":     valid + "data: [DONE]\n\n",
		"invalid JSON":       "data: {\n\n",
		"non SSE":            "opaque\n\n",
		"unrelated JSON":     routerChatFrame(`{"ok":true}`),
		"error finish":       routerChatFrame(`{"choices":[{"index":0,"delta":{"content":"not success"},"finish_reason":"error"}]}`) + "data: [DONE]\n\n",
		"prefix marker":      partial + "data: [DONE]invalid\n\n",
		"oversized frame":    "data: " + strings.Repeat("x", maxIncrementalSsePendingBytes+1) + "\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			reader := newProtocolBridgeBody(io.NopCloser(strings.NewReader(input)), newDirectRouterChatStream(), 4<<20)
			defer reader.Close()
			out, err := io.ReadAll(reader)
			if err == nil || bytes.Contains(out, []byte("data: [DONE]")) {
				t.Fatalf("failed stream emitted success: %s (%v)", out, err)
			}
			if name == "partial then error" {
				usage := reader.original.Result().Usage
				if usage.PromptTokens != 12 || usage.CompletionTokens != 2 || usage.TotalTokens != 14 {
					t.Fatalf("partial usage lost: %+v", usage)
				}
			}
			if name == "error with usage" {
				usage := reader.original.Result().Usage
				if usage.PromptTokens != 12 || usage.CompletionTokens != 4 || usage.TotalTokens != 16 {
					t.Fatalf("terminal failure usage was lost by normalization: %+v", usage)
				}
			}
		})
	}
	reader := newProtocolBridgeBody(io.NopCloser(strings.NewReader(valid)), newDirectRouterChatStream(), int64(len(valid)-1))
	defer reader.Close()
	out, err := io.ReadAll(reader)
	if !errors.Is(err, errMessagesChatStreamLimit) || bytes.Contains(out, []byte("data: [DONE]")) {
		t.Fatalf("raw input byte limit was bypassed: %s (%v)", out, err)
	}
	for _, bad := range []string{`{"error":{"message":"failed"}}`, `{"success":false,"choices":[{"index":0,"message":{"content":"fake"}}]}`, `{}`, `null`, `{"choices":[]}`, `{"choices":[{"index":0,"message":{"role":"assistant","content":"partial"}}]}`, `{"object":"response","choices":[{"index":0,"message":{}}]}`} {
		if _, err := normalizeDirectRouterChatJSON([]byte(bad), true); err == nil {
			t.Errorf("accepted invalid response: %s", bad)
		}
	}
	if _, err := normalizeDirectRouterChatJSON([]byte(routerChatFixtureJSON), false); err == nil {
		t.Fatal("accepted unreadable response")
	}
}

func TestDirectRouterChatSharedIdleAndCancellation(t *testing.T) {
	t.Run("idle", func(t *testing.T) {
		reader, writer := io.Pipe()
		defer writer.Close()
		body, idle := routerChatTestReader(t, &http.Response{Body: reader}, newDirectRouterChatStream(), 1<<20, 15*time.Millisecond)
		defer body.Close()
		out, err := io.ReadAll(body)
		if err == nil || !idle.guard.fired.Load() || bytes.Contains(out, []byte("[DONE]")) {
			t.Fatalf("shared idle guard did not fail: %s (%v)", out, err)
		}
	})
	for _, gzipBody := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel identity", true: "cancel gzip"}[gzipBody], func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if gzipBody {
					w.Header().Set("Content-Encoding", "gzip")
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer server.Close()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			req.Header.Set("Accept-Encoding", "gzip")
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, _ := routerChatTestReader(t, resp, newDirectRouterChatStream(), 1<<20, time.Second)
			defer body.Close()
			result := make(chan error, 1)
			go func() { _, err := io.ReadAll(body); result <- err }()
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation lost: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancellation did not unblock body")
			}
		})
	}
}
