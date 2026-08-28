package admin

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterAuthSettingsRoutes registers all /api/settings/auth routes.
// sessions may be nil (tests): rotation then skips session revocation.
func RegisterAuthSettingsRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config, sessions *auth.SessionManager) {
	handler := &authSettingsHandler{db: db, cfg: cfg, sessions: sessions}

	r.Get("/api/settings/auth/info", handler.getInfo)
	r.Post("/api/settings/auth/change", handler.changeToken)
}

type authSettingsHandler struct {
	db       *sqlx.DB
	cfg      *config.Config
	sessions *auth.SessionManager
}

// GET /api/settings/auth/info
func (h *authSettingsHandler) getInfo(w http.ResponseWriter, r *http.Request) {
	token := h.cfg.AuthToken
	var masked string
	if len(token) > 8 {
		masked = token[:4] + "****" + token[len(token)-4:]
	} else {
		masked = "****"
	}
	writeJSON(w, http.StatusOK, map[string]string{"masked": masked})
}

// POST /api/settings/auth/change
func (h *authSettingsHandler) changeToken(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OldToken string `json:"oldToken"`
		NewToken string `json:"newToken"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "All fields are required")
		return
	}

	body.OldToken = strings.TrimSpace(body.OldToken)
	body.NewToken = strings.TrimSpace(body.NewToken)

	if body.OldToken == "" || body.NewToken == "" {
		writeError(w, http.StatusBadRequest, "All fields are required")
		return
	}

	if len(body.NewToken) < 6 {
		writeError(w, http.StatusBadRequest, "New token must be at least 6 characters")
		return
	}

	// Constant-time compare (matches AdminAuth middleware). Reject unequal
	// lengths after a dummy compare so length mismatches do not short-circuit
	// before crypto/subtle.ConstantTimeCompare.
	if !constantTimeTokenEqual(body.OldToken, h.cfg.AuthToken) {
		writeError(w, http.StatusForbidden, "Old token verification failed")
		return
	}

	// Persist to settings table
	now := timeNowUTC()
	var existingCount int
	h.db.Get(&existingCount, "SELECT COUNT(*) FROM settings WHERE key = 'auth_token'")
	if existingCount > 0 {
		h.db.Exec(h.db.Rebind("UPDATE settings SET value = ? WHERE key = 'auth_token'"), jsonQuote(body.NewToken))
	} else {
		h.db.Exec(h.db.Rebind("INSERT INTO settings (key, value) VALUES (?, ?)"), "auth_token", jsonQuote(body.NewToken))
	}

	// Update runtime config
	h.cfg.AuthToken = body.NewToken

	// #1034: every session was minted against the OLD master token. Revoke
	// them all — rotation must end every live admin session, full stop.
	// (nil-safe when sessions are unavailable.)
	h.sessions.RevokeAll(r.Context())

	// Defense-in-depth: expire HttpOnly meta_monitor_auth so browsers drop the
	// dead cookie after AuthToken rotation (HMAC already invalidates server-side).
	// Only on success — failed change must not clear a still-valid session cookie.
	clearMonitorAuthCookies(w, r)

	// Log the change event
	// read is a BOOLEAN column on PostgreSQL: bind FALSE, not the integer
	// literal 0 (w18-pg-dialect: PG rejects integer literals for booleans).
	h.db.Exec(`INSERT INTO events (type, title, message, level, related_type, created_at, read)
		VALUES ('token', 'Admin login token updated', 'The admin login token was changed. Use the new token to log in.', 'warning', 'settings', ?, FALSE)`, now)

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Token updated",
	})
}

// constantTimeTokenEqual compares a and b as UTF-8 bytes in constant time when
// lengths match. Mismatched lengths return false after a dummy compare so the
// early-reject path still exercises ConstantTimeCompare.
func constantTimeTokenEqual(a, b string) bool {
	ab, bb := []byte(a), []byte(b)
	if len(ab) != len(bb) {
		_ = subtle.ConstantTimeCompare(ab, ab)
		return false
	}
	return subtle.ConstantTimeCompare(ab, bb) == 1
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func timeNowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
