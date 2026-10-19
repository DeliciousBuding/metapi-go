package proxyhandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestStreamFlushFailureReachesRelayAndRequestTerminal(t *testing.T) {
	var records []store.GatewayRequest
	handler := requestLifecycleMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		if err := writeDownstreamChunk(w, body, time.Second); !errors.Is(err, errFlushSentinel) {
			t.Errorf("relay flush error = %v, want %v", err, errFlushSentinel)
		}
	}), func(_ context.Context, entry store.GatewayRequest) error {
		records = append(records, entry)
		return nil
	})
	handler.ServeHTTP(&flushErrorWriter{header: make(http.Header)}, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if len(records) != 1 {
		t.Fatalf("request records = %d, want 1", len(records))
	}
	record := records[0]
	if record.HTTPStatus == nil || *record.HTTPStatus != http.StatusOK || record.TerminalStatus != "output_interrupted" || record.ReasonCode == nil || *record.ReasonCode != "downstream_write_failed" {
		t.Fatalf("flush failure changed request truth: status=%v terminal=%s reason=%v", record.HTTPStatus, record.TerminalStatus, record.ReasonCode)
	}
}

// Expose only the basic writer methods, without Flush or Unwrap support.
type writeOnlyStreamWriter struct{ http.ResponseWriter }

func TestStreamUnsupportedFlushDoesNotMarkWriteFailure(t *testing.T) {
	var records []store.GatewayRequest
	handler := requestLifecycleMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		if err := writeDownstreamChunk(w, body, time.Second); err != nil {
			t.Errorf("unsupported flush must remain a no-op: %v", err)
		}
		requestLifecycleFromContext(r.Context()).observe("success", true)
	}), func(_ context.Context, entry store.GatewayRequest) error {
		records = append(records, entry)
		return nil
	})
	handler.ServeHTTP(writeOnlyStreamWriter{httptest.NewRecorder()}, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if len(records) != 1 || records[0].TerminalStatus != "completed" || records[0].ReasonCode == nil || *records[0].ReasonCode != "normal_completion" {
		t.Fatalf("unsupported flush changed request truth: %+v", records)
	}
}
