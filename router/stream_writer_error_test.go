package router

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
)

type streamFlushFailureWriter struct {
	fakeStreamWriter
	err error
}

func (w *streamFlushFailureWriter) FlushError() error { return w.err }

type streamWriterUnwrapper struct{ http.ResponseWriter }

func (w streamWriterUnwrapper) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestStreamFlushFailureThroughProductionMiddleware(t *testing.T) {
	for _, withLifecycle := range []bool{false, true} {
		name := "logger"
		if withLifecycle {
			name = "logger-and-lifecycle"
		}
		t.Run(name, func(t *testing.T) {
			wantErr := errors.New("downstream flush failed")
			base := &streamFlushFailureWriter{err: wantErr}
			inner := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				controller := http.NewResponseController(w)
				if err := controller.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
					t.Fatalf("set deadline: %v", err)
				}
				if err := controller.Flush(); !errors.Is(err, wantErr) {
					t.Fatalf("flush error = %v, want %v", err, wantErr)
				}
			}))
			if withLifecycle {
				inner = proxyhandler.RequestLifecycle(inner)
			}
			handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				inner.ServeHTTP(w, r)
				recorder := w.(*statusRecorder)
				if recorder.Status() != http.StatusOK || recorder.BytesWritten() != 0 {
					t.Fatalf("flush-only accounting = status %d bytes %d, want 200/0", recorder.Status(), recorder.BytesWritten())
				}
			}))
			handler.ServeHTTP(streamWriterUnwrapper{base}, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			if !base.deadlineSet {
				t.Fatal("write deadline did not reach the writer through Unwrap")
			}
			if base.status != http.StatusOK {
				t.Fatalf("underlying status = %d, want 200", base.status)
			}
		})
	}
}
