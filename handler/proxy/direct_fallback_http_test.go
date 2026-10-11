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

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
)

var directFallbackFormats = map[string]string{"chat": "openai/chat_completions", "responses": "openai/responses", "messages": "anthropic/messages", "gemini": "gemini/contents"}
var directFallbackReplies = map[string]string{
	"chat":      `{"id":"chat_fixture","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	"responses": `{"id":"resp_fixture","object":"response","status":"completed","model":"provider-model","output":[{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"ok","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
	"messages":  `{"id":"msg_fixture","type":"message","role":"assistant","model":"provider-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`,
	"gemini":    `{"responseId":"gemini_fixture","modelVersion":"provider-model","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
}

type directFallbackFixture struct {
	mu       sync.Mutex
	calls    []string
	logs     []proxy.ProxyLogEntry
	failures map[string]int
	body     string
	partial  bool
}

func (f *directFallbackFixture) results() ([]string, []proxy.ProxyLogEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...), append([]proxy.ProxyLogEntry(nil), f.logs...)
}

func installDirectFallbackFixture(t *testing.T, order []string, failures map[string]int, body string, partial bool) *directFallbackFixture {
	t.Helper()
	f := &directFallbackFixture{failures: failures, body: body, partial: partial}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := strings.TrimPrefix(r.URL.Path, "/wire/")
		if i := strings.IndexAny(kind, "/:"); i >= 0 {
			kind = kind[:i]
		}
		f.mu.Lock()
		f.calls = append(f.calls, kind)
		f.mu.Unlock()
		request, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(request, &payload); err != nil {
			t.Errorf("invalid converted %s JSON: %s", kind, request)
		}
		switch kind {
		case "chat", "messages":
			if payload["messages"] == nil || payload["input"] != nil || payload["contents"] != nil || string(payload["model"]) != `"provider-model"` {
				t.Errorf("%s received wrong protocol/model: %s", kind, request)
			}
		case "responses":
			if payload["input"] == nil || payload["messages"] != nil || string(payload["model"]) != `"provider-model"` {
				t.Errorf("Responses received wrong protocol/model: %s", request)
			}
		case "gemini":
			if payload["contents"] == nil || payload["messages"] != nil || payload["model"] != nil || !strings.Contains(r.URL.Path, "provider-model:") {
				t.Errorf("Gemini received wrong protocol/model: %s %s", r.URL.Path, request)
			}
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
		if kind == "messages" {
			if r.Header.Get("X-Api-Key") != "fixture-key" || r.Header.Get("Authorization") != "" {
				t.Error("Messages fallback did not rebuild authentication")
			}
		} else if kind == "gemini" {
			if r.Header.Get("X-Goog-Api-Key") != "fixture-key" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" {
				t.Error("Gemini fallback did not rebuild authentication")
			}
		} else if r.Header.Get("Authorization") != "Bearer fixture-key" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Goog-Api-Key") != "" {
			t.Error("Bearer fallback did not rebuild authentication")
		}
		if status := f.failures[kind]; status != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, f.body)
			return
		}
		if f.partial && kind == "chat" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+`{"id":"partial","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","content":"already generated"}}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`+"\n\n")
			w.(http.Flusher).Flush()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, directFallbackReplies[kind])
	}))
	t.Cleanup(server.Close)
	var endpoints []any
	for _, kind := range []string{"chat", "responses", "messages", "gemini"} {
		baseURL, path := server.URL, "/wire/"+kind
		if kind == "gemini" {
			baseURL, path = server.URL+"/wire/gemini", ""
		}
		endpoints = append(endpoints, map[string]any{"api_format": directFallbackFormats[kind], "base_url": baseURL, "path": path})
	}
	var formats []string
	for _, kind := range order {
		formats = append(formats, directFallbackFormats[kind])
	}
	raw, err := json.Marshal(map[string]any{
		"version": "1.4", "channels": []any{map[string]any{
			"id": 1, "type": "openai", "name": "fallback fixture", "base_url": server.URL,
			"credentials": map[string]any{"apiKey": "fixture-key"}, "supported_models": []string{"provider-model"}, "endpoints": endpoints,
			"settings": map[string]any{"modelProtocols": []any{map[string]any{"model": "provider-model", "apiFormats": formats}}},
		}},
		"models": []any{map[string]any{"id": 1, "model_id": "client-alias", "status": "enabled", "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "provider-model"}}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	installAxonHubRawFixture(t, raw)
	cfg := *getUpstreamConfig()
	cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.logs = append(f.logs, entry)
		return nil
	}
	SetUpstreamConfig(&cfg)
	return f
}

func directFallbackHTTPRequest(client string, stream bool) *httptest.ResponseRecorder {
	path, body, handle := "/v1/chat/completions", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`, http.HandlerFunc(HandleChatCompletions)
	switch client {
	case "messages":
		path, handle = "/v1/messages", HandleClaudeMessages
	case "responses":
		path, body = "/v1/responses", `{"model":"client-alias","max_output_tokens":20,"input":[{"role":"user","content":"hello"}]}`
		handle = func(w http.ResponseWriter, r *http.Request) { HandleResponses(w, r, "/v1/responses") }
	case "gemini":
		path, body, handle = "/v1beta/models/client-alias:generateContent", `{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":20}}`, HandleGeminiGenerateContent
	}
	if stream {
		body = strings.TrimSuffix(body, "}") + `,"stream":true}`
	}
	req := makeProxyReq(http.MethodPost, path, body)
	req.Header.Set("X-Api-Key", "downstream-must-not-leak")
	req.Header.Set("X-Goog-Api-Key", "downstream-must-not-leak")
	req.Header.Set("Authorization", "Bearer downstream-must-not-leak")
	out := httptest.NewRecorder()
	handle(out, req)
	return out
}

func TestDirectFallbackHTTPProtocolMatrix(t *testing.T) {
	for _, client := range []string{"chat", "responses", "messages", "gemini"} {
		for _, next := range []string{"chat", "responses", "messages", "gemini"} {
			t.Run(client+"_to_"+next, func(t *testing.T) {
				order := []string{next, client}
				failures := map[string]int{client: 400}
				want := []string{client, next}
				if client == next {
					order, failures, want = []string{client}, nil, []string{client}
				}
				f := installDirectFallbackFixture(t, order, failures, `{"error":{"code":"model_not_found","message":"model not supported by this endpoint"}}`, false)
				out := directFallbackHTTPRequest(client, false)
				calls, logs := f.results()
				if out.Code != 200 || !strings.Contains(out.Body.String(), "ok") || !reflect.DeepEqual(calls, want) {
					t.Fatalf("status=%d calls=%v want=%v body=%s", out.Code, calls, want, out.Body.String())
				}
				if len(logs) != 1 || logs[0].Status != "success" || logs[0].TotalTokens == nil || *logs[0].TotalTokens != 2 {
					t.Fatalf("miss was billed/poisoned or success accounting lost: %+v", logs)
				}
			})
		}
	}
}

func TestDirectFallbackHTTPStopsUnsafeReplay(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", 401, `{"error":"unsupported model"}`},
		{"forbidden", 403, `{"error":"unsupported model"}`},
		{"rate limit", 429, `{"error":"unsupported model"}`},
		{"generic missing", 404, `{"error":"not found"}`},
		{"paid rejection", 400, `{"error":"unsupported model","usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`},
		{"already output", 400, `{"error":"unsupported model","choices":[{"message":{"content":"already generated"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := installDirectFallbackFixture(t, []string{"chat", "messages"}, map[string]int{"chat": tc.status}, tc.body, false)
			out := directFallbackHTTPRequest("chat", false)
			calls, logs := f.results()
			if out.Code == 200 || !reflect.DeepEqual(calls, []string{"chat"}) || len(logs) != 1 || logs[0].Status == "success" {
				t.Fatalf("unsafe rejection replayed: status=%d calls=%v logs=%+v", out.Code, calls, logs)
			}
			if tc.name == "paid rejection" && (logs[0].TotalTokens == nil || *logs[0].TotalTokens != 3) {
				t.Fatalf("paid failure usage lost: %+v", logs[0])
			}
		})
	}
	t.Run("operator disabled", func(t *testing.T) {
		f := installDirectFallbackFixture(t, []string{"chat", "messages"}, map[string]int{"chat": 400}, `{"error":"unsupported model"}`, false)
		config.SetRuntime(&config.RuntimeSettings{DisableCrossProtocolFallback: true})
		out := directFallbackHTTPRequest("chat", false)
		calls, _ := f.results()
		if out.Code == 200 || !reflect.DeepEqual(calls, []string{"chat"}) {
			t.Fatalf("disabled fallback ran: %d %v", out.Code, calls)
		}
	})
	t.Run("partial stream", func(t *testing.T) {
		f := installDirectFallbackFixture(t, []string{"chat", "messages"}, nil, "", true)
		out := directFallbackHTTPRequest("chat", true)
		calls, logs := f.results()
		if !reflect.DeepEqual(calls, []string{"chat"}) || len(logs) != 1 || logs[0].Status == "success" || !strings.Contains(out.Body.String(), "already generated") || logs[0].TotalTokens == nil || *logs[0].TotalTokens != 3 {
			t.Fatalf("partial stream replayed or lost usage: calls=%v logs=%+v body=%s", calls, logs, out.Body.String())
		}
	})
}

func TestDirectFallbackLocalConversionSkipsWithoutWriting(t *testing.T) {
	f := installDirectFallbackFixture(t, []string{"messages", "chat", "responses"}, map[string]int{"responses": 400}, `{"error":"unsupported model"}`, false)
	req := makeProxyReq(http.MethodPost, "/v1/responses", `{"model":"client-alias","max_output_tokens":20,"text":{"format":{"type":"json_object"}},"input":[{"role":"user","content":"hello"}]}`)
	out := httptest.NewRecorder()
	HandleResponses(out, req, "/v1/responses")
	calls, logs := f.results()
	if out.Code != 200 || !json.Valid(out.Body.Bytes()) || !reflect.DeepEqual(calls, []string{"responses", "chat"}) || len(logs) != 1 || logs[0].Status != "success" {
		t.Fatalf("local conversion wrote a response, lost original body, or made I/O: status=%d calls=%v logs=%+v body=%s", out.Code, calls, logs, out.Body.String())
	}
}

func TestDirectFallbackExhaustsEachProtocolOnce(t *testing.T) {
	f := installDirectFallbackFixture(t, []string{"gemini", "messages", "responses", "chat"}, map[string]int{"chat": 400, "responses": 400, "messages": 400, "gemini": 400}, `{"error":{"code":"model_not_found","message":"model not supported"}}`, false)
	out := directFallbackHTTPRequest("chat", false)
	calls, logs := f.results()
	if out.Code != 400 || !reflect.DeepEqual(calls, []string{"chat", "gemini", "messages", "responses"}) || len(logs) != 1 || logs[0].Status == "success" || !strings.Contains(out.Body.String(), "model_not_found") {
		t.Fatalf("exhaustion repeated a protocol or lost the final error: status=%d calls=%v logs=%+v body=%s", out.Code, calls, logs, out.Body.String())
	}
}
