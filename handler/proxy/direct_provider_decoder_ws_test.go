package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProviderDecoderWebsocketUpgradeDisableAndOriginalUsage(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for {
					if _, _, err = conn.Read(r.Context()); err != nil {
						return
					}
					n := calls.Add(1)
					native := fmt.Sprintf(`{"type":"response.completed","response":{"id":"decoder-ws-%d","status":"completed","model":"provider-model","output":[{"type":"function_call","id":"fc-decoder","call_id":"call-decoder","name":"echo","arguments":"{\"n\":9007199254740993}"}],"usage":{"input_tokens":7,"output_tokens":2,"total_tokens":9},"created_at":9007199254740993}}`, n)
					body, _ := json.Marshal(map[string]any{"vendor": map[string]any{"native": native, "model": "provider-model", "meter": map[string]int{"in": 7, "out": 2, "total": 9}}})
					if err = conn.Write(r.Context(), websocket.MessageText, body); err != nil {
						return
					}
				}
			}))
			defer upstream.Close()
			endpoints := store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http") + "/native", Auth: store.DirectAuthBearer}}
			db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", endpoints, store.DirectProtocolResponsesWebsocket, "")
			source := strings.ReplaceAll(providerDecoderSource, `"chat"`, `"responsesWebsocket"`)
			source = strings.ReplaceAll(source, `observations:{chat:`, `observations:{responsesWebsocket:`)
			source = strings.Replace(source, `return {method:"POST",body:i.body};`, `return {body:i.body};`, 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "decoder", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "decoder", Protocols: []string{"responsesWebsocket"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			ctx, conn := dialDirectResponsesGateway(t, "fixture-decoder-owner")
			input := `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hello"}],"max_output_tokens":16}`
			writeWSMessage(t, ctx, conn, input)
			_, raw, err := conn.Read(ctx)
			if err != nil || !strings.Contains(string(raw), `"created_at":9007199254740993`) {
				t.Fatalf("native integer lost before client parsing: %s %v", raw, err)
			}
			var frame map[string]any
			if err = json.Unmarshal(raw, &frame); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(frame)
			if frame["type"] != "response.completed" || !strings.Contains(string(encoded), "9007199254740993") {
				t.Fatalf("decoded native identity/opaque data lost: %s", encoded)
			}
			first := platformUsage(t, logs, "success", 7, 2)
			if first.UpstreamReportedModel == nil || *first.UpstreamReportedModel != "provider-model" {
				t.Fatal("source model was lost")
			}
			failed := strings.Replace(source, `return {body:JSON.parse(i.body).vendor.native};`, `throw new Error("synthetic-private-decoder");`, 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "decoder", failed, true, 1); err != nil {
				t.Fatal(err)
			}
			writeWSMessage(t, ctx, conn, input)
			frame = readWSFrame(t, ctx, conn)
			if frame["status"] != float64(502) || frame["type"] == "response.completed" {
				t.Fatalf("upgraded decoder failure was hidden: %+v", frame)
			}
			platformUsage(t, logs, "failed", 7, 2)
			if _, err := channelplugins.Save(t.Context(), db.DB, "decoder", failed, false, 2); err != nil {
				t.Fatal(err)
			}
			writeWSMessage(t, ctx, conn, input)
			frame = readWSFrame(t, ctx, conn)
			if frame["status"] != float64(503) || calls.Load() != 2 || len(logs) != 0 {
				t.Fatalf("disabled/replayed driver: frame=%+v calls=%d logs=%d", frame, calls.Load(), len(logs))
			}
		})
	}
}
