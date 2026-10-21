package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
	"golang.org/x/crypto/bcrypt"
)

func TestUserAuthPasswordAndOpaqueCredentials(t *testing.T) {
	db := sqlitetest.Open(t)
	users := store.NewGatewayUserStore(db)
	ctx := context.Background()
	g, err := users.CreateUserGroup(ctx, "auth", "Auth", true)
	if err != nil {
		t.Fatal(err)
	}
	u, err := users.CreateUser(ctx, "alice", "Alice", g.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	m := NewUserSessionManager(db)
	code, _, err := m.IssueActivation(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	var storedCode string
	if err := db.Get(&storedCode, `SELECT activation_hash FROM gateway_user_credentials WHERE user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if code == storedCode || len(code) != 64 || len(storedCode) != 64 {
		t.Fatal("raw activation stored")
	}
	for _, password := range []string{"short", strings.Repeat("x", 73), strings.Repeat("界", 25)} {
		if err := m.Activate(ctx, "alice", code, password); !errors.Is(err, ErrUserPassword) {
			t.Fatalf("accepted invalid password: %v", err)
		}
	}
	const password = "synthetic-user-password"
	if err := m.Activate(ctx, "alice", code, password); err != nil {
		t.Fatal(err)
	}
	var storedPassword string
	if err := db.Get(&storedPassword, `SELECT password_hash FROM gateway_user_credentials WHERE user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if cost, err := bcrypt.Cost([]byte(storedPassword)); err != nil || cost != 12 || storedPassword == password {
		t.Fatalf("password hash cost %d %v", cost, err)
	}
	raw, session, err := m.Login(ctx, " Alice ", password)
	if err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := db.Get(&hash, `SELECT token_hash FROM gateway_user_sessions WHERE user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if raw == hash || len(raw) != 64 || hash != hashSessionToken(raw) || session.UserID != u.ID {
		t.Fatal("raw session persisted or wrong identity")
	}
	for _, username := range []string{"alice", "absent"} {
		if _, _, err := m.Login(ctx, username, "wrong"); !errors.Is(err, ErrUserCredentials) {
			t.Fatalf("login %s: %v", username, err)
		}
	}
	if err := m.Revoke(ctx, raw); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM gateway_user_sessions`); err != nil || count != 0 {
		t.Fatal("logout not revoked")
	}
	for i := 0; i < 5; i++ {
		if !m.AllowAccount("account-limited") {
			t.Fatal("early account limit")
		}
	}
	if m.AllowAccount("ACCOUNT-LIMITED") {
		t.Fatal("canonical account limit bypass")
	}
}

func TestUserAuthCookieAndOrigin(t *testing.T) {
	for _, secure := range []bool{false, true} {
		url := "http://example.com"
		if secure {
			url = "https://example.com"
		}
		r := httptest.NewRequest("GET", url+"/api/user/session", nil)
		w := httptest.NewRecorder()
		SetUserSessionCookie(w, r, "synthetic", "auto")
		cookies := w.Result().Cookies()
		if len(cookies) != 1 {
			t.Fatal("cookie missing")
		}
		cookie := cookies[0]
		if cookie.Name == SessionCookieName || cookie.Path != "/api/user" || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure != secure {
			t.Fatal("unsafe cookie")
		}
	}
	for _, tc := range []struct {
		url, origin string
		status      int
	}{{"https://example.com/api/user/auth/login", "https://example.com", 204}, {"https://example.com/api/user/auth/login", "http://example.com", 403}, {"http://example.com/api/user/auth/login", "https://example.com", 403}, {"http://example.com/api/user/auth/login", "http://example.com:80", 403}} {
		r := httptest.NewRequest("POST", tc.url, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		UserSameOriginGuard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })).ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("origin %s on %s => %d", tc.origin, tc.url, w.Code)
		}
	}
}
