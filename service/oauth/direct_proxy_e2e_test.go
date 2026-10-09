package oauth_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestImportedClaudeOAuthRefreshAndToolRelayHTTP(t *testing.T) {
	var refreshCalls, relayCalls atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshCalls.Add(1)
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["refresh_token"] != "fixture-refresh" || body["client_id"] != "source-client" {
			t.Error("original refresh request was not preserved")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"fixture-fresh-token","refresh_token":"fixture-rotated-refresh","expires_in":3600}`)
	}))
	defer tokenServer.Close()
	restore := oauth.OverrideClaudeTokenURLForTest(tokenServer.URL)
	defer restore()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		relayCalls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer fixture-fresh-token" || r.Header.Get("x-api-key") != "" || r.URL.Query().Get("beta") != "true" {
			t.Error("selected refreshed Claude credential/wire not applied")
		}
		if !strings.Contains(string(body), `"name":"proxy_echo"`) {
			t.Errorf("Claude OAuth tool prefix not applied: %s", body)
		}
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		if request["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			events := []string{
				`{"type":"message_start","message":{"id":"msg-fixture","type":"message","role":"assistant","model":"provider-model","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":0}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call-fixture","name":"proxy_echo","input":{}}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"value\":\"receipt\"}"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":2}}`,
				`{"type":"message_stop"}`,
			}
			for _, event := range events {
				_, _ = io.WriteString(w, "data: "+event+"\n\n")
				w.(http.Flusher).Flush()
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"msg-fixture","type":"message","role":"assistant","model":"provider-model","content":[{"type":"tool_use","id":"call-fixture","name":"proxy_echo","input":{"value":"receipt"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":5,"output_tokens":2}}`)
		}
	}))
	defer upstream.Close()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"version": "1.4", "channels": []any{map[string]any{"id": 1, "type": "claudecode", "name": "fixture", "base_url": upstream.URL, "credentials": map[string]any{"oauth": map[string]any{"access_token": "fixture-old-token", "refresh_token": "fixture-refresh", "client_id": "source-client", "expires_at": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}}, "supported_models": []string{"provider-model"}}}, "models": []any{map[string]any{"id": 1, "model_id": "client-alias", "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "provider-model"}}}}}}})
	if _, err = backup.ImportAxonHubV14(db, payload, "oauth-http-fixture", false); err != nil {
		t.Fatal(err)
	}
	oldRuntime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	defer config.SetRuntime(oldRuntime)
	logs := make(chan proxy.ProxyLogEntry, 16)
	proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{Router: routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil), ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
		return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
	}, LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }})
	defer proxyhandler.SetUpstreamConfig(nil)
	for _, client := range []string{"chat", "responses", "messages", "gemini"} {
		for _, stream := range []bool{false, true} {
			t.Run(client+map[bool]string{true: "_stream", false: "_json"}[stream], func(t *testing.T) {
				path, body := claudeClientRequest(client, stream)
				downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
					switch client {
					case "chat":
						proxyhandler.HandleChatCompletions(w, r)
					case "responses":
						proxyhandler.HandleResponses(w, r, "")
					case "messages":
						proxyhandler.HandleClaudeMessages(w, r)
					case "gemini":
						proxyhandler.HandleGeminiGenerateContent(w, r)
					}
				}))
				defer downstream.Close()
				resp, err := http.Post(downstream.URL+path, "application/json", strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				output, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != 200 || !strings.Contains(string(output), `"name":"echo"`) || !strings.Contains(string(output), "receipt") || strings.Contains(string(output), "proxy_echo") {
					t.Fatalf("tool restoration failed: %d %s", resp.StatusCode, output)
				}
				select {
				case entry := <-logs:
					if entry.PromptTokens == nil || *entry.PromptTokens != 5 || entry.TotalTokens == nil || *entry.TotalTokens != 7 || entry.Status != "success" {
						t.Errorf("wrong actual usage/status: %+v", entry)
					}
				default:
					t.Error("missing relay log")
				}
			})
		}
	}
	if refreshCalls.Load() != 1 || relayCalls.Load() != 8 {
		t.Fatalf("refresh=%d relay=%d", refreshCalls.Load(), relayCalls.Load())
	}
	var secret string
	var state store.DirectOAuthState
	if err := db.QueryRow(`SELECT secret,oauth_state FROM upstream_credentials`).Scan(&secret, &state); err != nil {
		t.Fatal(err)
	}
	if secret != "fixture-fresh-token" || state.RefreshToken != "fixture-rotated-refresh" {
		t.Fatal("refreshed state was not persisted")
	}
}

func claudeClientRequest(client string, stream bool) (string, string) {
	path := "/v1/chat/completions"
	var body map[string]any
	switch client {
	case "chat":
		body = map[string]any{"model": "client-alias", "messages": []any{map[string]any{"role": "user", "content": "call echo"}}, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "echo", "parameters": map[string]any{"type": "object"}}}}}
	case "responses":
		path = "/v1/responses"
		body = map[string]any{"model": "client-alias", "input": "call echo", "tools": []any{map[string]any{"type": "function", "name": "echo", "parameters": map[string]any{"type": "object"}}}}
	case "messages":
		path = "/v1/messages"
		body = map[string]any{"model": "client-alias", "max_tokens": 32, "messages": []any{map[string]any{"role": "user", "content": "call echo"}}, "tools": []any{map[string]any{"name": "echo", "input_schema": map[string]any{"type": "object"}}}}
	case "gemini":
		path = "/v1beta/models/client-alias:generateContent"
		body = map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]any{"text": "call echo"}}}}, "tools": []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "echo", "parameters": map[string]any{"type": "OBJECT"}}}}}}
	}
	if client == "gemini" {
		if stream {
			path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
		}
	} else {
		body["stream"] = stream
	}
	raw, _ := json.Marshal(body)
	return path, string(raw)
}
