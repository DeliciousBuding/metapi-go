package proxyhandler

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
)

// writeSSEStreamError writes one terminal SSE error event (plus [DONE] on
// Chat Completions and legacy endpoints). The whole sequence shares a single downstream
// write window so a client that stopped reading cannot hold the relay past
// the budget, and a stale deadline from an earlier chunk cannot make these
// writes fail instantly.
func writeSSEStreamError(w http.ResponseWriter, path, message, typ string, budget time.Duration, sequence int64) error {
	clear := armStreamWriteDeadline(w, budget)
	defer clear()

	endpoint, _ := proxy.EndpointFromPath(path)
	if endpoint == proxy.EndpointMessages {
		// Anthropic errors are terminal error events, not OpenAI [DONE].
		typ = "api_error"
	}
	body := map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    typ,
		},
	}
	if endpoint == proxy.EndpointResponses {
		body = map[string]any{"code": typ, "message": message, "param": nil, "sequence_number": sequence}
	}
	if endpoint == proxy.EndpointMessages || endpoint == proxy.EndpointResponses {
		body["type"] = "error"
		if _, err := w.Write([]byte("event: error\n")); err != nil {
			return err
		}
	}
	payload, _ := json.Marshal(body)
	if _, err := w.Write(sseEvent(string(payload))); err != nil {
		return err
	}
	if endpoint != proxy.EndpointMessages && endpoint != proxy.EndpointResponses {
		if _, err := w.Write(sseDone()); err != nil {
			return err
		}
	}
	return flushStream(w)
}

// streamWriteTimeout returns the downstream per-write deadline for every
// relayed stream (SSE and native media byte streams) from the startup-loaded
// config singleton, falling back to DefaultProxyStreamWriteTimeoutSec when
// config is not loaded (tests) or the operator value is non-positive. Like the
// other PROXY_* timeouts the value is frozen at startup.
func streamWriteTimeout() time.Duration {
	sec := config.DefaultProxyStreamWriteTimeoutSec
	if cfg := config.GetSafe(); cfg != nil && cfg.ProxyStreamWriteTimeoutSec > 0 {
		sec = cfg.ProxyStreamWriteTimeoutSec
	}
	return time.Duration(sec) * time.Second
}

// writeDownstreamChunk relays one upstream chunk as a single bounded downstream
// write+flush. Both halves matter: net/http buffers small writes, so the
// socket write that can stall on a client that stopped reading usually happens
// in Flush, and a flush error means the connection is gone even when Write
// accepted the bytes.
func writeDownstreamChunk(w http.ResponseWriter, chunk []byte, budget time.Duration) error {
	clear := armStreamWriteDeadline(w, budget)
	defer clear()

	if _, err := w.Write(chunk); err != nil {
		return err
	}
	return flushStream(w)
}

// armStreamWriteDeadline bounds the caller's immediately following write and
// returns the function that clears the deadline again. Callers MUST clear it:
// the HTTP/2 server implements SetWriteDeadline with a per-stream timer that
// resets the stream when it fires, even while the handler is idle waiting for
// the next upstream chunk. Keeping it armed between writes would kill healthy
// streams whose chunk gaps exceed the write budget.
//
// Writers without SetWriteDeadline support (httptest recorders and custom test
// writers) keep their existing unbounded behavior. http.ResponseController
// unwraps middleware wrappers such as router.statusRecorder.
func armStreamWriteDeadline(w http.ResponseWriter, budget time.Duration) func() {
	clear := func() {}
	if budget <= 0 {
		return clear
	}
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(budget)); err != nil {
		return clear
	}
	return func() { clearStreamWriteDeadline(w) }
}

// clearStreamWriteDeadline removes any write deadline currently armed on the
// downstream connection. Writers without deadline support are left as-is.
func clearStreamWriteDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
}

// flushStream flushes the current downstream buffer. A writer without flush
// support keeps the historical no-op behavior (it cannot be a real streaming
// transport); any other error is a downstream failure the relay must act on
// instead of silently continuing to read the upstream.
func flushStream(w http.ResponseWriter) error {
	err := http.NewResponseController(w).Flush()
	if err != nil && errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

// isDownstreamWriteTimeout reports whether a failed Write/Flush failed because
// the per-write deadline expired (the client stopped reading). The relay still
// classifies it as a client disconnect; this only distinguishes the log line.
func isDownstreamWriteTimeout(err error) bool {
	var netErr net.Error
	return errors.Is(err, os.ErrDeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout())
}
