package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func serveWithRequestID(t *testing.T, clientID string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var contextID string
	handler := WithRequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextID = middleware.GetReqID(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	if clientID != "" {
		req.Header.Set("X-Request-Id", clientID)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, contextID
}

func TestWithRequestIDEchoesValidClientID(t *testing.T) {
	const clientID = "trace-abc.1_2:3"
	rec, contextID := serveWithRequestID(t, clientID)
	if got := rec.Header().Get("X-Request-Id"); got != clientID {
		t.Fatalf("response X-Request-Id = %q, want %q", got, clientID)
	}
	if contextID != clientID {
		t.Fatalf("context request id = %q, want %q", contextID, clientID)
	}
}

func TestWithRequestIDReplacesInvalidClientID(t *testing.T) {
	cases := map[string]string{
		"too long":     strings.Repeat("a", maxClientRequestIDLength+1),
		"whitespace":   "trace id",
		"control byte": "trace\x01id",
		"quotes":       `trace"id`,
		"percent":      "trace%0aid",
	}
	generated := regexp.MustCompile(`^req-[0-9a-f]{32}$`)
	for name, clientID := range cases {
		t.Run(name, func(t *testing.T) {
			rec, contextID := serveWithRequestID(t, clientID)
			got := rec.Header().Get("X-Request-Id")
			if got == clientID {
				t.Fatalf("invalid client id %q was echoed", clientID)
			}
			if !generated.MatchString(got) {
				t.Fatalf("generated id = %q, want req-<32 hex>", got)
			}
			if contextID != got {
				t.Fatalf("context id %q != response id %q", contextID, got)
			}
		})
	}
}

func TestNewRequestIDOmitsHostname(t *testing.T) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	generated := regexp.MustCompile(`^req-[0-9a-f]{32}$`)
	for i := 0; i < 20; i++ {
		id := newRequestID()
		if !generated.MatchString(id) {
			t.Fatalf("newRequestID() = %q, want req-<32 hex>", id)
		}
		if hostname != "" && strings.Contains(strings.ToLower(id), strings.ToLower(hostname)) {
			t.Fatalf("request id %q leaks hostname %q", id, hostname)
		}
	}
}
