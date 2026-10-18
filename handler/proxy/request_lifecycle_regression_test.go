package proxyhandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestGatewayLifecycleDeliveredCompletionSurvivesClose(t *testing.T) {
	for _, test := range []struct {
		name, path, payload, want string
	}{
		{"responses", "/v1/responses", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n", "completed"},
		{"chat", "/v1/chat/completions", "data: [DONE]\n\n", "completed"},
		{"messages", "/v1/messages", "data: {\"type\":\"message_stop\"}\n\n", "completed"},
		{"partial", "/v1/responses", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n", "client_cancelled"},
		{"foreign terminal", "/v1/chat/completions", "data: {\"type\":\"response.completed\"}\n\n", "client_cancelled"},
		{"incomplete status", "/v1/responses", "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"incomplete\"}}\n\n", "output_interrupted"},
		{"error then done", "/v1/chat/completions", "data: {\"error\":{\"message\":\"failed\"}}\n\ndata: [DONE]\n\n", "output_interrupted"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var record store.GatewayRequest
			state, request := newRequestLifecycle(httptest.NewRequestWithContext(ctx, "POST", test.path, nil), "http", func(_ context.Context, entry store.GatewayRequest) error { record = entry; return nil })
			writer := &requestResponseWriter{ResponseWriter: httptest.NewRecorder(), state: state}
			writer.Header().Set("Content-Type", "text/event-stream")
			// Exercise a terminal event split across downstream writes.
			for _, part := range []string{test.payload[:3], test.payload[3:]} {
				if _, err := writer.Write([]byte(part)); err != nil {
					t.Fatal(err)
				}
			}
			state.observe(shared.OutcomeSuccess, true)
			cancel()
			state.finish(request.Context())
			if record.TerminalStatus != test.want {
				t.Fatalf("terminal=%s want=%s", record.TerminalStatus, test.want)
			}
		})
	}
}

type cancelOnLifecycleOutput struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	marker string
}

func (w *cancelOnLifecycleOutput) Write(body []byte) (int, error) {
	n, err := w.ResponseRecorder.Write(body)
	if strings.Contains(w.Body.String(), w.marker) {
		w.cancel()
	}
	return n, err
}

func TestGatewayLifecycleProtocolCompletionBeforeUpstreamEOF(t *testing.T) {
	previous := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(previous); SetUpstreamConfig(nil) })
	for _, complete := range []bool{true, false} {
		name, marker, want := "partial", "partial-text", "client_cancelled"
		if complete {
			name, marker, want = "terminal", "[DONE]", "completed"
		}
		t.Run(name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial-text\"},\"finish_reason\":null}]}\n\n"))
				w.(http.Flusher).Flush()
				if complete {
					_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DO"))
					w.(http.Flusher).Flush()
					_, _ = w.Write([]byte("NE]\n\n"))
					w.(http.Flusher).Flush()
				}
				// The downstream must determine completion before transport EOF.
				<-r.Context().Done()
			}))
			defer upstream.Close()
			router := &upstreamTestRouter{selected: routing.SelectedChannel{Channel: store.RouteChannel{ID: 42, Enabled: true}, Account: store.Account{ID: 7, Status: "active"}, Site: store.Site{ID: 3, URL: upstream.URL, Status: "active"}, TokenValue: "synthetic-upstream", ActualModel: "gpt-4o"}}
			SetUpstreamConfig(&UpstreamConfig{Router: router, Executor: proxy.NewRuntimeExecutor(3 * time.Second), LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
			var record store.GatewayRequest
			handler := requestLifecycleMiddleware(http.HandlerFunc(HandleChatCompletions), func(_ context.Context, entry store.GatewayRequest) error { record = entry; return nil })
			request := makeProxyReq("POST", "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
			ctx, cancel := context.WithTimeout(request.Context(), 3*time.Second)
			defer cancel()
			request = request.WithContext(ctx)
			out := &cancelOnLifecycleOutput{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, marker: marker}
			handler.ServeHTTP(out, request)
			if !errors.Is(ctx.Err(), context.Canceled) || record.TerminalStatus != want || record.AttemptCount != 1 {
				t.Fatalf("ctx=%v terminal=%s want=%s attempts=%d body=%s", ctx.Err(), record.TerminalStatus, want, record.AttemptCount, out.Body.String())
			}
		})
	}
}

func TestGatewayLifecycleInitialSSEErrorKeepsHTTPDisposition(t *testing.T) {
	for _, dispatched := range []bool{false, true} {
		var record store.GatewayRequest
		state, request := newRequestLifecycle(httptest.NewRequest("POST", "/v1/chat/completions", nil), "http", func(_ context.Context, entry store.GatewayRequest) error { record = entry; return nil })
		if dispatched {
			request = request.WithContext(beginGatewayAttempt(request.Context()))
		}
		writer := &requestResponseWriter{ResponseWriter: httptest.NewRecorder(), state: state}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte("data: {\"error\":{\"message\":\"rate limited\"}}\n\n"))
		state.observe(shared.OutcomeClientError, true)
		state.finish(request.Context())
		want := "rejected_policy"
		if dispatched {
			want = "upstream_failed"
		}
		if record.TerminalStatus != want {
			t.Fatalf("dispatched=%t terminal=%s want=%s", dispatched, record.TerminalStatus, want)
		}
	}
}

func TestGatewayLifecycleWSInitialErrorIsNotInterruptedOutput(t *testing.T) {
	records := make(chan store.GatewayRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		state, request := newRequestLifecycle(r, "websocket", func(_ context.Context, entry store.GatewayRequest) error { records <- entry; return nil })
		ctx := beginGatewayAttempt(request.Context())
		state.observe(shared.OutcomeUpstreamError, true)
		session := responsesWSSession{conn: conn}
		if session.writeJSON(ctx, map[string]any{"type": "error", "error": map[string]any{"message": "upstream unavailable"}}) != nil {
			return
		}
		state.finish(ctx)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	if _, _, err := conn.Read(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case record := <-records:
		if record.TerminalStatus != "upstream_failed" || record.AttemptCount != 1 {
			t.Fatalf("initial WS error=%+v", record)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestGatewayLifecycleEndpointFallbackRecordsEveryDispatch(t *testing.T) {
	fixture := installDirectFallbackFixture(t, []string{"messages", "chat"}, map[string]int{"chat": 400}, `{"error":{"code":"model_not_found","message":"model not supported by this endpoint"}}`, false)
	var record store.GatewayRequest
	handler := requestLifecycleMiddleware(http.HandlerFunc(HandleChatCompletions), func(_ context.Context, entry store.GatewayRequest) error { record = entry; return nil })
	out := httptest.NewRecorder()
	handler.ServeHTTP(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`))
	calls, logs := fixture.results()
	if out.Code != http.StatusOK || len(calls) != 2 || record.AttemptCount != len(calls) || len(logs) != len(calls) || record.TerminalStatus != "completed" {
		t.Fatalf("status=%d calls=%v logs=%+v record=%+v", out.Code, calls, logs, record)
	}
	for ordinal, entry := range logs {
		if entry.GatewayRequestID != record.ID || entry.RetryCount != ordinal {
			t.Fatalf("attempt identity=%+v record=%+v", entry, record)
		}
	}
	if logs[0].Status != "failed" || logs[0].TotalTokens != nil || logs[0].EstimatedCost != 0 || logs[1].Status != "success" {
		t.Fatalf("fallback accounting=%+v", logs)
	}
}

func TestGatewayLifecycleDispatchCountDoesNotDependOnLogWriter(t *testing.T) {
	fixture := installDirectFallbackFixture(t, []string{"messages", "chat"}, map[string]int{"chat": 400}, `{"error":{"code":"model_not_found","message":"model not supported by this endpoint"}}`, false)
	cfg := *getUpstreamConfig()
	cfg.LogProxy = func(context.Context, proxy.ProxyLogEntry) error { return errors.New("synthetic persistence failure") }
	SetUpstreamConfig(&cfg)
	var record store.GatewayRequest
	handler := requestLifecycleMiddleware(http.HandlerFunc(HandleChatCompletions), func(_ context.Context, entry store.GatewayRequest) error { record = entry; return nil })
	handler.ServeHTTP(httptest.NewRecorder(), makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`))
	calls, _ := fixture.results()
	if len(calls) != 2 || record.AttemptCount != len(calls) {
		t.Fatalf("calls=%v record=%+v", calls, record)
	}
}

func TestGatewayLifecycleWSTurnInheritsSanitizedCorrelation(t *testing.T) {
	previous := config.RuntimeSafe()
	_ = store.CloseDatabase()
	cfg := &config.Config{DataDir: t.TempDir()}
	rt := &config.RuntimeSettings{DbType: "sqlite", DbUrl: ":memory:", ProxyToken: "synthetic-custom-token"}
	if err := store.EnsureRuntimeDatabase(cfg, rt); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.CloseDatabase() })
	if err := store.MigrateLegacyProxyToken(store.GetDB(), rt, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { config.SetRuntime(previous) })
	for _, externalID := range []string{"synthetic-custom-token", "safe-client-correlation"} {
		t.Run(externalID, func(t *testing.T) {
			records := make(chan store.GatewayRequest, 2)
			handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleResponsesWebsocket)), func(_ context.Context, entry store.GatewayRequest) error { records <- entry; return nil })
			server := httptest.NewServer(handler)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer synthetic-custom-token"}, "X-Request-Id": {externalID}}})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "done")
			if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.create","model":"m","generate":false,"input":[]}`)); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, _, err := conn.Read(ctx); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case record := <-records:
				if externalID == "synthetic-custom-token" && record.ExternalRequestID != nil {
					t.Fatal("turn restored a credential from the handshake header")
				}
				if externalID != "synthetic-custom-token" && (record.ExternalRequestID == nil || *record.ExternalRequestID != externalID) {
					t.Fatalf("safe correlation lost: %+v", record)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}

func TestGatewayLifecycleWSGeneratedOutputAndCompletion(t *testing.T) {
	for _, test := range []struct {
		kind, delta string
		output      bool
	}{
		{"response.output_text.delta", "text", true},
		{"response.function_call_arguments.delta", "{}", true},
		{"response.reasoning_text.delta", "thinking", true},
		{"response.reasoning_summary_text.delta", "summary", true},
		{"response.output_text.delta", "", false},
		{"response.created", "", false},
	} {
		t.Run(test.kind+test.delta, func(t *testing.T) {
			records := make(chan store.GatewayRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
				if err != nil {
					return
				}
				defer conn.Close(websocket.StatusNormalClosure, "done")
				turnCtx, cancel := context.WithCancel(r.Context())
				defer cancel()
				state, request := newRequestLifecycle(r.WithContext(turnCtx), "websocket", func(_ context.Context, entry store.GatewayRequest) error { records <- entry; return nil })
				session := responsesWSSession{conn: conn}
				if session.writeJSON(request.Context(), map[string]any{"type": test.kind, "delta": test.delta}) != nil || session.writeJSON(request.Context(), map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}) != nil {
					return
				}
				state.observe(shared.OutcomeSuccess, true)
				cancel()
				state.finish(request.Context())
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "done")
			for range 2 {
				if _, _, err := conn.Read(ctx); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case record := <-records:
				if (record.FirstOutputLatencyMs != nil) != test.output || record.TerminalStatus != "completed" {
					t.Fatalf("record=%+v wantOutput=%t", record, test.output)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}
