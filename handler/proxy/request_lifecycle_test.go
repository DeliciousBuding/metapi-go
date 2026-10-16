package proxyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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

func TestGatewayLifecycleDispatchResults(t *testing.T) {
	previous := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(previous); SetUpstreamConfig(nil) })
	for _, test := range []struct {
		name, events, terminal string
		abrupt                 bool
	}{
		{"complete", "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "completed", false},
		{"interrupted", "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n", "output_interrupted", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := sseChunkUpstream([]string{test.events}, 0)
			if test.abrupt {
				upstream.Close()
				upstream = abruptSSEUpstream(test.events)
			}
			defer upstream.Close()
			router := &upstreamTestRouter{selected: routing.SelectedChannel{Channel: store.RouteChannel{ID: 42, Enabled: true}, Account: store.Account{ID: 7, Status: "active"}, Site: store.Site{ID: 3, URL: upstream.URL, Status: "active"}, TokenValue: "synthetic-upstream", ActualModel: "gpt-4o"}}
			var attempts []proxy.ProxyLogEntry
			SetUpstreamConfig(&UpstreamConfig{Router: router, Executor: proxy.NewRuntimeExecutor(10 * time.Second), LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error {
				attempts = append(attempts, entry)
				return nil
			}})
			var records []store.GatewayRequest
			handler := requestLifecycleMiddleware(http.HandlerFunc(HandleChatCompletions), func(ctx context.Context, entry store.GatewayRequest) error {
				records = append(records, entry)
				return nil
			})
			req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
			req.Header.Set("X-Request-Id", "repeatable-client-id")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if len(records) != 1 || records[0].TerminalStatus != test.terminal || records[0].HTTPStatus == nil || *records[0].HTTPStatus != 200 || !records[0].OutputStarted {
				t.Fatalf("response=%d records=%+v", rec.Code, records)
			}
			if len(attempts) != 1 || attempts[0].GatewayRequestID != records[0].ID {
				t.Fatalf("attempt/request association: %+v %+v", attempts, records)
			}
			if records[0].AttemptCount != 1 {
				t.Fatalf("observed attempt count: %+v", records[0])
			}
			if records[0].ExternalRequestID == nil || *records[0].ExternalRequestID != "repeatable-client-id" || records[0].FirstOutputLatencyMs == nil {
				t.Fatalf("missing metadata: %+v", records[0])
			}
		})
	}
}

func TestGatewayLifecycleNoDispatchAndRepeatedExternalID(t *testing.T) {
	previous := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(previous); SetUpstreamConfig(nil) })
	SetUpstreamConfig(&UpstreamConfig{Router: &allFailedAlertRouter{}})
	var records []store.GatewayRequest
	handler := requestLifecycleMiddleware(http.HandlerFunc(HandleChatCompletions), func(_ context.Context, entry store.GatewayRequest) error {
		records = append(records, entry)
		return nil
	})
	for range 2 {
		req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"missing","messages":[]}`)
		req.Header.Set("X-Request-Id", "same-external")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	if len(records) != 2 || records[0].ID == records[1].ID {
		t.Fatalf("identities merged: %+v", records)
	}
	for _, record := range records {
		if record.TerminalStatus != "rejected_no_route" || record.HTTPStatus == nil || *record.HTTPStatus != 503 {
			t.Fatalf("wrong no-dispatch result: %+v", record)
		}
	}
}

type failedLifecycleWriter struct{ header http.Header }

func (w *failedLifecycleWriter) Header() http.Header       { return w.header }
func (w *failedLifecycleWriter) WriteHeader(int)           {}
func (w *failedLifecycleWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestGatewayLifecycleCancellationWriteFailureAndIdempotency(t *testing.T) {
	for _, cancellation := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequestWithContext(ctx, "POST", "/v1/responses", nil)
		var records []store.GatewayRequest
		state, request := newRequestLifecycle(req, "http", func(writeCtx context.Context, entry store.GatewayRequest) error {
			if writeCtx.Err() != nil {
				t.Error("completion writer inherited cancellation")
			}
			records = append(records, entry)
			return errors.New("synthetic write failure")
		})
		w := &requestResponseWriter{ResponseWriter: &failedLifecycleWriter{header: make(http.Header)}, state: state}
		_, _ = w.Write([]byte("synthetic"))
		state.observe(shared.OutcomeSuccess, true)
		if cancellation {
			cancel()
		}
		state.finish(request.Context())
		state.finish(request.Context())
		cancel()
		want := "output_interrupted"
		if cancellation {
			want = "client_cancelled"
		}
		if len(records) != 1 || records[0].TerminalStatus != want {
			t.Fatalf("cancel=%t records=%+v", cancellation, records)
		}
	}
}

func TestGatewayLifecycleAuthRejectionAndScope(t *testing.T) {
	var records []store.GatewayRequest
	handler := requestLifecycleMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(interface{ ObserveProxyKey(int64) }).ObserveProxyKey(17)
		w.(interface{ ObserveProxyRejection(string) }).ObserveProxyRejection("key_expired")
		w.WriteHeader(403)
	}), func(_ context.Context, entry store.GatewayRequest) error {
		records = append(records, entry)
		return nil
	})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/responses", nil))
	if len(records) != 1 || records[0].TerminalStatus != "rejected_policy" || records[0].ReasonCode == nil || *records[0].ReasonCode != "key_expired" || records[0].DownstreamAPIKeyID == nil || *records[0].DownstreamAPIKeyID != 17 {
		t.Fatalf("rejection: %+v", records)
	}
	for _, path := range []string{"/api/stats/overview", "/health", "/dashboard"} {
		if canonicalRequestPath(path) != nil {
			t.Fatalf("non-proxy path recorded: %s", path)
		}
	}
	if path := canonicalRequestPath("/v1beta/models/private-model:streamGenerateContent"); path == nil || strings.Contains(*path, "private-model") {
		t.Fatalf("unsafe gemini path: %v", path)
	}
	if path := canonicalRequestPath("/responses/compact"); path == nil || *path != "/v1/responses/compact" {
		t.Fatalf("compact alias missing: %v", path)
	}
}

func TestGatewayLifecycleCredentialNeverExternalID(t *testing.T) {
	previous := config.RuntimeSafe()
	config.UpdateRuntime(func(cfg *config.RuntimeSettings) { cfg.ProxyToken = "synthetic-nonstandard-token" })
	t.Cleanup(func() { config.SetRuntime(previous) })
	var record store.GatewayRequest
	handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })), func(_ context.Context, entry store.GatewayRequest) error { record = entry; return nil })
	for _, token := range []string{"synthetic-nonstandard-token", "synthetic-invalid-token"} {
		req := httptest.NewRequest("POST", "/v1/responses", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Request-Id", token)
		handler.ServeHTTP(httptest.NewRecorder(), req)
		if record.ExternalRequestID != nil {
			t.Fatalf("credential leaked into trace: %+v", record)
		}
	}
}

func TestGatewayLifecycleKnownFailureSurvivesLaterCancellation(t *testing.T) {
	for _, policy := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var record store.GatewayRequest
		state, request := newRequestLifecycle(httptest.NewRequestWithContext(ctx, "POST", "/v1/responses", nil), "http", func(_ context.Context, entry store.GatewayRequest) error { record = entry; return nil })
		w := &requestResponseWriter{ResponseWriter: httptest.NewRecorder(), state: state}
		if policy {
			w.ObserveProxyRejection("key_expired")
			w.WriteHeader(403)
		} else {
			w.WriteHeader(502)
			state.observe(shared.OutcomeUpstreamError, false)
		}
		cancel()
		state.finish(request.Context())
		want := "upstream_failed"
		if policy {
			want = "rejected_policy"
		}
		if record.TerminalStatus != want {
			t.Fatalf("known failure overwritten: %+v", record)
		}
	}
}

func TestGatewayLifecycleWebsocketNoRoute(t *testing.T) {
	previous := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(previous); SetUpstreamConfig(nil) })
	SetUpstreamConfig(&UpstreamConfig{Router: &allFailedAlertRouter{}})
	completed := make(chan store.GatewayRequest, 2)
	handler := requestLifecycleMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := &auth.ProxyAuthContext{Source: "global", Token: "synthetic", Policy: auth.EmptyDownstreamRoutingPolicy}
		HandleResponsesWebsocket(w, r.WithContext(auth.WithProxyAuth(r.Context(), pac)))
	}), func(_ context.Context, entry store.GatewayRequest) error { completed <- entry; return nil })
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	if err = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.create","model":"missing","input":[]}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err = conn.Read(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case record := <-completed:
		if record.TerminalStatus != "rejected_no_route" || record.AttemptCount != 0 || record.HTTPStatus != nil {
			t.Fatalf("no route turn: %+v", record)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestGatewayLifecycleWebsocketTurnNotHandshake(t *testing.T) {
	var mu sync.Mutex
	var records []store.GatewayRequest
	writer := func(_ context.Context, entry store.GatewayRequest) error {
		mu.Lock()
		records = append(records, entry)
		mu.Unlock()
		return nil
	}
	handler := requestLifecycleMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := &auth.ProxyAuthContext{Source: "global", Token: "synthetic", Policy: auth.EmptyDownstreamRoutingPolicy}
		HandleResponsesWebsocket(w, r.WithContext(auth.WithProxyAuth(r.Context(), pac)))
	}), writer)
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{`{"type":"response.create","model":"m","generate":false,"input":[]}`, `{"type":"unsupported","model":"m"}`} {
		if err = conn.Write(ctx, websocket.MessageText, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		reads := 1
		if strings.Contains(payload, "generate") {
			reads = 2
		}
		for range reads {
			if _, _, err = conn.Read(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = conn.Close(websocket.StatusNormalClosure, "done")
	deadline := time.After(time.Second)
	for {
		mu.Lock()
		count := len(records)
		mu.Unlock()
		if count == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("missing turn records")
		case <-time.After(time.Millisecond):
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(records) != 2 || records[0].TerminalStatus != "completed" || records[1].TerminalStatus != "rejected_policy" || records[0].ID == records[1].ID {
		t.Fatalf("turns: %+v", records)
	}
	for _, record := range records {
		if record.Transport != "websocket" || record.HTTPStatus != nil {
			t.Fatalf("handshake leaked: %+v", record)
		}
	}
}

func TestGatewayLifecycleNativeWSAttemptObserver(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		for {
			_, data, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			requests.Add(1)
			var body map[string]any
			_ = json.Unmarshal(data, &body)
			frame := map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_synthetic", "status": "completed", "output": []any{}}}
			if body["previous_response_id"] != nil {
				frame = map[string]any{"type": "error", "status": 400, "error": map[string]any{"code": "previous_response_not_found", "message": "previous_response_id not found"}}
			}
			raw, _ := json.Marshal(frame)
			if conn.Write(r.Context(), websocket.MessageText, raw) != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	for _, test := range []struct {
		name, url string
		body      map[string]any
		attempts  int
		failed    bool
	}{
		{"normal", upstream.URL, map[string]any{"model": "m", "input": []any{}}, 1, false},
		{"recovery", upstream.URL, map[string]any{"model": "m", "previous_response_id": "missing", "input": []any{}}, 2, false},
		{"dial", "http://127.0.0.1:1", map[string]any{"model": "m"}, 1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			id := "lifecycle-" + test.name
			defer CloseCodexWebsocketSession(id)
			attempts, failures := 0, 0
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := SendCodexWebsocketRequest(ctx, CodexWebsocketSendInput{SessionID: id, RequestURL: test.url, Body: test.body, ObserveAttempt: func(_ []map[string]any, err error, _ time.Duration) {
				attempts++
				if err != nil {
					failures++
				}
			}})
			if attempts != test.attempts || (err != nil) != test.failed {
				t.Fatalf("attempts=%d failures=%d err=%v", attempts, failures, err)
			}
			if test.name == "recovery" && failures != 1 {
				t.Fatalf("recovery failure not observed: %d", failures)
			}
		})
	}
}

func TestGatewayLifecycleNativeWSTurns(t *testing.T) {
	previous := config.RuntimeSafe()
	config.UpdateRuntime(func(cfg *config.RuntimeSettings) { cfg.CodexUpstreamWebsocketEnabled = true })
	t.Cleanup(func() { config.SetRuntime(previous); SetUpstreamConfig(nil) })
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		for {
			_, raw, err := conn.Read(r.Context())
			if err != nil {
				return
			}
			frame := `{"type":"response.completed","response":{"id":"resp_synthetic","status":"completed","output":[]}}`
			if strings.Contains(string(raw), "previous_response_id") {
				frame = `{"type":"error","status":400,"error":{"code":"previous_response_not_found","message":"previous_response_id not found"}}`
			}
			if conn.Write(r.Context(), websocket.MessageText, []byte(frame)) != nil {
				return
			}
		}
	}))
	defer upstream.Close()
	completed := make(chan store.GatewayRequest, 3)
	attempts := make(chan proxy.ProxyLogEntry, 3)
	selected := &upstreamTestRouter{selected: routing.SelectedChannel{Channel: store.RouteChannel{ID: 42, Enabled: true}, Account: store.Account{ID: 7, Status: "active"}, Site: store.Site{ID: 3, URL: upstream.URL, Platform: "codex", Status: "active"}, TokenValue: "synthetic", ActualModel: "m"}}
	SetUpstreamConfig(&UpstreamConfig{Router: selected, LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error { attempts <- entry; return nil }})
	handler := requestLifecycleMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := &auth.ProxyAuthContext{Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}
		HandleResponsesWebsocket(w, r.WithContext(auth.WithProxyAuth(r.Context(), pac)))
	}), func(_ context.Context, entry store.GatewayRequest) error { completed <- entry; return nil })
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	var lastID string
	for _, turn := range []struct {
		payload  string
		attempts int
	}{
		{`{"type":"response.create","model":"m","input":[]}`, 1},
		{`{"type":"response.create","model":"m","input":[]}`, 1},
		{`{"type":"response.create","model":"m","previous_response_id":"missing","input":[]}`, 2},
	} {
		if err := conn.Write(ctx, websocket.MessageText, []byte(turn.payload)); err != nil {
			t.Fatal(err)
		}
		_, raw, err := conn.Read(ctx)
		if err != nil || !strings.Contains(string(raw), "response.completed") {
			t.Fatalf("downstream completion: %s %v", raw, err)
		}
		select {
		case record := <-completed:
			if record.TerminalStatus != "completed" || record.HTTPStatus != nil || record.AttemptCount != turn.attempts || !record.OutputStarted || record.ID == lastID {
				t.Fatalf("native turn: %+v", record)
			}
			lastID = record.ID
			for ordinal := range turn.attempts {
				select {
				case attempt := <-attempts:
					if attempt.GatewayRequestID != record.ID || attempt.HTTPStatus != 0 || attempt.RetryCount != ordinal {
						t.Fatalf("native association: %+v", attempt)
					}
				case <-ctx.Done():
					t.Fatal("missing native attempt")
				}
			}
		case <-ctx.Done():
			t.Fatal("missing native turn")
		}
	}
}
