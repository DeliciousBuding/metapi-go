// Package shared provides common HTTP handler utilities used across
// admin and proxy handler packages.
package shared

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// APIError represents a structured JSON error response.
// Message is safe for public consumption; Internal is logged but never sent.
// JSON field names are camelCase-compatible public keys used by the admin UI:
// - error (string message)
// - detail (optional classifier / subtype)
// - request_id (optional ingress correlation id, snake_case for log/ops tools)
type APIError struct {
	Code      int    `json:"-"`
	Message   string `json:"error"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Internal  error  `json:"-"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Internal != nil {
		return e.Message + ": " + e.Internal.Error()
	}
	return e.Message
}

// WriteError writes a structured JSON error response to the client.
// It sets Content-Type: application/json and the given HTTP status code.
// Callers must use a non-2xx status for failures — never HTTP 200 with an error body.
func WriteError(w http.ResponseWriter, code int, message string) {
	WriteAPIError(w, &APIError{Code: code, Message: message})
}

// WriteAPIError writes the public fields of APIError with the given status.
// Status defaults to Code when Code > 0; otherwise 500.
// When RequestID is set, also mirrors it on the X-Request-Id response header
// if that header is still empty (ingress middleware usually sets it first).
func WriteAPIError(w http.ResponseWriter, err *APIError) {
	if err == nil {
		err = &APIError{Code: http.StatusInternalServerError, Message: "internal error"}
	}
	code := err.Code
	if code < 400 {
		// Guard against accidental silent success statuses for error bodies.
		if code == 0 {
			code = http.StatusInternalServerError
		} else {
			code = http.StatusBadRequest
		}
	}
	if err.RequestID != "" && w.Header().Get("X-Request-Id") == "" {
		w.Header().Set("X-Request-Id", err.RequestID)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if encErr := json.NewEncoder(w).Encode(APIError{
		Message:   err.Message,
		Detail:    err.Detail,
		RequestID: err.RequestID,
	}); encErr != nil {
		slog.Warn("shared: failed to write error response", "error", encErr, "request_id", err.RequestID)
	}
}

// WriteJSON writes v as a JSON response with Content-Type: application/json.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("shared: failed to write JSON response", "error", err)
	}
}
