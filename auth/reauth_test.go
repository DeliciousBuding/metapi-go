package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

)

// ---- RequireReauth gate (#1034) ----

func TestSensitiveAdminPathMatching(t *testing.T) {
	sensitive := []string{
		"/api/settings/backup/export",
		"/api/settings/backup/webdav/export",
		"/api/settings/auth/change",
		"/api/downstream-keys/1/export",
		"/api/downstream-keys/999/export",
	}
	for _, p := range sensitive {
		if !sensitiveAdminPath(p) {
			t.Errorf("%s must be sensitive", p)
		}
	}

	notSensitive := []string{
		"/api/settings/backup/import",
		"/api/settings/backup/webdav",
		"/api/settings/auth/info",
		"/api/downstream-keys",
		"/api/downstream-keys/1",
		"/api/downstream-keys/1/overview",
		"/api/downstream-keys/export",        // missing id segment
		"/api/downstream-keys/1/2/export",    // extra segment
		"/api/sites",
		"/api/downstream-keys/../settings/backup/export", // traversal never matches
	}
	for _, p := range notSensitive {
		if sensitiveAdminPath(p) {
			t.Errorf("%s must NOT be sensitive", p)
		}
	}
}

func TestRequireReauthGate(t *testing.T) {
	publishRuntimeAuthToken(t, "master-token")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := RequireReauth()(inner)

	do := func(path, confirm string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if confirm != "" {
			req.Header.Set(ReauthConfirmHeader, confirm)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	// Sensitive path: missing / wrong / correct confirmation.
	if code := do("/api/settings/backup/export", ""); code != http.StatusForbidden {
		t.Fatalf("no confirm status = %d, want 403", code)
	}
	if code := do("/api/settings/backup/export", "wrong-token"); code != http.StatusForbidden {
		t.Fatalf("wrong confirm status = %d, want 403", code)
	}
	if code := do("/api/settings/backup/export", "master-token"); code != http.StatusOK {
		t.Fatalf("correct confirm status = %d, want 200", code)
	}

	// Rejection body must be machine-readable for the UI reauth prompt.
	req := httptest.NewRequest(http.MethodPost, "/api/settings/auth/change", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rotation without confirm status = %d, want 403", rec.Code)
	}
	body := rec.Body.String()
	if !contains(body, `"reauthRequired":true`) {
		t.Fatalf("rejection body missing reauthRequired flag: %s", body)
	}

	// Non-sensitive paths pass through without the header.
	if code := do("/api/sites", ""); code != http.StatusOK {
		t.Fatalf("non-sensitive status = %d, want 200", code)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
