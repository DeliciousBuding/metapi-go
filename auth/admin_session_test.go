package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- AdminAuth dual-track middleware (#1034) ----

// publishRuntimeAuthToken publishes the admin bearer token through the
// global atomic RuntimeSettings snapshot that AdminAuth / RequireReauth
// read, and clears it again when the test ends.
func publishRuntimeAuthToken(t *testing.T, token string) {
	t.Helper()
	config.SetRuntime(&config.RuntimeSettings{AuthToken: token})
	t.Cleanup(func() { config.SetRuntime(nil) })
}

// newDualTrackHarness builds AdminAuth with a real in-memory session store so
// the cookie track runs production code paths.
func newDualTrackHarness(t *testing.T) (http.Handler, *SessionManager) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	sm := NewSessionManager(db, time.Hour)
	publishRuntimeAuthToken(t, "master-token")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			t.Error("handler reached without admin context marker")
		}
		w.WriteHeader(http.StatusOK)
	})
	return AdminAuth(sm)(inner), sm
}

func doAuthReq(t *testing.T, h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAdminAuth_SessionCookieTrack(t *testing.T) {
	h, sm := newDualTrackHarness(t)

	raw, _, err := sm.Create(t.Context(), "127.0.0.1", "ua")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: raw})
	if rec := doAuthReq(t, h, req); rec.Code != http.StatusOK {
		t.Fatalf("valid session cookie status = %d body=%s", rec.Code, rec.Body.String())
	}

	// Unknown cookie value → 401 Session expired (no Bearer presented).
	req = httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "forged-cookie-value"})
	if rec := doAuthReq(t, h, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("forged cookie status = %d, want 401", rec.Code)
	} else if !strings.Contains(rec.Body.String(), ErrorCodeAuthSessionExpired) {
		t.Fatalf("forged cookie body missing errorCode %q: %s", ErrorCodeAuthSessionExpired, rec.Body.String())
	}

	// Expired cookie → 401 and the row is gone.
	sm2 := sm
	shortRaw, _, err := sm2.Create(t.Context(), "", "")
	if err != nil {
		t.Fatalf("Create short: %v", err)
	}
	// Force-expire by revoking: semantics under test are identical (row gone).
	sm2.Revoke(t.Context(), shortRaw)
	req = httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: shortRaw})
	if rec := doAuthReq(t, h, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked cookie status = %d, want 401", rec.Code)
	}
}

func TestAdminAuth_BearerTrackStillWorks(t *testing.T) {
	h, _ := newDualTrackHarness(t)

	req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.Header.Set("Authorization", "Bearer master-token")
	if rec := doAuthReq(t, h, req); rec.Code != http.StatusOK {
		t.Fatalf("bearer master token status = %d, want 200", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	if rec := doAuthReq(t, h, req); rec.Code != http.StatusForbidden {
		t.Fatalf("wrong bearer status = %d, want 403", rec.Code)
	}
}

func TestAdminAuth_NoCredentials(t *testing.T) {
	h, _ := newDualTrackHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	rec := doAuthReq(t, h, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-credential status = %d, want 401", rec.Code)
	}
}

func TestAdminAuth_SessionLifecycleSurfaceIsPublic(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	sm := NewSessionManager(db, time.Hour)
	publishRuntimeAuthToken(t, "master-token")
	// Public routes bypass auth entirely, so the inner handler must not
	// require the admin context marker here.
	plain := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := AdminAuth(sm)(plain)

	for _, path := range []string{"/api/auth/login", "/api/auth/logout", "/api/auth/session"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if rec := doAuthReq(t, h, req); rec.Code != http.StatusOK {
			t.Fatalf("%s bypass status = %d, want 200 (public)", path, rec.Code)
		}
	}
	// ws-ticket is NOT public.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/ws-ticket", nil)
	if rec := doAuthReq(t, h, req); rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/auth/ws-ticket without auth status = %d, want 401", rec.Code)
	}
}

func TestAdminAuth_SessionActorInContext(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	sm := NewSessionManager(db, time.Hour)
	publishRuntimeAuthToken(t, "master-token")

	raw, sess, err := sm.Create(t.Context(), "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var actorID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actorID = GetAdminSessionID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := AdminAuth(sm)(inner)

	req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: raw})
	if rec := doAuthReq(t, h, req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if actorID != sess.TokenHash[:8] {
		t.Fatalf("actor = %q, want session hash prefix %q", actorID, sess.TokenHash[:8])
	}

	// Bearer track carries no session actor.
	actorID = "sentinel"
	req = httptest.NewRequest(http.MethodGet, "/api/anything", nil)
	req.Header.Set("Authorization", "Bearer master-token")
	doAuthReq(t, h, req)
	if actorID != "" {
		t.Fatalf("bearer-track actor = %q, want empty", actorID)
	}
}

// ---- Rate limiting applies to failed auth (#1034) ----

// TestRateLimitConstrainsFailedAuth composes the exact middleware order the
// router now uses (limiter BEFORE auth) and proves credential brute force
// hits 429 instead of unlimited 401/403s.
func TestRateLimitConstrainsFailedAuth(t *testing.T) {
	publishRuntimeAuthToken(t, "master-token")
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// burst=2, rps=0-refill would never recover; use rps=1/burst=2.
	h := AdminRateLimit(1, 2)(AdminAuth(nil)(inner))

	sawTooMany := false
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			sawTooMany = true
			break
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("attempt %d status = %d, want 403 before the bucket drains", i, rec.Code)
		}
	}
	if !sawTooMany {
		t.Fatal("failed-auth flood never hit 429 — limiter is not in front of auth")
	}
}

// TestAuthRateLimitOnlyTouchesAuthPaths verifies the strict /api/auth/*
// bucket leaves other paths untouched.
func TestAuthRateLimitOnlyTouchesAuthPaths(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := AuthRateLimit(1, 1)(inner)

	// /api/auth/login drains immediately (burst 1).
	var last int
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("/api/auth/login flood last status = %d, want 429", last)
	}

	// Other paths pass through even after the auth bucket drained.
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/sites", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("non-auth path status = %d, want 200", rec.Code)
		}
	}
}
