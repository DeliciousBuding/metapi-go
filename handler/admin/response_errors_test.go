package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

// TestWriteErrorWithRequest_IncludesRequestID verifies that the new helper
// populates the additive request_id field in the JSON body and mirrors it
// onto the X-Request-Id response header when a request ID is in context.
// This is the core of Fix 1: admin error bodies must carry request_id so
// API consumers can correlate errors with server logs (the proxy path
// already did this via handler/proxy/router.go).
func TestWriteErrorWithRequest_IncludesRequestID(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/sites", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.RequestIDKey, "req-test-456"))

	writeErrorWithRequest(rec, req, http.StatusBadRequest, "Invalid site payload: missing field 'name'")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-test-456" {
		t.Fatalf("X-Request-Id header = %q, want req-test-456", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if body["error"] != "Invalid site payload: missing field 'name'" {
		t.Fatalf("error field = %v", body["error"])
	}
	if body["request_id"] != "req-test-456" {
		t.Fatalf("request_id = %v, want req-test-456", body["request_id"])
	}
}

// TestWriteErrorWithRequest_OmitsRequestIDWhenAbsent verifies backward
// compatibility: when no request ID is in context (e.g. ad-hoc callers
// without the request-id middleware), the request_id field is omitted
// entirely (omitempty) — never serialized as null.
func TestWriteErrorWithRequest_OmitsRequestIDWhenAbsent(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/sites", nil)

	writeErrorWithRequest(rec, req, http.StatusBadRequest, "bad request")

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if _, ok := body["request_id"]; ok {
		t.Fatalf("request_id should be omitted when absent, got %v", body["request_id"])
	}
	if body["error"] != "bad request" {
		t.Fatalf("error = %v, want 'bad request'", body["error"])
	}
}

// TestWriteErrorWithRequest_DoesNotOverrideExistingHeader verifies that an
// ingress-set X-Request-Id header is preserved (not overwritten) while the
// body still carries the context request id. Mirrors the proxy path behavior.
func TestWriteErrorWithRequest_DoesNotOverrideExistingHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set("X-Request-Id", "ingress-id")
	req := httptest.NewRequest("GET", "/api/sites", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.RequestIDKey, "ctx-id"))

	writeErrorWithRequest(rec, req, http.StatusBadGateway, "boom")

	if got := rec.Header().Get("X-Request-Id"); got != "ingress-id" {
		t.Fatalf("X-Request-Id = %q, want ingress-id (not overridden)", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["request_id"] != "ctx-id" {
		t.Fatalf("body request_id = %v, want ctx-id", body["request_id"])
	}
}

// TestSites_Create_MalformedJSON_RequestIDAndFieldContext is an integration
// test covering Fix 1 (request_id in admin error body) + Fix 3 (generic
// decode errors include the parse error context). It sends malformed JSON
// to POST /api/sites with a known request ID injected into the context and
// verifies the 400 response carries both the request_id and the decode
// error suffix.
func TestSites_Create_MalformedJSON_RequestIDAndFieldContext(t *testing.T) {
	_, r := setupSitesTest(t)

	req := httptest.NewRequest("POST", "/api/sites", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.RequestIDKey, "req-int-789"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-int-789" {
		t.Fatalf("X-Request-Id = %q, want req-int-789", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if body["request_id"] != "req-int-789" {
		t.Fatalf("request_id = %v, want req-int-789", body["request_id"])
	}
	errMsg, _ := body["error"].(string)
	if !strings.HasPrefix(errMsg, "Invalid site payload:") {
		t.Fatalf("error message should include 'Invalid site payload:' prefix (Fix 3), got %q", errMsg)
	}
	if !strings.Contains(errMsg, "invalid character") {
		t.Fatalf("error message should include decode error context, got %q", errMsg)
	}
}

// TestRoutes_Create_MalformedJSON_FieldContext verifies Fix 3 for the
// token-routes create path: the generic "Invalid request body" message
// now includes the decode error context (e.g. the JSON parse failure).
func TestRoutes_Create_MalformedJSON_FieldContext(t *testing.T) {
	_, r := setupTokenRoutesTest(t)

	req := httptest.NewRequest("POST", "/api/routes", strings.NewReader("not json at all"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	errMsg, _ := body["error"].(string)
	if !strings.HasPrefix(errMsg, "Invalid request body:") {
		t.Fatalf("error message should include 'Invalid request body:' prefix (Fix 3), got %q", errMsg)
	}
}

// TestSites_Create_EmptyName_RequestIDInError verifies Fix 1 for a
// field-specific validation error (not just the decode path): the 400 for a
// missing 'name' field must also carry the request_id so the caller can
// correlate the validation rejection with server logs.
func TestSites_Create_EmptyName_RequestIDInError(t *testing.T) {
	_, r := setupSitesTest(t)

	body := map[string]any{"name": "  ", "url": "https://api.openai.com"}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/sites", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), middleware.RequestIDKey, "req-name-011"))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if resp["request_id"] != "req-name-011" {
		t.Fatalf("request_id = %v, want req-name-011", resp["request_id"])
	}
	if errMsg, _ := resp["error"].(string); !strings.Contains(errMsg, "name") {
		t.Fatalf("error message should name the failing field, got %q", errMsg)
	}
}
