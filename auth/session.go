// metapi-go/auth — server-side admin sessions (#1034 session model).
//
// The admin UI no longer persists the master token (config.AuthToken) in the
// browser. Login exchanges the master token for a random session credential
// carried in an HttpOnly, SameSite=Strict cookie; only the SHA-256 hash of
// the credential is persisted (admin_sessions, migration sc2_026). Sessions
// slide on activity and die on logout/revocation/expiry.
//
// The master token itself stays valid as a Bearer API credential (dual-track
// in AdminAuth) so external scripts/automation keep working; the UI simply
// never stores it client-side anymore.
//
// GC policy (kept deliberately simple): expired rows are purged lazily on
// validate and opportunistically on every new login. Session rows are tiny
// and login is rate-limited, so no scheduler job is warranted.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// SessionCookieName is the admin session cookie. HttpOnly + SameSite=Strict
// are always set; Secure follows cookieSecure().
const SessionCookieName = "metapi_session"

// wsTicketTTL bounds how long a one-time WebSocket ticket stays redeemable.
const wsTicketTTL = 60 * time.Second

// sessionTimeFormat is the fixed-precision RFC3339-UTC layout stored in
// admin_sessions. Fixed precision keeps lexicographic order == chronological
// order in both SQLite and PostgreSQL (the expiry sweep relies on it).
const sessionTimeFormat = "2006-01-02T15:04:05Z"

// Session is one admin_sessions row (timestamps parsed to time.Time).
type Session struct {
	// TokenHash is the SHA-256 hex of the session credential; the raw
	// credential never exists server-side after issuance. Doubles as the
	// stable, non-reversible audit actor id.
	TokenHash  string
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time
	ClientIP   string
	UserAgent  string
}

// SessionManager owns server-side admin session lifecycle (create / validate
// / revoke) plus one-time WebSocket tickets. A nil *SessionManager is valid
// and fails closed (no sessions, no tickets) so tests and DB-less boots keep
// working without special casing.
type SessionManager struct {
	db  *store.DB
	ttl time.Duration

	mu      sync.Mutex
	tickets map[string]time.Time // ticket -> expiry (wall clock)
}

// NewSessionManager builds a session manager over the store. db == nil
// returns nil (callers must treat a nil manager as "sessions unavailable").
// ttl <= 0 falls back to 12h so a misconfig can never mint eternal sessions.
func NewSessionManager(db *store.DB, ttl time.Duration) *SessionManager {
	if db == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &SessionManager{
		db:      db,
		ttl:     ttl,
		tickets: make(map[string]time.Time),
	}
}

// TTL returns the sliding session lifetime.
func (sm *SessionManager) TTL() time.Duration {
	if sm == nil {
		return 0
	}
	return sm.ttl
}

func nowSessionTime() time.Time {
	return time.Now().UTC().Truncate(time.Second)
}

func formatSessionTime(t time.Time) string {
	return t.UTC().Format(sessionTimeFormat)
}

func parseSessionTime(s string) time.Time {
	t, err := time.Parse(sessionTimeFormat, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func hashSessionToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func randomHex(nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: session randomness: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// Create issues a new session and returns the raw cookie credential (shown to
// the client exactly once, inside Set-Cookie). Expired rows are purged on the
// same statement budget (lazy GC, see package doc).
func (sm *SessionManager) Create(ctx context.Context, clientIP, userAgent string) (string, *Session, error) {
	if sm == nil {
		return "", nil, errors.New("auth: session store unavailable")
	}
	raw, err := randomHex(32)
	if err != nil {
		return "", nil, err
	}
	now := nowSessionTime()
	sess := &Session{
		TokenHash:  hashSessionToken(raw),
		CreatedAt:  now,
		LastSeenAt: now,
		ExpiresAt:  now.Add(sm.ttl),
		ClientIP:   clientIP,
		UserAgent:  userAgent,
	}
	// GC first so a login burst cannot pile stale rows behind the new one.
	if _, err := sm.db.ExecContext(ctx,
		`DELETE FROM admin_sessions WHERE expires_at <= ?`,
		formatSessionTime(now)); err != nil {
		return "", nil, fmt.Errorf("auth: session gc: %w", err)
	}
	if _, err := sm.db.ExecContext(ctx,
		`INSERT INTO admin_sessions (token_hash, created_at, last_seen_at, expires_at, client_ip, user_agent) VALUES (?, ?, ?, ?, ?, ?)`,
		sess.TokenHash,
		formatSessionTime(sess.CreatedAt),
		formatSessionTime(sess.LastSeenAt),
		formatSessionTime(sess.ExpiresAt),
		nullString(sess.ClientIP),
		nullString(sess.UserAgent),
	); err != nil {
		return "", nil, fmt.Errorf("auth: session create: %w", err)
	}
	return raw, sess, nil
}

// Validate resolves a raw cookie credential to a live session. Expired or
// unknown credentials return nil (the caller answers 401). A live session is
// slid forward (last_seen_at + expires_at) on every validation — admin
// traffic is rate-limited, so the per-request UPDATE is cheap enough and
// keeps the semantics trivial.
func (sm *SessionManager) Validate(ctx context.Context, rawToken string) *Session {
	if sm == nil || strings.TrimSpace(rawToken) == "" {
		return nil
	}
	hash := hashSessionToken(rawToken)
	var createdAt, lastSeenAt, expiresAt, clientIP, userAgent string
	err := sm.db.QueryRowxContext(ctx,
		`SELECT created_at, last_seen_at, expires_at, COALESCE(client_ip, ''), COALESCE(user_agent, '') FROM admin_sessions WHERE token_hash = ?`,
		hash).Scan(&createdAt, &lastSeenAt, &expiresAt, &clientIP, &userAgent)
	if err != nil {
		return nil
	}
	now := nowSessionTime()
	expires := parseSessionTime(expiresAt)
	if expires.IsZero() || !now.Before(expires) {
		// Lazy GC: the row is dead, drop it.
		_, _ = sm.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash = ?`, hash)
		return nil
	}
	slid := now.Add(sm.ttl)
	if _, err := sm.db.ExecContext(ctx,
		`UPDATE admin_sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`,
		formatSessionTime(now), formatSessionTime(slid), hash); err != nil {
		// A failed slide is not fatal: the row still validates until the
		// previously stored expiry. Log-free on purpose (hot path).
		slid = expires
	}
	return &Session{
		TokenHash:  hash,
		CreatedAt:  parseSessionTime(createdAt),
		LastSeenAt: now,
		ExpiresAt:  slid,
		ClientIP:   clientIP,
		UserAgent:  userAgent,
	}
}

// Revoke deletes the session behind a raw cookie credential (logout). Unknown
// tokens are a no-op so logout stays idempotent.
func (sm *SessionManager) Revoke(ctx context.Context, rawToken string) {
	if sm == nil || strings.TrimSpace(rawToken) == "" {
		return
	}
	_, _ = sm.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash = ?`, hashSessionToken(rawToken))
}

// RevokeAll invalidates every session (master token rotation). Sessions minted
// from the old token must not survive the rotation.
func (sm *SessionManager) RevokeAll(ctx context.Context) {
	if sm == nil {
		return
	}
	_, _ = sm.db.ExecContext(ctx, `DELETE FROM admin_sessions`)
}

// Count returns the number of live (unexpired) sessions; used by tests and
// future diagnostics surfaces.
func (sm *SessionManager) Count(ctx context.Context) int {
	if sm == nil {
		return 0
	}
	var n int
	if err := sm.db.QueryRowxContext(ctx,
		`SELECT COUNT(*) FROM admin_sessions WHERE expires_at > ?`,
		formatSessionTime(nowSessionTime())).Scan(&n); err != nil {
		return 0
	}
	return n
}

// ---------------------------------------------------------------------------
// One-time WebSocket tickets.
//
// Browser WebSockets cannot set headers, and the master token must not ride
// the URL (server logs, proxy logs, browser history). After login the SPA
// POSTs /api/auth/ws-ticket (cookie-authenticated) and dials the WS with the
// returned ticket. Tickets are single-use, 60s-lived, and in-memory: they
// are meaningless after a restart, which is exactly the desired blast radius.
// ---------------------------------------------------------------------------

// IssueWSTicket mints a one-time ticket for the realtime ops WebSocket.
func (sm *SessionManager) IssueWSTicket() (string, time.Duration) {
	if sm == nil {
		return "", 0
	}
	raw, err := randomHex(16)
	if err != nil {
		return "", 0
	}
	now := time.Now()
	sm.mu.Lock()
	defer sm.mu.Unlock()
	// Lazy sweep keeps the map bounded without a background goroutine.
	for ticket, expiry := range sm.tickets {
		if now.After(expiry) {
			delete(sm.tickets, ticket)
		}
	}
	sm.tickets[raw] = now.Add(wsTicketTTL)
	return raw, wsTicketTTL
}

// ConsumeWSTicket redeems a ticket exactly once. Unknown, expired, or
// already-consumed tickets return false.
func (sm *SessionManager) ConsumeWSTicket(ticket string) bool {
	if sm == nil || strings.TrimSpace(ticket) == "" {
		return false
	}
	now := time.Now()
	sm.mu.Lock()
	defer sm.mu.Unlock()
	expiry, ok := sm.tickets[ticket]
	if !ok {
		return false
	}
	// Always consumed: replaying an expired ticket must not get a second
	// chance if a re-issue collides on the same random value (astronomically
	// unlikely, but the delete is free).
	delete(sm.tickets, ticket)
	return now.Before(expiry) || now.Equal(expiry)
}

// ---------------------------------------------------------------------------
// Cookie plumbing.
// ---------------------------------------------------------------------------

// cookieSecure resolves the Secure flag for the request. mode: "true" always,
// "false" never, anything else (default "auto") follows the request protocol
// so local plain-HTTP dev keeps working while HTTPS stays protected.
func cookieSecure(r *http.Request, mode string) bool {
	switch mode {
	case "true":
		return true
	case "false":
		return false
	default: // "auto"
		if r.TLS != nil {
			return true
		}
		return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
	}
}

// SetSessionCookie writes the session cookie. Max-Age mirrors the sliding
// TTL; the browser drops the cookie around the same time the server expires
// the row (the server remains authoritative either way).
func SetSessionCookie(w http.ResponseWriter, r *http.Request, rawToken string, ttl time.Duration, secureMode string) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cookieSecure(r, secureMode),
	}
	http.SetCookie(w, cookie)
}

// ClearSessionCookie expires the session cookie. Same Path/SameSite/Secure
// attributes as issuance or the browser ignores the deletion.
func ClearSessionCookie(w http.ResponseWriter, r *http.Request, secureMode string) {
	cookie := &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   cookieSecure(r, secureMode),
	}
	http.SetCookie(w, cookie)
}

// SessionCookieFromRequest extracts the raw session credential from the
// request cookie, or "" when absent.
func SessionCookieFromRequest(r *http.Request) string {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

// nullString trims oversize text fields before INSERT so a hostile 10 MB
// User-Agent cannot bloat admin_sessions. Empty strings stay empty (column
// is nullable; COALESCE on read normalizes).
func nullString(s string) any {
	const maxLen = 512
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	if s == "" {
		return nil
	}
	return s
}

// ---------------------------------------------------------------------------
// Request-context identity for audit attribution.
// ---------------------------------------------------------------------------

type adminSessionIDKeyType struct{}

var adminSessionIDKey = adminSessionIDKeyType{}

// WithAdminSessionID records the session hash prefix on the request context
// so the audit middleware can attribute cookie-authenticated writes.
func WithAdminSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, adminSessionIDKey, sessionID)
}

// GetAdminSessionID returns the session actor id ("" when the request used
// the Bearer track or no auth at all).
func GetAdminSessionID(ctx context.Context) string {
	v, _ := ctx.Value(adminSessionIDKey).(string)
	return v
}

// constantTimeTokenEqual compares two tokens without leaking length or
// prefix timing: both sides are hashed first, then compared in constant time.
func constantTimeTokenEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}
