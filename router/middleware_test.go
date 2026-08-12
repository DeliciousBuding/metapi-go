package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestTrustedRealIPIgnoresForwardedHeadersWithoutTrustedProxy(t *testing.T) {
	handler := TrustedRealIP(&config.Config{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "203.0.113.10:12345" {
			t.Fatalf("RemoteAddr = %q, want original direct peer", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestTrustedRealIPUsesForwardedHeaderFromTrustedProxy(t *testing.T) {
	handler := TrustedRealIP(&config.Config{
		TrustedProxyCidrs: []string{"127.0.0.1/32"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "198.51.100.99" {
			t.Fatalf("RemoteAddr = %q, want forwarded client IP", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.99, 127.0.0.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestTrustedRealIPIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	handler := TrustedRealIP(&config.Config{
		TrustedProxyCidrs: []string{"127.0.0.1/32"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "203.0.113.10:12345" {
			t.Fatalf("RemoteAddr = %q, want original untrusted peer", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequestLoggerCapturesStatusAndBytes(t *testing.T) {
	var gotStatus, gotBytes int
	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("hello"))
		sw, ok := w.(*statusRecorder)
		if !ok {
			t.Fatalf("writer = %T, want *statusRecorder", w)
		}
		gotStatus = sw.Status()
		gotBytes = sw.BytesWritten()
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if gotStatus != http.StatusCreated {
		t.Fatalf("recorded status = %d, want 201", gotStatus)
	}
	if gotBytes != 5 {
		t.Fatalf("recorded bytes = %d, want 5", gotBytes)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body = %q, want hello", rec.Body.String())
	}
}

func TestRequestLoggerPreservesStreamingInterfaces(t *testing.T) {
	base := &fakeStreamWriter{}
	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Fatalf("wrapped writer lost http.Flusher")
		}
		sd, ok := w.(interface{ SetWriteDeadline(time.Time) error })
		if !ok {
			t.Fatalf("wrapped writer lost SetWriteDeadline")
		}
		if err := sd.SetWriteDeadline(time.Time{}); err != nil {
			t.Fatalf("SetWriteDeadline: %v", err)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(base, req)

	if !base.deadlineSet {
		t.Fatalf("SetWriteDeadline was not forwarded to the underlying writer")
	}
}

func TestRecovererReturns500OnPanic(t *testing.T) {
	handler := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

type fakeStreamWriter struct {
	header      http.Header
	status      int
	deadlineSet bool
}

func (f *fakeStreamWriter) Header() http.Header {
	if f.header == nil {
		f.header = http.Header{}
	}
	return f.header
}
func (f *fakeStreamWriter) WriteHeader(code int)        { f.status = code }
func (f *fakeStreamWriter) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakeStreamWriter) Flush()                      {}
func (f *fakeStreamWriter) SetWriteDeadline(t time.Time) error {
	f.deadlineSet = true
	return nil
}
