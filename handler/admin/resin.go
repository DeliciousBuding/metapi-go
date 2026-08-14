package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
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
func RegisterResinRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	h := &resinHandler{db: db, cfg: cfg}
	r.Get("/api/admin/resin/status", h.status)
}

type resinHandler struct {
	db  *sqlx.DB
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
	perSiteOverrides := h.perSiteOverrides()

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":            enabled,
		"resinUrl":           resinURL,
		"platformName":       platformName,
		"activeLeases":       activeLeases,
		"perSiteOverrides":   perSiteOverrides,
		"generatedAt":        time.Now().UTC().Format(time.RFC3339),
	})
}

// perSiteOverrides queries sites with a non-NULL resin_enabled column and
// returns them in admin-list order. NULL-inherit sites are excluded so the
// snapshot only lists operators' explicit decisions.
func (h *resinHandler) perSiteOverrides() []map[string]any {
	if h.db == nil {
		return []map[string]any{}
	}
	rows := queryRows(h.db, `
		SELECT id, name, platform, resin_enabled
		FROM sites
		WHERE resin_enabled IS NOT NULL
		ORDER BY sort_order, id
	`)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		siteID := coerceInt64(row["id"])
		name := coerceString(row["name"])
		platform := coerceString(row["platform"])
		resinEnabled := coerceBool(row["resinEnabled"])
		out = append(out, map[string]any{
			"siteId":        siteID,
			"name":          name,
			"platform":      platform,
			"resinEnabled":  resinEnabled,
		})
	}
	return out
}
