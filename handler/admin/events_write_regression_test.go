package admin

import (
	"net/http"
	"testing"
)

// Regression guards for the w18-pg-dialect incident class: event writers that
// bound the INTEGER literal 0 to events.read. On SQLite (INTEGER column) the
// row was written anyway, so these tests pass on both sides of the fix
// locally; the PostgreSQL type-error proof lives in
// store/boolean_bind_test.go (TestBooleanLiteralIntegerRejectedByPostgres),
// and CI test-pg exercises the same statements against native BOOLEAN.
// After the fix the writes use FALSE / Go-bool binds, which both dialects
// accept, so these guards pin the observable behavior (event row exists)
// rather than the dialect-specific failure mode.

func TestAuthSettingsChange_SuccessWritesEventRow(t *testing.T) {
	db, r, _ := setupAuthSettingsTest(t)

	resp := doPostJSON(t, r, "/api/settings/auth/change", map[string]any{
		"oldToken": "admin-auth-settings-token",
		"newToken": "rotated-admin-token-events",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.Code, resp.Body.String())
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM events WHERE type = 'token' AND related_type = 'settings'"); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 1 {
		t.Fatalf("token-rotation events = %d, want 1 (event write must survive on both dialects)", n)
	}
}

func TestSettingsRuntimeUpdate_WritesStatusEventRow(t *testing.T) {
	db, r, _ := setupEdgeTest(t)

	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"checkinIntervalHours": 8,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("update runtime: %d %s", resp.Code, resp.Body.String())
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM events WHERE type = 'status' AND title = 'Runtime settings updated'"); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 1 {
		t.Fatalf("runtime-settings events = %d, want 1 (event write must survive on both dialects)", n)
	}
}
