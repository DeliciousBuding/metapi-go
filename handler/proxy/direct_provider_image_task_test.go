package proxyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

const imageOperationModule = `export const meta={apiVersion:2,kind:"provider",key:"image-operation",name:"Image operation",version:"1.0.0",protocols:["imageGeneration"],credentialKinds:["api_key"],hooks:["submit","query","taskResult"],observations:{imageGeneration:{response:{taskIdPath:"/task_id",errorPath:"/failure",usagePaths:{input_tokens:"/meter/input",output_tokens:"/meter/output",total_tokens:"/meter/total"}}}},imageTasks:{imageGeneration:{query:{path:"/tasks/{taskId}",auth:"bearer"}}}};export function submit(i){return {method:"POST",body:i.body};}export function query(i){return {method:"GET",headers:{"X-Task-ID":i.taskId}};}export function taskResult(i){const r=JSON.parse(i.body);return r.task_status==="DONE"?{status:"completed",body:r.native}:{status:"pending"};}`

func TestProviderAsyncImageHTTPFacadeKeepsCurrentAuthorization(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"success", "revoked", "new-key", "new-target", "disabled-driver", "deleted-driver", "disabled-during-resolution", "upgraded-driver", "anonymous"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				var db *store.DB
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					count := calls.Add(1)
					wantAuth := "Bearer fixture-media-key"
					if mode == "anonymous" {
						wantAuth = ""
					}
					if r.Header.Get("Authorization") != wantAuth {
						t.Error("image facade changed its selected credential")
					}
					if r.Method == "POST" && r.URL.Path != "/images/generations" || r.Method == "GET" && r.URL.Path != "/tasks/one" || r.Method != "POST" && r.Method != "GET" {
						t.Errorf("facade changed its saved operation: %s %s", r.Method, r.URL.Path)
					}
					if count == 1 && mode == "revoked" {
						if _, err := db.Exec(`UPDATE upstream_grants SET enabled=FALSE`); err != nil {
							t.Error(err)
						}
					}
					if count == 1 && mode == "new-key" {
						if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET secret=?`), "rotated-fixture-media-key"); err != nil {
							t.Error(err)
						}
					}
					if count == 1 && mode == "disabled-driver" {
						if _, err := db.Exec(`UPDATE channel_plugins SET enabled=FALSE WHERE plugin_key='image-operation'`); err != nil {
							t.Error(err)
						}
					}
					if count == 1 && mode == "deleted-driver" {
						if _, err := db.Exec(`DELETE FROM channel_plugins WHERE plugin_key='image-operation'`); err != nil {
							t.Error(err)
						}
					}
					if count == 1 && mode == "upgraded-driver" {
						upgraded := strings.Replace(imageOperationModule, `version:"1.0.0"`, `version:"2.0.0"`, 1)
						upgraded = strings.Replace(upgraded, `const r=JSON.parse(i.body);`, `throw new Error("new program must not replace this attempt");const r=JSON.parse(i.body);`, 1)
						if _, err := channelplugins.Save(t.Context(), db.DB, "image-operation", upgraded, true, 1); err != nil {
							t.Error(err)
						}
					}
					if count == 1 && mode == "new-target" {
						var current store.DirectRequestConfig
						if err := db.Get(&current, `SELECT request_config FROM upstream_channels LIMIT 1`); err != nil {
							t.Error(err)
						} else {
							target := current.Provider.ImageQueries["imageGeneration"]
							address, _ := url.Parse(target.URL)
							address.Path, address.RawPath = "/changed/{taskId}", ""
							current.Provider.ImageQueries["imageGeneration"] = store.DirectImageQueryEndpoint{URL: address.String(), Auth: target.Auth}
							if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), current); err != nil {
								t.Error(err)
							}
						}
					}
					status, output := "PENDING", 0
					if count > 1 {
						status, output = "DONE", 2
					}
					w.Header().Set("Content-Type", "application/json")
					native := `{"created":1,"data":[{"b64_json":"c3ludGhldGljLWltYWdl"}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}`
					_ = json.NewEncoder(w).Encode(map[string]any{"task_id": "one", "task_status": status, "native": native, "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}})
				}))
				defer server.Close()
				endpoints := store.DirectEndpoints{ImageGeneration: &store.DirectEndpoint{URL: server.URL + "/images/generations", Auth: store.DirectAuthBearer}}
				if mode == "anonymous" {
					endpoints.ImageGeneration.Auth = store.DirectAuthNone
				}
				db = installDirectMediaRuntime(t, dialect, endpoints, store.DirectProtocolImageGeneration, "")
				source := imageOperationModule
				if mode == "anonymous" {
					source = strings.Replace(source, `credentialKinds:["api_key"]`, `credentialKinds:["none"]`, 1)
					if _, err := db.Exec(`UPDATE upstream_channels SET provider='ollama'`); err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(`UPDATE upstream_credentials SET kind='none',secret=''`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := channelplugins.Save(t.Context(), db.DB, "image-operation", source, true, 0); err != nil {
					t.Fatal(err)
				}
				requestConfig := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "image-operation", Protocols: []string{"imageGeneration"}, ImageQueries: map[string]store.DirectImageQueryEndpoint{"imageGeneration": {URL: server.URL + "/tasks/{taskId}", Auth: store.DirectAuthBearer}}}}
				if mode == "anonymous" {
					query := requestConfig.Provider.ImageQueries["imageGeneration"]
					query.Auth = store.DirectAuthNone
					requestConfig.Provider.ImageQueries["imageGeneration"] = query
				}
				if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), requestConfig); err != nil {
					t.Fatal(err)
				}
				cfg := getUpstreamConfig()
				cfg.ChannelPlugins = channelplugins.New(db.DB)
				if mode == "disabled-during-resolution" {
					resolve := cfg.ResolveDirectCredential
					var resolutions atomic.Int32
					cfg.ResolveDirectCredential = func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
						if resolutions.Add(1) == 2 {
							if _, err := db.Exec(`UPDATE channel_plugins SET enabled=FALSE WHERE plugin_key='image-operation'`); err != nil {
								return nil, err
							}
						}
						return resolve(ctx, id, proxyURL, force)
					}
				}
				var entries []proxy.ProxyLogEntry
				cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { entries = append(entries, entry); return nil }
				out := httptest.NewRecorder()
				HandleImagesGenerations(out, makeProxyReq("POST", "/v1/images/generations", `{"model":"client-alias","prompt":"fixture image"}`))
				if mode == "anonymous" {
					// Existing Ollama anonymous grants cover its text protocols.
					// A plugin declaration cannot expand that authority to images.
					if out.Code < 400 || calls.Load() != 0 || len(entries) != 0 {
						t.Fatal("an image declaration expanded anonymous protocol authority")
					}
					return
				}
				wantCalls, wantTokens, wantStatus := int32(2), int64(9), "success"
				success := mode == "success" || mode == "upgraded-driver"
				if !success {
					wantCalls, wantTokens, wantStatus = 1, 7, "failed"
				}
				if calls.Load() != wantCalls || len(entries) != 1 || entries[0].TotalTokens == nil || *entries[0].TotalTokens != wantTokens || entries[0].Status != wantStatus {
					t.Fatalf("facade replayed or lost its attempt/usage: status=%d calls=%d entries=%+v body=%s", out.Code, calls.Load(), entries, out.Body.String())
				}
				if success && (out.Code != 200 || !strings.Contains(out.Body.String(), "b64_json")) || !success && out.Code < 400 {
					t.Fatalf("facade did not enforce current authorization: %d %s", out.Code, out.Body.String())
				}
				if mode == "upgraded-driver" {
					// The next request captures the new program, unlike the
					// already submitted attempt above. Its result hook must fail.
					next := httptest.NewRecorder()
					HandleImagesGenerations(next, makeProxyReq("POST", "/v1/images/generations", `{"model":"client-alias","prompt":"next fixture image"}`))
					if next.Code < 400 || calls.Load() != 3 || len(entries) != 2 || entries[1].Status != "failed" || entries[1].TotalTokens == nil || *entries[1].TotalTokens != 9 {
						t.Fatal("the next image request ignored its upgraded program or replayed")
					}
				}
			})
		}
	}
}

func imageOperationFixture(t *testing.T, origin, source string) directImageTaskOperation {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := channelplugins.Save(t.Context(), db.DB, "image-operation", source, true, 0); err != nil {
		t.Fatal(err)
	}
	query := store.DirectImageQueryEndpoint{URL: origin + "/tasks/{taskId}", Auth: store.DirectAuthBearer}
	binding := store.DirectProviderBinding{Key: "image-operation", Protocols: []string{"imageGeneration"}, ImageQueries: map[string]store.DirectImageQueryEndpoint{"imageGeneration": query}}
	invocation, err := channelplugins.New(db.DB).ResolveDriver(t.Context(), binding, "imageGeneration", "api_key")
	if err != nil {
		t.Fatal(err)
	}
	return directImageTaskOperation{wire: &directProviderWire{Driver: &invocation, DriverImageQuery: &query, PluginProtocol: "imageGeneration", PluginModel: "vendor-image"}, pollInterval: time.Millisecond, beforeQuery: func(context.Context) (string, error) { return "synthetic-image-secret", nil }}
}

func TestProviderImageOperationUsesOneSubmissionAndOriginalCumulativeUsage(t *testing.T) {
	for _, mode := range []string{"success", "wrong-path", "wrong-task", "throw-result", "fake-usage", "poll-error", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			const taskID = "task/id ?#%"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/private" {
					t.Error("query followed a redirect to an unconfigured path")
					w.WriteHeader(500)
					return
				}
				count := calls.Add(1)
				if r.Header.Get("Authorization") != "Bearer synthetic-image-secret" {
					t.Error("query did not use host authentication")
				}
				if count == 1 && (r.Method != "POST" || r.URL.Path != "/images/generations") || count > 1 && (r.Method != "GET" || r.URL.EscapedPath() != "/tasks/"+url.PathEscape(taskID)) {
					t.Errorf("operation changed its authorized target: %s %s", r.Method, r.URL.EscapedPath())
				}
				w.Header().Set("Content-Type", "application/json")
				if mode == "redirect" && count > 1 {
					http.Redirect(w, r, "/private", http.StatusFound)
					return
				}
				if mode == "poll-error" && count > 1 {
					w.WriteHeader(403)
					fmt.Fprint(w, `{"error":{"message":"synthetic query denied"}}`)
					return
				}
				id, status, output := taskID, "PENDING", 0
				if mode == "wrong-task" && count > 1 {
					id = "other-task"
				}
				if count == 3 {
					status, output = "DONE", 2
				}
				nativeTotal := 9
				if mode == "fake-usage" {
					nativeTotal = 900
				}
				native := fmt.Sprintf(`{"created":1,"data":[{"b64_json":"c3ludGhldGljLWltYWdl"}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":%d},"extension":9007199254740993}`, nativeTotal)
				_ = json.NewEncoder(w).Encode(map[string]any{"task_id": id, "task_status": status, "native": native, "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}})
			}))
			defer server.Close()
			source := imageOperationModule
			if mode == "wrong-path" {
				source = strings.Replace(source, `method:"GET",headers`, `method:"GET",path:"/private",headers`, 1)
			}
			if mode == "throw-result" {
				source = strings.Replace(source, `const r=JSON.parse(i.body);`, `const r=JSON.parse(i.body);if(r.task_status==="DONE")throw new Error("synthetic-private-result");`, 1)
			}
			op := imageOperationFixture(t, server.URL, source)
			request, _ := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/images/generations", strings.NewReader(`{"model":"vendor-image","prompt":"fixture"}`))
			request.Header.Set("Authorization", "Bearer synthetic-image-secret")
			response, err := op.run(request)
			if mode == "success" || mode == "poll-error" {
				if err != nil || response == nil {
					t.Fatalf("image operation failed: %v", err)
				}
				body, _ := io.ReadAll(response.Body)
				_ = response.Body.Close()
				usage, _, observeErr := observeDirectDriverBody(op.wire, body, false)
				if mode == "success" {
					if response.StatusCode != 200 || calls.Load() != 3 || observeErr != nil || usage.TotalTokens != 9 || !strings.Contains(string(body), "9007199254740993") {
						t.Fatalf("image results or raw usage lost: calls=%d usage=%+v body=%s", calls.Load(), usage, body)
					}
				} else if response.StatusCode != 403 || calls.Load() != 2 || usage.TotalTokens != 7 {
					t.Fatalf("failed query lost earlier usage or replayed: calls=%d usage=%+v", calls.Load(), usage)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid or failed image operation succeeded")
			}
			wantCalls, wantTokens := int32(3), int64(9)
			if mode == "wrong-path" {
				wantCalls, wantTokens = 1, 7
			} else if mode == "wrong-task" || mode == "redirect" {
				wantCalls, wantTokens = 2, 7
			}
			if calls.Load() != wantCalls || directMediaFailureUsage(err).TotalTokens != wantTokens || strings.Contains(err.Error(), "synthetic-private-result") {
				t.Fatalf("failure replayed or erased observations: calls=%d usage=%+v error=%v", calls.Load(), directMediaFailureUsage(err), err)
			}
		})
	}
}

func TestProviderImageCancellationStopsThePendingQuery(t *testing.T) {
	entered, canceled := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method == "GET" {
			close(entered)
			<-r.Context().Done()
			close(canceled)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"task_id":"one","task_status":"PENDING","meter":{"input":7,"output":0,"total":7}}`)
	}))
	defer server.Close()
	op := imageOperationFixture(t, server.URL, imageOperationModule)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/images/generations", strings.NewReader(`{}`))
	result := make(chan error, 1)
	go func() { _, err := op.run(request); result <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("pending query did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || calls.Load() != 2 || directMediaFailureUsage(err).TotalTokens != 7 {
			t.Fatalf("cancel replayed or lost raw usage: calls=%d error=%v", calls.Load(), err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("image operation did not honor cancellation")
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("vendor query outlived canceled request")
	}
}

func TestProviderImageImmediateResultPreservesNativeFormatsAndUsage(t *testing.T) {
	for _, tc := range []struct {
		name, format, native string
		wantBase64           bool
		wantFailure          bool
	}{
		{"base64-default", "", `{"created":1,"data":[{"b64_json":"AAEC/w==","revised_prompt":"fixture revision"}],"extension":9007199254740993}`, true, false},
		{"base64-explicit", "b64_json", `{"created":1,"data":[{"b64_json":"AAEC/w=="}]}`, true, false},
		{"url-explicit", "url", `{"created":1,"data":[{"url":"https://cdn.invalid/image?signature=fixture"}]}`, false, false},
		{"data-uri-default", "", `{"created":1,"data":[{"url":"data:image/png;base64,AAEC/w=="}]}`, true, false},
		{"data-uri-url", "url", `{"created":1,"data":[{"url":"data:image/png;base64,AAEC/w=="}]}`, false, false},
		{"base64-not-url", "url", `{"created":1,"data":[{"b64_json":"AAEC/w=="}]}`, false, true},
		{"invalid-base64", "", `{"created":1,"data":[{"b64_json":"%%%"}]}`, false, true},
		{"invalid-data-uri", "", `{"created":1,"data":[{"url":"data:image/png;base64,%%%"}]}`, false, true},
		{"empty-data", "", `{"created":1,"data":[]}`, false, true},
		{"empty-item", "", `{"created":1,"data":[{}]}`, false, true},
		{"invalid-native-json", "", `{"created":1,`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost {
					t.Error("an immediate result caused a query")
				}
				w.Header().Set("Content-Type", "application/json")
				// Immediate completion need not invent a supplier task id.
				_ = json.NewEncoder(w).Encode(map[string]any{"task_status": "DONE", "native": tc.native, "meter": map[string]int{"input": 7, "output": 2, "total": 9}})
			}))
			defer server.Close()
			op := imageOperationFixture(t, server.URL, imageOperationModule)
			op.format = tc.format
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/images/generations", strings.NewReader(`{}`))
			response, err := op.run(request)
			if calls.Load() != 1 || op.wire.DriverImageUsage == nil || op.wire.DriverImageUsage.TotalTokens != 9 {
				t.Fatalf("immediate result replayed or erased supplier usage: calls=%d usage=%+v", calls.Load(), op.wire.DriverImageUsage)
			}
			if tc.wantFailure {
				if err == nil || directMediaFailureUsage(err).TotalTokens != 9 {
					t.Fatalf("invalid result succeeded or lost observed usage: %v", err)
				}
				return
			}
			if err != nil || response == nil {
				t.Fatalf("native image format rejected: %v", err)
			}
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr != nil || response.StatusCode != http.StatusOK || strings.Contains(string(body), `"b64_json"`) != tc.wantBase64 {
				t.Fatalf("wrong materialized image response: %s %v", body, readErr)
			}
			if tc.wantBase64 && !strings.Contains(string(body), "AAEC/w==") || tc.name == "base64-default" && string(body) != tc.native || tc.name == "url-explicit" && string(body) != tc.native {
				t.Fatalf("native image bytes or opaque fields changed: %s", body)
			}
		})
	}
}

func TestProviderImageInvalidTaskAndQueryJSONNeverReplaySubmission(t *testing.T) {
	for _, mode := range []string{"missing-id", "invalid-id", "oversized-id", "dot-id", "query-json", "submit-redirect"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count := calls.Add(1)
				if mode == "submit-redirect" {
					http.Redirect(w, r, "/private", http.StatusTemporaryRedirect)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if count > 1 {
					fmt.Fprint(w, `{"task_id":`)
					return
				}
				var id any = "one"
				switch mode {
				case "missing-id":
					id = ""
				case "invalid-id":
					id = 123
				case "oversized-id":
					id = strings.Repeat("x", 513)
				case "dot-id":
					id = ".."
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"task_id": id, "task_status": "PENDING", "meter": map[string]int{"input": 7, "output": 0, "total": 7}})
			}))
			defer server.Close()
			op := imageOperationFixture(t, server.URL, imageOperationModule)
			request, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/images/generations", strings.NewReader(`{}`))
			_, err := op.run(request)
			wantCalls := int32(1)
			if mode == "query-json" {
				wantCalls = 2
			}
			if err == nil || calls.Load() != wantCalls {
				t.Fatalf("invalid task/query succeeded or replayed: calls=%d err=%v", calls.Load(), err)
			}
			if mode == "query-json" && directMediaFailureUsage(err).TotalTokens != 7 {
				t.Fatal("malformed query erased earlier supplier usage")
			}
		})
	}
}

func TestProviderImageCredentiallessQueryDropsSubmissionAndSigningHeaders(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := calls.Add(1)
		if count > 1 {
			for _, name := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "X-Signing-Key", "Cookie"} {
				if r.Header.Get(name) != "" {
					t.Errorf("credentialless query leaked %s", name)
				}
			}
		}
		status, output := "PENDING", 0
		if count > 1 {
			status, output = "DONE", 2
		}
		w.Header().Set("Content-Type", "application/json")
		native := `{"created":1,"data":[{"b64_json":"c3ludGhldGljLWltYWdl"}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}`
		_ = json.NewEncoder(w).Encode(map[string]any{"task_id": "one", "task_status": status, "native": native, "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}})
	}))
	defer server.Close()
	source := strings.Replace(imageOperationModule, `credentialKinds:["api_key"],`, `credentialKinds:["api_key"],authentication:{type:"api_key",header:"X-Signing-Key",prefix:"Key "},`, 1)
	source = strings.Replace(source, `"X-Task-ID":i.taskId`, `"X-Task-ID":i.taskId,"X-Signing-Key":"spoof"`, 1)
	op := imageOperationFixture(t, server.URL, source)
	op.wire.DriverImageQuery.Auth = store.DirectAuthNone
	request, _ := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/images/generations", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer synthetic-image-secret")
	request.Header.Set("X-Signing-Key", "Key synthetic-image-secret")
	request.Header.Set("Cookie", "synthetic-cookie")
	response, err := op.run(request)
	if err != nil || response == nil || calls.Load() != 2 {
		t.Fatalf("credentialless flow failed: calls=%d error=%v", calls.Load(), err)
	}
	_ = response.Body.Close()
}
