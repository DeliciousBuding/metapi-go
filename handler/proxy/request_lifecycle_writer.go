package proxyhandler

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
)

type requestResponseWriter struct {
	http.ResponseWriter
	state    *requestLifecycle
	stream   *incrementalSseAnalyzer
	terminal *nativeTerminalBody
}

func (w *requestResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *requestResponseWriter) WriteHeader(status int) {
	if status >= 200 {
		w.state.mu.Lock()
		if w.state.entry.HTTPStatus == nil {
			value := status
			elapsed := time.Since(w.state.started).Milliseconds()
			w.state.entry.HTTPStatus = &value
			w.state.entry.FirstByteLatencyMs = &elapsed
		}
		w.state.mu.Unlock()
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *requestResponseWriter) Write(body []byte) (int, error) {
	w.state.mu.Lock()
	committed := w.state.entry.HTTPStatus != nil
	w.state.mu.Unlock()
	if !committed {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	if err == nil && n != len(body) {
		err = io.ErrShortWrite
	}
	w.state.mu.Lock()
	w.state.entry.OutputStarted = w.state.entry.OutputStarted || n > 0
	w.state.writeFailed = w.state.writeFailed || err != nil
	w.state.mu.Unlock()
	if err == nil {
		// Observe only bytes accepted by the downstream writer, not upstream
		// bytes read ahead of a failed client write.
		if w.stream == nil && (strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") || strings.HasPrefix(string(body), "data:") || strings.HasPrefix(string(body), "event:")) {
			w.stream = newIncrementalSseAnalyzer()
			w.terminal = &nativeTerminalBody{}
			if w.state.entry.DownstreamPath != nil {
				w.terminal.completionProtocol, _ = proxy.EndpointFromPath(*w.state.entry.DownstreamPath)
			}
			w.stream.onEvent = func(event SseEvent) {
				w.terminal.observeTerminalEvent(&event)
				w.state.mu.Lock()
				w.state.protocolCompleted = w.terminal.terminalSeen && !w.terminal.terminalFailed
				w.state.protocolFailed = w.state.protocolFailed || w.terminal.terminalFailed || IsSseErrorEvent(event)
				w.state.mu.Unlock()
			}
		}
		if w.stream != nil {
			w.stream.Push(body[:n])
		}
	}
	return n, err
}
func (w *requestResponseWriter) Flush() {
	_ = w.FlushError()
}
func (w *requestResponseWriter) FlushError() error {
	w.state.mu.Lock()
	committed := w.state.entry.HTTPStatus != nil
	w.state.mu.Unlock()
	if !committed {
		w.WriteHeader(http.StatusOK)
	}
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		w.state.mu.Lock()
		w.state.writeFailed = true
		w.state.mu.Unlock()
	}
	return err
}
func (w *requestResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.state.mu.Lock()
		w.state.hijacked = true
		w.state.mu.Unlock()
	}
	return conn, rw, err
}
func (w *requestResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{w}, r)
}

// These narrow writer capabilities let auth report safe rejection metadata
// without importing handlers or exposing credential values.
func (w *requestResponseWriter) ObserveProxyKey(id int64) {
	w.state.mu.Lock()
	w.state.entry.DownstreamAPIKeyID = &id
	w.state.mu.Unlock()
}
func (w *requestResponseWriter) ObserveProxyUser(userID, groupID *int64) {
	w.state.mu.Lock()
	w.state.entry.OwnerUserID, w.state.entry.UserGroupID = userID, groupID
	w.state.mu.Unlock()
}
func (w *requestResponseWriter) ObserveProxyCredential(token string) {
	w.state.mu.Lock()
	defer w.state.mu.Unlock()
	if w.state.entry.ExternalRequestID != nil && *w.state.entry.ExternalRequestID == token {
		w.state.entry.ExternalRequestID = nil
	}
}
func (w *requestResponseWriter) ObserveProxyRejection(code string) {
	w.state.reject(code)
}

func (w *requestResponseWriter) ObserveProxyPanic() {
	w.state.mu.Lock()
	w.state.gatewayFailure = "handler_panic"
	w.state.failureAfterOutput = w.state.entry.OutputStarted
	w.state.mu.Unlock()
}
