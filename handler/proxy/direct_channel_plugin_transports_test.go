package proxyhandler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func transportPluginSource(protocol string) string {
	return `export const meta={apiVersion:1,key:"transport",name:"Transport",version:"1.0.0",protocols:["` + protocol + `"],hooks:["request","response","event"]};
export function request(){return {body:[{op:"set",path:"vendor.mode",value:"true"}],headers:[{op:"set",path:"X-Plugin-Transport",value:"enabled"}]};}
export function response(){return {body:[{op:"set",path:"vendor.receipt",value:"1"}]};}
export function event(){return {body:[{op:"set",path:"vendor.receipt",value:"1"}]};}`
}

func TestChannelPluginNativeTransports(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, protocol := range []string{"responsesCompact", "responsesWebsocket"} {
			for _, change := range []string{"unchanged", "disable-during-attempt", "upgrade-during-attempt", "output-failure", "forge-usage", "forge-continuation", "request-continuation"} {
				t.Run(dialect+"/"+protocol+"/"+change, func(t *testing.T) {
					var calls atomic.Int32
					var db *store.DB
					original := transportPluginSource(protocol)
					if change == "output-failure" {
						original = strings.ReplaceAll(original, `return {body:[{op:"set",path:"vendor.receipt",value:"1"}]};`, `throw new Error("synthetic-private-hook-error");`)
					}
					if change == "forge-usage" {
						original = strings.ReplaceAll(original, `path:"vendor.receipt"`, `path:"usage.total_tokens"`)
					}
					if change == "forge-continuation" {
						original = strings.ReplaceAll(original, `path:"vendor.receipt"`, `path:"output"`)
					}
					if change == "request-continuation" {
						original = strings.ReplaceAll(original, `path:"vendor.mode"`, `path:"previous_response_id"`)
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Header.Get("Authorization") != "Bearer fixture-media-key" || r.Header.Get("X-Plugin-Transport") != "enabled" {
							t.Error("plugin header or credential missing")
						}
						var conn *websocket.Conn
						var input []byte
						var err error
						if protocol == "responsesWebsocket" {
							conn, err = websocket.Accept(w, r, nil)
							if err != nil {
								return
							}
							defer conn.CloseNow()
						}
						for {
							if conn != nil {
								_, input, err = conn.Read(r.Context())
								if err != nil {
									return
								}
							} else {
								input, err = io.ReadAll(r.Body)
							}
							n := calls.Add(1)
							if err != nil || !bytes.Contains(input, []byte(`"mode":true`)) || !bytes.Contains(input, []byte(`9007199254740993`)) || !bytes.Contains(input, []byte(`opaque-input`)) {
								t.Errorf("native/plugin input changed: %s err=%v", input, err)
							}
							if n == 1 && (change == "disable-during-attempt" || change == "upgrade-during-attempt") {
								source := strings.ReplaceAll(original, `value:"1"`, `value:"2"`)
								if _, err := channelplugins.Save(r.Context(), db.DB, "transport", source, change == "upgrade-during-attempt", 1); err != nil {
									t.Error(err)
								}
							}
							output := `{"id":"native-result","object":"response.compaction","output":[{"type":"compaction","encrypted_content":"opaque-output","extension":9007199254740993}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}}`
							if conn != nil {
								output = `{"type":"response.completed","response":{"id":"native-result","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9}},"opaque":"opaque-output","extension":9007199254740993}`
								_ = conn.Write(r.Context(), websocket.MessageText, []byte(output))
							} else {
								w.Header().Set("Content-Type", "application/json")
								_, _ = io.WriteString(w, output)
								return
							}
						}
					}))
					t.Cleanup(upstream.Close)
					endpoint := &store.DirectEndpoint{URL: upstream.URL, Auth: store.DirectAuthBearer}
					endpoints := store.DirectEndpoints{ResponsesCompact: endpoint}
					bit := store.DirectProtocolResponsesCompact
					if protocol == "responsesWebsocket" {
						endpoint.URL = "ws" + strings.TrimPrefix(upstream.URL, "http")
						endpoints = store.DirectEndpoints{ResponsesWebsocket: endpoint}
						bit = store.DirectProtocolResponsesWebsocket
					}
					db = installDirectMediaRuntime(t, dialect, endpoints, bit, "")
					if _, err := channelplugins.Save(t.Context(), db.DB, "transport", original, true, 0); err != nil {
						t.Fatal(err)
					}
					binding := store.DirectRequestConfig{Plugins: []store.DirectPluginBinding{{Key: "transport", Protocols: []string{protocol}}}}
					if err := channelplugins.ValidateBindings(t.Context(), db.DB, binding, endpoints); err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET request_config=?`), binding); err != nil {
						t.Fatal(err)
					}
					cfg := *getUpstreamConfig()
					cfg.ChannelPlugins = channelplugins.New(db.DB)
					logs := make(chan proxy.ProxyLogEntry, 4)
					cfg.LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error { logs <- entry; return nil }
					SetUpstreamConfig(&cfg)
					var wsCtx context.Context
					var wsConn *websocket.Conn
					invoke := func() string {
						if protocol == "responsesWebsocket" {
							if wsConn == nil {
								wsCtx, wsConn = dialDirectResponsesGateway(t, "synthetic-plugin-key")
							}
							writeWSMessage(t, wsCtx, wsConn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"opaque-input 9007199254740993"}],"opaque":9007199254740993}`)
							_, output, err := wsConn.Read(wsCtx)
							if err != nil {
								t.Fatal(err)
							}
							return string(output)
						}
						out := httptest.NewRecorder()
						HandleResponses(out, makeProxyReq("POST", "/v1/responses/compact", `{"model":"client-alias","input":[{"type":"compaction","encrypted_content":"opaque-input","extension":9007199254740993}]}`), "/v1/responses/compact")
						return out.Body.String()
					}
					output := invoke()
					if change == "request-continuation" {
						if calls.Load() != 0 || len(logs) != 0 || !strings.Contains(output, `"error"`) {
							t.Fatalf("plugin changed native continuation after authorization: calls=%d %s", calls.Load(), output)
						}
						return
					}
					failure := change == "output-failure" || change == "forge-usage" || change == "forge-continuation"
					if calls.Load() != 1 || strings.Contains(output, "synthetic-private-") {
						t.Fatalf("replayed/leaked output: calls=%d %s", calls.Load(), output)
					}
					if failure {
						if !strings.Contains(output, `"error"`) || strings.Contains(output, `"response.completed"`) {
							t.Fatalf("plugin failure became native success: %s", output)
						}
					} else if !strings.Contains(output, `"receipt":1`) || !strings.Contains(output, "opaque-output") || !strings.Contains(output, "9007199254740993") {
						t.Fatalf("native plugin/snapshot/precision lost: %s", output)
					}
					entry := <-logs
					if entry.TotalTokens == nil || *entry.TotalTokens != 9 || (entry.Status == "success") == failure {
						t.Fatalf("raw usage/status changed: %+v", entry)
					}
					if change == "disable-during-attempt" || change == "upgrade-during-attempt" {
						output = invoke()
						if change == "disable-during-attempt" {
							if calls.Load() != 1 || !strings.Contains(output, `"error"`) {
								t.Fatalf("disabled plugin executed next turn: %s", output)
							}
						} else if calls.Load() != 2 || !strings.Contains(output, `"receipt":2`) {
							t.Fatalf("next attempt did not capture upgrade: %s", output)
						}
					}
				})
			}
		}
	}
}

func TestWebsocketPluginCannotChangeToolArgumentsOrContinuation(t *testing.T) {
	for _, body := range []string{
		`{"type":"response.function_call_arguments.delta","delta":"{\"n\":9007199254740993}","call_id":"call-native"}`,
		`{"type":"response.function_call_arguments.done","arguments":"{\"n\":9007199254740993}","call_id":"call-native"}`,
	} {
		for _, field := range []string{"delta", "arguments", "call_id", "sequence_number", "part"} {
			out, _, err := requestoverride.ApplyBody([]byte(body), []requestoverride.Operation{{Op: "set", Path: field, Value: `"forged"`}}, requestoverride.Context{}, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			if sameChannelPluginControlFields("responsesWebsocket", []byte(body), out) {
				t.Fatalf("native tool control %s changed", field)
			}
		}
	}
}
