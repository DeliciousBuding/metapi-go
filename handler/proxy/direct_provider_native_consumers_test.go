package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderSynchronousMultipartConsumersKeepFileBytesInHost(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, path := range []string{"/v1/images/edits", "/v1/images/variations", "/v1/audio/transcriptions", "/v1/audio/translations"} {
			t.Run(dialect+path, func(t *testing.T) {
				var calls atomic.Int32
				data := []byte{0, 1, 2, 255, 13, 10, 0}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if r.FormValue("model") != "provider-model" || r.FormValue("prompt") != "overridden" || strings.Join(r.MultipartForm.Value["timestamp_granularities[]"], ",") != "word,segment" || r.Header.Get("X-Driver-Request") != "active" {
						t.Error("synchronous multipart metadata changed")
					}
					file := r.MultipartForm.File["file"][0]
					reader, err := file.Open()
					if err != nil {
						t.Error(err)
						return
					}
					defer reader.Close()
					raw, _ := io.ReadAll(reader)
					if !bytes.Equal(raw, data) || file.Filename != "source.wav" || file.Header.Get("Content-Type") != "audio/wav" {
						t.Error("multipart file data/name/type changed")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"text":"receipt","usage":{"input_tokens":7,"total_tokens":7}}`)
				}))
				defer upstream.Close()
				bit := proxy.DirectProtocolForPath(path)
				var endpoints store.DirectEndpoints
				key := ""
				for _, entry := range endpoints.Entries() {
					if entry.Protocol == bit {
						key = entry.Key
					}
				}
				raw, _ := json.Marshal(map[string]any{key: map[string]string{"url": upstream.URL + path, "auth": "bearer"}})
				_ = json.Unmarshal(raw, &endpoints)
				db := installDirectMediaRuntime(t, dialect, endpoints, bit, `{"prompt":"overridden"}`)
				source := fmt.Sprintf(`export const meta={apiVersion:2,kind:"provider",key:"native",name:"Native",version:"1.0.0",protocols:[%q],credentialKinds:["api_key"],requestKinds:["json","multipart"],hooks:["request","response"]};export function request(i){if(i.bodyKind!=="multipart"||Object.keys(i.files[0]).sort().join(",")!=="field,filename,mimeType,ref,size")throw new Error("metadata only");const b=JSON.parse(i.body);return {method:"POST",headers:{"X-Driver-Request":"active"},parts:[{field:"model",value:i.model},{field:"prompt",value:b.prompt},{field:"timestamp_granularities[]",value:b["timestamp_granularities[]"][0]},{field:"timestamp_granularities[]",value:b["timestamp_granularities[]"][1]},{field:"file",fileRef:i.files[0].ref}]};}export function response(i){return {body:i.body};}`, key)
				if _, err := channelplugins.Save(t.Context(), db.DB, "native", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "native", Protocols: []string{key}}})
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				_ = writer.WriteField("model", "client-alias")
				_ = writer.WriteField("prompt", "original")
				_ = writer.WriteField("timestamp_granularities[]", "word")
				_ = writer.WriteField("timestamp_granularities[]", "segment")
				part, _ := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="source.wav"`}, "Content-Type": {"audio/wav"}})
				_, _ = part.Write(data)
				_ = writer.Close()
				request := httptest.NewRequest("POST", path, &body)
				request.Header.Set("Content-Type", writer.FormDataContentType())
				request = request.WithContext(auth.WithProxyAuth(request.Context(), &auth.ProxyAuthContext{Source: "global", Token: "fixture-client"}))
				response := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(response, request)
				if response.Code != 200 || calls.Load() != 1 || !strings.Contains(response.Body.String(), "receipt") {
					t.Fatalf("multipart consumer failed: %d calls=%d %s", response.Code, calls.Load(), response.Body.String())
				}
			})
		}
	}
}

func TestProviderAudioBinaryConsumerDoesNotInvokeJSONHooks(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			data := []byte{0, 1, 255, 128, 13, 10, 0}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("X-Driver-Request") != "active" {
					t.Error("request hook absent")
				}
				w.Header().Set("Content-Type", "audio/mpeg")
				_, _ = w.Write(data)
			}))
			defer upstream.Close()
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{AudioSpeech: &store.DirectEndpoint{URL: upstream.URL + "/speech", Auth: store.DirectAuthBearer}}, store.DirectProtocolAudioSpeech, "")
			const source = `export const meta={apiVersion:2,kind:"provider",key:"native",name:"Native",version:"1.0.0",protocols:["audioSpeech"],credentialKinds:["api_key"],hooks:["request","response","decodeResponse"]};export function request(i){return {method:"POST",body:i.body,headers:{"X-Driver-Request":"active"}};}export function response(){throw new Error("binary must not enter JS");}export function decodeResponse(){throw new Error("binary must not enter JS");}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "native", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "native", Protocols: []string{"audioSpeech"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			response := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(response, makeProxyReq("POST", "/v1/audio/speech", `{"model":"client-alias","input":"fixture"}`))
			if response.Code != 200 || calls.Load() != 1 || response.Header().Get("Content-Type") != "audio/mpeg" || !bytes.Equal(response.Body.Bytes(), data) {
				t.Fatalf("binary response was altered: status=%d calls=%d bytes=%v", response.Code, calls.Load(), response.Body.Bytes())
			}
		})
	}
}

func TestProviderExplicitDriverSkipsBuiltinImagePreparation(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, profile := range []string{"openrouter-image", "minimax-image", "modelscope-image"} {
			t.Run(dialect+"/"+profile, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					var body map[string]json.RawMessage
					if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["prompt"]) != `"fixture prompt"` || string(body["model"]) != `"provider-model"` {
						t.Error("explicit driver did not receive native image request")
					}
					for _, field := range []string{"messages", "modalities", "size", "response_format", "width", "height"} {
						if _, exists := body[field]; exists {
							t.Errorf("explicit provider inherited built-in field %s", field)
						}
					}
					if r.Header.Get("X-ModelScope-Async-Mode") != "" || r.Header.Get("Authorization") != "Bearer fixture-media-key" {
						t.Error("explicit provider changed credential owner or inherited asynchronous mode")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"data":[{"url":"https://fixture.invalid/image"}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}`)
				}))
				defer upstream.Close()
				key, bit := "imageGeneration", store.DirectProtocolImageGeneration
				endpoint := &store.DirectEndpoint{URL: upstream.URL + "/images", Auth: store.DirectAuthBearer, Profile: profile}
				endpoints := store.DirectEndpoints{ImageGeneration: endpoint}
				if profile == "modelscope-image" {
					key, bit = "modelscopeImageGeneration", store.DirectProtocolModelScopeImageGeneration
					endpoints = store.DirectEndpoints{ModelScopeImageGeneration: endpoint}
				}
				db := installDirectMediaRuntime(t, dialect, endpoints, bit, "")
				source := fmt.Sprintf(`export const meta={apiVersion:2,kind:"provider",key:"native",name:"Native",version:"1.0.0",protocols:[%q],credentialKinds:["api_key"],hooks:["request","response"]};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}`, key)
				if _, err := channelplugins.Save(t.Context(), db.DB, "native", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "native", Protocols: []string{key}}})
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				response := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(response, makeProxyReq("POST", "/v1/images/generations", `{"model":"client-alias","prompt":"fixture prompt"}`))
				if response.Code != 200 || calls.Load() != 1 || !strings.Contains(response.Body.String(), `"data"`) {
					t.Fatalf("explicit image driver did not own its wire: %d calls=%d %s", response.Code, calls.Load(), response.Body.String())
				}
			})
		}
	}
}

func TestProviderExplicitImageFailureDoesNotReplayToAnotherCredential(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, profile := range []string{"openrouter-image", "minimax-image", "modelscope-image"} {
			t.Run(dialect+"/"+profile, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(503)
					_, _ = io.WriteString(w, `{"error":{"message":"fixture image failure"},"usage":{"total_tokens":7}}`)
				}))
				defer upstream.Close()
				key, bit := "imageGeneration", store.DirectProtocolImageGeneration
				endpoint := &store.DirectEndpoint{URL: upstream.URL + "/images", Auth: store.DirectAuthBearer, Profile: profile}
				endpoints := store.DirectEndpoints{ImageGeneration: endpoint}
				if profile == "modelscope-image" {
					key, bit = "modelscopeImageGeneration", store.DirectProtocolModelScopeImageGeneration
					endpoints = store.DirectEndpoints{ModelScopeImageGeneration: endpoint}
				}
				db := installDirectMediaRuntime(t, dialect, endpoints, bit, "")
				source := fmt.Sprintf(`export const meta={apiVersion:2,kind:"provider",key:"native",name:"Native",version:"1.0.0",protocols:[%q],credentialKinds:["api_key"],hooks:["request","response"]};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}`, key)
				if _, err := channelplugins.Save(t.Context(), db.DB, "native", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "native", Protocols: []string{key}}})
				for _, query := range []string{
					`INSERT INTO upstream_channels (id,origin_key,source_id,name,dialect,provider,enabled,base_url,endpoint_config,request_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) SELECT id+1000,origin_key,source_id+1000,name||' second',dialect,provider,enabled,base_url,endpoint_config,request_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex FROM upstream_channels`,
					`INSERT INTO upstream_credentials (id,origin_key,channel_id,source_id,name,secret,enabled) SELECT id+1000,origin_key,channel_id+1000,source_id+1000,name,secret,enabled FROM upstream_credentials`,
					`INSERT INTO upstream_models (id,origin_key,channel_id,source_id,name,enabled) SELECT id+1000,origin_key,channel_id+1000,source_id+1000,name,enabled FROM upstream_models`,
					`INSERT INTO upstream_grants (id,origin_key,source_id,model_id,credential_id,protocols,enabled) SELECT id+1000,origin_key,source_id+1000,model_id+1000,credential_id+1000,protocols,enabled FROM upstream_grants`,
					`INSERT INTO upstream_group_items (origin_key,group_id,source_id,grant_id,priority,weight,protocol_order) SELECT origin_key,group_id,source_id+1000,grant_id+1000,priority,weight,protocol_order FROM upstream_group_items`,
				} {
					if _, err := db.Exec(query); err != nil {
						t.Fatal(err)
					}
				}
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				previous := config.Get()
				cfg := *previous
				cfg.ProxyMaxChannelAttempts = 4
				config.Set(&cfg)
				defer config.Set(previous)
				var logs []proxy.ProxyLogEntry
				getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs = append(logs, entry); return nil }
				response := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(response, makeProxyReq("POST", "/v1/images/generations", `{"model":"client-alias","prompt":"fixture prompt"}`))
				if response.Code != 503 || calls.Load() != 1 || len(logs) != 1 || logs[0].TotalTokens == nil || *logs[0].TotalTokens != 7 {
					t.Fatalf("explicit image was replayed or lost billable failure: status=%d calls=%d logs=%+v", response.Code, calls.Load(), logs)
				}
			})
		}
	}
}

func TestProviderNativeJSONConsumersDispatchAndProject(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, endpoint := range []struct {
			key, path string
			bit       int
		}{
			{"completions", "/v1/completions", store.DirectProtocolCompletions},
			{"embeddings", "/v1/embeddings", store.DirectProtocolEmbeddings},
			{"rerank", "/v1/rerank", store.DirectProtocolRerank},
			{"imageGeneration", "/v1/images/generations", store.DirectProtocolImageGeneration},
			{"imageEdit", "/v1/images/edits", store.DirectProtocolImageEdit},
			{"imageVariation", "/v1/images/variations", store.DirectProtocolImageVariation},
			{"audioSpeech", "/v1/audio/speech", store.DirectProtocolAudioSpeech},
			{"audioTranscription", "/v1/audio/transcriptions", store.DirectProtocolAudioTranscription},
			{"audioTranslation", "/v1/audio/translations", store.DirectProtocolAudioTranslation},
			{"moderations", "/v1/moderations", store.DirectProtocolModerations},
			{"jinaEmbeddings", "/v1/embeddings", store.DirectProtocolJinaEmbeddings},
			{"modelscopeImageGeneration", "/v1/images/generations", store.DirectProtocolModelScopeImageGeneration},
			{"geminiEmbeddings", "/v1beta/models/client-alias:embedContent", store.DirectProtocolGeminiEmbeddings},
			{"systemOne", "/v1/systemone", store.DirectProtocolSystemOne},
			{"alphaSearch", "/v1/alpha/search", store.DirectProtocolAlphaSearch},
			{"decisions", "/v1/decisions", store.DirectProtocolDecisions},
		} {
			t.Run(dialect+"/"+endpoint.key, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("X-Driver-Request") != "active" || r.URL.Path != "/vendor/"+endpoint.key {
						t.Error("driver request/endpoint/authentication not applied")
					}
					if endpoint.key == "modelscopeImageGeneration" && r.Header.Get("X-ModelScope-Async-Mode") != "" {
						t.Error("explicit provider inherited built-in asynchronous mode")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"receipt":"native","extension":9007199254740993,"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}`)
				}))
				defer upstream.Close()
				var endpoints store.DirectEndpoints
				profile := ""
				if endpoint.key == "jinaEmbeddings" {
					profile = "jina-embeddings"
				}
				if endpoint.key == "modelscopeImageGeneration" {
					profile = "modelscope-image"
				}
				raw, _ := json.Marshal(map[string]any{endpoint.key: map[string]string{"url": upstream.URL + "/vendor/" + endpoint.key, "auth": "bearer", "profile": profile}})
				if err := json.Unmarshal(raw, &endpoints); err != nil {
					t.Fatal(err)
				}
				db := installDirectMediaRuntime(t, dialect, endpoints, endpoint.bit, "")
				source := fmt.Sprintf(`export const meta={apiVersion:2,kind:"provider",key:"native",name:"Native",version:"1.0.0",protocols:[%q],credentialKinds:["api_key"],hooks:["request","response"]};export function request(i){return {method:"POST",body:i.body,headers:{"X-Driver-Request":"active"}};}export function response(i){return {body:i.body.replace('"receipt":"native"','"receipt":"projected"')};}`, endpoint.key)
				if _, err := channelplugins.Save(t.Context(), db.DB, "native", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "native", Protocols: []string{endpoint.key}}})
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				request := makeProxyReq("POST", endpoint.path, `{"model":"client-alias","input":"fixture","prompt":"fixture","questions":["fixture question"]}`)
				response := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(response, request)
				if response.Code != 200 || calls.Load() != 1 || !strings.Contains(response.Body.String(), `"receipt":"projected"`) || !strings.Contains(response.Body.String(), "9007199254740993") {
					t.Fatalf("provider native consumer absent: %d calls=%d %s", response.Code, calls.Load(), response.Body.String())
				}
			})
		}
	}
}

func TestProviderOllamaNativeSlotKeepsRawUsageAcrossProjection(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("X-Driver-Request") != "active" {
					t.Error("Ollama provider was silently skipped")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"model":"provider-model","done":true,"done_reason":"stop","message":{"role":"assistant","content":"receipt"},"prompt_eval_count":5,"eval_count":2}`)
			}))
			defer upstream.Close()
			db := installOllamaHTTPRuntime(t, dialect, upstream.URL, false)
			const source = `export const meta={apiVersion:2,kind:"provider",key:"native",name:"Native",version:"1.0.0",protocols:["ollama"],credentialKinds:["api_key"],hooks:["request","response"]};export function request(i){return {method:"POST",body:i.body,headers:{"X-Driver-Request":"active"}};}export function response(i){return {body:i.body};}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "native", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "native", Protocols: []string{"ollama"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			var tokens int64
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error {
				if entry.TotalTokens != nil {
					tokens += *entry.TotalTokens
				}
				return nil
			}
			response := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(response, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"fixture"}]}`))
			if response.Code != 200 || calls.Load() != 1 || tokens != 7 {
				t.Fatalf("Ollama driver skipped raw facts: status=%d calls=%d tokens=%d %s", response.Code, calls.Load(), tokens, response.Body.String())
			}
		})
	}
}

func TestProviderNativeDecoderFailureRetainsOriginalUsage(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"normal", "throw", "forged-usage", "wrong-shape", "raw-error"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					native := `{"object":"list","data":[{"object":"embedding","embedding":[0.1],"index":0}],"model":"provider-model","usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}`
					raw := map[string]any{"native": native, "model": "provider-model", "meter": map[string]int{"input": 7, "output": 2, "total": 9}}
					if mode == "raw-error" {
						raw["error"] = "fixture-private-error"
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"raw": raw})
				}))
				defer upstream.Close()
				db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{Embeddings: &store.DirectEndpoint{URL: upstream.URL + "/embedding", Auth: store.DirectAuthBearer}}, store.DirectProtocolEmbeddings, "")
				source := `export const meta={apiVersion:2,kind:"provider",key:"native",name:"Native",version:"1.0.0",protocols:["embeddings"],credentialKinds:["api_key"],hooks:["request","response","decodeResponse"],observations:{embeddings:{response:{usagePaths:{input_tokens:"/raw/meter/input",output_tokens:"/raw/meter/output",total_tokens:"/raw/meter/total"},modelPath:"/raw/model",errorPath:"/raw/error"}}}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}export function decodeResponse(i){return {body:JSON.parse(i.body).raw.native};}`
				if mode == "throw" {
					source = strings.Replace(source, `return {body:JSON.parse(i.body).raw.native};`, `throw new Error("fixture-private-decoder");`, 1)
				}
				if mode == "forged-usage" {
					source = strings.Replace(source, `JSON.parse(i.body).raw.native`, `JSON.parse(i.body).raw.native.replace('"total_tokens":9','"total_tokens":90')`, 1)
				}
				if mode == "wrong-shape" {
					source = strings.Replace(source, `JSON.parse(i.body).raw.native`, `'{}'`, 1)
				}
				if _, err := channelplugins.Save(t.Context(), db.DB, "native", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "native", Protocols: []string{"embeddings"}}})
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				var tokens int64
				getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error {
					if entry.TotalTokens != nil {
						tokens += *entry.TotalTokens
					}
					return nil
				}
				response := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(response, makeProxyReq("POST", "/v1/embeddings", `{"model":"client-alias","input":"fixture"}`))
				want := 502
				if mode == "normal" {
					want = 200
				}
				if response.Code != want || calls.Load() != 1 || tokens != 9 || strings.Contains(response.Body.String(), "fixture-private") {
					t.Fatalf("decoder changed failure or raw billing: status=%d calls=%d tokens=%d %s", response.Code, calls.Load(), tokens, response.Body.String())
				}
			})
		}
	}
}

func TestProviderCompletionsSSEUsesEventHookAndRawUsage(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"object\":\"text_completion\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"text\":\"native-text\",\"finish_reason\":\"stop\"}],\"usage\":{\"input_tokens\":7,\"output_tokens\":2,\"total_tokens\":9}}\n\ndata: [DONE]\n\n")
			}))
			defer upstream.Close()
			db := installDirectMediaRuntime(t, dialect, store.DirectEndpoints{Completions: &store.DirectEndpoint{URL: upstream.URL + "/completion", Auth: store.DirectAuthBearer}}, store.DirectProtocolCompletions, "")
			const source = `export const meta={apiVersion:2,kind:"provider",key:"native",name:"Native",version:"1.0.0",protocols:["completions"],credentialKinds:["api_key"],hooks:["request","response","event"]};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}export function event(i){return {body:i.body.replace("native-text","projected-text")};}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "native", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "native", Protocols: []string{"completions"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			var tokens int64
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error {
				if entry.TotalTokens != nil {
					tokens += *entry.TotalTokens
				}
				return nil
			}
			response := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(response, makeProxyReq("POST", "/v1/completions", `{"model":"client-alias","prompt":"fixture","stream":true}`))
			if response.Code != 200 || calls.Load() != 1 || tokens != 9 || !strings.Contains(response.Body.String(), "projected-text") || !strings.Contains(response.Body.String(), "[DONE]") {
				t.Fatalf("completion SSE lost events or raw billing: status=%d calls=%d tokens=%d %s", response.Code, calls.Load(), tokens, response.Body.String())
			}
		})
	}
}
