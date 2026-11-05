package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/routecondition"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

type associationRecordingRouter struct {
	proxy.TokenRouterInterface
	requests chan *routecondition.Request
}

func (r *associationRecordingRouter) SelectChannel(ctx context.Context, model string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	var request *routecondition.Request
	if policy.ResolveAssociationRequest != nil {
		request = policy.ResolveAssociationRequest()
	}
	r.requests <- request
	return r.TokenRouterInterface.SelectChannel(ctx, model, policy)
}

func installConditionalWSRuntime(t *testing.T, predicate string, native bool) (*atomic.Int32, <-chan *routecondition.Request) {
	t.Helper()
	calls := &atomic.Int32{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Tier") != "" {
			t.Error("condition header was added to upstream forwarding")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-fixture\",\"status\":\"completed\",\"model\":\"provider-model\",\"output\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n")
	}))
	t.Cleanup(upstream.Close)
	db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{
		Responses: &store.DirectEndpoint{URL: upstream.URL + "/v1/responses", Auth: store.DirectAuthBearer},
	}, store.DirectProtocolResponses, "")
	condition, err := routecondition.Decode([]byte(`{"type":"group","conditions":[` + predicate + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	rules := store.DirectAssociationConditions{{
		Priority: 1, ProtocolOrder: store.DirectProtocolOrder{store.DirectProtocolResponses}, Condition: condition,
	}}
	if _, err := db.Exec(db.Rebind(`UPDATE upstream_group_items SET association_conditions=?`), rules); err != nil {
		t.Fatal(err)
	}
	routing.InvalidateCache()
	config.SetRuntime(&config.RuntimeSettings{CodexUpstreamWebsocketEnabled: native})
	cfg := *getUpstreamConfig()
	router := &associationRecordingRouter{TokenRouterInterface: cfg.Router, requests: make(chan *routecondition.Request, 8)}
	cfg.Router = router
	SetUpstreamConfig(&cfg)
	return calls, router.requests
}

func dialConditionalWS(t *testing.T, headers http.Header) (context.Context, *websocket.Conn) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := &auth.ProxyAuthContext{Token: "synthetic-condition-key", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}
		HandleResponsesWebsocket(w, r.WithContext(auth.WithProxyAuth(r.Context(), pac)))
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "done") })
	return ctx, conn
}

func readAssociationWSTerminal(t *testing.T, ctx context.Context, conn *websocket.Conn) string {
	t.Helper()
	for range 10 {
		frame := readWSFrame(t, ctx, conn)
		kind, _ := frame["type"].(string)
		if kind == "error" || kind == "response.completed" || kind == "response.failed" {
			return kind
		}
	}
	t.Fatal("missing terminal WS frame")
	return ""
}

func TestResponsesWSAssociationHeadersPreservedAcrossBridge(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, tc := range []struct {
			operator, header string
			allow            bool
		}{
			{"eq", "gold", true},
			{"ne", "gold", false},
			{"eq", "", false},
			{"ne", "", true},
		} {
			t.Run(fmt.Sprintf("native=%t/%s/header=%s", native, tc.operator, tc.header), func(t *testing.T) {
				calls, _ := installConditionalWSRuntime(t, fmt.Sprintf(`{"field":"request_header.X-Tier","operator":%q,"value":"gold"}`, tc.operator), native)
				headers := http.Header{}
				if tc.header != "" {
					headers.Set("X-Tier", tc.header)
				}
				ctx, conn := dialConditionalWS(t, headers)
				writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hello"}]}`)
				terminal := readAssociationWSTerminal(t, ctx, conn)
				wantCalls, wantTerminal := int32(0), "error"
				if tc.allow {
					wantCalls, wantTerminal = 1, "response.completed"
				}
				if calls.Load() != wantCalls || terminal != wantTerminal {
					t.Fatalf("dispatches=%d terminal=%s; want %d/%s", calls.Load(), terminal, wantCalls, wantTerminal)
				}
			})
		}
	}
}

func TestResponsesWSAssociationSnapshotSharedWithinTurnOnly(t *testing.T) {
	calls, requests := installConditionalWSRuntime(t, `{"field":"prompt_tokens","operator":"lt","value":20}`, true)
	ctx, conn := dialConditionalWS(t, http.Header{"X-Tier": {"gold"}})
	var previous *routecondition.Request
	for index, text := range []string{"hello", strings.Repeat("long-input", 40)} {
		raw, err := json.Marshal(map[string]any{
			"type": "response.create", "model": "client-alias",
			"input": []any{map[string]any{"role": "user", "content": text}},
		})
		if err != nil {
			t.Fatal(err)
		}
		writeWSMessage(t, ctx, conn, string(raw))
		terminal := readAssociationWSTerminal(t, ctx, conn)
		var observed [2]*routecondition.Request
		for i := range observed {
			select {
			case observed[i] = <-requests:
			case <-ctx.Done():
				t.Fatal("missing native/bridge routing observation")
			}
		}
		native, bridged := observed[0], observed[1]
		if native == nil || bridged == nil || native != bridged || native.Now.IsZero() || native.Now != bridged.Now || native.Headers["X-Tier"] != "gold" {
			t.Fatalf("native and bridge must share one immutable request: native=%p bridge=%p", native, bridged)
		}
		if index == 0 {
			if calls.Load() != 1 || terminal != "response.completed" || native.PromptTokens >= 20 {
				t.Fatalf("first turn was not dispatched: calls=%d terminal=%s tokens=%d", calls.Load(), terminal, native.PromptTokens)
			}
		} else if native == previous || native.PromptTokens <= previous.PromptTokens || native.PromptTokens < 20 || calls.Load() != 1 || terminal != "error" {
			t.Fatalf("second turn reused the first body or dispatched: calls=%d terminal=%s tokens=%d", calls.Load(), terminal, native.PromptTokens)
		}
		previous = native
	}
}
