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

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
	"github.com/google/uuid"
)

func TestDirectOpenCodeInternalWireKeepsLogicalGrant(t *testing.T) {
	config := store.DirectEndpoints{
		Chat:      &store.DirectEndpoint{URL: "https://go.example/chat", Auth: store.DirectAuthBearer, Profile: "opencode-go", ModelWireURLs: &store.DirectModelWireURLs{Responses: "https://go.example/responses", Messages: "https://go.example/messages"}},
		Responses: &store.DirectEndpoint{URL: "https://custom.example/responses", Auth: store.DirectAuthBearer},
	}
	direct := &store.DirectUpstreamCandidate{Provider: "opencode_go", Protocols: store.DirectProtocolChat, Endpoints: config}
	path, err := directSelectedPath(direct, "/v1/responses", "gpt-fixture", false)
	if err != nil || path != "/v1/chat/completions" {
		t.Fatalf("logical selection = %s, %v", path, err)
	}
	resolved, actual, _, err := resolveDirectOpenCodeEndpoint(config.Chat, direct.Provider, path, "gpt-fixture")
	if err != nil || resolved.URL != "https://go.example/responses" || actual != "/v1/responses" || resolved.Auth != store.DirectAuthBearer {
		t.Fatalf("internal Responses = %+v %s %v", resolved, actual, err)
	}
	resolved, actual, _, err = resolveDirectOpenCodeEndpoint(config.Chat, direct.Provider, path, "qwen3-fixture")
	if err != nil || resolved.URL != "https://go.example/messages" || actual != "/v1/messages" || resolved.Auth != store.DirectAuthAPIKey {
		t.Fatalf("internal Messages = %+v %s %v", resolved, actual, err)
	}
	if direct.Protocols != store.DirectProtocolChat || config.Chat.URL != "https://go.example/chat" || config.Chat.ModelWireURLs == nil {
		t.Fatal("wire resolution changed stored endpoints or grant")
	}
	resolved, actual, _, err = resolveDirectOpenCodeEndpoint(config.Responses, direct.Provider, "/v1/responses", "qwen3-fixture")
	if err != nil || resolved != config.Responses || actual != "/v1/responses" {
		t.Fatalf("custom endpoint changed: %+v %s %v", resolved, actual, err)
	}
	if _, _, _, err := resolveDirectOpenCodeEndpoint(config.Chat, "other", path, "gpt-fixture"); err == nil {
		t.Fatal("OpenCode profile accepted a different provider")
	}
}

func TestDirectOpenCodeSessionPrecedenceAndRetryStability(t *testing.T) {
	for _, tc := range []struct {
		name, current, id, affinity, fallback, want string
	}{
		{"current", " current ", "id", "legacy", "context", "current"},
		{"session-id", " \t", " id ", "legacy", "context", "id"},
		{"legacy", "", "", " legacy ", "context", "legacy"},
		{"context", "", "", "", " context ", "context"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := make(http.Header)
			headers.Set("X-Opencode-Session", tc.current)
			headers.Set("X-Session-Id", tc.id)
			headers.Set("X-Session-Affinity", tc.affinity)
			if got := directOpenCodeSessionID(headers, tc.fallback); got != tc.want {
				t.Fatalf("session = %q, want %q", got, tc.want)
			}
		})
	}
	fallback := directOpenCodeSessionID(nil, "")
	if _, err := uuid.Parse(fallback); err != nil {
		t.Fatalf("fallback is not a UUID: %v", err)
	}
	if other := directOpenCodeSessionID(nil, ""); other == fallback {
		t.Fatal("unrelated requests shared a generated session")
	}
	endpoint := &store.DirectEndpoint{URL: "https://example.test/exact?tenant=one", Auth: store.DirectAuthBearer}
	for range 2 {
		wire, err := prepareDirectOpenCodeWire(endpoint, "", []byte(`{"model":"gpt-fixture","input":"hello"}`), nil, fallback)
		if err != nil || wire.Headers.Get("X-Opencode-Session") != fallback {
			t.Fatalf("retry lost request-scoped session: %v", err)
		}
	}
}

func TestDirectOpenCodeFamilyPrefixesAreCaseSensitive(t *testing.T) {
	for _, tc := range []struct {
		model, profile string
		protocol       int
	}{
		{"deepseek-v4-flash", "deepseek", store.DirectProtocolChat},
		{"deepseek", "deepseek", store.DirectProtocolChat},
		{"grok-4.5", "", store.DirectProtocolResponses},
		{"gpt-5.6-luna", "", store.DirectProtocolResponses},
		{"minimax-m3", "", store.DirectProtocolMessages},
		{"qwen3.8-max", "", store.DirectProtocolMessages},
		{"qwen2.5", "", store.DirectProtocolChat},
		{"glm-5.2", "", store.DirectProtocolChat},
		{"DeepSeek-v4", "", store.DirectProtocolChat},
		{"GPT-5.6", "", store.DirectProtocolChat},
		{"MiniMax-m3", "", store.DirectProtocolChat},
		{"Qwen3.8-max", "", store.DirectProtocolChat},
		{"vendor/gpt-5.6", "", store.DirectProtocolChat},
		{" gpt-5.6", "", store.DirectProtocolChat},
	} {
		t.Run(tc.model, func(t *testing.T) {
			protocol, profile := directOpenCodeRouteForModel(tc.model)
			if protocol != tc.protocol || profile != tc.profile {
				t.Fatalf("route = (%d, %q), want (%d, %q)", protocol, profile, tc.protocol, tc.profile)
			}
		})
	}
}

func TestDirectOpenCodeDeepSeekBodyAndExplicitCustomIsolation(t *testing.T) {
	body := []byte(`{"model":"deepseek-v4-flash","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-before","type":"function","function":{"name":"echo","arguments":"{\"number\":9007199254740993}"}}]},{"role":"tool","tool_call_id":"call-before","content":"receipt-result"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"minimum":9007199254740993}}}],"reasoning_effort":"high","response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":{"type":"object"}}},"opaque":{"large":9007199254740993}}`)
	endpoint := &store.DirectEndpoint{URL: "https://example.test/exact/custom?tenant=one", Auth: store.DirectAuthBearer}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer client-secret")
	headers.Set("X-Api-Key", "client-key")
	headers.Set("X-Opencode-Session", "session-fixture")
	original := append([]byte(nil), body...)
	for _, profile := range []string{"deepseek", ""} {
		wire, err := prepareDirectOpenCodeWire(endpoint, profile, body, headers, "fallback")
		if err != nil {
			t.Fatal(err)
		}
		if wire.Endpoint != endpoint || wire.Endpoint.URL != "https://example.test/exact/custom?tenant=one" || wire.ForceStream {
			t.Fatal("wire changed the resolved endpoint or forced streaming")
		}
		if len(wire.Headers) != 1 || wire.Headers.Get("X-Opencode-Session") != "session-fixture" {
			t.Fatalf("provider helper copied unrelated client headers: %v", wire.Headers)
		}
		if profile == "" {
			if !bytes.Equal(wire.Body, original) {
				t.Fatalf("explicit custom Chat changed model-specific fields: %s", wire.Body)
			}
			continue
		}
		for _, marker := range []string{`"thinking":{"type":"enabled"}`, `"reasoning_content":""`, `"response_format":{"type":"json_object"}`, "9007199254740993", "receipt-result", "call-before"} {
			if !bytes.Contains(wire.Body, []byte(marker)) {
				t.Errorf("missing DeepSeek semantics %s: %s", marker, wire.Body)
			}
		}
		if bytes.Contains(wire.Body, []byte("json_schema")) {
			t.Fatalf("DeepSeek retained unsupported schema mode: %s", wire.Body)
		}
	}
	if !bytes.Equal(body, original) {
		t.Fatal("body input was mutated")
	}
	disabled, err := prepareDirectOpenCodeWire(endpoint, "deepseek", []byte(`{"messages":[{"role":"user","content":"hi"}],"reasoning_effort":"none"}`), nil, "fixed")
	if err != nil || !bytes.Contains(disabled.Body, []byte(`"type":"disabled"`)) || bytes.Contains(disabled.Body, []byte(`reasoning_effort`)) {
		t.Fatalf("DeepSeek none was not disabled: %v", err)
	}
	for _, invalid := range []string{`[]`, `{"messages":[]}`, `{"messages":[{"role":"user"}],"response_format":"json_schema"}`} {
		if _, err := prepareDirectOpenCodeWire(endpoint, "deepseek", []byte(invalid), nil, "fixed"); err == nil {
			t.Errorf("accepted invalid DeepSeek body: %s", invalid)
		}
	}
	if _, err := prepareDirectOpenCodeWire(nil, "", body, nil, "fixed"); err == nil {
		t.Error("accepted missing selected endpoint")
	}
	if _, err := prepareDirectOpenCodeWire(endpoint, "unknown", body, nil, "fixed"); err == nil {
		t.Error("accepted unknown body profile")
	}
}

// This exercises the helper -> existing converters -> actual HTTP executor ->
// response/stream bridge and usage logger. It intentionally supplies an already
// selected endpoint: persistent grants and import routing are separate tests.
func TestDirectOpenCodeWireHTTPProtocolMatrix(t *testing.T) {
	for _, family := range []struct {
		model string
		index int
	}{
		{"deepseek-v4-flash", 0}, {"grok-4.5", 1}, {"gpt-5.6-luna", 1},
		{"minimax-m3", 2}, {"qwen3.8-max", 2}, {"glm-5.2", 0}, {"GPT-5.6", 0},
	} {
		for _, down := range directWireFixtures {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream_%t", family.model, down.provider, stream), func(t *testing.T) {
					up := directWireFixtures[family.index]
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						body, _ := io.ReadAll(r.Body)
						var payload map[string]json.RawMessage
						if err := json.Unmarshal(body, &payload); err != nil {
							t.Error(err)
						}
						if r.URL.Path != "/zen/go"+up.path || string(payload["model"]) != fmt.Sprintf("%q", family.model) {
							t.Errorf("wrong final model or route: %s %s", r.URL.Path, body)
						}
						var tools []map[string]json.RawMessage
						if err := json.Unmarshal(payload["tools"], &tools); err != nil || len(tools) != 1 {
							t.Errorf("invalid tool declaration: %s", body)
						} else {
							switch up.provider {
							case "openai_responses":
								if payload["input"] == nil || payload["messages"] != nil || tools[0]["name"] == nil || tools[0]["function"] != nil {
									t.Errorf("wrong request protocol for Responses: %s", body)
								}
							case "anthropic":
								if payload["messages"] == nil || tools[0]["input_schema"] == nil || tools[0]["function"] != nil {
									t.Errorf("wrong request protocol for Messages: %s", body)
								}
							case "openai":
								if payload["messages"] == nil || tools[0]["function"] == nil {
									t.Errorf("wrong request protocol for Chat: %s", body)
								}
							}
						}
						if r.Header.Get("X-Opencode-Session") != "conversation-fixture" {
							t.Error("session affinity lost")
						}
						for _, marker := range []string{"receipt-history", "echo"} {
							if !strings.Contains(string(body), marker) {
								t.Errorf("request lost %s: %s", marker, body)
							}
						}
						if up.provider == "anthropic" {
							if r.Header.Get("X-Api-Key") != "fixture-upstream-key" || r.Header.Get("Authorization") != "" || r.Header.Get("Anthropic-Version") == "" {
								t.Error("wrong Messages authentication or version")
							}
						} else if r.Header.Get("Authorization") != "Bearer fixture-upstream-key" || r.Header.Get("X-Api-Key") != "" {
							t.Error("wrong Chat/Responses authentication")
						}
						if r.Header.Get("X-Goog-Api-Key") != "" {
							t.Error("downstream Google credential leaked")
						}
						if family.model == "deepseek-v4-flash" && !bytes.Contains(body, []byte(`"thinking":{"type":"enabled"}`)) {
							t.Error("DeepSeek profile not applied")
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							for _, frame := range directNativeSSE(up) {
								_, _ = io.WriteString(w, frame)
							}
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, up.response)
						}
					}))
					defer upstream.Close()
					out, log := runOpenCodeWireAttempt(t, upstream.URL+"/zen/go"+up.path, family.model, down, stream)
					if out.Code != http.StatusOK || calls.Load() != 1 {
						t.Fatalf("status=%d calls=%d body=%s", out.Code, calls.Load(), out.Body.String())
					}
					for _, marker := range []string{"receipt-text", "receipt-tool", down.toolMarker} {
						if !strings.Contains(out.Body.String(), marker) {
							t.Errorf("response missing %s: %s", marker, out.Body.String())
						}
					}
					if log.PromptTokens == nil || *log.PromptTokens != 5 || log.CompletionTokens == nil || *log.CompletionTokens != 2 || log.TotalTokens == nil || *log.TotalTokens != 7 {
						t.Errorf("actual wire usage lost: %+v", log)
					}
				})
			}
		}
	}
}

func runOpenCodeWireAttempt(t *testing.T, target, model string, down directWireFixture, stream bool) (*httptest.ResponseRecorder, proxy.ProxyLogEntry) {
	t.Helper()
	protocol, profile := directOpenCodeRouteForModel(model)
	path := proxy.PathForEndpoint(directEndpointFromBit(protocol))
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(down.request), &obj); err != nil {
		t.Fatal(err)
	}
	obj["model"], _ = json.Marshal(model)
	obj["stream"] = json.RawMessage(fmt.Sprint(stream))
	body, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	body, err = directConvertRequest(body, down.path, path, model, stream, messages.Options{})
	if err != nil {
		t.Fatal(err)
	}
	auth := store.DirectAuthBearer
	if protocol == store.DirectProtocolMessages {
		auth = store.DirectAuthAPIKey
	}
	endpoint := &store.DirectEndpoint{URL: target, Auth: auth}
	req := httptest.NewRequest(http.MethodPost, down.path, strings.NewReader(down.request))
	req.Header.Set("X-Session-Affinity", "conversation-fixture")
	req.Header.Set("Authorization", "Bearer downstream-private")
	req.Header.Set("X-Api-Key", "downstream-private")
	req.Header.Set("X-Goog-Api-Key", "downstream-private")
	wire, err := prepareDirectOpenCodeWire(endpoint, profile, body, req.Header, "request-fallback")
	if err != nil {
		t.Fatal(err)
	}
	req = req.WithContext(withDirectProviderWire(req.Context(), wire))
	selected := &routing.SelectedChannel{TokenValue: "fixture-upstream-key", ActualModel: model, Direct: &store.DirectUpstreamCandidate{Provider: "opencode_go"}}
	ctx := &Ctx{DownstreamPath: down.path, RequestedModel: "client-alias", IsStream: stream}
	logs := make(chan proxy.ProxyLogEntry, 1)
	cfg := &UpstreamConfig{LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }}
	out := httptest.NewRecorder()
	finished, pending, cont := dispatchEndpointAttemptWithContinue(out, req, ctx, cfg, selected, model, nil, path, "application/json", wire.Body, 5000, 0, 0, true, true, stream, stream && protocol == store.DirectProtocolChat, "request-fixture", messages.Options{})
	if !finished || pending != nil || cont {
		t.Fatalf("wire attempt did not terminate: finished=%t pending=%v continue=%t", finished, pending, cont)
	}
	select {
	case entry := <-logs:
		return out, entry
	default:
		t.Fatal("missing proxy usage log")
		return nil, proxy.ProxyLogEntry{}
	}
}
