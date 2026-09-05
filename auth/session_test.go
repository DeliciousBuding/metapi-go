package auth

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- SessionManager lifecycle (#1034) ----

func newTestSessionManager(t *testing.T, ttl time.Duration) *SessionManager {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return NewSessionManager(db, ttl)
}

func TestSessionCreateValidateRevoke(t *testing.T) {
	sm := newTestSessionManager(t, time.Hour)
	ctx := context.Background()

	raw, sess, err := sm.Create(ctx, "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if raw == "" || sess == nil {
		t.Fatal("Create returned empty credential/session")
	}
	if sess.TokenHash == raw || strings.Contains(raw, sess.TokenHash) {
		t.Fatal("raw credential must never equal/contain its stored hash")
	}
	if sm.Count(ctx) != 1 {
		t.Fatalf("Count = %d, want 1", sm.Count(ctx))
	}

	got := sm.Validate(ctx, raw)
	if got == nil {
		t.Fatal("Validate(live token) = nil, want session")
	}
	if got.TokenHash != sess.TokenHash {
		t.Fatalf("Validate returned %s, want %s", got.TokenHash, sess.TokenHash)
	}

	// A truncated/altered credential must not validate.
	if sm.Validate(ctx, raw[:len(raw)-2]) != nil {
		t.Fatal("Validate accepted a mutated credential")
	}

	sm.Revoke(ctx, raw)
	if sm.Validate(ctx, raw) != nil {
		t.Fatal("Validate after Revoke must be nil")
	}
	if sm.Count(ctx) != 0 {
		t.Fatalf("Count after revoke = %d, want 0", sm.Count(ctx))
	}
	// Revoking an unknown token is a no-op, not an error.
	sm.Revoke(ctx, "deadbeef")
}

func TestSessionExpiryAndLazyGC(t *testing.T) {
	// 1s TTL: validate after expiry must fail AND delete the row (lazy GC).
	sm := newTestSessionManager(t, time.Second)
	ctx := context.Background()

	raw, _, err := sm.Create(ctx, "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if sm.Validate(ctx, raw) != nil {
		t.Fatal("Validate must reject an expired session")
	}
	if sm.Count(ctx) != 0 {
		t.Fatalf("expired row not GC'd: Count = %d", sm.Count(ctx))
	}
}

func TestSessionSlidingExpiry(t *testing.T) {
	sm := newTestSessionManager(t, 10*time.Second)
	ctx := context.Background()

	raw, sess, err := sm.Create(ctx, "", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	first := sess.ExpiresAt
	time.Sleep(1100 * time.Millisecond)
	got := sm.Validate(ctx, raw)
	if got == nil {
		t.Fatal("Validate inside TTL = nil")
	}
	if !got.ExpiresAt.After(first) {
		t.Fatalf("expiry did not slide: was %s, now %s", first, got.ExpiresAt)
	}
}

func TestSessionCreatePurgesExpiredRows(t *testing.T) {
	// TTL 2s, not 1s: expires_at is stored at SECOND precision (RFC3339 without
	// fractional part), so a row's real lifetime can be up to 1s shorter than
	// the TTL depending on where inside the second it was created. With a 1s
	// TTL the window between Create #2 and Count could cross a whole-second
	// boundary and count the brand-new row as already expired (observed as a
	// CI red on a docs-only PR: "Count = 0, want 1"). A 2s TTL leaves a full
	// second of margin, which no scheduler stall between two local SQL
	// statements can eat.
	sm := newTestSessionManager(t, 2*time.Second)
	ctx := context.Background()

	if _, _, err := sm.Create(ctx, "", ""); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	time.Sleep(2100 * time.Millisecond)
	if _, _, err := sm.Create(ctx, "", ""); err != nil {
		t.Fatalf("Create #2: %v", err)
	}
	if n := sm.Count(ctx); n != 1 {
		t.Fatalf("Count = %d, want 1 (expired row purged on login)", n)
	}
}

func TestSessionRevokeAllOnRotation(t *testing.T) {
	sm := newTestSessionManager(t, time.Hour)
	ctx := context.Background()

	raws := make([]string, 3)
	for i := range raws {
		raw, _, err := sm.Create(ctx, "", "")
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		raws[i] = raw
	}
	sm.RevokeAll(ctx)
	for i, raw := range raws {
		if sm.Validate(ctx, raw) != nil {
			t.Fatalf("session %d survived RevokeAll", i)
		}
	}
}

func TestSessionManagerNilFailsClosed(t *testing.T) {
	var sm *SessionManager
	if sm.TTL() != 0 {
		t.Fatal("nil manager TTL must be 0")
	}
	if _, _, err := sm.Create(context.Background(), "", ""); err == nil {
		t.Fatal("nil manager Create must error")
	}
	if sm.Validate(context.Background(), "anything") != nil {
		t.Fatal("nil manager Validate must be nil")
	}
	sm.Revoke(context.Background(), "anything") // must not panic
	sm.RevokeAll(context.Background())          // must not panic
	if ticket, _ := sm.IssueWSTicket(); ticket != "" {
		t.Fatal("nil manager must not mint tickets")
	}
	if sm.ConsumeWSTicket("x") {
		t.Fatal("nil manager must not consume tickets")
	}
}

// ---- One-time WebSocket tickets ----

func TestWSTicketSingleUseAndExpiry(t *testing.T) {
	sm := newTestSessionManager(t, time.Hour)

	ticket, ttl := sm.IssueWSTicket()
	if ticket == "" || ttl <= 0 {
		t.Fatalf("IssueWSTicket = %q, %v", ticket, ttl)
	}
	if !sm.ConsumeWSTicket(ticket) {
		t.Fatal("first consume of a fresh ticket must succeed")
	}
	if sm.ConsumeWSTicket(ticket) {
		t.Fatal("second consume must fail (single-use)")
	}
	if sm.ConsumeWSTicket("never-issued") {
		t.Fatal("unknown ticket must fail")
	}
	if sm.ConsumeWSTicket("") {
		t.Fatal("empty ticket must fail")
	}
}

// ---- Cookie plumbing ----

func TestSessionCookieSecureModes(t *testing.T) {
	cases := []struct {
		name   string
		mode   string
		tls    bool
		xfp    string
		secure bool
	}{
		{"auto plain http", "auto", false, "", false},
		{"auto https via X-Forwarded-Proto", "auto", false, "https", true},
		{"auto tls", "auto", true, "", true},
		{"forced true", "true", false, "", true},
		{"forced false even on tls", "false", true, "https", false},
		{"garbage falls back to auto", "banana", false, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tc.xfp != "" {
				req.Header.Set("X-Forwarded-Proto", tc.xfp)
			}
			if got := cookieSecure(req, tc.mode); got != tc.secure {
				t.Fatalf("cookieSecure(%q) = %v, want %v", tc.mode, got, tc.secure)
			}
		})
	}
}

func TestSetAndClearSessionCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	SetSessionCookie(rec, req, "raw-credential", 12*time.Hour, "auto")
	setCookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range setCookies {
		if c.Name == SessionCookieName {
			found = c
		}
	}
	if found == nil {
		t.Fatal("SetSessionCookie did not emit the session cookie")
	}
	if found.Value != "raw-credential" || !found.HttpOnly || found.SameSite != http.SameSiteStrictMode || found.Path != "/" {
		t.Fatalf("cookie attrs wrong: %+v", found)
	}
	if found.MaxAge != int((12 * time.Hour).Seconds()) {
		t.Fatalf("MaxAge = %d, want %d", found.MaxAge, int((12*time.Hour).Seconds()))
	}

	// Round-trip: the emitted header must be parseable back by the request.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Cookie", found.Name+"="+found.Value)
	if got := SessionCookieFromRequest(req2); got != "raw-credential" {
		t.Fatalf("SessionCookieFromRequest = %q", got)
	}
	if got := SessionCookieFromRequest(httptest.NewRequest(http.MethodGet, "/", nil)); got != "" {
		t.Fatalf("missing cookie must read empty, got %q", got)
	}

	recClear := httptest.NewRecorder()
	ClearSessionCookie(recClear, req, "auto")
	var cleared *http.Cookie
	for _, c := range recClear.Result().Cookies() {
		if c.Name == SessionCookieName {
			cleared = c
		}
	}
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("ClearSessionCookie must expire the cookie, got %+v", cleared)
	}
}

func TestConstantTimeTokenEqual(t *testing.T) {
	if !constantTimeTokenEqual("same-token", "same-token") {
		t.Fatal("equal tokens must compare equal")
	}
	if constantTimeTokenEqual("token-a", "token-b") {
		t.Fatal("different tokens must not compare equal")
	}
	if constantTimeTokenEqual("short", "much-longer-token") {
		t.Fatal("length mismatch must not compare equal")
	}
	if constantTimeTokenEqual("", "x") {
		t.Fatal("empty vs non-empty must not compare equal")
	}
}

func TestNullStringTruncation(t *testing.T) {
	long := strings.Repeat("U", 4096)
	got, ok := nullString(long).(string)
	if !ok || len(got) != 512 {
		t.Fatalf("nullString(long) len = %d, want 512", len(got))
	}
	if nullString("   ") != nil {
		t.Fatal("whitespace-only must map to nil")
	}
}
