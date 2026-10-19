package proxyhandler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// setStreamWriteBudgetForTest publishes the two SSE liveness budgets used by
// the relay: the per-write downstream budget and the upstream chunk-gap guard.
// The byte cap is raised so a stalled socket, not the cap, ends the stream.
func setStreamWriteBudgetForTest(t *testing.T, writeSec, idleSec int) {
	t.Helper()
	prev := config.GetSafe()
	config.Set(&config.Config{
		ProxyMaxChannelAttempts:     10,
		ProxySSEWriteTimeoutSec:     writeSec,
		ProxyStreamIdleTimeoutSec:   idleSec,
		ProxyMaxStreamResponseBytes: 1 << 30,
	})
	t.Cleanup(func() { config.Set(prev) })
}

type streamRelayResult struct {
	outcome streamOutcome
	elapsed time.Duration
}

// endlessSSEBody streams valid-looking SSE data forever. It can only end via
// the downstream write budget, the byte cap, or cancellation — exactly the
// conditions a stalled client must not be able to postpone.
type endlessSSEBody struct {
	pattern    []byte
	closeOnce  sync.Once
	closeCalls atomic.Int32
}

func (b *endlessSSEBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := 0
	for n < len(p) {
		n += copy(p[n:], b.pattern)
	}
	return n, nil
}

func (b *endlessSSEBody) Close() error {
	b.closeOnce.Do(func() { b.closeCalls.Add(1) })
	return nil
}

const endlessSSEPattern = `data: {"choices":[{"index":0,"delta":{"content":"x"}}]}` + "\n\n"

// runStreamRelayHandler wraps the production relay in a real HTTP handler,
// arms the same whole-request deadline router.ProxyWriteDeadline installs, and
// closes the upstream body exactly like the dispatch loop does.
func runStreamRelayHandler(results chan<- streamRelayResult, body io.ReadCloser) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       body,
		}
		var outcome streamOutcome
		func() {
			defer resp.Body.Close()
			_, outcome, _ = handleStreamUpstream(w, r, resp, 0)
		}()
		results <- streamRelayResult{outcome: outcome, elapsed: time.Since(started)}
	})
}

func startStreamRelayServer(t *testing.T, handler http.Handler, writeTimeout time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.WriteTimeout = writeTimeout
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func startStreamRelayH2Server(t *testing.T, handler http.Handler, writeTimeout time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.WriteTimeout = writeTimeout
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// TestStreamDownstreamSlowClientReleasesHTTP1 stops reading the response after
// the request is sent. The relay must return as a client disconnect once the
// socket cannot accept more data, instead of holding the goroutine, the
// upstream body and the concurrency slot forever.
func TestStreamDownstreamSlowClientReleasesHTTP1(t *testing.T) {
	setStreamWriteBudgetForTest(t, 1, 10)

	body := &endlessSSEBody{pattern: []byte(endlessSSEPattern)}
	results := make(chan streamRelayResult, 1)
	srv := startStreamRelayServer(t, runStreamRelayHandler(results, body), 200*time.Millisecond)

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "GET /v1/chat/completions HTTP/1.1\r\nHost: metapi\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	// Deliberately never read: the response and then the socket buffers fill.

	var res streamRelayResult
	select {
	case res = <-results:
	case <-time.After(20 * time.Second):
		t.Fatal("relay did not return after the downstream stopped reading")
	}

	if res.outcome != streamEndedClientDisconnect {
		t.Fatalf("outcome = %v, want clientDisconnect (a stalled client must not be an upstream fault)", res.outcome)
	}
	if res.elapsed < 500*time.Millisecond {
		t.Fatalf("relay returned after %v, before the 1s write budget could expire", res.elapsed)
	}
	if got := body.closeCalls.Load(); got != 1 {
		t.Fatalf("upstream body closed %d times, want exactly 1", got)
	}
}

// TestStreamDownstreamSlowClientReleasesHTTP2 is the HTTP/2 equivalent: the
// client receives headers and then never reads the body, so the stream window
// stays exhausted. The per-write deadline must end the relay and release the
// upstream body.
func TestStreamDownstreamSlowClientReleasesHTTP2(t *testing.T) {
	setStreamWriteBudgetForTest(t, 1, 10)

	body := &endlessSSEBody{pattern: []byte(endlessSSEPattern)}
	results := make(chan streamRelayResult, 1)
	srv := startStreamRelayH2Server(t, runStreamRelayHandler(results, body), 200*time.Millisecond)

	resp, err := srv.Client().Get(srv.URL + "/v1/chat/completions")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.ProtoMajor != 2 {
		resp.Body.Close()
		t.Fatalf("response proto = %s, want HTTP/2", resp.Proto)
	}
	// Never read the body until the relay reports its ending.

	var res streamRelayResult
	select {
	case res = <-results:
	case <-time.After(20 * time.Second):
		resp.Body.Close()
		t.Fatal("relay did not return after the HTTP/2 stream window stalled")
	}
	resp.Body.Close()

	if res.outcome != streamEndedClientDisconnect {
		t.Fatalf("outcome = %v, want clientDisconnect", res.outcome)
	}
	if got := body.closeCalls.Load(); got != 1 {
		t.Fatalf("upstream body closed %d times, want exactly 1", got)
	}
}

// TestStreamChunkGapsBeyondWriteBudgetStayHealthy proves the write deadline is
// cleared between chunks. The inherited whole-request deadline is 200ms and
// the per-write budget is 1s; the upstream waits 900ms for its first chunk and
// then 1.5s between chunks. An armed-between-writes deadline would reset the
// HTTP/2 stream, while a cleared one leaves a healthy long stream untouched.
func TestStreamChunkGapsBeyondWriteBudgetStayHealthy(t *testing.T) {
	const (
		firstChunkDelay  = 900 * time.Millisecond
		gapBetweenChunks = 1500 * time.Millisecond
	)
	chunk1 := `data: {"choices":[{"index":0,"delta":{"content":"hello"}}]}` + "\n\n"
	chunk2 := `data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}` + "\n\n"
	done := "data: [DONE]\n\n"

	tests := []struct {
		name  string
		start func(*testing.T, http.Handler, time.Duration) *httptest.Server
		proto func(*testing.T, *http.Response)
	}{
		{
			name:  "http1",
			start: startStreamRelayServer,
		},
		{
			name:  "http2",
			start: startStreamRelayH2Server,
			proto: func(t *testing.T, resp *http.Response) {
				t.Helper()
				if resp.ProtoMajor != 2 {
					t.Fatalf("response proto = %s, want HTTP/2", resp.Proto)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setStreamWriteBudgetForTest(t, 1, 10)

			pr, pw := io.Pipe()
			go func() {
				defer pw.Close()
				time.Sleep(firstChunkDelay)
				if _, err := io.WriteString(pw, chunk1); err != nil {
					return
				}
				time.Sleep(gapBetweenChunks)
				_, _ = io.WriteString(pw, chunk2+done)
			}()

			results := make(chan streamRelayResult, 1)
			srv := tc.start(t, runStreamRelayHandler(results, pr), 200*time.Millisecond)

			started := time.Now()
			resp, err := srv.Client().Get(srv.URL + "/v1/chat/completions")
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			defer resp.Body.Close()
			if tc.proto != nil {
				tc.proto(t, resp)
			}
			relayed, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading relayed stream: %v", err)
			}
			if string(relayed) != chunk1+chunk2+done {
				t.Fatalf("relayed body = %q, want the complete stream", relayed)
			}
			// The producer starts just before the request, so allow a small
			// skew against the two deliberate sleeps.
			if elapsed := time.Since(started); elapsed < firstChunkDelay+gapBetweenChunks-100*time.Millisecond {
				t.Fatalf("stream finished after %v, before both deliberate gaps", elapsed)
			}

			res := <-results
			if res.outcome != streamEndedNormally {
				t.Fatalf("outcome = %v, want normally", res.outcome)
			}
		})
	}
}

// deadlineRecordingWriter records every write-deadline change the relay makes
// while behaving like a normal ResponseRecorder.
type deadlineRecordingWriter struct {
	httptest.ResponseRecorder
	deadlines []time.Time
}

func (w *deadlineRecordingWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

// TestStreamWriteDeadlineArmedOnlyAroundWrite asserts the bounded window does
// not outlive the write. Leaving it armed is fatal on HTTP/2, where an expired
// deadline resets the stream even between chunks.
func TestStreamWriteDeadlineArmedOnlyAroundWrite(t *testing.T) {
	rec := &deadlineRecordingWriter{}
	before := time.Now()
	if err := writeDownstreamChunk(rec, []byte("data: x\n\n"), time.Second); err != nil {
		t.Fatalf("writeDownstreamChunk: %v", err)
	}
	if len(rec.deadlines) != 2 {
		t.Fatalf("deadline changes = %d, want arm+clear", len(rec.deadlines))
	}
	armed := rec.deadlines[0]
	if armed.IsZero() {
		t.Fatal("first change must arm a non-zero deadline")
	}
	if d := armed.Sub(before); d < 500*time.Millisecond || d > 2*time.Second {
		t.Fatalf("armed deadline %v after start, want about the 1s budget", d)
	}
	if !rec.deadlines[1].IsZero() {
		t.Fatalf("second deadline change = %v, want cleared (zero)", rec.deadlines[1])
	}

	// A writer without deadline support keeps the historical unbounded
	// behavior instead of failing the relay.
	if err := writeDownstreamChunk(httptest.NewRecorder(), []byte("data: y\n\n"), time.Second); err != nil {
		t.Fatalf("writer without SetWriteDeadline must still relay: %v", err)
	}
}

var errFlushSentinel = errors.New("downstream flush failed")

type flushErrorWriter struct {
	header http.Header
	body   bytes.Buffer
}

func (w *flushErrorWriter) Header() http.Header { return w.header }

func (w *flushErrorWriter) Write(p []byte) (int, error) { return w.body.Write(p) }

func (w *flushErrorWriter) WriteHeader(int) {}

func (w *flushErrorWriter) FlushError() error { return errFlushSentinel }

// unwrapOnlyWriter hides every optional interface behind one Unwrap hop, the
// shape middleware wrappers have in the real server.
type unwrapOnlyWriter struct {
	inner http.ResponseWriter
}

func (w unwrapOnlyWriter) Header() http.Header { return w.inner.Header() }

func (w unwrapOnlyWriter) Write(p []byte) (int, error) { return w.inner.Write(p) }

func (w unwrapOnlyWriter) WriteHeader(status int) { w.inner.WriteHeader(status) }

func (w unwrapOnlyWriter) Unwrap() http.ResponseWriter { return w.inner }

// TestStreamFlushErrorIsReportedThroughUnwrap ensures a flush failure is not
// swallowed: ResponseController must unwrap the middleware wrapper, surface
// the error, and the relay must be able to treat it as a downstream failure.
func TestStreamFlushErrorIsReportedThroughUnwrap(t *testing.T) {
	inner := &flushErrorWriter{header: make(http.Header)}
	err := writeDownstreamChunk(unwrapOnlyWriter{inner: inner}, []byte("data: x\n\n"), time.Second)
	if !errors.Is(err, errFlushSentinel) {
		t.Fatalf("err = %v, want the flush error", err)
	}
	if got := inner.body.String(); got != "data: x\n\n" {
		t.Fatalf("bytes before the flush failure = %q, want the relayed chunk", got)
	}
}

// TestStreamWriteTimeoutConfig covers the operator-facing default and the
// shared clamp for unusable values.
func TestStreamWriteTimeoutConfig(t *testing.T) {
	prev := config.GetSafe()
	defer config.Set(prev)

	wantDefault := time.Duration(config.DefaultProxySSEWriteTimeoutSec) * time.Second
	config.Set(nil)
	if got := streamWriteTimeout(); got != wantDefault {
		t.Fatalf("nil config: got %v, want %v", got, wantDefault)
	}
	config.Set(&config.Config{})
	if got := streamWriteTimeout(); got != wantDefault {
		t.Fatalf("zero config: got %v, want %v", got, wantDefault)
	}
	config.Set(&config.Config{ProxySSEWriteTimeoutSec: 7})
	if got := streamWriteTimeout(); got != 7*time.Second {
		t.Fatalf("configured: got %v, want 7s", got)
	}

	for _, raw := range []string{"0", "-5", "not-a-number"} {
		cfg, _ := config.Load(map[string]string{"PROXY_SSE_WRITE_TIMEOUT_SEC": raw})
		if cfg.ProxySSEWriteTimeoutSec != config.DefaultProxySSEWriteTimeoutSec {
			t.Fatalf("%q parsed to %d, want the %d default", raw, cfg.ProxySSEWriteTimeoutSec, config.DefaultProxySSEWriteTimeoutSec)
		}
	}
	cfg, _ := config.Load(map[string]string{"PROXY_SSE_WRITE_TIMEOUT_SEC": "45"})
	if cfg.ProxySSEWriteTimeoutSec != 45 {
		t.Fatalf("explicit value parsed to %d, want 45", cfg.ProxySSEWriteTimeoutSec)
	}
}

// TestStreamWriteTimeoutReleasesSiteConcurrencySlot drives the real dispatch
// path (site limiter acquire → relay → 1s write timeout → release) against a
// TCP client that stops reading, then proves a follow-up request can take the
// same slot again.
func TestStreamWriteTimeoutReleasesSiteConcurrencySlot(t *testing.T) {
	setStreamWriteBudgetForTest(t, 1, 10)

	var upstreamRequests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		if upstreamRequests.Add(1) == 1 {
			// Stalled-client request: keep the upstream producing so the relay
			// can only stop by giving up on the downstream write.
			line := []byte(endlessSSEPattern)
			for {
				if _, err := w.Write(line); err != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
		_, _ = io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(upstream.Close)

	const siteID int64 = 77
	limiter := proxy.NewSiteConcurrencyLimiter()
	SetUpstreamConfig(&UpstreamConfig{
		Router: &upstreamTestRouter{selected: routing.SelectedChannel{
			Channel:     store.RouteChannel{ID: 101, Enabled: true},
			Account:     store.Account{ID: 7, Status: "active"},
			Site:        store.Site{ID: siteID, URL: upstream.URL, Status: "active", MaxConcurrency: 1},
			TokenValue:  "upstream-token",
			ActualModel: "gpt-4o-upstream",
		}},
		SiteLimiter: limiter,
	})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	requestBody := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	finished := make(chan struct{}, 1)
	srv := startStreamRelayServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatchUpstream(w, r, &Ctx{
			RequestedModel: "gpt-4o",
			DownstreamPath: "/v1/chat/completions",
			RawBody:        []byte(requestBody),
			IsStream:       true,
			MaxRetries:     1,
		})
		finished <- struct{}{}
	}), 200*time.Millisecond)

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	request := "POST /v1/chat/completions HTTP/1.1\r\nHost: metapi\r\nContent-Type: application/json\r\nContent-Length: " +
		fmt.Sprint(len(requestBody)) + "\r\n\r\n" + requestBody
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write request: %v", err)
	}
	// Never read: the relay must time out its downstream write.

	deadline := time.Now().Add(5 * time.Second)
	for limiter.ActiveCount(siteID) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("dispatch never acquired the site slot")
		}
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-finished:
	case <-time.After(15 * time.Second):
		t.Fatal("dispatch did not return after the downstream write deadline")
	}
	if got := limiter.ActiveCount(siteID); got != 0 {
		t.Fatalf("after the write timeout ActiveCount=%d, want 0 (slot leaked)", got)
	}

	// The slot is genuinely free: a reading client is admitted and served.
	resp, err := srv.Client().Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(requestBody))
	if err != nil {
		t.Fatalf("follow-up request: %v", err)
	}
	relayed, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		t.Fatalf("follow-up body: %v", readErr)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("follow-up status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(string(relayed), `"content":"ok"`) || !strings.Contains(string(relayed), "data: [DONE]") {
		t.Fatalf("follow-up body = %q, want a complete relay", relayed)
	}
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("follow-up dispatch did not finish")
	}
	if got := limiter.ActiveCount(siteID); got != 0 {
		t.Fatalf("after the follow-up ActiveCount=%d, want 0", got)
	}
}
