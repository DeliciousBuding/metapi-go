package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterResinRoutes mounts the Resin observability endpoints behind admin auth.
//
// Currently exposes:
//
//	GET /api/admin/resin/status — global enable flag, resin URL (masked),
//	active lease snapshot, and per-site override list.
//
// All responses are read-only: this surface is observability-only and does
// not mutate Resin state or the lease tracker.
func RegisterResinRoutes(r chi.Router, db *store.DB, cfg *config.Config) {
	h := &resinHandler{db: db, cfg: cfg}
	r.Get("/api/admin/resin/status", h.status)
}

type resinHandler struct {
	db  *store.DB
	cfg *config.Config
}

// GET /api/admin/resin/status
//
// Returns:
//   - enabled: global Resin toggle (RESIN_ENABLED + non-empty RESIN_URL)
//   - resinUrl: the configured RESIN_URL (masked when sensitive)
//   - platformName: explicit RESIN_PLATFORM_NAME override (may be empty)
//   - activeLeases: list of {accountId, lastUsed} entries whose last-used
//     timestamp is fresher than the 5-minute stale TTL
//   - perSiteOverrides: list of {siteId, name, platform, resinEnabled} for
//     sites whose resin_enabled column is non-NULL (explicit opt-in or
//     opt-out). nil-inherit rows are intentionally absent so the snapshot
//     stays bounded.
func (h *resinHandler) status(w http.ResponseWriter, r *http.Request) {
	enabled := service.ResinGlobalEnabled(h.cfg)
	resinURL := ""
	if h.cfg != nil {
		resinURL = strings.TrimSpace(h.cfg.ResinURL)
	}
	platformName := ""
	if h.cfg != nil {
		platformName = strings.TrimSpace(h.cfg.ResinPlatformName)
	}

	activeLeases := service.ActiveLeases()
	perSiteOverrides, err := h.db.ResinSiteOverrides()
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load resin per-site overrides")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":          enabled,
		"resinUrl":         resinURL,
		"platformName":     platformName,
		"activeLeases":     activeLeases,
		"perSiteOverrides": perSiteOverrides,
		"generatedAt":      time.Now().UTC().Format(time.RFC3339),
	})
}
