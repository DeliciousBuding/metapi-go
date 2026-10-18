package router

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// maxClientRequestIDLength bounds a client-supplied correlation id before it
// is echoed or written to logs/audit records (audit L5).
const maxClientRequestIDLength = 128

// clientRequestIDPattern is the character allow-list for client-supplied ids:
// ASCII letters, digits, dot, underscore, colon and hyphen. Anything else
// (whitespace, control bytes, quotes, format directives) is discarded.
var clientRequestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

// requestIDFallback is used only if crypto/rand is unavailable; it keeps ids
// unique without embedding the process hostname (audit L5).
var requestIDFallback atomic.Uint64

// WithRequestID validates or generates the request id, sets X-Request-Id on the
// response, and stores it in the chi context for downstream correlation.
//
// Generated ids carry no hostname/PID prefix: chi's default is "host/pid-N",
// which leaks process identity when responses or logs leave the host.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := validClientRequestID(r.Header.Get("X-Request-Id"))
		if requestID == "" {
			requestID = newRequestID()
		}
		r = r.WithContext(context.WithValue(r.Context(), middleware.RequestIDKey, requestID))
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r)
	})
}

// validClientRequestID returns the client id when it passes the length and
// character allow-list, or "" when it must be replaced.
func validClientRequestID(candidate string) string {
	if len(candidate) == 0 || len(candidate) > maxClientRequestIDLength {
		return ""
	}
	if !clientRequestIDPattern.MatchString(candidate) {
		return ""
	}
	return candidate
}

// newRequestID returns a random, host-independent request id.
func newRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return "req-" + hex.EncodeToString(raw[:])
	}
	return fmt.Sprintf("req-%d-%d", time.Now().UnixNano(), requestIDFallback.Add(1))
}

// RequestIDFromContext extracts the request ID from context.
// Returns an empty string if not set.
func RequestIDFromContext(r *http.Request) string {
	return middleware.GetReqID(r.Context())
}
