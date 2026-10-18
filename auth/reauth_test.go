package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// ---- RequireReauth gate (#1034) ----

func TestSensitiveAdminRoutePatterns(t *testing.T) {
	sensitive := []struct {
		method, pattern string
	}{
		{http.MethodGet, "/api/settings/backup/export"},
		{http.MethodGet, "/api/settings/backup/webdav/export"},
		{http.MethodPost, "/api/settings/backup/import"},
		{http.MethodPost, "/api/settings/backup/webdav/import"},
		{http.MethodPut, "/api/settings/backup/webdav"},
		{http.MethodPost, "/api/settings/database/migrate"},
		{http.MethodPost, "/api/settings/database/test-connection"},
		{http.MethodPost, "/api/settings/system-proxy/test"},
		{http.MethodPost, "/api/settings/maintenance/factory-reset"},
		{http.MethodPost, "/api/settings/maintenance/clear-usage"},
		{http.MethodPost, "/api/settings/auth/change"},
		{http.MethodGet, "/api/downstream-keys/{id}/export"},
		{http.MethodPost, "/api/downstream-keys"},
		{http.MethodPost, "/api/downstream-keys/batch"},
		{http.MethodGet, "/api/account-tokens/{id}/value"},
	}
	for _, tc := range sensitive {
		if !sensitiveAdminOperation(tc.method, tc.pattern) {
			t.Errorf("%s %s must be sensitive", tc.method, tc.pattern)
		}
	}

	notSensitive := []struct {
		method, pattern string
	}{
		{http.MethodGet, "/api/settings/backup/import/preview"},
		{http.MethodGet, "/api/settings/backup/webdav"},
		{http.MethodGet, "/api/settings/auth/info"},
		{http.MethodGet, "/api/downstream-keys"},
		{http.MethodGet, "/api/downstream-keys/{id}"},
		{http.MethodPut, "/api/downstream-keys/{id}"}, // handler gates proxyUrl/key rotation only
		{http.MethodDelete, "/api/downstream-keys/{id}"},
		{http.MethodGet, "/api/downstream-keys/{id}/overview"},
		{http.MethodGet, "/api/sites"},
		{http.MethodGet, "/api/account-tokens"},
	}
	for _, tc := range notSensitive {
		if sensitiveAdminOperation(tc.method, tc.pattern) {
			t.Errorf("%s %s must NOT be sensitive", tc.method, tc.pattern)
		}
	}
}

// The middleware must key on the chi route pattern, not the raw request path:
// an encoded id ("+1", "%209") must not slip past the gate while the handler
// still resolves it to key 9.
func TestRequireReauthUsesRoutePattern(t *testing.T) {
	publishRuntimeAuthToken(t, "master-token")
	r := chi.NewRouter()
	r.Use(RequireReauth())
	r.Get("/api/downstream-keys/{id}/export", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	do := func(path, confirm string) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if confirm != "" {
			req.Header.Set(ReauthConfirmHeader, confirm)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	for _, path := range []string{
		"/api/downstream-keys/9/export",
		"/api/downstream-keys/+9/export",
		"/api/downstream-keys/%209%20/export",
		"/api/downstream-keys/%C2%A09/export",
		"/api/downstream-keys/%E3%80%809/export",
	} {
		if code := do(path, ""); code != http.StatusForbidden {
			t.Errorf("%s without confirmation = %d, want 403", path, code)
		}
	}
	if code := do("/api/downstream-keys/+9/export", "master-token"); code != http.StatusOK {
		t.Errorf("confirmed request = %d, want 200", code)
	}
}

func TestRequireReauthSetsNoStore(t *testing.T) {
	publishRuntimeAuthToken(t, "master-token")
	r := chi.NewRouter()
	r.Use(RequireReauth())
	r.Get("/api/settings/backup/export", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/settings/backup/export", nil)
	req.Header.Set(ReauthConfirmHeader, "master-token")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Cache-Control"); got == "" || !contains(got, "no-store") {
		t.Fatalf("sensitive response Cache-Control = %q, want no-store", got)
	}
}

func TestSameOriginGuard(t *testing.T) {
	h := SameOriginGuard([]string{"https://admin.example.com"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	do := func(method, origin, fetchSite, host string) int {
		req := httptest.NewRequest(method, "http://gateway.example.com/api/sites", nil)
		req.Host = host
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if fetchSite != "" {
			req.Header.Set("Sec-Fetch-Site", fetchSite)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do(http.MethodPost, "https://evil.example", "", "gateway.example.com"); code != http.StatusForbidden {
		t.Errorf("cross-site origin = %d, want 403", code)
	}
	if code := do(http.MethodPost, "http://gateway.example.com", "", "gateway.example.com"); code != http.StatusOK {
		t.Errorf("same-origin request = %d, want 200", code)
	}
	if code := do(http.MethodPost, "https://admin.example.com", "", "gateway.example.com"); code != http.StatusOK {
		t.Errorf("explicitly allowed origin = %d, want 200", code)
	}
	if code := do(http.MethodPost, "", "same-site", "gateway.example.com"); code != http.StatusForbidden {
		t.Errorf("same-site fetch metadata = %d, want 403", code)
	}
	if code := do(http.MethodPost, "", "same-origin", "gateway.example.com"); code != http.StatusOK {
		t.Errorf("same-origin fetch metadata = %d, want 200", code)
	}
	if code := do(http.MethodPost, "", "", "gateway.example.com"); code != http.StatusOK {
		t.Errorf("non-browser client without headers = %d, want 200", code)
	}
	if code := do(http.MethodGet, "https://evil.example", "", "gateway.example.com"); code != http.StatusOK {
		t.Errorf("GET must not be gated = %d, want 200", code)
	}
}

func TestSensitiveAdminPathMatching(t *testing.T) {
	sensitive := []struct{ method, path string }{
		{http.MethodGet, "/api/settings/backup/export"},
		{http.MethodGet, "/api/settings/backup/webdav/export"},
		{http.MethodPost, "/api/settings/auth/change"},
		{http.MethodGet, "/api/downstream-keys/1/export"},
		{http.MethodGet, "/api/downstream-keys/999/export"},
		{http.MethodGet, "/api/downstream-keys/+1/export"},
		{http.MethodGet, "/api/downstream-keys/%209%20/export"},
		{http.MethodGet, "/api/downstream-keys/%C2%A01/export"},
		{http.MethodGet, "/api/downstream-keys/%E3%80%801/export"},
		{http.MethodGet, "/api/downstream-keys/..%2f..%2fadmin/export"},
		{http.MethodGet, "/api/downstream-keys/x/export"},
	}
	for _, tc := range sensitive {
		if !sensitiveAdminPath(tc.method, tc.path) {
			t.Errorf("%s %s must be sensitive", tc.method, tc.path)
		}
	}

	notSensitive := []struct{ method, path string }{
		{http.MethodGet, "/api/settings/backup/import/preview"},
		{http.MethodGet, "/api/settings/backup/webdav"},
		{http.MethodGet, "/api/settings/auth/info"},
		{http.MethodGet, "/api/downstream-keys"},
		{http.MethodGet, "/api/downstream-keys/1"},
		{http.MethodGet, "/api/downstream-keys/1/overview"},
		{http.MethodGet, "/api/downstream-keys/export"},     // missing id segment
		{http.MethodGet, "/api/downstream-keys/1/2/export"}, // extra segment
		{http.MethodGet, "/api/sites"},
	}
	for _, tc := range notSensitive {
		if sensitiveAdminPath(tc.method, tc.path) {
			t.Errorf("%s %s must NOT be sensitive", tc.method, tc.path)
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
