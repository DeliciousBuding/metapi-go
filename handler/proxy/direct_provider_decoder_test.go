package proxyhandler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const providerDecoderSource = `export const meta={apiVersion:2,kind:"provider",key:"decoder",name:"Decoder",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response","event","decodeResponse","decodeEvent"],observations:{chat:{response:{usagePaths:{prompt_tokens:"/vendor/meter/in",completion_tokens:"/vendor/meter/out",total_tokens:"/vendor/meter/total"},modelPath:"/vendor/model",errorPath:"/vendor/error"},event:{usagePaths:{prompt_tokens:"/vendor/meter/in",completion_tokens:"/vendor/meter/out",total_tokens:"/vendor/meter/total"},modelPath:"/vendor/model",errorPath:"/vendor/error"}}}};export function request(i){return {method:"POST",body:i.body};}export function decodeResponse(i){return {body:JSON.parse(i.body).vendor.native};}export function decodeEvent(i){return decodeResponse(i);}export function response(i){return {body:i.body};}export function event(i){return {body:i.body};}`

func TestProviderDecoderActualJSONAndSSEObservesBeforeHooks(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, mode := range []string{"normal", "decode-failure", "projection-failure", "forged-usage", "case-shadow-usage", "original-error"} {
				t.Run(mode, func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						raw, _ := io.ReadAll(r.Body)
						stream := strings.Contains(string(raw), `"stream":true`)
						native := `{"id":"fixture-decoder","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"vendor receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9},"extension":9007199254740993}`
						if stream {
							native = `{"id":"fixture-decoder","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"content":"vendor receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9},"extension":9007199254740993}`
						}
						vendor := map[string]any{"native": native, "meter": map[string]int{"in": 7, "out": 2, "total": 9}, "model": "provider-model"}
						if mode == "original-error" {
							vendor["error"] = map[string]any{"message": "synthetic-private-error"}
						}
						body, _ := json.Marshal(map[string]any{"vendor": vendor})
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write(body)
						}
					}))
					defer upstream.Close()
					endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/generate", Auth: store.DirectAuthBearer}}
					db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", endpoints, store.DirectProtocolChat, "")
					source := providerDecoderSource
					if mode == "decode-failure" {
						source = strings.Replace(source, `return {body:JSON.parse(i.body).vendor.native};`, `throw new Error("synthetic-private-decoder");`, 1)
					}
					if mode == "projection-failure" {
						source = strings.ReplaceAll(source, `return {body:i.body};`, `throw new Error("synthetic-private-projection");`)
					}
					if mode == "forged-usage" {
						source = strings.Replace(source, `JSON.parse(i.body).vendor.native`, `JSON.parse(i.body).vendor.native.replace('"total_tokens":9','"total_tokens":90')`, 1)
					}
					if mode == "case-shadow-usage" {
						source = strings.Replace(source, `JSON.parse(i.body).vendor.native`, `JSON.parse(i.body).vendor.native.slice(0,-1)+',"Usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}'`, 1)
					}
					if _, err := channelplugins.Save(t.Context(), db.DB, "decoder", source, true, 0); err != nil {
						t.Fatalf("provider decoder cannot be installed: %v", err)
					}
					binding := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "decoder", Protocols: []string{"chat"}}}
					videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, binding)
					getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
					for _, stream := range []bool{false, true} {
						w := httptest.NewRecorder()
						directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)))
						if mode == "normal" {
							if w.Code != 200 || !strings.Contains(w.Body.String(), "vendor receipt") || !strings.Contains(w.Body.String(), "9007199254740993") {
								t.Fatalf("decoder output is unusable: stream=%v status=%d body=%s", stream, w.Code, w.Body.String())
							}
						} else if !stream && w.Code != 502 || stream && (strings.Contains(w.Body.String(), "vendor receipt") || !strings.Contains(w.Body.String(), "error")) {
							t.Fatalf("decoder/projection failure became success: mode=%s stream=%v status=%d body=%s", mode, stream, w.Code, w.Body.String())
						}
						if strings.Contains(w.Body.String(), "synthetic-private") {
							t.Fatal("private decoder or source error leaked")
						}
						select {
						case log := <-logs:
							if log.UsageSource != "upstream" || log.TotalTokens == nil || *log.TotalTokens != 9 || log.PromptTokens == nil || *log.PromptTokens != 7 || log.CompletionTokens == nil || *log.CompletionTokens != 2 {
								t.Fatalf("original vendor facts were lost: %+v", log)
							}
							if mode == "original-error" {
								if log.UpstreamReportedModel != nil {
									t.Fatal("error body became a verified model identity")
								}
							} else if log.UpstreamReportedModel == nil || *log.UpstreamReportedModel != "provider-model" {
								t.Fatal("source model metadata was lost")
							}
						case <-time.After(3 * time.Second):
							t.Fatal("provider attempt was not recorded")
						}
					}
					if calls.Load() != 2 || len(logs) != 0 {
						t.Fatalf("decoder failure replayed upstream: calls=%d extraLogs=%d", calls.Load(), len(logs))
					}
				})
			}
		})
	}
}

func TestProviderDecoderFlatVendorFields(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				body := `{"type":"vendor.delta","request_id":"vendor-flat","engine":"provider-model","reply":"flat vendor receipt","finished":true,"meter":{"in":7,"out":2,"total":9}}`
				if strings.Contains(string(raw), `"stream":true`) {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "event: vendor.event\nid: vendor-id\nretry: 50\ndata: %s\n\ndata: [DONE]\n\n", body)
				} else {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, body)
				}
			}))
			defer upstream.Close()
			db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/generate", Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			source := strings.ReplaceAll(providerDecoderSource, "/vendor/meter/", "/meter/")
			source = strings.ReplaceAll(source, "/vendor/model", "/engine")
			source = strings.ReplaceAll(source, `return {body:JSON.parse(i.body).vendor.native};`, `const p=JSON.parse(i.body);const choice=i.stream?{index:0,delta:{content:p.reply},finish_reason:p.finished?"stop":null}:{index:0,message:{role:"assistant",content:p.reply},finish_reason:"stop"};return {body:JSON.stringify({id:p.request_id,object:i.stream?"chat.completion.chunk":"chat.completion",model:p.engine,choices:[choice],usage:{prompt_tokens:p.meter.in,completion_tokens:p.meter.out,total_tokens:p.meter.total}})};`)
			if _, err := channelplugins.Save(t.Context(), db.DB, "decoder", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "decoder", Protocols: []string{"chat"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			for _, stream := range []bool{false, true} {
				w := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)))
				if w.Code != 200 || !strings.Contains(w.Body.String(), "flat vendor receipt") {
					t.Fatalf("flat schema was not converted: %v %d %s", stream, w.Code, w.Body.String())
				}
				if stream && (!strings.Contains(w.Body.String(), "event: message") || strings.Contains(w.Body.String(), "event: vendor.event") || !strings.Contains(w.Body.String(), "id: vendor-id") || !strings.Contains(w.Body.String(), "retry: 50")) {
					t.Fatalf("decoded SSE event name or transport metadata is wrong: %s", w.Body.String())
				}
				platformUsage(t, logs, "success", 7, 2)
			}
		})
	}
}

func TestProviderDecoderLimitRetainsKnownNativeUsage(t *testing.T) {
	// Use a real manager snapshot instead of a public program override.
	db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://fixture.invalid/generate", Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
	if _, err := channelplugins.Save(t.Context(), db.DB, "decoder", providerDecoderSource, true, 0); err != nil {
		t.Fatal(err)
	}
	invocation, err := channelplugins.New(db.DB).ResolveDriver(t.Context(), store.DirectProviderBinding{Key: "decoder", Protocols: []string{"chat"}}, "chat", store.DirectCredentialAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	oversized := []byte(`{"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9},"padding":"` + strings.Repeat("x", channelplugin.MaxInputBytes) + `"}`)
	_, usage, err := decodeDirectDriverPayload(t.Context(), &directProviderWire{Driver: &invocation, PluginProtocol: "chat"}, oversized, false, 200)
	if err == nil || usage.TotalTokens != 9 || usage.Source != usageSourceUpstream {
		t.Fatalf("execution limit discarded known native facts: %+v %v", usage, err)
	}
}

func TestProviderDecoderRequiredStreamAggregationRetainsRawFacts(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, fail := range []bool{false, true} {
				t.Run(fmt.Sprint(fail), func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						raw, _ := io.ReadAll(r.Body)
						if !strings.Contains(string(raw), `"stream":true`) {
							t.Error("required stream was not sent")
						}
						w.Header().Set("Content-Type", "text/event-stream")
						native := `{"id":"aggregate-decoder","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"content":"aggregate vendor receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`
						body, _ := json.Marshal(map[string]any{"vendor": map[string]any{"native": native, "model": "provider-model", "meter": map[string]int{"in": 7, "out": 2, "total": 9}}})
						fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
					}))
					defer upstream.Close()
					db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/generate", Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
					source := providerDecoderSource
					if fail {
						source = strings.Replace(source, `return {body:JSON.parse(i.body).vendor.native};`, `throw new Error("synthetic-private-decoder");`, 1)
					}
					if _, err := channelplugins.Save(t.Context(), db.DB, "decoder", source, true, 0); err != nil {
						t.Fatal(err)
					}
					videoExec(t, db, `UPDATE upstream_channels SET request_config=?,stream_policy='require'`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "decoder", Protocols: []string{"chat"}}})
					getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
					prior := config.GetSafe()
					next := *prior
					next.ProxyMaxChannelAttempts = 4
					config.Set(&next)
					defer config.Set(prior)
					routing.InvalidateCache()
					w := httptest.NewRecorder()
					directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"stream":false}`))
					status, logStatus := 200, "success"
					if fail {
						status, logStatus = 502, "failed"
					}
					if w.Code != status || calls.Load() != 1 || strings.Contains(w.Body.String(), "synthetic-private") || strings.Contains(w.Body.String(), "data:") {
						t.Fatalf("aggregate decoder replay/output changed: %d calls=%d body=%s", w.Code, calls.Load(), w.Body.String())
					}
					if !fail && !strings.Contains(w.Body.String(), "aggregate vendor receipt") {
						t.Fatal("decoded aggregate content lost")
					}
					platformUsage(t, logs, logStatus, 7, 2)
				})
			}
		})
	}
}

func TestWireProjectionCannotIntroduceResponseControlAliases(t *testing.T) {
	for _, field := range []string{"MODEL", "Usage", "Response"} {
		t.Run(field, func(t *testing.T) {
			module := `export const meta={apiVersion:1,key:"aliases",name:"Aliases",version:"1.0.0",protocols:["chat"],hooks:["response"]};export function response(){return {body:[{op:"set",path:"` + field + `",value:'"shadow"'}]};}`
			program, err := channelplugin.Compile(t.Context(), module)
			if err != nil {
				t.Fatal(err)
			}
			wire := &directProviderWire{PluginProtocol: "chat", Plugins: []channelplugins.Invocation{{Program: program}}}
			if _, err = applyDirectChannelResponsePlugins(t.Context(), wire, []byte(`{"model":"provider-model","usage":{"total_tokens":9},"choices":[]}`), "response"); err != channelplugin.ErrResult {
				t.Fatalf("wire hook introduced %s: %v", field, err)
			}
		})
	}
}
