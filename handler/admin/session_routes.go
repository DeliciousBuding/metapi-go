// metapi-go/handler/admin — session lifecycle endpoints (#1034 session model).
//
// POST /api/auth/login    master token -> HttpOnly session cookie (public)
// GET  /api/auth/session  bootstrap probe: is this browser authenticated?   (public)
// POST /api/auth/logout   revoke the session row + clear the cookie          (public)
// POST /api/auth/ws-ticket mint a one-time 60s ticket for the ops WebSocket  (authed)
//
// login/logout/session are registered in AdminAuth's public allowlist: login
// must be reachable without a session (it is the place the master token is
// presented, protected by the strict /api/auth/* rate bucket), and
// logout/session stay idempotent/read-only so an expired browser can always
// recover the sign-in state without a 401 redirect loop.
package admin

import (
	"net"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/go-chi/chi/v5"
)

// RegisterSessionRoutes mounts the admin session lifecycle endpoints.
// sessions may be nil (DB-less boot): all endpoints then fail closed.
func RegisterSessionRoutes(r chi.Router, cfg *config.Config, sessions *auth.SessionManager) {
	h := &sessionRoutesHandler{cfg: cfg, sessions: sessions}
	r.Post("/api/auth/login", h.login)
	r.Get("/api/auth/session", h.status)
	r.Post("/api/auth/logout", h.logout)
	r.Post("/api/auth/ws-ticket", h.wsTicket)
}

type sessionRoutesHandler struct {
	cfg      *config.Config
	sessions *auth.SessionManager
}

type loginRequest struct {
	Token string `json:"token"`
}

// POST /api/auth/login — exchanges the master token for a server-side
// session. The raw credential exists only inside the Set-Cookie header; the
// database keeps its SHA-256 hash.
func (h *sessionRoutesHandler) login(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, "session store unavailable")
		return
	}
	var body loginRequest
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if body.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if !constantTimeEqual(config.Runtime().AuthToken, body.Token) {
		// Same status/message shape as AdminAuth's Bearer rejection so the
		// sign-in form keeps its existing error classification.
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Invalid token"})
		return
	}

	clientIP := extractAdminClientIP(r)
	raw, sess, err := h.sessions.Create(r.Context(), clientIP, r.UserAgent())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	auth.SetSessionCookie(w, r, raw, h.sessions.TTL(), h.cfg.AdminSessionCookieSecure)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"expiresAt":     sess.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
		"ttlMinutes":    h.cfg.AdminSessionTTLMinutes,
	})
}

// GET /api/auth/session — bootstrap probe for the SPA. Answers 200 in every
// case; authenticated=false means "show the sign-in page". The cookie track
// reports the slid expiry; the Bearer track (scripts probing the API) is
// reported as source="token".
func (h *sessionRoutesHandler) status(w http.ResponseWriter, r *http.Request) {
	if cookieToken := auth.SessionCookieFromRequest(r); cookieToken != "" && h.sessions != nil {
		if sess := h.sessions.Validate(r.Context(), cookieToken); sess != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"authenticated": true,
				"source":        "session",
				"expiresAt":     sess.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
			})
			return
		}
	}
	if bearer := bearerTokenFromRequest(r); bearer != "" &&
		constantTimeEqual(config.Runtime().AuthToken, bearer) {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated": true,
			"source":        "token",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

// POST /api/auth/logout — revokes the server-side session and clears the
// cookie. Idempotent by design: an unknown/expired cookie still yields 200
// so the UI can always land on the sign-in page.
func (h *sessionRoutesHandler) logout(w http.ResponseWriter, r *http.Request) {
	if cookieToken := auth.SessionCookieFromRequest(r); cookieToken != "" {
		h.sessions.Revoke(r.Context(), cookieToken)
	}
	auth.ClearSessionCookie(w, r, h.cfg.AdminSessionCookieSecure)
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// POST /api/auth/ws-ticket — mints a one-time, 60-second ticket the SPA
// presents to the realtime ops WebSocket. Replaces the legacy ?token= query
// parameter: the master token never appears in URLs (logs, history, proxy
// trails). Requires a live session or Bearer master token (AdminAuth).
func (h *sessionRoutesHandler) wsTicket(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		writeError(w, http.StatusServiceUnavailable, "session store unavailable")
		return
	}
	ticket, ttl := h.sessions.IssueWSTicket()
	if ticket == "" {
		writeError(w, http.StatusInternalServerError, "could not mint ws ticket")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":           ticket,
		"expiresInSeconds": int(ttl.Seconds()),
	})
}

// extractAdminClientIP mirrors AdminAuth's IP derivation (RemoteAddr, no
// forwarded headers) so admin_sessions.client_ip matches what the allowlist
// sees.
func extractAdminClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// bearerTokenFromRequest extracts the token from an Authorization: Bearer
// header ("" when absent or not a Bearer scheme).
func bearerTokenFromRequest(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}
