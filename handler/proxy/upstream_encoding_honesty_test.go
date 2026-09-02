package proxyhandler

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// These tests pin compression-encoding honesty on the PRODUCTION dispatch path
// (HandleChatCompletions -> dispatchSelectedUpstream ->
// dispatchEndpointAttemptWithContinue -> buffered relay / handleStreamUpstream).
// They deliberately assert on what reached the wire and on the three
// observability surfaces (channel health, proxy_logs, the downstream response)
// rather than on private helpers.

const encodingTestCompletionJSON = `{"id":"chatcmpl-enc","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"hello there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`

const encodingTestStreamBody = `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`

const encodingTestBufferedBody = `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`

// undecodableStreamPayload stands in for a brotli body. The stdlib has no
// brotli encoder and this lane must not add a dependency, so the fixture uses
// gzip bytes labelled "br": equally opaque to us, and the contract under test is
// "we do not decode br and we do not pretend we did", not the codec itself.
var undecodableStreamPayload = mustGzip([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello there\"}}]}\n\ndata: [DONE]\n\n"))

// lockedBuffer is a slog sink safe against a concurrently logging test in the
// same package (the -race gate must stay clean).
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureWarnLogs redirects the default logger at WARN+ and restores it on
// cleanup.
func captureWarnLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	out := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return out
}

// upstreamRequestRecorder records, per wire request, the Accept-Encoding the
// upstream actually saw and the path it was asked for. Mutex-guarded because the
// httptest server handler runs on its own goroutine.
type upstreamRequestRecorder struct {
	mu              sync.Mutex
	acceptEncodings []string
	paths           []string
	errors          []string
}

func (c *upstreamRequestRecorder) record(r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acceptEncodings = append(c.acceptEncodings, r.Header.Get("Accept-Encoding"))
	c.paths = append(c.paths, r.URL.Path)
}

func (c *upstreamRequestRecorder) encodings() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.acceptEncodings...)
}

func (c *upstreamRequestRecorder) hits() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.paths)
}

// negotiatingGzipUpstream behaves like a normal gateway: it gzip-encodes only
// when the request asked for gzip.
func negotiatingGzipUpstream(rec *upstreamRequestRecorder, body []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			_, _ = w.Write(body)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(mustGzip(body))
	}))
}

// fixedEncodingUpstream ignores negotiation entirely and always answers with the
// given Content-Encoding and raw bytes — the "gateway that compresses no matter
// what we asked for" case.
func fixedEncodingUpstream(rec *upstreamRequestRecorder, contentType, contentEncoding string, raw []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Encoding", contentEncoding)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(raw)
	}))
}

func mustGzip(raw []byte) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func mustZlib(raw []byte) []byte {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// runEncodingDispatch drives one chat completion through the production handler
// and returns the three observability surfaces plus the downstream response.
// Cross-protocol fallback is disabled so exactly one upstream attempt happens
// and the assertions stay single-valued.
func runEncodingDispatch(
	t *testing.T,
	upstreamURL string,
	requestBody string,
	siteCustomHeaders map[string]string,
	runtimeMutator func(*config.RuntimeSettings),
) (*upstreamTestRouter, *[]proxy.ProxyLogEntry, *httptest.ResponseRecorder) {
	t.Helper()

	return runEncodingDispatchWith(t, upstreamURL, "", requestBody, siteCustomHeaders, runtimeMutator)
}

// runEncodingDispatchWithProxyURL is the site-proxy variant: the site carries a
// ProxyURL, which routes the dispatch through platform.DoWithProxy instead of
// the executor.
func runEncodingDispatchWithProxyURL(
	t *testing.T,
	upstreamURL string,
	proxyURL string,
	requestBody string,
	siteCustomHeaders map[string]string,
) (*upstreamTestRouter, *[]proxy.ProxyLogEntry, *httptest.ResponseRecorder) {
	t.Helper()
	return runEncodingDispatchWith(t, upstreamURL, proxyURL, requestBody, siteCustomHeaders, nil)
}

func runEncodingDispatchWith(
	t *testing.T,
	upstreamURL string,
	proxyURL string,
	requestBody string,
	siteCustomHeaders map[string]string,
	runtimeMutator func(*config.RuntimeSettings),
) (*upstreamTestRouter, *[]proxy.ProxyLogEntry, *httptest.ResponseRecorder) {
	t.Helper()

	prevCfg := config.GetSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(prevCfg) })

	prevRt := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{DisableCrossProtocolFallback: true})
	if runtimeMutator != nil {
		config.UpdateRuntime(runtimeMutator)
	}
	t.Cleanup(func() { config.SetRuntime(prevRt) })

	site := store.Site{ID: 3, URL: upstreamURL, Status: "active"}
	if proxyURL != "" {
		site.ProxyURL = &proxyURL
	}
	if len(siteCustomHeaders) > 0 {
		var sb strings.Builder
		sb.WriteString("{")
		first := true
		for k, v := range siteCustomHeaders {
			if !first {
				sb.WriteString(",")
			}
			first = false
			sb.WriteString(`"` + k + `":"` + v + `"`)
		}
		sb.WriteString("}")
		raw := sb.String()
		site.CustomHeaders = &raw
	}

	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 42, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        site,
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

	req := makeProxyReq("POST", "/v1/chat/completions", requestBody)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)
	return router, logs, rec
}

// describeFailures renders the recorded channel failures readably so a red run
// names the exact status/reason that poisoned the channel.
func describeFailures(router *upstreamTestRouter) string {
	parts := make([]string, 0, len(router.failures))
	for _, f := range router.failures {
		status := "nil"
		if f.status != nil {
			status = strconv.Itoa(*f.status)
		}
		text := "nil"
		if f.errorText != nil {
			text = strconv.Quote(*f.errorText)
		}
		parts = append(parts, fmt.Sprintf("{channelID:%d status:%s errorText:%s}", f.channelID, status, text))
	}
	return strings.Join(parts, " ")
}

// describeLogs renders the proxy_log rows readably for the same reason.
func describeLogs(logs *[]proxy.ProxyLogEntry) string {
	parts := make([]string, 0, len(*logs))
	for _, e := range *logs {
		total := "nil"
		if e.TotalTokens != nil {
			total = strconv.FormatInt(*e.TotalTokens, 10)
		}
		parts = append(parts, fmt.Sprintf("{status:%q http_status:%d usage_source:%q total_tokens:%s}", e.Status, e.HTTPStatus, e.UsageSource, total))
	}
	return strings.Join(parts, " ")
}

// assertNoFailureRecorded is the core anti-false-failure assertion: the channel
// was not poisoned and the proxy_log row is a success row.
func assertNoFailureRecorded(t *testing.T, router *upstreamTestRouter, logs *[]proxy.ProxyLogEntry) {
	t.Helper()
	if len(router.failures) != 0 {
		t.Fatalf("recordUpstreamFailure calls = %d (%s), want 0 — an unreadable/encoded body must never be recorded as an upstream failure; proxy_logs = %s",
			len(router.failures), describeFailures(router), describeLogs(logs))
	}
	if len(router.successes) != 1 {
		t.Fatalf("recordUpstreamSuccess calls = %d (%#v), want exactly 1", len(router.successes), router.successes)
	}
	if len(*logs) != 1 {
		t.Fatalf("proxy_logs rows = %d (%s), want exactly 1", len(*logs), describeLogs(logs))
	}
	entry := (*logs)[0]
	if entry.Status != "success" {
		t.Fatalf("proxy_logs status = %q, want %q", entry.Status, "success")
	}
	if entry.HTTPStatus != http.StatusOK {
		t.Fatalf("proxy_logs http_status = %d, want 200", entry.HTTPStatus)
	}
}

// assertUsageUnknown pins "never invent tokens": the row must carry the explicit
// unknown source and zeroed token counters.
func assertUsageUnknown(t *testing.T, logs *[]proxy.ProxyLogEntry) {
	t.Helper()
	entry := (*logs)[0]
	if entry.UsageSource != "unknown" {
		t.Fatalf("proxy_logs usage_source = %q, want %q (tokens must never be invented for a body we could not read)", entry.UsageSource, "unknown")
	}
	for name, got := range map[string]*int64{
		"prompt_tokens":     entry.PromptTokens,
		"completion_tokens": entry.CompletionTokens,
		"total_tokens":      entry.TotalTokens,
	} {
		if got != nil && *got != 0 {
			t.Fatalf("proxy_logs %s = %d, want 0/unknown", name, *got)
		}
	}
}

// assertUsageAccounted pins the opposite: a body we could read must be billed.
func assertUsageAccounted(t *testing.T, logs *[]proxy.ProxyLogEntry) {
	t.Helper()
	entry := (*logs)[0]
	if entry.UsageSource != "upstream" {
		t.Fatalf("usage was not accounted from a readable body: proxy_logs = %s, want usage_source %q with 11/7/18 tokens", describeLogs(logs), "upstream")
	}
	if entry.PromptTokens == nil || *entry.PromptTokens != 11 {
		t.Fatalf("proxy_logs prompt_tokens = %v, want 11", entry.PromptTokens)
	}
	if entry.CompletionTokens == nil || *entry.CompletionTokens != 7 {
		t.Fatalf("proxy_logs completion_tokens = %v, want 7", entry.CompletionTokens)
	}
	if entry.TotalTokens == nil || *entry.TotalTokens != 18 {
		t.Fatalf("proxy_logs total_tokens = %v, want 18", entry.TotalTokens)
	}
}

// ---- 1. transparent-gzip invariant (regression anchor, green before & after) ----

func TestUpstreamTransparentGzipIsDecodedJudgedAndAccounted(t *testing.T) {
	rec := &upstreamRequestRecorder{}
	upstream := negotiatingGzipUpstream(rec, []byte(encodingTestCompletionJSON))
	t.Cleanup(upstream.Close)

	// PROXY_EMPTY_CONTENT_FAIL is on so this test also proves the content judge
	// saw the DECOMPRESSED body: a healthy answer must not be judged empty.
	warns := captureWarnLogs(t)
	router, logs, resp := runEncodingDispatch(t, upstream.URL, encodingTestBufferedBody, nil,
		func(rt *config.RuntimeSettings) { rt.ProxyEmptyContentFailEnabled = true })

	encodings := rec.encodings()
	if len(encodings) != 1 {
		t.Fatalf("upstream hits = %d (%v), want exactly 1", len(encodings), encodings)
	}
	if encodings[0] != "gzip" {
		t.Fatalf("outbound Accept-Encoding = %q, want %q (net/http must add it itself so it transparently decodes)", encodings[0], "gzip")
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("downstream status = %d, want 200", resp.Code)
	}
	if got := resp.Body.String(); got != encodingTestCompletionJSON {
		t.Fatalf("downstream body = %q, want the decompressed upstream JSON", got)
	}
	if got := resp.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("downstream Content-Encoding = %q, want empty (net/http re-frames the decoded body)", got)
	}
	assertNoFailureRecorded(t, router, logs)
	assertUsageAccounted(t, logs)
	if strings.Contains(warns.String(), "content encoding metapi does not decode") {
		t.Fatalf("transparent gzip must not warn about an undecodable body:\n%s", warns.String())
	}
}

// ---- 2. site custom_headers must never set the outbound Accept-Encoding ----

func TestSiteCustomAcceptEncodingNeverReachesTheUpstream(t *testing.T) {
	rec := &upstreamRequestRecorder{}
	upstream := negotiatingGzipUpstream(rec, []byte(encodingTestCompletionJSON))
	t.Cleanup(upstream.Close)

	_, logs, _ := runEncodingDispatch(t, upstream.URL, encodingTestBufferedBody,
		map[string]string{"Accept-Encoding": "br", "X-Site-Header": "keep-me"}, nil)

	encodings := rec.encodings()
	if len(encodings) != 1 {
		t.Fatalf("upstream hits = %d (%v), want exactly 1", len(encodings), encodings)
	}
	// The site-configured value must never reach the wire: it decides whether we
	// can read the answer at all. What does reach the wire is net/http's own
	// "gzip", re-added exactly because our outbound request no longer carries an
	// explicit Accept-Encoding.
	if strings.Contains(strings.ToLower(encodings[0]), "br") {
		t.Fatalf("outbound Accept-Encoding = %q — the site's custom_headers value reached the upstream, which switches off transparent decoding", encodings[0])
	}
	if encodings[0] != "gzip" {
		t.Fatalf("outbound Accept-Encoding = %q, want %q", encodings[0], "gzip")
	}
	// Proving the strip did not collateral-damage the rest of custom_headers is
	// done by the unit test; here we only need the accounting to stay correct.
	assertUsageAccounted(t, logs)
}

// ---- 3. the false failure: an undecodable STREAM body must not be recorded as one ----

func TestUndecodableStreamBodyIsNotRecordedAsFailure(t *testing.T) {
	rec := &upstreamRequestRecorder{}
	upstream := fixedEncodingUpstream(rec, "text/event-stream", "br", undecodableStreamPayload)
	t.Cleanup(upstream.Close)

	warns := captureWarnLogs(t)
	router, logs, resp := runEncodingDispatch(t, upstream.URL, encodingTestStreamBody, nil,
		func(rt *config.RuntimeSettings) { rt.ProxyEmptyContentFailEnabled = true })

	// Core invariant first: a body nobody could read is not evidence of an
	// upstream failure, so the channel must not be poisoned by it.
	assertNoFailureRecorded(t, router, logs)
	assertUsageUnknown(t, logs)

	if resp.Code != http.StatusOK {
		t.Fatalf("downstream status = %d, want 200 (the stream already began)", resp.Code)
	}
	if !bytes.Equal(resp.Body.Bytes(), undecodableStreamPayload) {
		t.Fatalf("downstream body = %d bytes, want the upstream's %d bytes relayed verbatim", resp.Body.Len(), len(undecodableStreamPayload))
	}
	if got := resp.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("downstream Content-Encoding = %q, want %q (verbatim passthrough must keep the upstream's truthful framing so the client can still decode it)", got, "br")
	}
	if !strings.Contains(warns.String(), "content encoding metapi does not decode") {
		t.Fatalf("expected the stable undecodable-encoding WARN, got:\n%s", warns.String())
	}
	if !strings.Contains(warns.String(), "content_encoding=br") {
		t.Fatalf("WARN must name the encoding, got:\n%s", warns.String())
	}
}

// ---- 4. an undecodable BUFFERED body is relayed verbatim, judged as nothing ----

func TestUndecodableBufferedBodyIsRelayedVerbatimWithoutFailure(t *testing.T) {
	payload := mustGzip([]byte(encodingTestCompletionJSON))
	rec := &upstreamRequestRecorder{}
	upstream := fixedEncodingUpstream(rec, "application/json", "br", payload)
	t.Cleanup(upstream.Close)

	warns := captureWarnLogs(t)
	router, logs, resp := runEncodingDispatch(t, upstream.URL, encodingTestBufferedBody, nil,
		func(rt *config.RuntimeSettings) { rt.ProxyEmptyContentFailEnabled = true })

	assertNoFailureRecorded(t, router, logs)
	assertUsageUnknown(t, logs)

	if resp.Code != http.StatusOK {
		t.Fatalf("downstream status = %d, want 200", resp.Code)
	}
	if !bytes.Equal(resp.Body.Bytes(), payload) {
		t.Fatalf("downstream body = %d bytes, want the upstream's %d bytes relayed verbatim", resp.Body.Len(), len(payload))
	}
	if got := resp.Header().Get("Content-Encoding"); got != "br" {
		t.Fatalf("downstream Content-Encoding = %q, want %q", got, "br")
	}
	if !strings.Contains(warns.String(), "content encoding metapi does not decode") {
		t.Fatalf("expected the stable undecodable-encoding WARN, got:\n%s", warns.String())
	}
}

// ---- 5. forced gzip body: usage and judgement must run on the DECODED content ----

func TestForcedGzipBodyIsDecodedForUsageAndJudgement(t *testing.T) {
	rec := &upstreamRequestRecorder{}
	upstream := fixedEncodingUpstream(rec, "application/json", "gzip", mustGzip([]byte(encodingTestCompletionJSON)))
	t.Cleanup(upstream.Close)

	router, logs, resp := runEncodingDispatch(t, upstream.URL, encodingTestBufferedBody, nil,
		func(rt *config.RuntimeSettings) { rt.ProxyEmptyContentFailEnabled = true })

	// Accounting first: a body we could read must be billed for what it says.
	assertNoFailureRecorded(t, router, logs)
	assertUsageAccounted(t, logs)

	if resp.Code != http.StatusOK {
		t.Fatalf("downstream status = %d, want 200", resp.Code)
	}
	if got := resp.Body.String(); got != encodingTestCompletionJSON {
		t.Fatalf("downstream body = %q, want the decompressed upstream JSON", got)
	}
	if got := resp.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("downstream Content-Encoding = %q, want empty (we re-frame the decoded body)", got)
	}
}

// ---- 6. forced deflate body: net/http never decodes deflate, so this pins our own decoder ----

func TestForcedDeflateBodyIsDecodedForUsageAndJudgement(t *testing.T) {
	rec := &upstreamRequestRecorder{}
	upstream := fixedEncodingUpstream(rec, "application/json", "deflate", mustZlib([]byte(encodingTestCompletionJSON)))
	t.Cleanup(upstream.Close)

	router, logs, resp := runEncodingDispatch(t, upstream.URL, encodingTestBufferedBody, nil,
		func(rt *config.RuntimeSettings) { rt.ProxyEmptyContentFailEnabled = true })

	// net/http never decodes "deflate", so this is the case where OUR decoder is
	// the only thing standing between the upstream's tokens and the bill.
	assertNoFailureRecorded(t, router, logs)
	assertUsageAccounted(t, logs)

	if resp.Code != http.StatusOK {
		t.Fatalf("downstream status = %d, want 200", resp.Code)
	}
	if got := resp.Body.String(); got != encodingTestCompletionJSON {
		t.Fatalf("downstream body = %q, want the inflated upstream JSON", got)
	}
	if got := resp.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("downstream Content-Encoding = %q, want empty (we re-frame the decoded body)", got)
	}
}

// ---- 7. keyword judgement must see through an encoded body (no false success) ----

func TestErrorKeywordInsideEncodedBodyIsStillJudged(t *testing.T) {
	errBody := []byte(`{"error":{"message":"quota exhausted upstream","type":"rate_limit_error"}}`)
	rec := &upstreamRequestRecorder{}
	upstream := fixedEncodingUpstream(rec, "application/json", "deflate", mustZlib(errBody))
	t.Cleanup(upstream.Close)

	router, logs, resp := runEncodingDispatch(t, upstream.URL, encodingTestBufferedBody, nil,
		func(rt *config.RuntimeSettings) { rt.ProxyErrorKeywords = []string{"quota exhausted"} })

	if resp.Code != http.StatusBadGateway {
		t.Fatalf("downstream status = %d, want 502 — the keyword scan must see through the encoded body", resp.Code)
	}
	if len(router.failures) != 1 {
		t.Fatalf("recordUpstreamFailure calls = %d (%#v), want exactly 1", len(router.failures), router.failures)
	}
	f := router.failures[0]
	if f.status == nil || *f.status != http.StatusBadGateway {
		t.Fatalf("failure status = %v, want 502", f.status)
	}
	if f.errorText == nil || !strings.Contains(*f.errorText, "quota exhausted") {
		t.Fatalf("failure errorText = %v, want it to name the matched keyword", f.errorText)
	}
	if len(*logs) != 1 {
		t.Fatalf("proxy_logs rows = %d (%#v), want exactly 1", len(*logs), *logs)
	}
	if (*logs)[0].Status == "success" {
		t.Fatal("proxy_logs recorded a success for an upstream answer that matched a failure keyword")
	}
}

// forwardingProxyUpstream is a minimal HTTP forward proxy: it records the
// headers that actually left our process and relays the request to the real
// upstream. DisableCompression keeps the proxy from adding an Accept-Encoding of
// its own or decoding the answer, so what it records is exactly what the data
// plane put on the wire and what comes back is still the upstream's framing.
func forwardingProxyUpstream(rec *upstreamRequestRecorder, target *httptest.Server) *httptest.Server {
	transport := &http.Transport{DisableCompression: true}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		outReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
		if err != nil {
			t_Errorf(rec, "proxy build request: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		outReq.Header = r.Header.Clone()
		outReq.Header.Set("X-Forwarded-By", "b3-test-proxy")
		upstreamResp, err := transport.RoundTrip(outReq)
		if err != nil {
			t_Errorf(rec, "proxy roundtrip: %v", err)
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer upstreamResp.Body.Close()
		for name, values := range upstreamResp.Header {
			for _, v := range values {
				w.Header().Add(name, v)
			}
		}
		w.WriteHeader(upstreamResp.StatusCode)
		_, _ = io.Copy(w, upstreamResp.Body)
	}))
}

// t_Errorf stores a proxy-side error on the recorder: the proxy handler runs on
// its own goroutine, where t.Fatalf is not allowed.
func t_Errorf(rec *upstreamRequestRecorder, format string, args ...any) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.errors = append(rec.errors, fmt.Sprintf(format, args...))
}

func (c *upstreamRequestRecorder) errorText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Join(c.errors, "; ")
}

// TestSiteCustomAcceptEncodingNeverReachesTheUpstreamViaSiteProxy covers the
// site-proxy dispatch branch (platform.DoWithProxy), which re-applies
// proxyConfig.CustomHeaders to the request AFTER the data plane built it. That
// re-apply is why the strip has to clear the custom_headers map at the source
// and not just req.Header.
func TestSiteCustomAcceptEncodingNeverReachesTheUpstreamViaSiteProxy(t *testing.T) {
	target := negotiatingGzipUpstream(&upstreamRequestRecorder{}, []byte(encodingTestCompletionJSON))
	t.Cleanup(target.Close)

	rec := &upstreamRequestRecorder{}
	proxySrv := forwardingProxyUpstream(rec, target)
	t.Cleanup(proxySrv.Close)

	router, logs, resp := runEncodingDispatchWithProxyURL(t, target.URL, proxySrv.URL, encodingTestBufferedBody,
		map[string]string{"Accept-Encoding": "br", "X-Site-Header": "keep-me"})

	if errs := rec.errorText(); errs != "" {
		t.Fatalf("forward proxy errors: %s", errs)
	}
	encodings := rec.encodings()
	if len(encodings) != 1 {
		t.Fatalf("proxy hits = %d (%v), want exactly 1", len(encodings), encodings)
	}
	if strings.Contains(strings.ToLower(encodings[0]), "br") {
		t.Fatalf("outbound Accept-Encoding through the site proxy = %q — platform re-applied the site's custom_headers value after our strip", encodings[0])
	}
	if encodings[0] != "gzip" {
		t.Fatalf("outbound Accept-Encoding through the site proxy = %q, want %q", encodings[0], "gzip")
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("downstream status = %d, want 200", resp.Code)
	}
	if got := resp.Body.String(); got != encodingTestCompletionJSON {
		t.Fatalf("downstream body = %q, want the upstream JSON", got)
	}
	assertNoFailureRecorded(t, router, logs)
	assertUsageAccounted(t, logs)
}
