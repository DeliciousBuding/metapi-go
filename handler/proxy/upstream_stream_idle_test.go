package proxyhandler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestStreamIdleGuardFiresAfterDeadline verifies the watchdog closes the body
// once no chunk arrives within the idle window.
func TestStreamIdleGuardFiresAfterDeadline(t *testing.T) {
	fired := make(chan struct{}, 1)
	g := newStreamIdleGuard(50*time.Millisecond, func() { fired <- struct{}{} })
	defer g.stop()
	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("idle guard did not fire within 2s of the deadline")
	}
	if !g.fired.Load() {
		t.Fatal("guard fired flag not set after firing")
	}
}

// TestStreamIdleGuardResetExtendsWindow verifies continuous chunks keep
// pushing the deadline forward so a healthy stream never trips the guard.
func TestStreamIdleGuardResetExtendsWindow(t *testing.T) {
	fired := make(chan struct{}, 1)
	g := newStreamIdleGuard(100*time.Millisecond, func() { fired <- struct{}{} })
	defer g.stop()
	deadline := time.After(350 * time.Millisecond)
	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			if g.fired.Load() {
				t.Fatal("guard fired flag set despite continuous resets")
			}
			return
		case <-tick.C:
			g.reset()
		case <-fired:
			t.Fatal("guard fired despite continuous chunks")
		}
	}
}

func setStreamIdleTimeoutForTest(t *testing.T, sec int) {
	t.Helper()
	prev := config.GetSafe()
	config.Set(&config.Config{
		ProxyMaxChannelAttempts:   10,
		ProxyStreamIdleTimeoutSec: sec,
	})
	t.Cleanup(func() { config.Set(prev) })
}

// TestStreamIdleTimeoutFallback covers the config lookup: nil config and
// non-positive values fall back to the default; positive values win.
func TestStreamIdleTimeoutFallback(t *testing.T) {
	prev := config.GetSafe()
	defer config.Set(prev)

	wantDefault := time.Duration(config.DefaultProxyStreamIdleTimeoutSec) * time.Second
	config.Set(nil)
	if got := streamIdleTimeout(); got != wantDefault {
		t.Fatalf("nil config: got %v, want default %v", got, wantDefault)
	}
	config.Set(&config.Config{})
	if got := streamIdleTimeout(); got != wantDefault {
		t.Fatalf("zero config: got %v, want default %v", got, wantDefault)
	}
	config.Set(&config.Config{ProxyStreamIdleTimeoutSec: 7})
	if got := streamIdleTimeout(); got != 7*time.Second {
		t.Fatalf("configured: got %v, want 7s", got)
	}
}

// TestHandleStreamUpstreamIdleTimeoutAbortsStalledStream is the end-to-end
// idle path: one chunk flows, the upstream goes silent, the guard aborts the
// stream after the window, the client keeps the relayed prefix and receives
// the distinct idle-timeout SSE error event, and the outcome is reported.
func TestHandleStreamUpstreamIdleTimeoutAbortsStalledStream(t *testing.T) {
	setStreamIdleTimeoutForTest(t, 1)

	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		// Upstream goes silent: no more chunks, body never closed by writer.
	}()

	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	resp.Header.Set("Content-Type", "text/event-stream")
	defer resp.Body.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	start := time.Now()
	usage, outcome, _ := handleStreamUpstream(rec, req, resp, 5)
	elapsed := time.Since(start)

	if outcome != streamEndedIdleTimeout {
		t.Fatalf("outcome = %v, want streamEndedIdleTimeout", outcome)
	}
	if usage.Found {
		t.Fatal("idle-aborted stream must not invent usage")
	}
	if elapsed < 900*time.Millisecond || elapsed > 10*time.Second {
		t.Fatalf("idle abort took %v; expected ~1s window", elapsed)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "upstream stream idle timeout") {
		t.Fatalf("response missing idle-timeout SSE error event: %q", body)
	}
	if !strings.Contains(body, "hi") {
		t.Fatalf("relayed prefix chunk lost: %q", body)
	}
}

// TestHandleStreamUpstreamFlowingStreamCompletesNormally proves a stream that
// keeps sending chunks inside the idle window runs to its clean EOF with no
// idle intervention.
func TestHandleStreamUpstreamFlowingStreamCompletesNormally(t *testing.T) {
	setStreamIdleTimeoutForTest(t, 1)

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < 5; i++ {
			if _, err := pw.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")); err != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: pr}
	resp.Header.Set("Content-Type", "text/event-stream")
	defer resp.Body.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	_, outcome, _ := handleStreamUpstream(rec, req, resp, 5)
	if outcome != streamEndedNormally {
		t.Fatalf("outcome = %v, want streamEndedNormally", outcome)
	}
	if body := rec.Body.String(); strings.Contains(body, "idle timeout") {
		t.Fatalf("flowing stream got idle error event: %q", body)
	}
}
