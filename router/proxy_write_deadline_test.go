package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// deadlineWriter records the write deadline a handler arms. httptest.Recorder
// does not implement SetWriteDeadline, so the real server path needs a stand-in.
type deadlineWriter struct {
	http.ResponseWriter
	called   bool
	deadline time.Time
}

func (d *deadlineWriter) SetWriteDeadline(t time.Time) error {
	d.called = true
	d.deadline = t
	return nil
}

// unwrapper mimics chi's middleware.WrapResponseWriter shape: the global
// RequestLogger wraps every ResponseWriter, so the deadline must survive one
// Unwrap() hop to reach the real connection.
type unwrapper struct{ inner http.ResponseWriter }

func (u unwrapper) Header() http.Header         { return u.inner.Header() }
func (u unwrapper) Write(p []byte) (int, error) { return u.inner.Write(p) }
func (u unwrapper) WriteHeader(code int)        { u.inner.WriteHeader(code) }
func (u unwrapper) Unwrap() http.ResponseWriter { return u.inner }

func TestProxyWriteDeadline_ArmsBudgetAboveExecutorCeiling(t *testing.T) {
	before := time.Now()
	writer := &deadlineWriter{ResponseWriter: httptest.NewRecorder()}

	ProxyWriteDeadline(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(writer, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))

	if !writer.called {
		t.Fatal("proxy surface must re-arm the connection write deadline")
	}
	elapsed := writer.deadline.Sub(before)
	ceiling := proxy.RequestCeiling(0)
	if elapsed < ceiling {
		t.Errorf("write budget %v is below the executor ceiling %v — the inversion is back", elapsed, ceiling)
	}
	if elapsed > ceiling+30*time.Minute {
		t.Errorf("write budget %v is unbounded relative to ceiling %v", elapsed, ceiling)
	}
}

func TestProxyWriteDeadline_ReachesWrappedWriter(t *testing.T) {
	writer := &deadlineWriter{ResponseWriter: httptest.NewRecorder()}
	wrapped := unwrapper{inner: writer}

	ProxyWriteDeadline(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).
		ServeHTTP(wrapped, httptest.NewRequest(http.MethodPost, "/v1/messages", nil))

	if !writer.called {
		t.Error("deadline must be armed through a chi-style wrapped ResponseWriter")
	}
}
