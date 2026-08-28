package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/go-chi/chi/v5"
)

// ---- B2: live ops WebSocket ----

// GET /api/admin/ops/ws?ticket=<one-time ticket> — realtime traffic push.
// Browser WebSocket API cannot set headers, and the master token must never
// ride a URL (server/proxy logs, browser history), so the SPA first mints a
// one-time 60s ticket via POST /api/auth/ws-ticket (cookie-authenticated,
// #1034 session model) and dials with that ticket instead. The legacy
// ?token=<master token> query path is gone for good. The endpoint stays
// mounted OUTSIDE the header-auth admin group; ticket validation happens
// here against the same SessionManager AdminAuth trusts.

// Server pushes one JSON frame per second:

//	{"lifetime": 1234, "uptimeSeconds": 3600, "points": [{"ts":..., "total": 5, "success": 4},...]}

// where points cover the last 300s (zero-filled). `lifetime` is the monotonic
// request counter; `uptimeSeconds` is the process wall-clock runtime — the
// frontend renders the latter as the panel's uptime metric. This instance's
// own traffic only (multi-instance honesty: no cross-instance aggregation).

// RegisterOpsWSRoutes mounts the live ops WebSocket endpoint. sessions may
// be nil (DB-less boot): the endpoint then fails closed.
func RegisterOpsWSRoutes(r chi.Router, cfg *config.Config, sessions *auth.SessionManager) {
	h := &opsWSHandler{cfg: cfg, sessions: sessions}
	r.Get("/api/admin/ops/ws", h.serve)
}

type opsWSHandler struct {
	cfg      *config.Config
	sessions *auth.SessionManager
}

func (h *opsWSHandler) serve(w http.ResponseWriter, r *http.Request) {
	// Redeem a one-time ticket (#1034). The ticket was minted for a live
	// admin session via POST /api/auth/ws-ticket; it is single-use and
	// 60s-lived, so a leaked URL dies almost immediately and never reveals
	// the master token or the session cookie.
	got := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if h.sessions == nil || got == "" || !h.sessions.ConsumeWSTicket(got) {
		writeJSON(w, http.StatusForbidden, map[string]string{"message": "invalid ticket"})
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Restrict cross-origin WebSocket upgrades to the operator-configured
		// admin origins (ADMIN_CORS_ALLOWED_ORIGINS) plus localhost/127.0.0.1
		// for dev. When no origins are configured, the slice is empty which
		// makes coder/websocket enforce same-origin only (Origin host must
		// match the request Host) — the safe default for an unconfigured box.
		// The token check above is unaffected; this only tightens the
		// browser-origin gate so a cross-site page cannot ride an admin's
		// session via WebSocket.
		OriginPatterns: opsWSOriginPatterns(h.cfg),
	})
	if err != nil {
		return // handshake failure already written
	}
	defer conn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Push the first frame immediately so the panel isn't blank.
	if !pushOpsFrame(ctx, conn) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !pushOpsFrame(ctx, conn) {
				return
			}
		}
	}
}

// pushOpsFrame writes one snapshot frame; returns false when the peer is gone.
func pushOpsFrame(ctx context.Context, conn *websocket.Conn) bool {
	points, lifetime, uptimeSeconds := shared.RealtimeSnapshot()
	payload := map[string]any{
		"lifetime":      lifetime,
		"uptimeSeconds": uptimeSeconds,
		"points":        points,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	// Non-blocking write with a context deadline; a slow/closed client just
	// disconnects on timeout.
	ctx2, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	err = conn.Write(ctx2, websocket.MessageText, b)
	return err == nil
}

// constantTimeEqual compares two strings in constant time.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// opsWSOriginPatterns returns the authorized WebSocket origin host patterns.
//
// When AdminCorsAllowedOrigins is empty the result is nil, which makes
// coder/websocket enforce same-origin only (the Origin host must match the
// request Host). When origins are configured, the configured entries are
// returned alongside localhost/127.0.0.1 so local dev against a configured
// box still works. Patterns are matched against the Origin URL host (or
// scheme://host when the pattern itself contains "://"), matching the
// library's authenticateOrigin rules.
func opsWSOriginPatterns(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	configured := cfg.AdminCorsAllowedOrigins
	if len(configured) == 0 {
		return nil
	}
	patterns := make([]string, 0, len(configured)+2)
	patterns = append(patterns, configured...)
	patterns = append(patterns, "localhost", "127.0.0.1")
	return patterns
}
