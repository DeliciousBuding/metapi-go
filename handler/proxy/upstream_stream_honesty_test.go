package proxyhandler

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// The tests in this file pin the PRODUCTION dispatch path
// (HandleChatCompletions -> dispatchEndpointAttemptWithContinue ->
// handleStreamUpstream). They deliberately do not exercise the parallel
// fallback implementation, which is not what serves traffic.

// abruptSSEUpstream writes a partial SSE answer over a hijacked connection and
// then closes it without the terminating zero-length chunk. Chunked framing is
// what makes the client see a read ERROR instead of a clean EOF — that is the
// difference between "upstream finished" and "upstream died mid-stream".
func abruptSSEUpstream(prefix string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			return
		}
		_, _ = bufrw.WriteString("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
		writeSSEChunk(bufrw, prefix)
		_ = bufrw.Flush()
		_ = conn.Close()
	}))
}

func writeSSEChunk(bufrw *bufio.ReadWriter, payload string) {
	_, _ = fmt.Fprintf(bufrw, "%x\r\n%s\r\n", len(payload), payload)
}

// sseChunkUpstream streams the given events with flushes and then ends the
// response cleanly (proper chunked terminator).
func sseChunkUpstream(events []string, stallAfter time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, ev := range events {
			_, _ = w.Write([]byte(ev))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if stallAfter > 0 {
			time.Sleep(stallAfter)
		}
	}))
}

// runStreamDispatch drives one streaming chat completion through the production
// handler and returns everything the three observability surfaces recorded:
// channel health (router.failures / router.successes), the proxy_logs rows and
// the downstream response.
func runStreamDispatch(t *testing.T, upstreamURL string, cfgMutator func(*config.Config)) (*upstreamTestRouter, *[]proxy.ProxyLogEntry, *httptest.ResponseRecorder) {
	t.Helper()

	prevCfg := config.GetSafe()
	cfg := &config.Config{ProxyMaxChannelAttempts: 1}
	if cfgMutator != nil {
		cfgMutator(cfg)
	}
	config.Set(cfg)
	t.Cleanup(func() { config.Set(prevCfg) })

	prevRt := config.RuntimeSafe()
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.DisableCrossProtocolFallback = false })
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 42, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 3, URL: upstreamURL, Status: "active"},
		TokenValue:  "upstream-token",
		ActualModel: "gpt-4o",
	}}
	logs := &[]proxy.ProxyLogEntry{}
	SetUpstreamConfig(&UpstreamConfig{
		Router:   router,
		Executor: proxy.NewRuntimeExecutor(10 * time.Second),
		LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error {
			*logs = append(*logs, entry)
			return nil
		},
	})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)
	return router, logs, rec
}

// assertFailureSurface checks the three surfaces agree the attempt failed:
// channel health got a failure (and no success), and the proxy_logs row is a
// failure row with a non-200 http_status. This is the exact lie the fix
// removes: a broken stream used to record success + 200 + outcome=success.
func assertFailureSurface(t *testing.T, router *upstreamTestRouter, logs *[]proxy.ProxyLogEntry, wantStatus int, wantReasonPart string) {
	t.Helper()
	if len(router.failures) != 1 {
		t.Fatalf("recordUpstreamFailure calls = %d (%#v), want exactly 1", len(router.failures), router.failures)
	}
	if len(router.successes) != 0 {
		t.Fatalf("recordUpstreamSuccess calls = %#v, want none for a broken stream", router.successes)
	}
	f := router.failures[0]
	if f.channelID != 42 {
		t.Fatalf("failure channelID = %d, want 42", f.channelID)
	}
	if f.status == nil || *f.status != wantStatus {
		t.Fatalf("failure status = %v, want %d", f.status, wantStatus)
	}
	if f.errorText == nil || !strings.Contains(*f.errorText, wantReasonPart) {
		t.Fatalf("failure errorText = %v, want it to contain %q", f.errorText, wantReasonPart)
	}
	if len(*logs) == 0 {
		t.Fatal("no proxy_logs row written for the failed stream")
	}
	entry := (*logs)[len(*logs)-1]
	if entry.Status == "success" {
		t.Fatalf("proxy_logs status = %q, want a failure state", entry.Status)
	}
	if entry.HTTPStatus == http.StatusOK {
		t.Fatalf("proxy_logs http_status = 200, want non-200 for a broken stream")
	}
	if entry.HTTPStatus != wantStatus {
		t.Fatalf("proxy_logs http_status = %d, want %d", entry.HTTPStatus, wantStatus)
	}
	if entry.ErrorMessage == nil || !strings.Contains(*entry.ErrorMessage, wantReasonPart) {
		t.Fatalf("proxy_logs error message = %v, want it to contain %q", entry.ErrorMessage, wantReasonPart)
	}
	if entry.IsStream == nil || !*entry.IsStream {
		t.Fatalf("proxy_logs is_stream = %v, want true", entry.IsStream)
	}
}

// TestDispatchStreamMidstreamUpstreamFaultIsRecordedAsFailure is nail ①: the
// upstream relays a partial answer and then dies. Channel health, proxy_logs
// and the terminal metric must all say failure.
func TestDispatchStreamMidstreamUpstreamFaultIsRecordedAsFailure(t *testing.T) {
	upstream := abruptSSEUpstream("data: {\"choices\":[{\"delta\":{\"content\":\"partial answer\"}}]}\n\n")
	t.Cleanup(upstream.Close)

	router, logs, rec := runStreamDispatch(t, upstream.URL, nil)

	if body := rec.Body.String(); !strings.Contains(body, "partial answer") {
		t.Fatalf("relayed prefix lost; body = %q", body)
	}
	if !strings.Contains(rec.Body.String(), "upstream stream interrupted") {
		t.Fatalf("client got no SSE error event; body = %q", rec.Body.String())
	}
	assertFailureSurface(t, router, logs, http.StatusBadGateway, "interrupted")
}

// TestDispatchStreamByteLimitTruncationIsRecordedAsFailure is nail ②: the
// answer is cut short by PROXY_MAX_STREAM_RESPONSE_BYTES, so the client
// received an incomplete answer and the attempt must be recorded as a failure.
func TestDispatchStreamByteLimitTruncationIsRecordedAsFailure(t *testing.T) {
	upstream := sseChunkUpstream([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"AAAAAAAAAAAAAAAAAAAA\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"BBBBBBBBBBBBBBBBBBBB\"}}]}\n\n",
		"data: {\"choices\":[{\"delta\":{\"content\":\"CCCCCCCCCCCCCCCCCCCC\"}}]}\n\n",
		"data: [DONE]\n\n",
	}, 0)
	t.Cleanup(upstream.Close)

	router, logs, rec := runStreamDispatch(t, upstream.URL, func(c *config.Config) {
		c.ProxyMaxStreamResponseBytes = 80
	})

	if !strings.Contains(rec.Body.String(), "exceeded configured byte limit") {
		t.Fatalf("client got no byte-limit SSE error event; body = %q", rec.Body.String())
	}
	assertFailureSurface(t, router, logs, http.StatusBadGateway, "truncated")
}

// TestDispatchStreamIdleTimeoutKeepsFailureRecording is nail ③, the regression
// nail: the idle path already recorded a failure before this change and must
// keep doing exactly that (408 + failure row), unchanged.
func TestDispatchStreamIdleTimeoutKeepsFailureRecording(t *testing.T) {
	upstream := sseChunkUpstream([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n",
	}, 3*time.Second)
	t.Cleanup(upstream.Close)

	router, logs, rec := runStreamDispatch(t, upstream.URL, func(c *config.Config) {
		c.ProxyStreamIdleTimeoutSec = 1
	})

	if !strings.Contains(rec.Body.String(), "upstream stream idle timeout") {
		t.Fatalf("client got no idle-timeout SSE error event; body = %q", rec.Body.String())
	}
	assertFailureSurface(t, router, logs, http.StatusRequestTimeout, "idle timeout")
}

// TestDispatchStreamCleanEOFStillRecordsSuccess is nail ④: a stream that ends
// cleanly is still a success — the fix must not turn honest answers into
// failures.
func TestDispatchStreamCleanEOFStillRecordsSuccess(t *testing.T) {
	upstream := sseChunkUpstream([]string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n",
		"data: {\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4,\"total_tokens\":7}}\n\n",
		"data: [DONE]\n\n",
	}, 0)
	t.Cleanup(upstream.Close)

	router, logs, rec := runStreamDispatch(t, upstream.URL, nil)

	if !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("clean stream not relayed; body = %q", rec.Body.String())
	}
	if len(router.failures) != 0 {
		t.Fatalf("recordUpstreamFailure calls = %#v, want none for a clean stream", router.failures)
	}
	if len(router.successes) != 1 {
		t.Fatalf("recordUpstreamSuccess calls = %#v, want exactly 1", router.successes)
	}
	if len(*logs) == 0 {
		t.Fatal("no proxy_logs row written for the clean stream")
	}
	entry := (*logs)[len(*logs)-1]
	if entry.HTTPStatus != http.StatusOK {
		t.Fatalf("proxy_logs http_status = %d, want 200 for a clean stream", entry.HTTPStatus)
	}
	if entry.ErrorMessage != nil {
		t.Fatalf("proxy_logs error = %q, want none for a clean stream", *entry.ErrorMessage)
	}
	if entry.TotalTokens == nil || *entry.TotalTokens != 7 {
		t.Fatalf("proxy_logs total_tokens = %v, want 7 (stream usage still accounted)", entry.TotalTokens)
	}
}

// TestStreamFailureVerdictIsTheSingleOwnerOfStreamEndings pins the mapping
// itself: every non-normal, non-client-driven ending yields a failure status,
// a reason and a terminal outcome; clean EOF and downstream disconnect do not.
func TestStreamFailureVerdictIsTheSingleOwnerOfStreamEndings(t *testing.T) {
	cases := []struct {
		end          streamOutcome
		wantFailed   bool
		wantStatus   int
		wantTerminal string
		wantReason   string
	}{
		{streamEndedNormally, false, 0, "", ""},
		{streamEndedClientDisconnect, false, 0, "", ""},
		{streamEndedIdleTimeout, true, http.StatusRequestTimeout, "timeout", "idle timeout"},
		{streamEndedUpstreamFault, true, http.StatusBadGateway, "upstream_error", "interrupted"},
		{streamEndedTruncated, true, http.StatusBadGateway, "upstream_error", "truncated"},
	}
	for _, tc := range cases {
		status, reason, terminal, failed := streamFailureVerdict(tc.end, 30)
		if failed != tc.wantFailed {
			t.Fatalf("%v: failed = %v, want %v", tc.end, failed, tc.wantFailed)
		}
		if !failed {
			if status != 0 || reason != "" || terminal != "" {
				t.Fatalf("%v: non-failure ending must stay empty, got %d/%q/%q", tc.end, status, reason, terminal)
			}
			continue
		}
		if status != tc.wantStatus {
			t.Fatalf("%v: status = %d, want %d", tc.end, status, tc.wantStatus)
		}
		if terminal != tc.wantTerminal {
			t.Fatalf("%v: terminal = %q, want %q", tc.end, terminal, tc.wantTerminal)
		}
		if !strings.Contains(reason, tc.wantReason) {
			t.Fatalf("%v: reason = %q, want it to contain %q", tc.end, reason, tc.wantReason)
		}
		if status == http.StatusOK {
			t.Fatalf("%v: a failed stream must never be reported with http_status 200", tc.end)
		}
	}
}

// TestContentJudgeAgreesAcrossBufferedAndStreamPaths proves the single content
// judge gives the SAME verdict for the same upstream answer on both paths: the
// buffered facts upstream.go builds and the streaming facts the bounded SSE
// analyzer produces are fed to one function, so judgement strength can no
// longer diverge.
func TestContentJudgeAgreesAcrossBufferedAndStreamPaths(t *testing.T) {
	prevRt := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{
		ProxyErrorKeywords:           []string{"overloaded"},
		ProxyEmptyContentFailEnabled: true,
	})
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	fixtures := []struct {
		name string
		// bufferedBody is the raw body the buffered path hands the judge.
		bufferedBody string
		// sseBody is the same upstream answer as the streaming path sees it.
		sseBody string
		// completion tokens both paths agree the upstream reported.
		completionTokens int
		wantFailed       bool
		wantCode         proxy.FailureCode
	}{
		{
			name:             "keyword failure in an error payload",
			bufferedBody:     `{"error":{"message":"server is overloaded"}}`,
			sseBody:          "data: {\"error\":{\"message\":\"server is overloaded\"}}\n\n",
			completionTokens: 0,
			wantFailed:       true,
			wantCode:         proxy.FailureCodeErrorKeyword,
		},
		{
			name:             "no completion output at all",
			bufferedBody:     `{"id":"chatcmpl_empty"}`,
			sseBody:          "",
			completionTokens: 0,
			wantFailed:       true,
			wantCode:         proxy.FailureCodeEmptyContent,
		},
		{
			name:             "healthy answer with content",
			bufferedBody:     `{"choices":[{"message":{"content":"hello"}}]}`,
			sseBody:          "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n",
			completionTokens: 3,
			wantFailed:       false,
			wantCode:         proxy.FailureCodeNone,
		},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			usage := &proxy.UsageSummary{CompletionTokens: tc.completionTokens, TotalTokens: tc.completionTokens}

			// Buffered path: exactly the facts handler/proxy/upstream.go builds.
			buffered := proxy.JudgeUpstreamContent(proxy.UpstreamContentFacts{
				StatusCode: http.StatusOK,
				RawText:    tc.bufferedBody,
				Usage:      usage,
			})

			// Stream path: run the real bounded analyzer over the SSE encoding of
			// the same answer, then judge through the same production helper.
			analyzer := newIncrementalSseAnalyzer()
			if tc.sseBody != "" {
				analyzer.Push([]byte(tc.sseBody))
			}
			result := analyzer.Result()
			result.Usage.CompletionTokens = int64(tc.completionTokens)
			streamed := judgeStreamContent(http.StatusOK, result, 1, false)

			if buffered != streamed {
				t.Fatalf("verdicts diverge for the same upstream content:\n buffered = %+v\n stream   = %+v", buffered, streamed)
			}
			if buffered.Failed != tc.wantFailed {
				t.Fatalf("failed = %v, want %v (verdict %+v)", buffered.Failed, tc.wantFailed, buffered)
			}
			if buffered.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", buffered.Code, tc.wantCode)
			}
			if tc.wantFailed && buffered.Status == http.StatusOK {
				t.Fatalf("a content failure must never carry http_status 200, got %+v", buffered)
			}
		})
	}
}

// TestShouldContinueEndpointFallbackSingleOwnerFourQuadrants pins the one
// decision function: transport-level errors now consult the same-site abort
// policy (they used to bypass it via a bare "not the last endpoint" check),
// intentionally different first-byte-timeout class is expressed as a parameter
// instead of a second implementation at the call site.
func TestShouldContinueEndpointFallbackSingleOwnerFourQuadrants(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		errText  string
		isLast   bool
		disable  bool
		class    endpointFailureClass
		wantCont bool
	}{
		// Four quadrants for transport errors with candidates remaining.
		{"transport error, candidates remain, policy allows", http.StatusBadGateway, "dial tcp 10.0.0.1:443: no route to host", false, false, endpointFailureTransport, true},
		{"transport error, candidates remain, abort policy fires", http.StatusBadGateway, "dial tcp 10.0.0.1:443: connect: connection refused", false, false, endpointFailureTransport, false},
		{"transport error, no candidates remain", http.StatusBadGateway, "dial tcp 10.0.0.1:443: no route to host", true, false, endpointFailureTransport, false},
		{"transport error, cross-protocol fallback disabled", http.StatusBadGateway, "dial tcp 10.0.0.1:443: no route to host", false, true, endpointFailureTransport, false},
		// Body read failures share the transport class.
		{"read error, connection reset, abort policy fires", http.StatusBadGateway, "read tcp: connection reset by peer", false, false, endpointFailureTransport, false},
		{"read error, generic, candidates remain", http.StatusBadGateway, "unexpected EOF", false, false, endpointFailureTransport, true},
		// The intentional difference, expressed as a parameter.
		{"first-byte timeout is exempt from the abort policy", http.StatusRequestTimeout, "first byte timeout", false, false, endpointFailureFirstByteTimeout, true},
		{"first-byte timeout still respects the last-candidate guard", http.StatusRequestTimeout, "first byte timeout", true, false, endpointFailureFirstByteTimeout, false},
		// Response class must keep its historical verdicts.
		{"systemic 503 aborts endpoint fallback", http.StatusServiceUnavailable, "service unavailable", false, false, endpointFailureResponse, false},
		{"protocol hint downgrades to the next endpoint", http.StatusBadRequest, "please use /v1/messages", false, false, endpointFailureResponse, true},
		{"404 on a protocol path downgrades", http.StatusNotFound, "not found", false, false, endpointFailureResponse, true},
		{"plain 400 does not continue", http.StatusBadRequest, "invalid request body", false, false, endpointFailureResponse, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldContinueEndpointFallback(tc.status, tc.errText, tc.isLast, tc.disable, tc.class)
			if got != tc.wantCont {
				t.Fatalf("shouldContinueEndpointFallback(%d, %q, isLast=%v, disable=%v, class=%d) = %v, want %v",
					tc.status, tc.errText, tc.isLast, tc.disable, tc.class, got, tc.wantCont)
			}
		})
	}
}

// TestTransportErrorFallbackGoesThroughTheSingleDecisionFunction is the
// end-to-end half of the F3 nail: a transport-level "connection refused" on the
// primary protocol path is covered by the same-site abort policy, so the
// dispatcher must stop walking the protocol candidate list and report the
// failure for the PRIMARY path. Historically transport errors bypassed the
// abort policy: the dispatcher silently walked every remaining candidate and
// only reported the last one.
func TestTransportErrorFallbackGoesThroughTheSingleDecisionFunction(t *testing.T) {
	// A port nobody listens on: the dial fails with "connection refused", which
	// the same-site abort policy covers.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	deadURL := "http://" + ln.Addr().String()
	_ = ln.Close()

	prevCfg := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(prevCfg) })
	prevRt := config.RuntimeSafe()
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.DisableCrossProtocolFallback = false })
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 42, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 3, URL: deadURL, Status: "active"},
		TokenValue:  "upstream-token",
		ActualModel: "gpt-4o",
	}}
	logs := &[]proxy.ProxyLogEntry{}
	SetUpstreamConfig(&UpstreamConfig{
		Router:   router,
		Executor: proxy.NewRuntimeExecutor(10 * time.Second),
		LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error {
			*logs = append(*logs, entry)
			return nil
		},
	})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/chat/completions", `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)

	if len(router.failures) != 1 {
		t.Fatalf("recordUpstreamFailure calls = %d (%#v), want exactly 1", len(router.failures), router.failures)
	}
	if router.failures[0].errorText == nil || !strings.Contains(*router.failures[0].errorText, "connection refused") {
		t.Fatalf("failure errorText = %v, want a connection-refused transport error", router.failures[0].errorText)
	}
	if len(*logs) != 1 {
		t.Fatalf("proxy_logs rows = %d, want exactly 1", len(*logs))
	}
	if (*logs)[0].UpstreamPath == nil || *(*logs)[0].UpstreamPath != "/v1/chat/completions" {
		t.Fatalf("failure reported for upstream_path = %q, want the primary /v1/chat/completions: the abort policy must stop the candidate walk on the first transport failure", *(*logs)[0].UpstreamPath)
	}
	if rec.Code == http.StatusOK {
		t.Fatalf("downstream status = 200, want a failure for a refused upstream")
	}
}
