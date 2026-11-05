package backup_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/tidwall/gjson"
)

func TestAxonHubRequestOperationsReachWireWithoutChangingToolsOrStream(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				if gjson.GetBytes(body, "model").String() != "provider-model" ||
					gjson.GetBytes(body, "stream").Bool() != stream ||
					gjson.GetBytes(body, "temperature").Float() != .25 ||
					gjson.GetBytes(body, "metadata.client").String() != "client-alias" ||
					gjson.GetBytes(body, "messages.0.role").String() != "system" ||
					gjson.GetBytes(body, "reasoning_effort").String() != "max" ||
					!strings.Contains(string(body), "9007199254740993") ||
					r.Header.Get("X-Source") != "" || r.Header.Get("X-Copied") != "first" ||
					r.Header.Get("X-Mapped") != "provider-model" || r.Header.Get("X-Private") != "" ||
					r.Header.Get("Authorization") != "Bearer fixture-media-key" {
					t.Errorf("unexpected transformed wire: %s headers=%v", body, r.Header)
				}
				providerWireResponse(w, "chat", "provider-model", stream, false)
			}))
			defer upstream.Close()
			router := installAxonHubHTTPFixture(t, "qiniu", upstream.URL, "", "", "chat", func(channel map[string]any) {
				channel["settings"] = map[string]any{
					"transformOptions": map[string]any{"replaceDeveloperRoleWithSystem": true, "reasoningEffortMapping": []any{map[string]any{"from": "high", "to": "max"}}},
					"bodyOverrideOperations": []any{
						map[string]any{"op": "set", "path": "temperature", "value": "0.25"},
						map[string]any{"op": "set", "path": "metadata.client", "value": "{{.RequestModel}}"},
						map[string]any{"op": "copy", "from": "tools", "to": "tool_copy"},
						map[string]any{"op": "array_append", "path": "tools", "value": "[]"},
					},
					"headerOverrideOperations": []any{
						map[string]any{"op": "set", "path": "X-Source", "value": "first"},
						map[string]any{"op": "copy", "from": "X-Source", "to": "X-Copied"},
						map[string]any{"op": "delete", "path": "X-Source"},
						map[string]any{"op": "set", "path": "X-Mapped", "value": "{{.Model}}"},
						map[string]any{"op": "set", "path": "X-Private", "value": `{{index .RequestHeader "Authorization"}}`},
					},
				}
			})
			body := fmt.Sprintf(`{"model":"client-alias","stream":%t,"reasoning_effort":"high","messages":[{"role":"developer","content":"rule"},{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object","properties":{"number":{"const":9007199254740993}}}}}]}`, stream)
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", body)
			if out.Code != 200 || calls.Load() != 1 || !strings.Contains(out.Body.String(), "provider-receipt") {
				t.Fatalf("relay=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}

func TestAxonHubRequestOperationsToolsAndCancellation(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if gjson.GetBytes(body, "metadata.alias").String() != "client-alias" {
			t.Errorf("ordered template missing: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), `"role":"tool"`) {
			if !strings.Contains(string(body), `"tool_call_id":"call-fixture"`) {
				t.Errorf("tool round-trip identity changed: %s", body)
			}
			providerWireResponse(w, "chat", "provider-model", false, false)
		} else {
			_, _ = io.WriteString(w, `{"id":"fixture","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-fixture","type":"function","function":{"name":"echo","arguments":"{\"value\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
		}
	}))
	defer upstream.Close()
	router := installAxonHubHTTPFixture(t, "qiniu", upstream.URL, "", "", "chat", func(channel map[string]any) {
		channel["settings"] = map[string]any{"bodyOverrideOperations": []any{
			map[string]any{"op": "set", "path": "metadata.alias", "value": "{{.RequestModel}}"},
		}}
	})
	first := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"echo"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object"}}}]}`)
	if first.Code != 200 || !strings.Contains(first.Body.String(), "9007199254740993") {
		t.Fatalf("tool response=%d %s", first.Code, first.Body.String())
	}
	second := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"echo"},{"role":"assistant","tool_calls":[{"id":"call-fixture","type":"function","function":{"name":"echo","arguments":"{\"value\":9007199254740993}"}}]},{"role":"tool","tool_call_id":"call-fixture","content":"9007199254740993"}]}`)
	if second.Code != 200 || !strings.Contains(second.Body.String(), "provider-receipt") || calls.Load() != 2 {
		t.Fatalf("tool return=%d %s calls=%d", second.Code, second.Body.String(), calls.Load())
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"client-alias","messages":[{"role":"user","content":"cancelled"}]}`))
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(auth.WithProxyAuth(ctx, &auth.ProxyAuthContext{Source: "global", Token: "synthetic-downstream"}))
	out := httptest.NewRecorder()
	router.ServeHTTP(out, req)
	if calls.Load() != 2 {
		t.Fatal("cancelled request reached the upstream")
	}
}

func TestAxonHubRequestTransformProtocolSerializerOptions(t *testing.T) {
	for _, protocol := range []string{"messages", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				field := "system"
				if protocol == "responses" {
					field = "input"
				}
				if !gjson.GetBytes(body, field).IsArray() {
					t.Errorf("%s not serialized as array: %s", field, body)
				}
				providerWireResponse(w, protocol, "provider-model", false, false)
			}))
			defer upstream.Close()
			provider, format := "anthropic", "anthropic/messages"
			body := `{"model":"client-alias","system":"rule","messages":[{"role":"user","content":"hello"}],"max_tokens":16}`
			if protocol == "responses" {
				provider, format = "openai_responses", "openai/responses"
				body = `{"model":"client-alias","input":"hello","max_output_tokens":16}`
			}
			router := installAxonHubHTTPFixture(t, provider, upstream.URL, format, "", "chat", func(channel map[string]any) {
				settings := channel["settings"].(map[string]any)
				settings["transformOptions"] = map[string]any{"forceArrayInstructions": true, "forceArrayInputs": true}
			})
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/"+protocol, body)
			if out.Code != 200 || !strings.Contains(out.Body.String(), "provider-receipt") {
				t.Fatalf("relay=%d body=%s", out.Code, out.Body.String())
			}
		})
	}
}

func TestAxonHubRequestOperationsDoNotAccumulateAcrossEndpointFallback(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				if strings.Count(string(body), "operation-marker") != 1 {
					t.Errorf("attempt body was mutated twice: %s", body)
				}
				if strings.HasSuffix(r.URL.Path, "chat/completions") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"error":{"message":"unsupported endpoint"}}`)
					return
				}
				providerWireResponse(w, "messages", "provider-model", stream, false)
			}))
			defer upstream.Close()
			router := installAxonHubHTTPFixture(t, "qiniu", upstream.URL, "", "", "chat", func(channel map[string]any) {
				channel["endpoints"] = []any{map[string]any{"api_format": "anthropic/messages", "path": "/messages"}}
				channel["settings"] = map[string]any{"bodyOverrideOperations": []any{
					map[string]any{"op": "array_append", "path": "messages", "value": `{"role":"user","content":"operation-marker"}`},
				}}
			})
			raw := fmt.Sprintf(`{"model":"client-alias","stream":%t,"messages":[{"role":"user","content":"hello"}],"max_tokens":32}`, stream)
			out := requestAxonHubNativeHTTP(router, http.MethodPost, "/v1/chat/completions", raw)
			if out.Code != 200 || calls.Load() != 2 || !strings.Contains(out.Body.String(), "provider-receipt") {
				t.Fatalf("fallback=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
			}
		})
	}
}
