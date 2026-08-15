package proxyhandler

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errorAfterReader emits the given payload on the first Read and then returns
// err on every subsequent Read. It models a mid-stream upstream failure where
// the connection resets after partial data — the exact case the SSE error
// event must surface to the client.
type errorAfterReader struct {
	payload  []byte
	consumed bool
	err      error
}

func (r *errorAfterReader) Read(p []byte) (int, error) {
	if !r.consumed {
		n := copy(p, r.payload)
		r.consumed = true
		return n, nil
	}
	return 0, r.err
}

func (r *errorAfterReader) Close() error { return nil }

// TestHandleStreamUpstreamEmitsErrorEventOnReadError verifies that a
// non-EOF read error from the upstream body triggers a final SSE error
// event (plus [DONE]) before the connection closes. Without this event the
// client would only see a truncated stream and have to infer failure from
// the missing [DONE] marker.
func TestHandleStreamUpstreamEmitsErrorEventOnReadError(t *testing.T) {
	raw := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"
	body := &errorAfterReader{
		payload: []byte(raw),
		err:     errors.New("upstream connection reset"),
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(body),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	handleStreamUpstream(rec, req, resp, 7)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	out := rec.Body.String()

	if !strings.HasPrefix(out, raw) {
		t.Fatalf("body should start with the partial upstream chunk; got prefix %q", prefix(out, len(raw)))
	}
	if !strings.Contains(out, `data: {"error":{"message":"upstream stream interrupted","type":"upstream_error"}}`) {
		t.Fatalf("body = %q, want SSE error event for upstream stream interrupted", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("body = %q, want [DONE] marker after error event", out)
	}
	// Error event must come after the partial data, not replace it.
	errorIdx := strings.Index(out, "upstream stream interrupted")
	doneIdx := strings.Index(out, "data: [DONE]")
	dataIdx := strings.Index(out, `data: {"choices"`)
	if errorIdx < 0 || doneIdx < 0 || dataIdx < 0 {
		t.Fatalf("missing expected segments in body: %q", out)
	}
	if dataIdx > errorIdx {
		t.Fatalf("partial data chunk must precede the error event; dataIdx=%d errorIdx=%d", dataIdx, errorIdx)
	}
	if errorIdx > doneIdx {
		t.Fatalf("error event must precede the [DONE] marker; errorIdx=%d doneIdx=%d", errorIdx, doneIdx)
	}
}

// TestHandleStreamUpstreamNoErrorEventOnCleanEOF verifies a clean EOF (the
// normal end-of-stream path) does NOT inject an error event — only non-EOF
// read errors surface the upstream_error event.
func TestHandleStreamUpstreamNoErrorEventOnCleanEOF(t *testing.T) {
	raw := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(raw)),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	handleStreamUpstream(rec, req, resp, 3)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	out := rec.Body.String()
	if out != raw {
		t.Fatalf("body = %q, want clean relay of upstream body", out)
	}
	if strings.Contains(out, "upstream stream interrupted") {
		t.Fatalf("clean EOF must not emit an upstream_error event; body=%q", out)
	}
}

func prefix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
