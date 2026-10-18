package admin

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/admintoken"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterAuthSettingsRoutes registers all /api/settings/auth routes.
// sessions may be nil (tests): rotation then skips session revocation.
func RegisterAuthSettingsRoutes(r chi.Router, db *sqlx.DB, sessions *auth.SessionManager) {
	handler := &authSettingsHandler{db: db, sessions: sessions}

	r.Get("/api/settings/auth/info", handler.getInfo)
	r.Post("/api/settings/auth/change", handler.changeToken)
}

type authSettingsHandler struct {
	db       *sqlx.DB
	sessions *auth.SessionManager
}

// GET /api/settings/auth/info
func (h *authSettingsHandler) getInfo(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)
	token := strings.TrimSpace(config.Runtime().AuthToken)
	// The master token itself must never cross the wire. Report only whether
	// the operator replaced the shipped default and how long it is, which is
	// enough for the settings form to describe the current state.
	writeJSON(w, http.StatusOK, map[string]any{
		"customized": token != "" && token != config.DefaultAuthToken,
		"configured": token != "",
		"length":     len(token),
	})
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

	if body.OldToken == "" {
		writeError(w, http.StatusBadRequest, "Old token is required")
		return
	}

	// Operators no longer invent the replacement secret: an empty newToken is
	// generated server-side below. An explicit value stays supported for
	// automation, but it must clear the same strength floor the startup check
	// applies (16 bytes; 32+ recommended).
	if body.NewToken != "" && len(body.NewToken) < 16 {
		writeError(w, http.StatusBadRequest, "New token must be at least 16 characters; omit it to have the server generate one")
		return
	}

	// Constant-time compare (matches AdminAuth middleware). Reject unequal
	// lengths after a dummy compare so length mismatches do not short-circuit
	// before crypto/subtle.ConstantTimeCompare.
	if !constantTimeTokenEqual(body.OldToken, config.Runtime().AuthToken) {
		writeError(w, http.StatusForbidden, "Old token verification failed")
		return
	}

	generated := body.NewToken == ""
	if generated {
		token, err := admintoken.Generate()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to generate a new token")
			return
		}
		body.NewToken = token
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

	// Publish the rotated token atomically; every subsequent admin/proxy
	// auth check sees the new value without a restart.
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.AuthToken = body.NewToken })

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
	// literal 0 (PG rejects integer literals for booleans).
	h.db.Exec(h.db.Rebind(`INSERT INTO events (type, title, message, level, related_type, created_at, read)
		VALUES ('token', 'Admin login token updated', 'The admin login token was changed. Use the new token to log in.', 'warning', 'settings', ?, FALSE)`), now)

	// Keep the 0600 handoff file in sync when this deployment uses it. A
	// custom rotation overwrites the previous value instead of leaving a dead
	// token behind; the token itself is never logged.
	if cfg := config.GetSafe(); cfg != nil && cfg.DataDir != "" {
		if path, err := admintoken.WriteFile(cfg.DataDir, body.NewToken); err != nil {
			slog.Warn("rotated admin token could not be written to the handoff file", "error", err)
		} else if generated {
			slog.Warn("generated a new admin token", "path", path, "hint", "read it once, store it securely, and remove the file")
		}
	}

	resp := map[string]any{
		"success": true,
		"message": "Token updated",
	}
	if generated {
		// The server chose the secret, so the caller gets exactly one chance
		// to capture it. The response itself is no-store (reauth gate).
		resp["newToken"] = body.NewToken
	}
	writeJSON(w, http.StatusOK, resp)
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
