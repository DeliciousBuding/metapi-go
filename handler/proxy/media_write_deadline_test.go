package proxyhandler

import (
	"bytes"
	"context"
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

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// mediaBinaryPattern is deliberately opaque: no SSE framing, no UTF-8
// assumption. A stalled relay must not invent a JSON error or SSE event after
// the last real byte.
var mediaBinaryPattern = []byte{
	0x49, 0x44, 0x33, 0x04, 0x00, 0x00, 0x00, 0x00,
	0xff, 0xfe, 0x00, 0x01, 0x80, 0x7f, 0xc3, 0x28,
}

// endlessMediaBody streams opaque media bytes forever so the relay can only
// end through the downstream write budget, the byte cap or cancellation.
type endlessMediaBody struct {
	pattern    []byte
	closeOnce  sync.Once
	closeCalls atomic.Int32
}

func (b *endlessMediaBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	n := 0
	for n < len(p) {
		n += copy(p[n:], b.pattern)
	}
	return n, nil
}

func (b *endlessMediaBody) Close() error {
	b.closeOnce.Do(func() { b.closeCalls.Add(1) })
	return nil
}

// runMediaRelayHandler mirrors the dispatch call site: the whole-request
// write deadline is already armed by the proxy surface, handleMediaByteStream
// owns the response, and the upstream body is closed exactly once.
func runMediaRelayHandler(results chan<- streamRelayResult, body io.ReadCloser, header http.Header) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     header,
			Body:       body,
		}
		var outcome streamOutcome
		func() {
			defer resp.Body.Close()
			_, outcome = handleMediaByteStream(w, r, resp)
		}()
		results <- streamRelayResult{outcome: outcome, elapsed: time.Since(started)}
	})
}

// TestMediaDownstreamSlowClientReleasesHTTP1 stops reading the response after
// sending the request. The native media relay must return as a client
// disconnect once the socket cannot accept more data instead of holding the
// goroutine, upstream body and concurrency slot forever.
func TestMediaDownstreamSlowClientReleasesHTTP1(t *testing.T) {
	setStreamWriteBudgetForTest(t, 1, 10)

	body := &endlessMediaBody{pattern: mediaBinaryPattern}
	results := make(chan streamRelayResult, 1)
	srv := startStreamRelayServer(t, runMediaRelayHandler(results, body, http.Header{
		"Content-Type": []string{"audio/mpeg"},
	}), 200*time.Millisecond)

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn, "GET /v1/audio/speech HTTP/1.1\r\nHost: metapi\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	// Deliberately never read: the response and then the socket buffers fill.

	var res streamRelayResult
	select {
	case res = <-results:
	case <-time.After(8 * time.Second):
		t.Fatal("media relay did not return after the downstream stopped reading")
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

// TestMediaDownstreamSlowClientReleasesHTTP2 is the HTTP/2 equivalent: the
// client receives headers and never reads the body, so the stream window stays
// exhausted. The per-write deadline must end the relay and release the
// upstream body.
func TestMediaDownstreamSlowClientReleasesHTTP2(t *testing.T) {
	setStreamWriteBudgetForTest(t, 1, 10)

	body := &endlessMediaBody{pattern: mediaBinaryPattern}
	results := make(chan streamRelayResult, 1)
	srv := startStreamRelayH2Server(t, runMediaRelayHandler(results, body, http.Header{
		"Content-Type": []string{"audio/mpeg"},
	}), 200*time.Millisecond)

	resp, err := srv.Client().Get(srv.URL + "/v1/audio/speech")
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
	case <-time.After(8 * time.Second):
		resp.Body.Close()
		t.Fatal("media relay did not return after the HTTP/2 stream window stalled")
	}
	resp.Body.Close()

	if res.outcome != streamEndedClientDisconnect {
		t.Fatalf("outcome = %v, want clientDisconnect", res.outcome)
	}
	if got := body.closeCalls.Load(); got != 1 {
		t.Fatalf("upstream body closed %d times, want exactly 1", got)
	}
}

// TestMediaChunkGapsBeyondWriteBudgetStayHealthy proves the media relay also
// clears the deadline between writes: a long first byte and a long gap between
// chunks are healthy as long as the bytes keep coming, and the relayed body
// plus whitelisted headers stay byte-identical.
func TestMediaChunkGapsBeyondWriteBudgetStayHealthy(t *testing.T) {
	const (
		firstChunkDelay  = 900 * time.Millisecond
		gapBetweenChunks = 1500 * time.Millisecond
	)
	first := append([]byte(nil), mediaBinaryPattern...)
	second := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0xff, 0xfe, 0x7f, 0x80}

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
				if _, err := pw.Write(first); err != nil {
					return
				}
				time.Sleep(gapBetweenChunks)
				_, _ = pw.Write(second)
			}()

			results := make(chan streamRelayResult, 1)
			srv := tc.start(t, runMediaRelayHandler(results, pr, http.Header{
				"Content-Type":        []string{"audio/mpeg"},
				"Content-Disposition": []string{`attachment; filename="fixture.mp3"`},
				"Content-Language":    []string{"zh"},
			}), 200*time.Millisecond)

			started := time.Now()
			resp, err := srv.Client().Get(srv.URL + "/v1/audio/speech")
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			defer resp.Body.Close()
			if tc.proto != nil {
				tc.proto(t, resp)
			}
			relayed, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("reading relayed media: %v", err)
			}
			want := append(append([]byte(nil), first...), second...)
			if !bytes.Equal(relayed, want) {
				t.Fatalf("relayed body = %x, want the exact upstream bytes %x", relayed, want)
			}
			if strings.Contains(string(relayed), "data:") || bytes.Contains(relayed, []byte(`{"error"`)) {
				t.Fatalf("media relay appended a protocol event: %x", relayed)
			}
			if got := resp.Header.Get("Content-Type"); got != "audio/mpeg" {
				t.Fatalf("Content-Type = %q, want audio/mpeg", got)
			}
			if got := resp.Header.Get("Content-Disposition"); got != `attachment; filename="fixture.mp3"` {
				t.Fatalf("Content-Disposition = %q, want the upstream value", got)
			}
			if got := resp.Header.Get("Content-Language"); got != "zh" {
				t.Fatalf("Content-Language = %q, want zh", got)
			}
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

// TestMediaFlushErrorReachesRequestLifecycle ensures the media relay treats a
// failed flush as a downstream failure instead of silently reading the
// upstream forever, and that the production response wrapper records the
// interrupted write. The error crosses the Unwrap hop real middleware uses.
func TestMediaFlushErrorReachesRequestLifecycle(t *testing.T) {
	var records []store.GatewayRequest
	handler := requestLifecycleMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := io.NopCloser(bytes.NewReader(append(mediaBinaryPattern, mediaBinaryPattern...)))
		_, outcome := handleMediaByteStream(w, r, &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       body,
		})
		if outcome != streamEndedClientDisconnect {
			t.Errorf("outcome = %v, want clientDisconnect after the flush error", outcome)
		}
	}), func(_ context.Context, entry store.GatewayRequest) error {
		records = append(records, entry)
		return nil
	})

	inner := &flushErrorWriter{header: make(http.Header)}
	handler.ServeHTTP(inner, httptest.NewRequest(http.MethodGet, "/v1/audio/speech", nil))

	// Write accepted the chunk; Flush is where the connection failure becomes
	// visible. The relay must stop at that error instead of reading on.
	want := append(append([]byte(nil), mediaBinaryPattern...), mediaBinaryPattern...)
	if !bytes.Equal(inner.body.Bytes(), want) {
		t.Fatalf("bytes before the flush failure = %x, want the upstream chunk %x", inner.body.Bytes(), want)
	}
	if len(records) != 1 {
		t.Fatalf("request records = %d, want 1", len(records))
	}
	record := records[0]
	if record.TerminalStatus != "output_interrupted" || record.ReasonCode == nil || *record.ReasonCode != "downstream_write_failed" {
		t.Fatalf("flush failure changed request truth: terminal=%s reason=%v", record.TerminalStatus, record.ReasonCode)
	}
}

// TestMediaWriteDeadlineReleasesSiteConcurrencySlot drives the real dispatch
// path (site limiter acquire -> media relay -> 1s write timeout -> release)
// against a TCP client that stops reading, then proves the failed downstream
// write is not recorded as an upstream fault and a follow-up request can take
// the same slot again.
func TestMediaWriteDeadlineReleasesSiteConcurrencySlot(t *testing.T) {
	setStreamWriteBudgetForTest(t, 1, 10)

	var upstreamRequests atomic.Int32
	upstreamClosed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("upstream path = %q, want /v1/audio/speech", r.URL.Path)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		if upstreamRequests.Add(1) == 1 {
			defer close(upstreamClosed)
			// Stalled-client request: keep producing so the relay can only stop
			// by giving up on the downstream write.
			for {
				if _, err := w.Write(mediaBinaryPattern); err != nil {
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
		_, _ = w.Write([]byte("ID3-final"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(upstream.Close)

	const siteID int64 = 78
	limiter := proxy.NewSiteConcurrencyLimiter()
	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 102, Enabled: true},
		Account:     store.Account{ID: 8, Status: "active"},
		Site:        store.Site{ID: siteID, URL: upstream.URL, Status: "active", MaxConcurrency: 1},
		TokenValue:  "upstream-token",
		ActualModel: "tts-upstream",
	}}
	SetUpstreamConfig(&UpstreamConfig{Router: router, SiteLimiter: limiter})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	var logs []proxy.ProxyLogEntry
	getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error {
		logs = append(logs, entry)
		return nil
	}

	requestBody := `{"model":"client-alias","input":"hi","voice":"alloy"}`
	finished := make(chan struct{}, 1)
	srv := startStreamRelayServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatchUpstream(w, r, &Ctx{
			RequestedModel: "client-alias",
			DownstreamPath: "/v1/audio/speech",
			RawBody:        []byte(requestBody),
			MaxRetries:     1,
		})
		finished <- struct{}{}
	}), 200*time.Millisecond)

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	request := "POST /v1/audio/speech HTTP/1.1\r\nHost: metapi\r\nContent-Type: application/json\r\nContent-Length: " +
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
	select {
	case <-upstreamClosed:
	case <-time.After(5 * time.Second):
		t.Fatal("closing the stalled downstream did not close the upstream request")
	}
	if len(router.failures) != 0 {
		t.Fatalf("client write failure was recorded as an upstream fault: %+v", router.failures)
	}
	if len(router.successes) != 1 {
		t.Fatalf("successes = %d, want 1 (client disconnect is not an upstream fault)", len(router.successes))
	}
	if len(logs) == 0 {
		t.Fatal("client disconnect produced no proxy_log entry")
	}
	for _, entry := range logs {
		if entry.UsageSource != "unknown" || entry.TotalTokens != nil {
			t.Fatalf("media usage was invented: source=%q total=%v", entry.UsageSource, entry.TotalTokens)
		}
	}

	// The slot is genuinely free: a reading client is admitted and served.
	resp, err := srv.Client().Post(srv.URL+"/v1/audio/speech", "application/json", strings.NewReader(requestBody))
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
	if !bytes.Equal(relayed, []byte("ID3-final")) {
		t.Fatalf("follow-up body = %x, want the complete media payload", relayed)
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
