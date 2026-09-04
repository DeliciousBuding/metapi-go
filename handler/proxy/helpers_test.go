package proxyhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- writeJSONError ----

func TestWriteJSONError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSONError(rec, 400, "bad request", "invalid_request_error")

	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "bad request") {
		t.Errorf("body = %q, should contain 'bad request'", body)
	}
	if !strings.Contains(body, "invalid_request_error") {
		t.Errorf("body = %q, should contain 'invalid_request_error'", body)
	}

	// Verify valid JSON
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
}

func TestWriteJSONError_SpecialChars(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSONError(rec, 500, `special "quote" \end`, "server_error")

	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	errObj := m["error"].(map[string]any)
	if errObj["message"] != `special "quote" \end` {
		t.Errorf("message = %q", errObj["message"])
	}
}

func TestWriteJSONErrorWithRequest_IncludesRequestID(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSONErrorWithRequest(rec, 503, "All channels exhausted", "server_error", "req-trace-9")

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-trace-9" {
		t.Fatalf("X-Request-Id = %q", got)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v body=%s", err, rec.Body.String())
	}
	errObj := m["error"].(map[string]any)
	if errObj["request_id"] != "req-trace-9" {
		t.Fatalf("request_id = %v", errObj["request_id"])
	}
}

// ---- writeJSON ----

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, 200, map[string]any{"key": "value"})

	if rec.Code != 200 {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["key"] != "value" {
		t.Errorf("key = %v", m["key"])
	}
}

func TestWriteJSON_NestedStructures(t *testing.T) {
	rec := httptest.NewRecorder()
	body := map[string]any{
		"string": "hello",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"nil":    nil,
		"array":  []any{"a", "b"},
		"object": map[string]any{"inner": "val"},
	}
	writeJSON(rec, 200, body)

	var m map[string]any
	json.Unmarshal(rec.Body.Bytes(), &m)
	if m["string"] != "hello" {
		t.Error("string mismatch")
	}
	if m["int"] != float64(42) { // JSON numbers are float64
		t.Error("int mismatch")
	}
	if m["bool"] != true {
		t.Error("bool mismatch")
	}
	if m["nil"] != nil {
		t.Error("nil mismatch")
	}
}

func TestWriteJSON_StatusCodeRange(t *testing.T) {
	codes := []int{200, 201, 400, 401, 403, 404, 500, 502}
	for _, code := range codes {
		rec := httptest.NewRecorder()
		writeJSON(rec, code, map[string]any{"ok": true})
		if rec.Code != code {
			t.Errorf("writeJSON status %d, got %d", code, rec.Code)
		}
	}
}

// ---- sseEvent ----

func TestSSEEvent(t *testing.T) {
	data := `{"test":true}`
	got := sseEvent(data)
	want := "data: " + data + "\n\n"
	if string(got) != want {
		t.Errorf("sseEvent = %q, want %q", string(got), want)
	}
}

// ---- sseDone ----

func TestSSEDone(t *testing.T) {
	got := sseDone()
	want := "data: [DONE]\n\n"
	if string(got) != want {
		t.Errorf("sseDone = %q, want %q", string(got), want)
	}
}

// ---- writeSSEHeaders ----

func TestWriteSSEHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSSEHeaders(rec)

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if conn := rec.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q", conn)
	}
}

// ---- GetProxyAuth ----

func TestGetProxyAuth_Nil(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	got := GetProxyAuth(req)
	if got != nil {
		t.Errorf("GetProxyAuth should return nil for unauthenticated request")
	}
}

// ---- HeaderMapFromRequest ----

func TestHeaderMapFromRequest(t *testing.T) {
	header := http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {"Bearer test"},
		"X-Custom":      {"val1", "val2"},
		"Empty":         {},
	}
	result := HeaderMapFromRequest(header)

	if result["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q", result["Content-Type"])
	}
	if result["Authorization"] != "Bearer test" {
		t.Errorf("Authorization = %q", result["Authorization"])
	}
	if result["X-Custom"] != "val1" {
		t.Errorf("X-Custom = %q (should be first value)", result["X-Custom"])
	}
	if _, ok := result["Empty"]; ok {
		t.Error("Empty header should not be included")
	}
	if _, ok := result["Non-Existent"]; ok {
		t.Error("Non-Existent header should not be in map")
	}
}
