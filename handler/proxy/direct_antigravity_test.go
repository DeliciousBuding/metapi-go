package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectAntigravityImportedJSONAndSSE(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, down := range directWireFixtures {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", dialect, down.provider, stream), func(t *testing.T) {
					calls := 0
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						want := "/v1internal:generateContent"
						if stream {
							want = "/v1internal:streamGenerateContent"
						}
						if r.URL.Path != want || (r.URL.Query().Get("alt") == "sse") != stream {
							t.Errorf("wrong action: %s", r.URL.String())
						}
						if r.Header.Get("Authorization") != "Bearer synthetic-access" || r.Header.Get("X-Goog-Api-Key") != "" || r.Header.Get("X-Api-Key") != "" {
							t.Error("wrong OAuth boundary")
						}
						var envelope struct {
							Project, Model, RequestID string
							Request                   json.RawMessage
						}
						if json.NewDecoder(r.Body).Decode(&envelope) != nil || envelope.Project != "synthetic-project" ||
							envelope.Model != "provider-model" || !strings.HasPrefix(envelope.RequestID, "agent-") ||
							!strings.Contains(string(envelope.Request), "receipt-history") {
							t.Errorf("request envelope lost semantics: %+v", envelope)
						}
						raw := `{"response":` + directWireFixtures[3].response + `}`
						if calls == 1 {
							if !strings.Contains(string(envelope.Request), `"mode":"VALIDATED"`) {
								t.Error("source tool mode was not applied")
							}
							if down.provider == "gemini" {
								raw = strings.Replace(raw, `"functionCall":`, `"thoughtSignature":"signature-fixture","functionCall":`, 1)
							}
						} else {
							if !strings.Contains(string(envelope.Request), `"functionResponse"`) || !strings.Contains(string(envelope.Request), "receipt-followup") {
								t.Errorf("tool result did not return to provider: %s", envelope.Request)
							}
							if down.provider == "gemini" && !strings.Contains(string(envelope.Request), `"thoughtSignature":"signature-fixture"`) {
								t.Error("native thought signature did not round trip")
							}
							raw = `{"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"receipt-final"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":2,"totalTokenCount":10}}}`
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = fmt.Fprint(w, "data: "+raw+"\n\n")
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, raw)
						}
					}))
					defer upstream.Close()
					endpoints := store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: upstream.URL + "/v1internal", Auth: store.DirectAuthBearer, Profile: "antigravity"}}
					db := installDirectMediaRuntime(t, dialect, endpoints, store.DirectProtocolGemini, "")
					raw, _ := json.Marshal(map[string]any{
						"version": "1.4", "channels": []any{map[string]any{
							"id": 1, "type": "antigravity", "name": "synthetic",
							"credentials":      map[string]any{"apiKey": "synthetic-refresh|synthetic-project"},
							"supported_models": []string{"provider-model"},
						}},
						"models": []any{map[string]any{
							"id": 1, "model_id": "client-alias", "settings": map[string]any{
								"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "provider-model"}}},
							},
						}},
					})
					if _, err := backup.ImportAxonHubV14(db, raw, "media-fixture", false); err != nil {
						t.Fatal(err)
					}
					// The synthetic transport overrides only the destination after the
					// real source import; grants, credential kind and project come from it.
					if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET endpoint_config=?`), endpoints); err != nil {
						t.Fatal(err)
					}
					definition := oauth.GetProviderDefinition("antigravity")
					previous := definition.RefreshAccessToken
					defer func() { definition.RefreshAccessToken = previous }()
					exchanges := 0
					definition.RefreshAccessToken = func(_ context.Context, input oauth.RefreshTokenInput) (*oauth.TokenSet, error) {
						exchanges++
						if input.RefreshToken != "synthetic-refresh" || input.OAuth == nil || input.OAuth.ProjectID != "synthetic-project" {
							t.Error("lazy credential resolution lost imported identity")
						}
						return &oauth.TokenSet{AccessToken: "synthetic-access", TokenExpiresAt: time.Now().Add(time.Hour).UnixMilli(), ProjectID: input.OAuth.ProjectID}, nil
					}
					var usage proxy.ProxyLogEntry
					getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { usage = entry; return nil }
					body, path := down.request, down.path
					if stream {
						if down.provider == "gemini" {
							path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
						} else {
							var object map[string]any
							_ = json.Unmarshal([]byte(body), &object)
							object["stream"] = true
							encoded, _ := json.Marshal(object)
							body = string(encoded)
						}
					}
					out := httptest.NewRecorder()
					down.handler(out, makeProxyReq(http.MethodPost, path, body))
					if out.Code != http.StatusOK || calls != 1 {
						t.Fatalf("status=%d calls=%d body=%s", out.Code, calls, out.Body.String())
					}
					if exchanges != 1 {
						t.Fatalf("empty imported token was not exchanged once: %d", exchanges)
					}
					for _, marker := range []string{"receipt-text", "receipt-tool", down.toolMarker} {
						if !strings.Contains(out.Body.String(), marker) {
							t.Errorf("lost %s: %s", marker, out.Body.String())
						}
					}
					if strings.Contains(out.Body.String(), `"type":"error"`) || usage.TotalTokens == nil || *usage.TotalTokens != 7 {
						t.Errorf("stream or original usage lost: %s usage=%+v", out.Body.String(), usage)
					}
					second := antigravityToolReply(down.provider)
					if stream && down.provider != "gemini" {
						second = strings.TrimSuffix(second, "}") + `,"stream":true}`
					}
					out = httptest.NewRecorder()
					down.handler(out, makeProxyReq(http.MethodPost, path, second))
					if out.Code != http.StatusOK || calls != 2 || exchanges != 1 || !strings.Contains(out.Body.String(), "receipt-final") {
						t.Fatalf("tool roundtrip failed: status=%d calls=%d exchange=%d body=%s", out.Code, calls, exchanges, out.Body.String())
					}
					if usage.TotalTokens == nil || *usage.TotalTokens != 10 {
						t.Errorf("second turn usage lost: %+v", usage)
					}
				})
			}
		}
	}
}

func antigravityToolReply(provider string) string {
	switch provider {
	case "openai":
		return `{"model":"client-alias","messages":[{"role":"user","content":"receipt-history"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-fixture","type":"function","function":{"name":"echo","arguments":"{\"value\":\"receipt-tool\"}"}}]},{"role":"tool","tool_call_id":"call-fixture","content":"receipt-followup"}]}`
	case "openai_responses":
		return `{"model":"client-alias","input":[{"role":"user","content":"receipt-history"},{"type":"function_call","id":"fc-fixture","call_id":"call-fixture","name":"echo","arguments":"{\"value\":\"receipt-tool\"}"},{"type":"function_call_output","call_id":"call-fixture","output":"receipt-followup"}]}`
	case "anthropic":
		return `{"model":"client-alias","max_tokens":64,"messages":[{"role":"user","content":"receipt-history"},{"role":"assistant","content":[{"type":"tool_use","id":"call-fixture","name":"echo","input":{"value":"receipt-tool"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-fixture","content":"receipt-followup"}]}]}`
	default:
		return `{"contents":[{"role":"user","parts":[{"text":"receipt-history"}]},{"role":"model","parts":[{"thoughtSignature":"signature-fixture","functionCall":{"id":"call-fixture","name":"echo","args":{"value":"receipt-tool"}}}]},{"role":"user","parts":[{"functionResponse":{"id":"call-fixture","name":"echo","response":{"value":"receipt-followup"}}}]}]}`
	}
}

func TestDirectAntigravityEnvelopeFailuresAndGrantBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		stream, deny bool
	}{
		{"missing envelope", `{"candidates":[]}`, false, false},
		{"null response", `{"response":null}`, false, false},
		{"error envelope", `{"response":{"error":{"message":"synthetic failure"}}}`, false, false},
		{"malformed stream", `{"response":null}`, true, false},
		{"grant disallows Gemini", `{"response":{}}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprint(w, "data: "+tc.body+"\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, tc.body)
				}
			}))
			defer upstream.Close()
			protocol := store.DirectProtocolGemini
			if tc.deny {
				protocol = store.DirectProtocolChat
			}
			db := installDirectMediaRuntime(t, store.DialectSQLite,
				store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: upstream.URL + "/v1internal", Auth: store.DirectAuthBearer, Profile: "antigravity"}}, protocol, "")
			if _, err := db.Exec(`UPDATE upstream_channels SET provider='antigravity'`); err != nil {
				t.Fatal(err)
			}
			state, _ := oauth.ParseAntigravityImportMaterial("synthetic-refresh|synthetic-project")
			state.ExpiresAt = time.Now().Add(time.Hour).UnixMilli()
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET secret=?,kind=?,oauth_state=?`), "synthetic-access", store.DirectCredentialOAuth, state); err != nil {
				t.Fatal(err)
			}
			path := "/v1beta/models/client-alias:generateContent"
			if tc.stream {
				path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
			}
			out := httptest.NewRecorder()
			HandleGeminiGenerateContent(out, makeProxyReq(http.MethodPost, path, directWireFixtures[3].request))
			if tc.deny {
				if out.Code < 400 || calls != 0 {
					t.Fatalf("grant escaped boundary: status=%d calls=%d", out.Code, calls)
				}
			} else if tc.stream {
				if calls != 1 || !strings.Contains(out.Body.String(), "error") {
					t.Fatalf("malformed stream was successful: %s", out.Body.String())
				}
			} else if out.Code != http.StatusBadGateway || calls != 1 {
				t.Fatalf("invalid envelope was successful: status=%d calls=%d body=%s", out.Code, calls, out.Body.String())
			}
		})
	}
}

func TestDirectAntigravityStreamCancellationClosesTheSelectedAttempt(t *testing.T) {
	started, cancelled := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"response\":{\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"partial\"}]}}]}}\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer upstream.Close()
	db := installDirectMediaRuntime(t, store.DialectSQLite,
		store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: upstream.URL + "/v1internal", Auth: store.DirectAuthBearer, Profile: "antigravity"}}, store.DirectProtocolGemini, "")
	if _, err := db.Exec(`UPDATE upstream_channels SET provider='antigravity'`); err != nil {
		t.Fatal(err)
	}
	state, _ := oauth.ParseAntigravityImportMaterial("synthetic-refresh|synthetic-project")
	state.ExpiresAt = time.Now().Add(time.Hour).UnixMilli()
	if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET secret=?,kind=?,oauth_state=?`), "synthetic-access", store.DirectCredentialOAuth, state); err != nil {
		t.Fatal(err)
	}
	request := makeProxyReq(http.MethodPost, "/v1beta/models/client-alias:streamGenerateContent", directWireFixtures[3].request)
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		HandleGeminiGenerateContent(httptest.NewRecorder(), request.WithContext(ctx))
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("selected upstream never started")
	}
	cancel()
	for _, completion := range []<-chan struct{}{cancelled, finished} {
		select {
		case <-completion:
		case <-time.After(5 * time.Second):
			t.Fatal("cancellation did not release upstream and downstream")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("cancellation replayed the request %d times", calls.Load())
	}
}
