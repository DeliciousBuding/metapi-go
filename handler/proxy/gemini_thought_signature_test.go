package proxyhandler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"

	generate_content "github.com/deliciousbuding/metapi-go/transform/gemini/generate_content"
)

func TestSanitizeUpstreamJSONBody_InjectsGemini3ThoughtSignature(t *testing.T) {
	body := map[string]any{
		"contents": []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "weather?"}},
			},
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{
							"name": "get_weather",
							"args": map[string]any{"city": "Tokyo"},
						},
					},
				},
			},
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"functionResponse": map[string]any{
							"name":     "get_weather",
							"response": map[string]any{"temp": "22C"},
						},
					},
				},
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	out, err := sanitizeUpstreamJSONBody(
		raw,
		"gemini",
		"/v1beta/models/gemini-3-flash-preview:generateContent",
		"gemini-3-flash-preview",
	)
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	var next map[string]any
	if err := json.Unmarshal(out, &next); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	fcParts := collectSanitizeFunctionCallParts(next["contents"])
	if len(fcParts) != 1 {
		t.Fatalf("expected 1 functionCall part, got %d (%s)", len(fcParts), string(out))
	}
	if fcParts[0]["thoughtSignature"] != generate_content.DummyThoughtSignature {
		t.Fatalf("expected dummy thoughtSignature, got %v", fcParts[0]["thoughtSignature"])
	}
}

func TestSanitizeUpstreamJSONBody_PreservesRealThoughtSignature(t *testing.T) {
	body := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{
							"name": "read",
							"args": map[string]any{"path": "/tmp"},
						},
						"thoughtSignature": "real_sig_preserve",
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(body)
	out, err := sanitizeUpstreamJSONBody(
		raw,
		"gemini",
		"/v1beta/models/gemini-3.5-flash:generateContent",
		"gemini-3.5-flash",
	)
	if err != nil {
		t.Fatal(err)
	}
	var next map[string]any
	if err := json.Unmarshal(out, &next); err != nil {
		t.Fatal(err)
	}
	fcParts := collectSanitizeFunctionCallParts(next["contents"])
	if len(fcParts) != 1 || fcParts[0]["thoughtSignature"] != "real_sig_preserve" {
		t.Fatalf("expected preserved real sig, got %#v", fcParts)
	}
}

func TestSanitizeUpstreamJSONBody_GeminiCLIRequestEnvelope(t *testing.T) {
	body := map[string]any{
		"model": "gemini-3-pro-preview",
		"request": map[string]any{
			"contents": []any{
				map[string]any{
					"role": "model",
					"parts": []any{
						map[string]any{
							"functionCall": map[string]any{
								"name": "tool_a",
								"args": map[string]any{},
							},
						},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(body)
	out, err := sanitizeUpstreamJSONBody(raw, "gemini-cli", "/v1internal::generateContent", "gemini-3-pro-preview")
	if err != nil {
		t.Fatal(err)
	}
	var next map[string]any
	if err := json.Unmarshal(out, &next); err != nil {
		t.Fatal(err)
	}
	req, ok := next["request"].(map[string]any)
	if !ok {
		t.Fatalf("expected request envelope, got %T", next["request"])
	}
	fcParts := collectSanitizeFunctionCallParts(req["contents"])
	if len(fcParts) != 1 || fcParts[0]["thoughtSignature"] != generate_content.DummyThoughtSignature {
		t.Fatalf("expected CLI envelope inject, got %#v full=%s", fcParts, string(out))
	}
}

func TestSanitizeUpstreamJSONBody_OpenAIMessagesOnNativePath(t *testing.T) {
	body := map[string]any{
		"model": "gemini-3-flash-preview",
		"messages": []any{
			map[string]any{"role": "user", "content": "hi"},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_1",
						"type": "function",
						"function": map[string]any{
							"name":      "ping",
							"arguments": `{}`,
						},
						"provider_specific_fields": map[string]any{
							"thought_signature": "from_openai_psf",
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_1", "content": `{"ok":true}`},
		},
	}
	raw, _ := json.Marshal(body)
	out, err := sanitizeUpstreamJSONBody(
		raw,
		"gemini",
		"/v1beta/models/gemini-3-flash-preview:generateContent",
		"gemini-3-flash-preview",
	)
	if err != nil {
		t.Fatal(err)
	}
	var next map[string]any
	if err := json.Unmarshal(out, &next); err != nil {
		t.Fatal(err)
	}
	if _, ok := next["contents"]; !ok {
		t.Fatalf("expected OpenAI→Gemini rebuild with contents, got keys %v", mapKeys(next))
	}
	fcParts := collectSanitizeFunctionCallParts(next["contents"])
	if len(fcParts) != 1 || fcParts[0]["thoughtSignature"] != "from_openai_psf" {
		t.Fatalf("expected preserved OpenAI provider sig, got %#v body=%s", fcParts, string(out))
	}
}

func TestSanitizeUpstreamJSONBody_SkipsNonGeminiPlatform(t *testing.T) {
	body := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{"name": "x", "args": map[string]any{}},
					},
				},
			},
		},
	}
	raw, _ := json.Marshal(body)
	out, err := sanitizeUpstreamJSONBody(raw, "openai", "/v1beta/models/gemini-3:generateContent", "gemini-3-flash-preview")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(raw) {
		// Non-gemini platform must not rewrite even on generateContent path.
		t.Fatalf("expected passthrough for non-gemini platform")
	}
}

func TestSanitizeUpstreamJSONBody_SkipsWithoutToolHistory(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`)
	out, err := sanitizeUpstreamJSONBody(body, "gemini", "/v1beta/models/gemini-3:generateContent", "gemini-3-flash-preview")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(body) {
		t.Fatalf("expected no rewrite without functionCall/tool_calls")
	}
}

func TestNeedsGeminiThoughtSignatureSanitize(t *testing.T) {
	raw := []byte(`{"contents":[{"parts":[{"functionCall":{"name":"a"}}]}]}`)
	if !needsGeminiThoughtSignatureSanitize("gemini", "/v1beta/models/x:generateContent", raw) {
		t.Fatal("expected true for gemini generateContent + functionCall")
	}
	if needsGeminiThoughtSignatureSanitize("openai", "/v1beta/models/x:generateContent", raw) {
		t.Fatal("expected false for non-gemini platform")
	}
	if needsGeminiThoughtSignatureSanitize("gemini", "/v1/chat/completions", raw) {
		t.Fatal("expected false for OpenAI chat path")
	}
	if needsGeminiThoughtSignatureSanitize("gemini", "/v1beta/models/x:generateContent", []byte(`{"contents":[]}`)) {
		t.Fatal("expected false without tool markers")
	}
}

func collectSanitizeFunctionCallParts(contents any) []map[string]any {
	var out []map[string]any
	switch arr := contents.(type) {
	case []any:
		for _, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch parts := m["parts"].(type) {
			case []any:
				for _, p := range parts {
					pm, ok := p.(map[string]any)
					if !ok {
						continue
					}
					if _, ok := pm["functionCall"]; ok {
						out = append(out, pm)
					}
				}
			}
		}
	}
	return out
}

func mapKeys(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ",")
}

func TestSiteSanitizedToolNumberOnWire(t *testing.T) {
	for _, test := range []struct {
		path, platform, model, body string
		handler                     http.HandlerFunc
	}{
		{"/v1beta/models/gemini-3-flash:generateContent", "gemini", "gemini-3-flash", `{"contents":[{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"id":9007199254740993}}}]}]}`, HandleGeminiGenerateContent},
		{"/v1/responses", "openai", "fixture", `{"model":"fixture","input":[{"role":"user","content":"hello"}],"tools":[{"type":"function","name":"lookup","parameters":{"properties":{"id":{"const":9007199254740993}}}}]}`, func(w http.ResponseWriter, r *http.Request) { HandleResponses(w, r, r.URL.Path) }},
	} {
		t.Run(test.platform, func(t *testing.T) {
			observed := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				observed <- string(b)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"resp","output":[]}`)
			}))
			defer server.Close()
			previous := getUpstreamConfig()
			t.Cleanup(func() { SetUpstreamConfig(previous) })
			SetUpstreamConfig(&UpstreamConfig{Router: &upstreamTestRouter{selected: routing.SelectedChannel{Channel: store.RouteChannel{ID: 42, Enabled: true}, Account: store.Account{ID: 7, Status: "active"}, Site: store.Site{ID: 3, URL: server.URL, Platform: test.platform, Status: "active"}, ActualModel: test.model, TokenValue: "synthetic"}}})
			out := httptest.NewRecorder()
			test.handler(out, makeProxyReq("POST", test.path, test.body))
			select {
			case got := <-observed:
				if !strings.Contains(got, "9007199254740993") {
					t.Fatalf("tool integer changed: %s", got)
				}
			default:
				t.Fatalf("no dispatch: %d %s", out.Code, out.Body.String())
			}
		})
	}
}

func TestSanitizePreservesInvalidTrailingJSON(t *testing.T) {
	for _, tail := range []string{" {}", " false", " trailing"} {
		raw := `{"input":"hello","model":"fixture"}` + tail
		got, err := sanitizeUpstreamJSONBody([]byte(raw), "openai", "/v1/responses", "fixture")
		if err != nil || string(got) != raw {
			t.Fatalf("invalid body truncated: %s, %v", got, err)
		}
	}
}
