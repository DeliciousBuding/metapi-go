package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterMonitorRoutes registers the Bearer-protected /api/monitor
// configuration surface. The cookie-authenticated iframe proxy lives in
// RegisterMonitorProxyRoutes and must be wired OUTSIDE the admin auth group
// (see router/router.go).
func RegisterMonitorRoutes(r chi.Router, db *store.DB, cfg *config.Config) {
	handler := &monitorHandler{db: db, cfg: cfg}

	r.Get("/api/monitor/config", handler.getConfig)
	RegisterMonitorHealthRoute(r, db) // GET /api/monitor/health (observability projection)
	r.Put("/api/monitor/config", handler.saveConfig)
	r.Post("/api/monitor/session", handler.createSession)
	// DELETE clears the HttpOnly meta_monitor_auth cookie on admin logout.
	// Frontend JS cannot clear HttpOnly cookies; this is the honest clear path.
	r.Delete("/api/monitor/session", handler.clearSession)
}

// RegisterMonitorProxyRoutes registers the /monitor-proxy/ldoh* iframe proxy
// surface. Authentication is the HttpOnly meta_monitor_auth cookie minted by
// createSession — iframe sub-resource requests cannot carry an Authorization
// header, so mounting these routes behind the Bearer AdminAuth middleware
// breaks the LDOH iframe. The handler enforces
// the cookie itself via ensureMonitorAuth; keep this registrar outside any
// Bearer-gated group.
func RegisterMonitorProxyRoutes(r chi.Router, db *store.DB, cfg *config.Config) {
	handler := &monitorHandler{db: db, cfg: cfg}

	r.HandleFunc("/monitor-proxy/ldoh", handler.ldohProxy)
	r.HandleFunc("/monitor-proxy/ldoh/", handler.ldohProxy)
	r.HandleFunc("/monitor-proxy/ldoh/*", handler.ldohProxy)
}

type monitorHandler struct {
	db  *store.DB
	cfg *config.Config

	ldohClientOnce sync.Once
	ldohHTTPClient *http.Client
}

// SECURITY: The LDOH upstream session cookie (ld_auth_session=…) is stored
// PLAINTEXT in the settings table (key = monitor_ldoh_cookie). Anyone with
// read access to the database — a leaked backup, a SQL injection in another
// handler, or a snapshot on a shared host — can impersonate the LDOH session
// until it expires upstream. This is an accepted short-term trade-off: the
// value is already a bearer secret handed to the browser, and the settings
// table is not field-level encrypted today. A future improvement should
// encrypt this secret at rest (e.g. AES-GCM keyed by AccountCredentialSecret)
// so a database read alone does not yield a usable session cookie. Until then,
// treat DB access as equivalent to LDOH credential disclosure.
//
// Backup imports are therefore forbidden from setting this key: it is listed
// in service/backup.RuntimeLocalSettingKeys, which both import paths enforce.
// A malicious import planting monitor_ldoh_cookie would pin the monitored
// session to an attacker-controlled credential.
const ldohCookieSettingKey = "monitor_ldoh_cookie"

// GET /api/monitor/config
func (h *monitorHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	ldohCookie := getSettingValue(h.db, ldohCookieSettingKey)
	writeJSON(w, http.StatusOK, map[string]any{
		"ldohCookieConfigured": ldohCookie != "",
		"ldohCookieMasked":     maskValue(ldohCookie),
	})
}

// PUT /api/monitor/config
func (h *monitorHandler) saveConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LdohCookie *string `json:"ldohCookie"`
	}
	rawBody, readErr := readAdminJSONBody(r.Body)
	if readErr != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(strings.TrimSpace(string(rawBody))) > 0 {
		if err := rejectDuplicateJSONKeys(rawBody); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		if err := json.Unmarshal(rawBody, &body); err != nil {
			writeError(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	raw := ""
	if body.LdohCookie != nil {
		raw = strings.TrimSpace(*body.LdohCookie)
	}

	if raw == "" {
		if err := upsertSettingDB(h.db, ldohCookieSettingKey, ""); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to clear LDOH cookie")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success":              true,
			"message":              "LDOH cookie cleared",
			"ldohCookieConfigured": false,
			"ldohCookieMasked":     "",
		})
		return
	}

	normalized := normalizeLdohCookie(raw)
	if !strings.HasPrefix(normalized, "ld_auth_session=") || len(normalized) < 24 {
		writeError(w, http.StatusBadRequest, "invalid cookie format; provide ld_auth_session or its value")
		return
	}

	if err := upsertSettingDB(h.db, ldohCookieSettingKey, normalized); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save LDOH cookie")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":              true,
		"message":              "LDOH cookie saved",
		"ldohCookieConfigured": true,
		"ldohCookieMasked":     maskValue(normalized),
	})
}

func normalizeLdohCookie(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "ld_auth_session=") {
		firstPair := strings.TrimSpace(strings.Split(trimmed, ";")[0])
		if strings.HasPrefix(firstPair, "ld_auth_session=") {
			return firstPair
		}
	}
	return "ld_auth_session=" + trimmed
}

func maskValue(value string) string {
	if value == "" {
		return ""
	}
	// Prefer masking the cookie value portion after '=' when present.
	raw := value
	if idx := strings.Index(value, "="); idx >= 0 {
		raw = value[idx+1:]
	}
	if len(raw) <= 10 {
		if len(raw) < 2 {
			return "****"
		}
		return raw[:2] + "****"
	}
	return raw[:6] + "****" + raw[len(raw)-4:]
}

func getSettingValue(db *store.DB, key string) string {
	value, err := store.NewSettingsStore(db).Get(key)
	if err != nil {
		return ""
	}
	// Value is stored as JSON-encoded string
	val := strings.TrimSpace(value)
	if strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`) {
		var unquoted string
		if err := json.Unmarshal([]byte(val), &unquoted); err == nil {
			return unquoted
		}
		val = val[1 : len(val)-1]
	}
	return val
}
