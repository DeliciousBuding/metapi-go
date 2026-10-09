package proxyhandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func installAxonHubEndpointFixture(t *testing.T, provider, baseURL string, endpoints []map[string]any) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{
		"version": "1.4", "channels": []any{map[string]any{
			"id": 1, "type": provider, "name": "endpoint fixture", "base_url": baseURL,
			"credentials":      map[string]any{"apiKey": "fixture-upstream-key"},
			"supported_models": []string{"provider-model"}, "endpoints": endpoints,
		}},
		"models": []any{map[string]any{"id": 1, "model_id": "client-alias", "status": "enabled",
			"settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "provider-model"}}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backup.ImportAxonHubV14(db, raw, "endpoint-fixture", false); err != nil {
		t.Fatal(err)
	}
	oldRuntime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(oldRuntime) })
	router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
	previous := getUpstreamConfig()
	SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
	t.Cleanup(func() { SetUpstreamConfig(previous) })
}

func TestAxonHubImportedEndpointsReachDistinctUpstreams(t *testing.T) {
	type requestCase struct {
		format, path, body, response, auth string
		handle                             http.HandlerFunc
	}
	cases := []requestCase{
		{"openai/chat_completions", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"temperature":0.3}`, `{"id":"chat-fixture","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, "bearer", HandleChatCompletions},
		{"openai/responses", "/v1/responses", `{"model":"client-alias","input":[{"type":"reasoning","id":"reasoning-fixture","encrypted_content":"opaque","summary":[]},{"role":"user","content":"continue"}],"store":false}`, `{"id":"resp-fixture","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`, "bearer", func(w http.ResponseWriter, r *http.Request) { HandleResponses(w, r, "") }},
		{"anthropic/messages", "/v1/messages", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"metadata":{"user_id":"fixture-user"}}`, `{"id":"msg-fixture","type":"message","role":"assistant","model":"provider-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`, "x-api-key", HandleClaudeMessages},
	}
	var endpoints []map[string]any
	calls := make([]int, len(cases))
	for i, tc := range cases {
		endpointPath := "/endpoint"
		if i == 2 {
			// A provider's custom path is not its wire protocol identity.
			endpointPath = "/v1/chat/completions"
		}
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls[i]++
			if r.URL.Path != "/prefix"+endpointPath || r.URL.RawQuery != "" {
				t.Errorf("%s URL=%s", tc.format, r.URL.String())
			}
			if tc.auth == "x-api-key" {
				if r.Header.Get("x-api-key") != "fixture-upstream-key" || r.Header.Get("Authorization") != "" || r.Header.Get("anthropic-version") == "" {
					t.Errorf("wrong Messages authentication")
				}
			} else if r.Header.Get("Authorization") != "Bearer fixture-upstream-key" || r.Header.Get("x-api-key") != "" {
				t.Errorf("wrong Bearer authentication")
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			var got, want map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Error(err)
			}
			if err := json.Unmarshal([]byte(tc.body), &want); err != nil {
				t.Error(err)
			}
			want["model"] = "provider-model"
			if !reflect.DeepEqual(got, want) {
				t.Errorf("native %s body changed beyond alias: got=%s", tc.format, body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, tc.response)
		}))
		t.Cleanup(upstream.Close)
		endpoints = append(endpoints, map[string]any{"api_format": tc.format, "base_url": upstream.URL + "/prefix", "path": endpointPath})
	}
	installAxonHubEndpointFixture(t, "openai", "http://127.0.0.1:1", endpoints)
	for i, tc := range cases {
		out := httptest.NewRecorder()
		req := makeProxyReq(http.MethodPost, tc.path, tc.body)
		req.Header.Set("Authorization", "Bearer downstream-secret")
		req.Header.Set("x-api-key", "downstream-secret")
		tc.handle(out, req)
		if out.Code != http.StatusOK || calls[i] != 1 {
			t.Fatalf("%s status=%d calls=%d body=%s", tc.format, out.Code, calls[i], out.Body.String())
		}
	}
}

func TestAxonHubProviderDefaultsReachRealHandler(t *testing.T) {
	for _, tc := range []struct{ provider, basePath, wantPath, protocol string }{
		{"doubao", "/api", "/api/v3/chat/completions", "chat"},
		{"zhipu", "/api/paas", "/api/paas/v4/chat/completions", "chat"},
		{"gemini_openai", "", "/v1beta/openai/chat/completions", "chat"},
		{"openrouter", "/api", "/api/chat/completions", "chat"},
		{"xai_responses", "/v1", "/v1/responses", "responses"},
		{"bailian_responses", "/compatible-mode/v1", "/compatible-mode/v1/responses", "responses"},
		{"zenmux_responses", "/api/v1", "/api/v1/responses", "responses"},
		{"openai_responses", "/custom/infer##", "/custom/infer", "responses"},
	} {
		t.Run(tc.provider+tc.basePath, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != tc.wantPath {
					t.Errorf("path=%s want=%s", r.URL.Path, tc.wantPath)
				}
				if r.Header.Get("Authorization") != "Bearer fixture-upstream-key" {
					t.Error("wrong authentication")
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.protocol == "responses" {
					_, _ = io.WriteString(w, `{"id":"resp-fixture","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`)
				} else {
					_, _ = io.WriteString(w, `{"id":"chat-fixture","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
				}
			}))
			defer upstream.Close()
			installAxonHubEndpointFixture(t, tc.provider, upstream.URL+tc.basePath, nil)
			out := httptest.NewRecorder()
			if tc.protocol == "responses" {
				HandleResponses(out, makeProxyReq("POST", "/v1/responses", `{"model":"client-alias","input":"hello"}`), "")
			} else {
				HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
			}
			if out.Code != 200 || calls != 1 {
				t.Fatalf("status=%d calls=%d body=%s", out.Code, calls, out.Body.String())
			}
		})
	}
}

func TestAxonHubCommandCodeMessagesUsesBearer(t *testing.T) {
	calls := 0
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/messages" || r.Header.Get("Authorization") != "Bearer fixture-upstream-key" || r.Header.Get("x-api-key") != "" {
			t.Errorf("Command Code request has wrong path/auth")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg-fixture","type":"message","role":"assistant","model":"provider-model","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer upstream.Close()
	previous := defaultUpstreamClient
	defaultUpstreamClient = upstream.Client()
	t.Cleanup(func() { defaultUpstreamClient = previous })
	installAxonHubEndpointFixture(t, "commandcode_anthropic", upstream.URL+"/api", nil)
	out := httptest.NewRecorder()
	HandleClaudeMessages(out, makeProxyReq("POST", "/v1/messages", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`))
	if out.Code != 200 || calls != 1 {
		t.Fatalf("status=%d calls=%d body=%s", out.Code, calls, out.Body.String())
	}
}
