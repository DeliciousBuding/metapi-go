package proxyhandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func installDirectCodexImageHTTPFixture(t *testing.T, provider, kind, endpointURL string) (*upstreamTestRouter, <-chan proxy.ProxyLogEntry, *store.DB) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO upstream_channels(id,origin_key,source_id,name,dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES(1,'fixture',1,'fixture','generic',?,1,'https://provider.invalid','/v1/chat/completions','/v1/responses','/v1/messages',0,'','[]','','')`, provider)
	if err != nil {
		t.Fatal(err)
	}
	state := store.DirectOAuthState{}
	if kind == store.DirectCredentialOAuth {
		state = store.DirectOAuthState{AccountID: "database-account", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
	}
	_, err = db.Exec(`INSERT INTO upstream_credentials(id,origin_key,channel_id,source_id,name,secret,kind,oauth_state,enabled) VALUES(1,'fixture',1,1,'default','fixture-database-secret',?,?,1)`, kind, state)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := &store.DirectEndpoint{URL: endpointURL, Auth: store.DirectAuthBearer, Profile: "codex-image", RequestModel: "actual-responses-model"}
	mask := store.DirectProtocolImageGeneration | store.DirectProtocolImageEdit
	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel: store.RouteChannel{ID: -1, Enabled: true}, TokenValue: "stale-selected-secret", ActualModel: "actual-image-model",
		Direct: &store.DirectUpstreamCandidate{ChannelID: 1, CredentialID: 1, CredentialKind: kind, Provider: provider, Protocols: mask, Endpoints: store.DirectEndpoints{ImageGeneration: endpoint, ImageEdit: endpoint}},
	}}
	logs := make(chan proxy.ProxyLogEntry, 4)
	old := getUpstreamConfig()
	SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }, ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
		return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
	}})
	t.Cleanup(func() { SetUpstreamConfig(old) })
	oldRuntime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(oldRuntime) })
	return router, logs, db
}

func codexImageHTTPGateway(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
		defer func() {
			if r.MultipartForm != nil {
				_ = r.MultipartForm.RemoveAll()
			}
		}()
		if r.URL.Path == "/v1/images/edits" {
			HandleImagesEdits(w, r)
		} else {
			HandleImagesGenerations(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func codexImageHTTPInput(t *testing.T, form, edit, stream bool) (string, string, io.Reader) {
	t.Helper()
	path := "/v1/images/generations"
	if edit {
		path = "/v1/images/edits"
	}
	if !form {
		payload := map[string]any{"model": "client-model", "prompt": "draw a fixture", "stream": stream, "quality": "high"}
		if edit {
			payload["images"] = []any{map[string]string{"image_url": "data:image/png;base64," + codexImageFixture}}
		}
		raw, _ := json.Marshal(payload)
		return path, "application/json", bytes.NewReader(raw)
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for key, value := range map[string]string{"model": "client-model", "prompt": "draw a fixture", "stream": fmt.Sprint(stream), "quality": "high"} {
		_ = writer.WriteField(key, value)
	}
	part, _ := writer.CreateFormFile("image[]", "fixture.png")
	data, _ := base64.StdEncoding.DecodeString(codexImageFixture)
	_, _ = part.Write(data)
	_ = writer.Close()
	return path, writer.FormDataContentType(), &buffer
}

func TestDirectCodexImagesHTTPMatrix(t *testing.T) {
	for _, provider := range []string{"codex", "fenno"} {
		for _, kind := range []string{store.DirectCredentialAPIKey, store.DirectCredentialOAuth} {
			for _, edit := range []bool{false, true} {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s_%s_edit_%t_stream_%t", provider, kind, edit, stream), func(t *testing.T) {
						called := make(chan struct{}, 1)
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							called <- struct{}{}
							var payload map[string]any
							if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
								t.Error(err)
								w.WriteHeader(400)
								return
							}
							tools, ok := payload["tools"].([]any)
							if !ok || len(tools) != 1 {
								t.Error("missing image tool")
								w.WriteHeader(400)
								return
							}
							tool := tools[0].(map[string]any)
							if r.URL.Path != "/exact-provider-responses" || payload["model"] != "actual-responses-model" || tool["model"] != "actual-image-model" || payload["stream"] != true || payload["store"] != false || tool["quality"] != "high" {
								t.Error("wrong endpoint, model or Codex wire")
							}
							if r.Header.Get("Authorization") != "Bearer fixture-database-secret" || r.Header.Get("Session-Id") == "" || r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Goog-Api-Key") != "" {
								t.Error("credential or Codex identity mismatch")
							}
							account := ""
							if kind == store.DirectCredentialOAuth {
								account = "database-account"
							}
							if r.Header.Get("Chatgpt-Account-Id") != account || r.Header.Get("X-Openai-Internal-Codex-Responses-Lite") != "" {
								t.Error("downstream account or privileged header leaked")
							}
							if edit {
								raw, _ := json.Marshal(payload["input"])
								if !bytes.Contains(raw, []byte(codexImageFixture)) {
									t.Error("image input lost")
								}
							}
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = io.WriteString(w, codexImageFrames(codexImageFixture, false))
						}))
						defer upstream.Close()
						_, logs, _ := installDirectCodexImageHTTPFixture(t, provider, kind, upstream.URL+"/exact-provider-responses")
						gateway := codexImageHTTPGateway(t)
						// Alternate JSON and multipart edits for both output modes.
						path, ct, body := codexImageHTTPInput(t, edit && kind == store.DirectCredentialOAuth, edit, stream)
						req, _ := http.NewRequest("POST", gateway.URL+path, body)
						req.Header.Set("Content-Type", ct)
						req.Header.Set("Chatgpt-Account-Id", "client-spoof")
						req.Header.Set("Authorization", "Bearer client-spoof")
						req.Header.Set("X-Api-Key", "client-spoof")
						req.Header.Set("X-Openai-Internal-Codex-Responses-Lite", "true")
						resp, err := http.DefaultClient.Do(req)
						if err != nil {
							t.Fatal(err)
						}
						output, err := io.ReadAll(resp.Body)
						_ = resp.Body.Close()
						if err != nil || resp.StatusCode != 200 {
							t.Fatalf("HTTP result status=%d err=%v body=%s", resp.StatusCode, err, output)
						}
						if !bytes.Contains(output, []byte(codexImageFixture)) || !bytes.Contains(output, []byte(`"total_tokens":14`)) || bytes.Contains(output, []byte("secret")) || bytes.Contains(output, []byte("response.output")) {
							t.Fatalf("Images result lost data/usage or leaked source: %s", output)
						}
						if stream && !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") || !stream && !json.Valid(output) {
							t.Fatal("wrong downstream response representation")
						}
						select {
						case <-called:
						default:
							t.Fatal("upstream was not called")
						}
						select {
						case entry := <-logs:
							if entry.Status != "success" || entry.TotalTokens == nil || *entry.TotalTokens != 14 {
								t.Fatalf("actual usage/accounting lost: %+v", entry)
							}
							encoded, _ := json.Marshal(entry)
							if bytes.Contains(encoded, []byte("secret")) {
								t.Fatal("credential persisted in proxy log")
							}
						case <-time.After(time.Second):
							t.Fatal("missing proxy log")
						}
					})
				}
			}
		}
	}
}

func TestDirectCodexImagesHTTPFailures(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, failure := range []string{"truncated", "tool_failed", "http_error", "expired", "multipart_limit", "json_limit"} {
			t.Run(fmt.Sprintf("%s_stream_%t", failure, stream), func(t *testing.T) {
				called := make(chan struct{}, 1)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called <- struct{}{}
					if failure == "http_error" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(429)
						_, _ = io.WriteString(w, `{"error":{"message":"rate limited","type":"rate_limit_error"}}`)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					raw := codexImageFrames(codexImageFixture, false)
					if failure == "truncated" {
						raw = strings.Split(raw, `data: {"type":"response.completed"`)[0]
					}
					if failure == "tool_failed" {
						raw = strings.Replace(raw, `"status":"generating"`, `"status":"failed"`, 1)
					}
					_, _ = io.WriteString(w, raw)
				}))
				defer upstream.Close()
				_, _, db := installDirectCodexImageHTTPFixture(t, "codex", store.DirectCredentialOAuth, upstream.URL)
				if failure == "expired" {
					_, err := db.Exec(`UPDATE upstream_credentials SET oauth_state=? WHERE id=1`, store.DirectOAuthState{ExpiresAt: 1})
					if err != nil {
						t.Fatal(err)
					}
				}
				if failure == "multipart_limit" || failure == "json_limit" {
					t.Setenv("PROXY_MAX_MULTIPART_FILE_BYTES", "4")
				}
				gateway := codexImageHTTPGateway(t)
				path, ct, body := codexImageHTTPInput(t, failure == "multipart_limit", true, stream)
				resp, err := http.Post(gateway.URL+path, ct, body)
				if err != nil {
					t.Fatal(err)
				}
				output, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				want := 502
				if stream && (failure == "tool_failed" || failure == "truncated") {
					want = 200
				}
				if failure == "http_error" {
					want = 429
				}
				if failure == "expired" {
					want = 503
				}
				if failure == "multipart_limit" || failure == "json_limit" {
					want = 413
				}
				if resp.StatusCode != want || bytes.Contains(output, []byte("image_edit.completed")) || bytes.Contains(output, []byte("fixture-database-secret")) {
					t.Fatalf("wrong failure status=%d want=%d body=%s", resp.StatusCode, want, output)
				}
				if failure == "expired" || failure == "multipart_limit" || failure == "json_limit" {
					select {
					case <-called:
						t.Fatal("invalid input/credential reached upstream")
					default:
					}
				}
			})
		}
	}
}

func TestDirectCodexImagesHTTPCancellationDoesNotPoisonChannel(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `data: {"type":"response.created","response":{"created_at":123}}`+"\n\n")
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
				close(stopped)
			}))
			defer upstream.Close()
			router, _, _ := installDirectCodexImageHTTPFixture(t, "codex", store.DirectCredentialOAuth, upstream.URL)
			gatewayDone := make(chan struct{})
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(gatewayDone)
				r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{Token: "fixture-client", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
				HandleImagesGenerations(w, r)
			}))
			defer gateway.Close()
			path, ct, body := codexImageHTTPInput(t, false, false, stream)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, _ := http.NewRequestWithContext(ctx, "POST", gateway.URL+path, body)
			req.Header.Set("Content-Type", ct)
			clientDone := make(chan struct{})
			go func() {
				defer close(clientDone)
				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("upstream did not start")
			}
			cancel()
			for _, ch := range []<-chan struct{}{clientDone, stopped, gatewayDone} {
				select {
				case <-ch:
				case <-time.After(3 * time.Second):
					t.Fatal("image cancellation did not release HTTP transport")
				}
			}
			if len(router.failures) != 0 {
				t.Fatalf("client cancellation poisoned channel: %+v", router.failures)
			}
		})
	}
}

func TestDirectCodexImagesHTTPIdleTimeout(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			old := config.Get()
			next := *old
			next.ProxyStreamIdleTimeoutSec = 1
			config.Set(&next)
			t.Cleanup(func() { config.Set(old) })
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, `data: {"type":"response.image_generation_call.partial_image","partial_image_index":0,"partial_image_b64":"`+codexImageFixture+`"}`+"\n\n")
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			defer upstream.Close()
			_, logs, _ := installDirectCodexImageHTTPFixture(t, "fenno", store.DirectCredentialAPIKey, upstream.URL)
			gateway := codexImageHTTPGateway(t)
			path, ct, body := codexImageHTTPInput(t, false, false, stream)
			resp, err := http.Post(gateway.URL+path, ct, body)
			if err != nil {
				t.Fatal(err)
			}
			output, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if !stream && resp.StatusCode != 502 || !bytes.Contains(output, []byte("idle timeout")) || bytes.Contains(output, []byte("image_generation.completed")) {
				t.Fatalf("idle timeout appeared complete: %d %s", resp.StatusCode, output)
			}
			select {
			case entry := <-logs:
				if entry.Status != "failed" {
					t.Fatalf("idle timeout logged %s", entry.Status)
				}
			case <-time.After(time.Second):
				t.Fatal("missing timeout log")
			}
		})
	}
}

func TestDirectCodexImagesHTTPLargeFinalWithoutPreviews(t *testing.T) {
	image := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 1<<20))
	for _, tc := range []struct{ stream, usage bool }{{false, true}, {true, true}, {false, false}, {true, false}} {
		t.Run(fmt.Sprintf("stream_%t_usage_%t", tc.stream, tc.usage), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				raw := codexImageFrames(image, true)
				if !tc.usage {
					raw = strings.Replace(raw, `,"usage":{"input_tokens":5,"output_tokens":9,"total_tokens":14,"input_tokens_details":{"cached_tokens":2}}`, "", 1)
				}
				for _, frame := range strings.Split(raw, "\n\n") {
					if !strings.Contains(frame, "partial_image") && frame != "" {
						_, _ = io.WriteString(w, frame+"\n\n")
					}
				}
			}))
			defer upstream.Close()
			_, logs, _ := installDirectCodexImageHTTPFixture(t, "codex", store.DirectCredentialAPIKey, upstream.URL)
			config.SetRuntime(&config.RuntimeSettings{ProxyEmptyContentFailEnabled: true})
			gateway := codexImageHTTPGateway(t)
			path, ct, body := codexImageHTTPInput(t, false, false, tc.stream)
			resp, err := http.Post(gateway.URL+path, ct, body)
			if err != nil {
				t.Fatal(err)
			}
			output, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil || resp.StatusCode != 200 || !bytes.Contains(output, []byte(image)) {
				t.Fatalf("large image not relayed: status=%d err=%v", resp.StatusCode, err)
			}
			select {
			case entry := <-logs:
				if entry.Status != "success" || tc.usage && (entry.TotalTokens == nil || *entry.TotalTokens != 14) || entry.FirstOutputLatencyMs == nil {
					t.Fatalf("large image wrongly judged or metered: %+v", entry)
				}
			case <-time.After(time.Second):
				t.Fatal("missing large image proxy log")
			}
		})
	}
}
