package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// ---- Session lifecycle endpoints (#1034) ----

type sessionTestEnv struct {
	router   *chi.Mux
	sessions *auth.SessionManager
	cfg      *config.Config
}

// setupSessionRoutes mirrors the production middleware order relevant to the
// session model: rate limiting is irrelevant here, so the harness stacks
// AdminAuth -> RequireReauth -> session + auth-settings routes.
func setupSessionRoutes(t *testing.T) *sessionTestEnv {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	cfg := &config.Config{AuthToken: "master-token-abc123"}
	sessions := auth.NewSessionManager(db, time.Hour)

	r := chi.NewRouter()
	r.Use(auth.AdminAuth(cfg, sessions))
	r.Use(auth.RequireReauth(cfg))
	RegisterSessionRoutes(r, cfg, sessions)
	RegisterAuthSettingsRoutes(r, db.DB, cfg, sessions)
	// Protected probe route standing in for the ordinary admin surface.
	r.Get("/api/probe", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	return &sessionTestEnv{router: r, sessions: sessions, cfg: cfg}
}

func (env *sessionTestEnv) doJSON(t *testing.T, method, path string, body any, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	return rec
}

func sessionCookieFrom(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.SessionCookieName {
			return c
		}
	}
	return nil
}

func (env *sessionTestEnv) login(t *testing.T, token string) *httptest.ResponseRecorder {
	return env.doJSON(t, http.MethodPost, "/api/auth/login", map[string]string{"token": token})
}

func TestLoginIssuesSessionCookie(t *testing.T) {
	env := setupSessionRoutes(t)

	rec := env.login(t, "master-token-abc123")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", rec.Code, rec.Body.String())
	}
	cookie := sessionCookieFrom(rec)
	if cookie == nil {
		t.Fatal("login did not set the session cookie")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("cookie attrs wrong: %+v", cookie)
	}
	if strings.TrimSpace(cookie.Value) == "" {
		t.Fatal("cookie value is empty")
	}

	var resp struct {
		Authenticated bool   `json:"authenticated"`
		ExpiresAt     string `json:"expiresAt"`
		TTLMinutes    int    `json:"ttlMinutes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal login response: %v (%s)", err, rec.Body.String())
	}
	if !resp.Authenticated || resp.ExpiresAt == "" {
		t.Fatalf("login response = %+v", resp)
	}
	if env.sessions.Count(t.Context()) != 1 {
		t.Fatalf("session rows = %d, want 1", env.sessions.Count(t.Context()))
	}

	// The cookie authenticates ordinary admin routes.
	rec = env.doJSON(t, http.MethodGet, "/api/probe", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("probe with session cookie status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLoginRejectsBadToken(t *testing.T) {
	env := setupSessionRoutes(t)

	if rec := env.login(t, "wrong-token"); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong token status = %d, want 403", rec.Code)
	}
	if rec := env.login(t, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty token status = %d, want 400", rec.Code)
	}
	if env.sessions.Count(t.Context()) != 0 {
		t.Fatal("failed logins must not create sessions")
	}
}

func TestSessionStatusProbe(t *testing.T) {
	env := setupSessionRoutes(t)

	// Anonymous: 200 + authenticated=false (bootstrap-friendly).
	rec := env.doJSON(t, http.MethodGet, "/api/auth/session", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status probe code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"authenticated":false`) {
		t.Fatalf("anonymous probe body = %s", rec.Body.String())
	}

	// Session track.
	cookie := sessionCookieFrom(env.login(t, "master-token-abc123"))
	rec = env.doJSON(t, http.MethodGet, "/api/auth/session", nil, cookie)
	if !strings.Contains(rec.Body.String(), `"source":"session"`) {
		t.Fatalf("session probe body = %s", rec.Body.String())
	}

	// Bearer track.
	req := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	req.Header.Set("Authorization", "Bearer master-token-abc123")
	recB := httptest.NewRecorder()
	env.router.ServeHTTP(recB, req)
	if !strings.Contains(recB.Body.String(), `"source":"token"`) {
		t.Fatalf("bearer probe body = %s", recB.Body.String())
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	env := setupSessionRoutes(t)

	cookie := sessionCookieFrom(env.login(t, "master-token-abc123"))
	if env.sessions.Count(t.Context()) != 1 {
		t.Fatal("expected one live session after login")
	}

	rec := env.doJSON(t, http.MethodPost, "/api/auth/logout", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("logout status = %d", rec.Code)
	}
	if env.sessions.Count(t.Context()) != 0 {
		t.Fatal("logout must delete the session row")
	}

	// The old cookie is dead on protected routes.
	rec = env.doJSON(t, http.MethodGet, "/api/probe", nil, cookie)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("probe after logout status = %d, want 401", rec.Code)
	}

	// Logout is idempotent without a cookie.
	rec = env.doJSON(t, http.MethodPost, "/api/auth/logout", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("bare logout status = %d, want 200", rec.Code)
	}
}

func TestWSTicketRequiresAuthAndIsSingleUse(t *testing.T) {
	env := setupSessionRoutes(t)

	// Unauthenticated mint is rejected.
	rec := env.doJSON(t, http.MethodPost, "/api/auth/ws-ticket", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated ws-ticket status = %d, want 401", rec.Code)
	}

	// Cookie-authenticated mint works.
	cookie := sessionCookieFrom(env.login(t, "master-token-abc123"))
	rec = env.doJSON(t, http.MethodPost, "/api/auth/ws-ticket", nil, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("ws-ticket status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Ticket           string `json:"ticket"`
		ExpiresInSeconds int    `json:"expiresInSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal ws-ticket: %v", err)
	}
	if resp.Ticket == "" || resp.ExpiresInSeconds <= 0 {
		t.Fatalf("ws-ticket response = %+v", resp)
	}
	if !env.sessions.ConsumeWSTicket(resp.Ticket) {
		t.Fatal("minted ticket must redeem exactly once")
	}
	if env.sessions.ConsumeWSTicket(resp.Ticket) {
		t.Fatal("ticket replay must fail")
	}
}

func TestTokenRotationRevokesAllSessionsAndRequiresReauth(t *testing.T) {
	env := setupSessionRoutes(t)

	cookie := sessionCookieFrom(env.login(t, "master-token-abc123"))

	// Rotation without the re-confirmation header is rejected.
	rec := env.doJSON(t, http.MethodPost, "/api/settings/auth/change", map[string]string{
		"oldToken": "master-token-abc123",
		"newToken": "rotated-token-xyz789",
	}, cookie)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("rotation without confirm status = %d, want 403", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"reauthRequired":true`) {
		t.Fatalf("rotation rejection body = %s", rec.Body.String())
	}

	// With the master token re-presented, rotation succeeds.
	req := httptest.NewRequest(http.MethodPost, "/api/settings/auth/change",
		strings.NewReader(`{"oldToken":"master-token-abc123","newToken":"rotated-token-xyz789"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	req.Header.Set(auth.ReauthConfirmHeader, "master-token-abc123")
	recB := httptest.NewRecorder()
	env.router.ServeHTTP(recB, req)
	if recB.Code != http.StatusOK {
		t.Fatalf("rotation status = %d body=%s", recB.Code, recB.Body.String())
	}

	// The pre-rotation session is dead even though its TTL had not passed.
	rec = env.doJSON(t, http.MethodGet, "/api/probe", nil, cookie)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("probe with pre-rotation cookie status = %d, want 401", rec.Code)
	}

	// The new master token authenticates the Bearer track.
	req = httptest.NewRequest(http.MethodGet, "/api/probe", nil)
	req.Header.Set("Authorization", "Bearer rotated-token-xyz789")
	recB = httptest.NewRecorder()
	env.router.ServeHTTP(recB, req)
	if recB.Code != http.StatusOK {
		t.Fatalf("probe with rotated bearer status = %d", recB.Code)
	}

	// Login now requires the new token.
	if rec := env.login(t, "master-token-abc123"); rec.Code != http.StatusForbidden {
		t.Fatalf("login with old token status = %d, want 403", rec.Code)
	}
	if rec := env.login(t, "rotated-token-xyz789"); rec.Code != http.StatusOK {
		t.Fatalf("login with new token status = %d, want 200", rec.Code)
	}
}
