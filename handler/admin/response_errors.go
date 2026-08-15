package admin

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/go-chi/chi/v5/middleware"
)

// writeError writes a unified admin API error: non-2xx status + camelCase JSON
// {"error":"..."}. Prefer this over writeJSON(..., {"success":false,...}) for
// mutation failure paths so clients never see HTTP 200 with an error body.
//
// writeError does NOT populate request_id because it has no request access.
// Use writeErrorWithRequest for any call site that has the *http.Request so
// API consumers can correlate error bodies with server logs (the proxy path
// already emits request_id via handler/proxy/router.go).
func writeError(w http.ResponseWriter, code int, message string) {
	shared.WriteError(w, code, message)
}

// writeErrorWithRequest writes a unified admin API error and, when a request
// ID is present in the request context (set by router.WithRequestID via
// chi's middleware.RequestID), mirrors it into the JSON body as the additive
// "request_id" field and onto the X-Request-Id response header when unset.
//
// Backward compatibility: when no request ID is in context (e.g. ad-hoc
// callers without the request-id middleware), the request_id field is
// omitted entirely (omitempty) — never serialized as null.
func writeErrorWithRequest(w http.ResponseWriter, r *http.Request, code int, message string) {
	requestID := middleware.GetReqID(r.Context())
	shared.WriteAPIError(w, &shared.APIError{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	})
}
