package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/web"
)

// newMonitorWiringRouter builds the full production router against a real
// SQLite database so the monitor routes are registered exactly as in
// cmd/server (they are skipped when the database is absent).
func newMonitorWiringRouter(t *testing.T) (http.Handler, *config.Config) {
	t.Helper()
	dataDir := t.TempDir()
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
		DbType:           store.DialectSQLite,
		DbUrl:            filepath.Join(dataDir, "monitor-wiring.db"),
		DataDir:          dataDir,
	}
	if err := store.EnsureRuntimeDatabase(cfg); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	t.Cleanup(func() {
		_ = store.CloseDatabase()
	})
	return New(cfg, web.Dist), cfg
}

// TestMonitorProxyReachableWithoutBearerHeader is the Wave 4 S-line F1
// regression: /monitor-proxy/* serves the LDOH iframe and authenticates via
// the HttpOnly meta_monitor_auth cookie, so it must not sit behind the Bearer
// AdminAuth middleware. Cookie-only requests have to reach the monitor
// handler (which answers with its own error shapes) instead of dying at the
// admin middleware with "Missing Authorization header".
func TestMonitorProxyReachableWithoutBearerHeader(t *testing.T) {
	r, cfg := newMonitorWiringRouter(t)

	// No credentials at all: the monitor handler's own 401 shape must answer,
	// not the Bearer middleware's.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/monitor-proxy/ldoh/", nil)
	r.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), "Missing Authorization header") {
		t.Fatalf("cookie-only surface answered with the Bearer middleware 401 (body=%s); "+
			"/monitor-proxy/* is still wired inside the AdminAuth group", rec.Body.String())
	}
	if rec.Code != http.StatusUnauthorized ||
		!strings.Contains(rec.Body.String(), "Missing or invalid monitor session") {
		t.Fatalf("status = %d body = %s, want the monitor handler 401 'Missing or invalid monitor session'",
			rec.Code, rec.Body.String())
	}

	// Mint a monitor session through the admin API (Bearer), then use ONLY the
	// resulting cookie — the real iframe flow.
	mintRec := httptest.NewRecorder()
	mintReq := httptest.NewRequest(http.MethodPost, "/api/monitor/session", nil)
	mintReq.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	r.ServeHTTP(mintRec, mintReq)
	if mintRec.Code != http.StatusOK {
		t.Fatalf("mint monitor session status = %d body=%s, want 200", mintRec.Code, mintRec.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range mintRec.Result().Cookies() {
		if cookie.Name == "meta_monitor_auth" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatalf("POST /api/monitor/session did not set meta_monitor_auth; cookies=%v", mintRec.Result().Cookies())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/monitor-proxy/ldoh/", nil)
	req.AddCookie(sessionCookie)
	r.ServeHTTP(rec, req)

	// Session cookie is valid but no LDOH cookie is configured: the handler's
	// plain-text 400 proves the request passed authentication and reached the
	// LDOH proxy logic.
	if rec.Code != http.StatusBadRequest ||
		!strings.Contains(rec.Body.String(), "LDOH cookie not configured") {
		t.Fatalf("cookie-only request status = %d body = %s, want monitor handler 400 'LDOH cookie not configured'",
			rec.Code, rec.Body.String())
	}
}

// TestMonitorAPIRoutesStayBehindBearerAuth pins the other half of the F1
// fix: only /monitor-proxy/* leaves the AdminAuth group; the /api/monitor
// configuration surface stays Bearer-protected.
func TestMonitorAPIRoutesStayBehindBearerAuth(t *testing.T) {
	r, cfg := newMonitorWiringRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/monitor/config", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized ||
		!strings.Contains(rec.Body.String(), "Missing Authorization header") {
		t.Fatalf("/api/monitor/config without Bearer status = %d body=%s, want admin middleware 401",
			rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/monitor/config", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.AuthToken)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/monitor/config with Bearer status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
}
