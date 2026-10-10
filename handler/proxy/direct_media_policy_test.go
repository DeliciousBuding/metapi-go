package proxyhandler

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
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectMediaGrantAndMemberIsolation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, scenario := range []string{"grant", "member", "endpoint", "key policy", "generation", "upstream404"} {
				t.Run(scenario, func(t *testing.T) {
					var calls, chat atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.URL.Path == "/chat" {
							chat.Add(1)
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(404)
						_, _ = io.WriteString(w, `{"error":{"message":"please use /v1/chat/completions"}}`)
					}))
					defer server.Close()
					endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: server.URL + "/chat", Auth: store.DirectAuthBearer}, Embeddings: &store.DirectEndpoint{URL: server.URL + "/embedding", Auth: store.DirectAuthBearer}}
					bits := store.DirectProtocolEmbeddings | store.DirectProtocolChat
					if scenario == "grant" {
						bits = store.DirectProtocolChat
					}
					if scenario == "endpoint" {
						endpoints.Embeddings = nil
					}
					if scenario == "generation" {
						bits = store.DirectProtocolEmbeddings
					}
					db := installDirectMediaRuntime(t, dialect, endpoints, bits, "")
					if scenario == "member" {
						if _, err := db.Exec(`UPDATE upstream_group_items SET protocol_order='[2]'`); err != nil {
							t.Fatal(err)
						}
					}
					path := "/v1/embeddings"
					if scenario == "generation" {
						path = "/v1/chat/completions"
					}
					req := makeProxyReq("POST", path, `{"model":"client-alias","input":"hi","messages":[{"role":"user","content":"hi"}]}`)
					if scenario == "key policy" {
						none := []int64{}
						auth.GetProxyAuth(req.Context()).Policy.AccessPolicy = &store.DownstreamAccessPolicy{AllowedUpstreamChannelIDs: &none}
					}
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, req)
					if scenario == "upstream404" {
						if out.Code != 404 || calls.Load() != 1 || chat.Load() != 0 {
							t.Fatalf("media error fell back across protocols: %d calls=%d chat=%d", out.Code, calls.Load(), chat.Load())
						}
					} else if out.Code < 400 || calls.Load() != 0 {
						t.Fatalf("media authorization escaped %s: %d calls=%d", scenario, out.Code, calls.Load())
					}
				})
			}

		})
	}
}

func TestDirectMediaJinaAndGenericSelection(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, order := range []string{"[131072,64]", "[64,131072]", "[131072]"} {
				t.Run(order, func(t *testing.T) {
					var gotPath, task string
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						gotPath = r.URL.Path
						var body map[string]json.RawMessage
						_ = json.NewDecoder(r.Body).Decode(&body)
						_ = json.Unmarshal(body["task"], &task)
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"data":[{"embedding":[0.25],"index":0}]}`)
					}))
					defer server.Close()
					endpoints := store.DirectEndpoints{Embeddings: &store.DirectEndpoint{URL: server.URL + "/generic", Auth: store.DirectAuthBearer}, JinaEmbeddings: &store.DirectEndpoint{URL: server.URL + "/jina", Auth: store.DirectAuthBearer, Profile: "jina-embeddings"}}
					db := installDirectMediaRuntime(t, dialect, endpoints, store.DirectProtocolEmbeddings|store.DirectProtocolJinaEmbeddings, "")
					if _, err := db.Exec(db.Rebind(`UPDATE upstream_group_items SET protocol_order=?`), order); err != nil {
						t.Fatal(err)
					}
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/embeddings", `{"model":"client-alias","input":"hi"}`))
					wantPath, wantTask := "/jina", "text-matching"
					if order == "[64,131072]" {
						wantPath, wantTask = "/generic", ""
					}
					if out.Code != 200 || gotPath != wantPath || task != wantTask {
						t.Fatalf("wrong distinct format: status=%d path=%s task=%s", out.Code, gotPath, task)
					}
					if wantPath == "/jina" {
						out = httptest.NewRecorder()
						directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/embeddings", `{"model":"client-alias","input":"hi","task":"retrieval.query"}`))
						if out.Code != 200 || task != "retrieval.query" {
							t.Fatal("explicit Jina task overwritten")
						}
					}
				})
			}

		})
	}
}

func TestDirectMediaGeminiEmbedding(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, body := range []string{`{"content":{"parts":[{"text":"hello"}]},"outputDimensionality":128}`, `{"model":"models/client-alias","content":{"parts":[{"text":"hello"}]},"opaque":9007199254740993}`} {
				t.Run(body, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						raw, _ := io.ReadAll(r.Body)
						if r.URL.Path != "/models/provider-model:embedContent" || r.Header.Get("x-goog-api-key") != "fixture-media-key" || r.Header.Get("Authorization") != "" {
							t.Errorf("wrong native Gemini endpoint/auth: %s", r.URL.Path)
						}
						if strings.Contains(body, `"model"`) {
							if !strings.Contains(string(raw), `"model":"models/provider-model"`) || !strings.Contains(string(raw), "9007199254740993") {
								t.Errorf("Gemini body model/precision changed: %s", raw)
							}
						} else if strings.Contains(string(raw), `"model"`) {
							t.Errorf("invented Gemini body model: %s", raw)
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"embedding":{"values":[0.1,0.2]}}`)
					}))
					defer server.Close()
					installDirectMediaRuntime(t, dialect, store.DirectEndpoints{GeminiEmbeddings: &store.DirectEndpoint{URL: server.URL + "/models", Auth: store.DirectAuthGoogle, ModelPath: true}}, store.DirectProtocolGeminiEmbeddings, "")
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1beta/models/client-alias:embedContent", body))
					if out.Code != 200 {
						t.Fatalf("Gemini embedContent: %d %s", out.Code, out.Body.String())
					}
					out = httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/embeddings", `{"model":"client-alias","input":"hi"}`))
					if out.Code < 400 {
						t.Fatal("OpenAI embedding crossed into native Gemini without converter")
					}
				})
			}

		})
	}
}

func TestDirectMediaModelScopeSelection(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
				t.Run(path, func(t *testing.T) {
					var posts, polls, generic atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if r.URL.Path == "/api/images/generations" {
							posts.Add(1)
							if r.Header.Get("X-ModelScope-Async-Mode") != "true" {
								t.Error("missing async provider wire header")
							}
							raw, _ := io.ReadAll(r.Body)
							if path == "/v1/images/edits" && !strings.Contains(string(raw), `"image_url"`) {
								t.Errorf("image edit input not transformed: %s", raw)
							}
							_, _ = io.WriteString(w, `{"task_id":"fixture","usage":{"total_tokens":7}}`)
						} else if r.URL.Path == "/api/tasks/fixture" {
							polls.Add(1)
							_, _ = io.WriteString(w, `{"task_status":"SUCCEED","output_images":[{"b64_json":"YWJj"}]}`)
						} else {
							generic.Add(1)
							_, _ = io.WriteString(w, `{"data":[]}`)
						}
					}))
					defer server.Close()
					endpoints := store.DirectEndpoints{ImageGeneration: &store.DirectEndpoint{URL: server.URL + "/generic", Auth: store.DirectAuthBearer}, ImageEdit: &store.DirectEndpoint{URL: server.URL + "/generic-edit", Auth: store.DirectAuthBearer}, ModelScopeImageGeneration: &store.DirectEndpoint{URL: server.URL + "/api/images/generations", Auth: store.DirectAuthBearer, Profile: "modelscope-image"}}
					db := installDirectMediaRuntime(t, dialect, endpoints, store.DirectProtocolImageGeneration|store.DirectProtocolImageEdit|store.DirectProtocolModelScopeImageGeneration, "")
					if _, err := db.Exec(`UPDATE upstream_group_items SET protocol_order='[262144]'`); err != nil {
						t.Fatal(err)
					}
					out := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", path, `{"model":"client-alias","prompt":"cat","image":"https://image.invalid/reference.png"}`))
					if out.Code != 200 || posts.Load() != 1 || polls.Load() != 1 || generic.Load() != 0 || !strings.Contains(out.Body.String(), "YWJj") {
						t.Fatalf("wrong ModelScope format selection: %d posts=%d polls=%d generic=%d %s", out.Code, posts.Load(), polls.Load(), generic.Load(), out.Body.String())
					}
				})
			}

		})
	}
}

func TestDirectMediaGeminiBatchEmbedding(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, override := range []string{"", `{"requests":[{"model":"models/override","content":{"parts":[{"text":"replacement"}]},"opaque":9007199254740993}]}`} {
				t.Run(override, func(t *testing.T) {
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						var body map[string]json.RawMessage
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Error(err)
						}
						if r.URL.Path != "/models/provider-model:batchEmbedContents" || r.Header.Get("x-goog-api-key") != "fixture-media-key" || body["model"] != nil {
							t.Errorf("wrong batch URL/auth/top-level model: %s %s", r.URL.Path, body["model"])
						}
						var requests []map[string]json.RawMessage
						_ = json.Unmarshal(body["requests"], &requests)
						want := 2
						if override != "" {
							want = 1
						}
						if len(requests) != want || !strings.Contains(string(body["requests"]), "9007199254740993") {
							t.Errorf("batch request/precision lost: %s", body["requests"])
						}
						for _, request := range requests {
							if string(request["model"]) != `"models/provider-model"` {
								t.Errorf("batch item escaped mapped model: %s", request["model"])
							}
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"embeddings":[{"values":[0,0.5]}]}`)
					}))
					defer server.Close()
					installDirectMediaRuntime(t, dialect, store.DirectEndpoints{GeminiEmbeddings: &store.DirectEndpoint{URL: server.URL + "/models", Auth: store.DirectAuthGoogle, ModelPath: true}}, store.DirectProtocolGeminiEmbeddings, override)
					config.RuntimeSafe().ProxyEmptyContentFailEnabled = true
					out := httptest.NewRecorder()
					body := `{"requests":[{"model":"models/client-alias","content":{"parts":[{"text":"first"}]},"opaque":9007199254740993},{"content":{"parts":[{"text":"second"}]}}]}`
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1beta/models/client-alias:batchEmbedContents", body))
					if out.Code != 200 || calls.Load() != 1 {
						t.Fatalf("native batch embedding failed: %d calls=%d %s", out.Code, calls.Load(), out.Body.String())
					}
					if override == "" {
						for _, bad := range []string{`null`, `{"requests":[]}`, `{"requests":[null]}`} {
							out := httptest.NewRecorder()
							directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1beta/models/client-alias:batchEmbedContents", bad))
							if out.Code != 400 || calls.Load() != 1 {
								t.Errorf("invalid batch reached provider: %d calls=%d", out.Code, calls.Load())
							}
						}
					}
				})
			}
		})
	}
}

func TestDirectMediaModelSuffixIsOpaque(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), "reasoning_effort") || !strings.Contains(string(body), `"model":"provider-model"`) {
					t.Errorf("media model became reasoning request: %s", body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"data":[{"embedding":[0.5]}]}`)
			}))
			defer server.Close()
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{Embeddings: &store.DirectEndpoint{URL: server.URL + "/embeddings", Auth: store.DirectAuthBearer}}, store.DirectProtocolEmbeddings, "")
			if _, err := db.Exec(`UPDATE token_routes SET model_pattern='client-alias-high'`); err != nil {
				t.Fatal(err)
			}
			out := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/embeddings", `{"model":"client-alias-high","input":"hi"}`))
			if out.Code != 200 {
				t.Fatalf("media suffix changed model routing: %d %s", out.Code, out.Body.String())
			}
		})
	}
}

func TestDirectMediaVideoContentIsAlwaysOpaque(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, typ := range []string{"application/json", "text/event-stream"} {
				t.Run(typ, func(t *testing.T) {
					content := `{"id":"opaque-file-id","model":"opaque-file-model","value":9007199254740993}`
					if typ == "text/event-stream" {
						content = "data: " + content + "\n\ndata: [DONE]\n\n"
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if r.Method == "POST" {
							_, _ = io.WriteString(w, `{"id":"provider-video","object":"video","status":"queued"}`)
						} else {
							w.Header().Set("Content-Type", typ)
							_, _ = io.WriteString(w, content)
						}
					}))
					defer upstream.Close()
					_, server, _ := directVideoFixture(t, dialect, upstream.URL)
					config.SetRuntime(&config.RuntimeSettings{ProxyEmptyContentFailEnabled: true})
					id := createdVideoID(t, server)
					status, header, body := videoHTTP(t, server, "GET", "/v1/videos/"+id+"/content", "", "client-one")
					if status != 200 || header.Get("Content-Type") != typ || string(body) != content {
						t.Fatalf("content was parsed as metadata: %d %s %s", status, header.Get("Content-Type"), body)
					}
				})
			}
		})
	}
}

func TestDirectMediaVideoFailedTaskRemainsAResource(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					_, _ = io.WriteString(w, `{"id":"provider-video","object":"video","status":"queued"}`)
				} else {
					_, _ = io.WriteString(w, `{"id":"provider-video","object":"video","status":"failed","error":{"code":"video_generation_failed","message":"task could not be generated"}}`)
				}
			}))
			defer upstream.Close()
			_, server, _ := directVideoFixture(t, dialect, upstream.URL)
			config.SetRuntime(&config.RuntimeSettings{ProxyEmptyContentFailEnabled: true})
			id := createdVideoID(t, server)
			status, _, body := videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
			if status != 200 || !strings.Contains(string(body), `"id":"`+id+`"`) || !strings.Contains(string(body), `"status":"failed"`) || !strings.Contains(string(body), `"code":"video_generation_failed"`) {
				t.Fatalf("populated failed task treated as empty upstream response: %d %s", status, body)
			}
		})
	}
}

func TestDirectMediaAsyncFailureDoesNotResubmit(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var posts, polls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posts.Add(1)
					_, _ = io.WriteString(w, `{"task_id":"created-once","usage":{"total_tokens":7}}`)
				} else {
					polls.Add(1)
					w.WriteHeader(503)
					_, _ = io.WriteString(w, `{"error":{"message":"temporary poll failure"}}`)
				}
			}))
			defer server.Close()
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{ModelScopeImageGeneration: &store.DirectEndpoint{URL: server.URL + "/images/generations", Auth: store.DirectAuthBearer, Profile: "modelscope-image"}}, store.DirectProtocolModelScopeImageGeneration, "")
			// Two independent credentials/channels are linked into the same group;
			// a single candidate would hide accidental cross-channel resubmission.
			for _, query := range []string{
				`INSERT INTO upstream_channels (id,origin_key,source_id,name,dialect,provider,enabled,base_url,endpoint_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) SELECT id+1000,origin_key,source_id+1000,name||' second',dialect,provider,enabled,base_url,endpoint_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex FROM upstream_channels`,
				`INSERT INTO upstream_credentials (id,origin_key,channel_id,source_id,name,secret,enabled) SELECT id+1000,origin_key,channel_id+1000,source_id+1000,name,secret,enabled FROM upstream_credentials`,
				`INSERT INTO upstream_models (id,origin_key,channel_id,source_id,name,enabled) SELECT id+1000,origin_key,channel_id+1000,source_id+1000,name,enabled FROM upstream_models`,
				`INSERT INTO upstream_grants (id,origin_key,source_id,model_id,credential_id,protocols,enabled) SELECT id+1000,origin_key,source_id+1000,model_id+1000,credential_id+1000,protocols,enabled FROM upstream_grants`,
				`INSERT INTO upstream_group_items (origin_key,group_id,source_id,grant_id,priority,weight,protocol_order) SELECT origin_key,group_id,source_id+1000,grant_id+1000,priority,weight,protocol_order FROM upstream_group_items`,
			} {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			var logs []proxy.ProxyLogEntry
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }
			out := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/images/generations", `{"model":"client-alias","prompt":"cat"}`))
			if out.Code < 500 || posts.Load() != 1 || polls.Load() != 1 {
				t.Fatalf("async poll failure submitted again: status=%d posts=%d polls=%d", out.Code, posts.Load(), polls.Load())
			}
			if len(logs) != 1 || logs[0].TotalTokens == nil || *logs[0].TotalTokens != 7 || logs[0].UsageSource == "unknown" {
				t.Fatalf("measured submission usage lost: %+v", logs)
			}
		})
	}
}

func TestDirectMediaBinaryCancellationIsNotUpstreamFailure(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			started := make(chan struct{})
			upstreamCanceled := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "audio/mpeg")
				_, _ = w.Write([]byte("ID3!"))
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
				close(upstreamCanceled)
			}))
			defer server.Close()
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{AudioSpeech: &store.DirectEndpoint{URL: server.URL + "/speech", Auth: store.DirectAuthBearer}}, store.DirectProtocolAudioSpeech, "")
			req := makeProxyReq("POST", "/v1/audio/speech", `{"model":"client-alias","input":"hi"}`)
			ctx, cancel := context.WithCancel(req.Context())
			defer cancel()
			req = req.WithContext(ctx)
			out := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); directMediaRouter().ServeHTTP(out, req) }()
			<-started
			cancel()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("media cancel did not stop relay")
			}
			select {
			case <-upstreamCanceled:
			case <-time.After(3 * time.Second):
				t.Fatal("downstream cancel did not reach upstream")
			}
			var failures int
			if err := db.Get(&failures, `SELECT SUM(fail_count) FROM upstream_grants`); err != nil || failures != 0 {
				t.Fatalf("client cancel poisoned grant: %d %v", failures, err)
			}

		})
	}
}

func TestDirectMediaBinaryLimitKeepsUnknownUsage(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			old := config.GetSafe()
			cfg := *old
			cfg.ProxyMaxStreamResponseBytes = 8
			config.Set(&cfg)
			defer config.Set(old)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "audio/mpeg")
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				_, _ = io.WriteString(w, "01234567890123456789")
			}))
			defer server.Close()
			installDirectMediaRuntime(t, dialect, store.DirectEndpoints{AudioSpeech: &store.DirectEndpoint{URL: server.URL + "/speech", Auth: store.DirectAuthBearer}}, store.DirectProtocolAudioSpeech, "")
			var log proxy.ProxyLogEntry
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { log = entry; return nil }
			out := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/audio/speech", `{"model":"client-alias","input":"hi"}`))
			if out.Body.String() != "01234567" || out.Header().Get("Content-Type") != "audio/mpeg" {
				t.Fatalf("binary limit corrupted payload: %q %q", out.Header().Get("Content-Type"), out.Body.String())
			}
			if log.HTTPStatus != 502 || log.UsageSource != "unknown" || log.TotalTokens != nil {
				t.Fatalf("unknown media usage became billed zero or limit looked successful: %+v", log)
			}

		})
	}
}
