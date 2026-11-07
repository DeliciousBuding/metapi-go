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

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const channelPluginSource = `export const meta={apiVersion:1,key:"fixture",name:"Fixture",version:"1.0.0",protocols:["chat"],hooks:["request","response","event"],options:[{key:"mode",label:"Mode",type:"string",default:"safe"}]};export function request(i){return {body:[{op:"set",path:"vendor.mode",value:i.options.mode}],headers:[{op:"set",path:"X-Plugin-Mode",value:i.options.mode}]};}export function response(i){return {body:[{op:"set",path:"vendor.receipt",value:"true"}]};}export function event(i){return {body:[{op:"set",path:"vendor.receipt",value:"true"}]};}`

const channelProviderSource = `export const meta={apiVersion:2,kind:"provider",key:"driver",name:"Driver",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response","event"],options:[{key:"mode",label:"Mode",type:"string",default:"safe"}]};export function request(i){return {method:"POST",path:"/provider/generate",body:i.body,headers:{"X-Provider-Mode":i.options.mode}};}export function response(i){return {body:i.body.replace('"content":"hello"','"content":"provider hello"')};}export function event(i){return response(i);}`

func TestProviderDriverCannotShadowModelOrDropOutputCaps(t *testing.T) {
	for _, change := range []string{"duplicate-model", "case-shadow-model", "drop-cap", "endpoint-escape"} {
		t.Run(change, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"fixture","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`)
			}))
			t.Cleanup(upstream.Close)
			db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/provider/generate", Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			replacement := `body:i.body.slice(0,-1)+',"model":"forbidden-model"}'`
			if change == "case-shadow-model" {
				replacement = `body:i.body.slice(0,-1)+',"MODEL":"forbidden-model"}'`
			}
			if change == "drop-cap" {
				replacement = `body:JSON.stringify({...JSON.parse(i.body),max_tokens:undefined})`
			}
			source := strings.Replace(channelProviderSource, "body:i.body,headers:", replacement+",headers:", 1)
			if change == "endpoint-escape" {
				source = strings.Replace(channelProviderSource, `path:"/provider/generate"`, `path:"/other-operation"`, 1)
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "driver", source, true, 0); err != nil {
				t.Fatal(err)
			}
			binding := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "driver", Protocols: []string{"chat"}}}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, binding)
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			w := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"max_tokens":16}`))
			if w.Code != 502 || calls.Load() != 0 {
				t.Fatalf("driver bypassed selected model/output cap: change=%s status=%d calls=%d", change, w.Code, calls.Load())
			}
		})
	}
}

func TestInstalledProviderDriverActualHTTPJSONAndSSE(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var dispatched atomic.Int64
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched.Add(1)
				raw, _ := io.ReadAll(r.Body)
				if r.URL.Path != "/provider/generate" || r.Header.Get("X-Provider-Mode") != "custom" || r.Header.Get("Authorization") != "Bearer fixture-media-key" || !strings.Contains(string(raw), "9007199254740993") {
					t.Errorf("provider descriptor/auth/input not used: path=%s body=%s", r.URL.Path, raw)
				}
				if strings.Contains(string(raw), `"stream":true`) {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
					fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2,\"total_tokens\":9}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"fixture","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`)
				}
			}))
			defer up.Close()
			endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: up.URL + "/provider/generate", Auth: store.DirectAuthBearer}}
			db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", endpoints, store.DirectProtocolChat, "")
			if _, err := channelplugins.Save(t.Context(), db.DB, "driver", channelProviderSource, true, 0); err != nil {
				t.Fatal(err)
			}
			binding := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "driver", Protocols: []string{"chat"}, Options: map[string]json.RawMessage{"mode": json.RawMessage(`"custom"`)}}}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), binding); err != nil {
				t.Fatal(err)
			}
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			for _, stream := range []bool{false, true} {
				w := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"seed":9007199254740993,"stream":%t}`, stream)))
				if w.Code != 200 || !strings.Contains(w.Body.String(), "provider hello") {
					t.Fatalf("driver was bypassed: stream=%v %d %s", stream, w.Code, w.Body.String())
				}
				platformUsage(t, logs, "success", 7, 2)
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "driver", channelProviderSource, false, 1); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
			if w.Code != 503 || dispatched.Load() != 2 {
				t.Fatalf("disabled driver bypassed: %d calls=%d", w.Code, dispatched.Load())
			}
		})
	}
}

func TestInstalledChannelPluginActualHTTPJSONAndSSE(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var dispatched atomic.Int64
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched.Add(1)
				raw, _ := io.ReadAll(r.Body)
				if r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("X-Plugin-Mode") != "custom" || !strings.Contains(string(raw), `"mode":"custom"`) || !strings.Contains(string(raw), `9007199254740993`) {
					t.Errorf("plugin/auth/native wire: %s", raw)
				}
				if strings.Contains(string(raw), `"stream":true`) {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
					fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"model\":\"provider-model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":2,\"total_tokens\":9}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"fixture","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2,"total_tokens":9}}`)
				}
			}))
			defer up.Close()
			endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: up.URL + "/v1/chat/completions", Auth: store.DirectAuthBearer}}
			db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", endpoints, store.DirectProtocolChat, "")
			if _, err := channelplugins.Save(t.Context(), db.DB, "fixture", channelPluginSource, true, 0); err != nil {
				t.Fatal(err)
			}
			config := store.DirectRequestConfig{Plugins: []store.DirectPluginBinding{{Key: "fixture", Protocols: []string{"chat"}, Options: map[string]json.RawMessage{"mode": json.RawMessage(`"custom"`)}}}}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), config); err != nil {
				t.Fatal(err)
			}
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			for _, stream := range []bool{false, true} {
				r := makeProxyReq(http.MethodPost, "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"seed":9007199254740993,"stream":%t}`, stream))
				w := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(w, r)
				if w.Code != 200 || !strings.Contains(w.Body.String(), `"receipt":true`) {
					t.Fatalf("hook not executed stream=%v: %d %s", stream, w.Code, w.Body.String())
				}
				if stream && !strings.Contains(w.Body.String(), "[DONE]") {
					t.Fatal("terminal lost")
				}
				platformUsage(t, logs, "success", 7, 2)
			}
			if _, err := channelplugins.Save(t.Context(), db.DB, "fixture", channelPluginSource, false, 1); err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
			if w.Code != 503 || dispatched.Load() != 2 {
				t.Fatalf("disabled plugin dispatched: %d %d", w.Code, dispatched.Load())
			}
		})
	}
}

func TestChannelResponsePluginsCannotForgeUsageOrTerminal(t *testing.T) {
	for _, operation := range []string{`{op:"delete",path:"type"}`, `{op:"set",path:"usage.prompt_tokens",value:"0"}`, `{op:"delete",path:"choices.0.finish_reason"}`} {
		source := `export const meta={apiVersion:1,key:"fixture",name:"Fixture",version:"1.0.0",protocols:["chat"],hooks:["event"]};export function event(){return {body:[` + operation + `]};}`
		p, err := channelplugin.Compile(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}
		wire := &directProviderWire{PluginProtocol: "chat", Plugins: []channelplugins.Invocation{{Program: p}}}
		if _, err = applyDirectChannelResponsePlugins(t.Context(), wire, []byte(`{"type":"fixture","choices":[{"finish_reason":"stop"}],"usage":{"prompt_tokens":7}}`), "event"); err == nil {
			t.Fatalf("control field modified: %s", operation)
		}
	}
}

func TestChannelPluginFailureAfterDispatchDoesNotReplay(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			var dispatched atomic.Int64
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dispatched.Add(1)
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"fixture","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
				}
			}))
			defer up.Close()
			db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: up.URL + "/v1/chat/completions", Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			previous := config.GetSafe()
			next := config.Config{}
			if previous != nil {
				next = *previous
			}
			next.ProxyMaxChannelAttempts = 4
			config.Set(&next)
			t.Cleanup(func() { config.Set(previous) })
			source := `export const meta={apiVersion:1,key:"fixture",name:"Fixture",version:"1.0.0",protocols:["chat"],hooks:["response","event"]};export function response(){throw new Error("synthetic-private-exception");}export function event(){throw new Error("synthetic-private-exception");}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "fixture", source, true, 0); err != nil {
				t.Fatal(err)
			}
			binding := store.DirectRequestConfig{Plugins: []store.DirectPluginBinding{{Key: "fixture", Protocols: []string{"chat"}}}}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), binding); err != nil {
				t.Fatal(err)
			}
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			w := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(w, makeProxyReq("POST", "/v1/chat/completions", fmt.Sprintf(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}],"stream":%t}`, stream)))
			if dispatched.Load() != 1 || strings.Contains(w.Body.String(), "synthetic-private-exception") {
				t.Fatalf("plugin failure replayed/leaked: %d %s", dispatched.Load(), w.Body.String())
			}
			if !stream && w.Code != 502 {
				t.Fatalf("buffered failure became success: %d", w.Code)
			}
			if stream && !strings.Contains(w.Body.String(), "upstream stream interrupted") {
				t.Fatalf("missing stream failure: %s", w.Body.String())
			}
		})
	}
}

func TestChannelPluginNativeEnvelopeControls(t *testing.T) {
	for _, test := range []struct{ protocol, body, operation string }{
		{"messages", `{"type":"message_start","message":{"id":"m","model":"fixture","usage":{"input_tokens":7}}}`, `{op:"set",path:"message.usage.input_tokens",value:"0"}`},
		{"messages", `{"type":"message_start","message":{"id":"m","model":"fixture","usage":{"input_tokens":7}}}`, `{op:"set",path:"message.model",value:"different"}`},
		{"messages", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":2}}`, `{op:"set",path:"delta.stop_reason",value:"end_turn"}`},
		{"gemini", `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":2}}`, `{op:"set",path:"usageMetadata.promptTokenCount",value:"0"}`},
	} {
		t.Run(test.protocol+test.operation, func(t *testing.T) {
			source := `export const meta={apiVersion:1,key:"fixture",name:"Fixture",version:"1.0.0",protocols:["` + test.protocol + `"],hooks:["event"]};export function event(){return {body:[` + test.operation + `]};}`
			p, err := channelplugin.Compile(t.Context(), source)
			if err != nil {
				t.Fatal(err)
			}
			wire := &directProviderWire{PluginProtocol: test.protocol, Plugins: []channelplugins.Invocation{{Program: p}}}
			if _, err = applyDirectChannelResponsePlugins(t.Context(), wire, []byte(test.body), "event"); err == nil {
				t.Fatal("native control/usage field mutated")
			}
		})
	}
}
