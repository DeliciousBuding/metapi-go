package admin

import (
	"net/http"
	"strconv"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterOauthRoutes registers all /api/oauth routes.
// Start/rebind issue cryptographically random server-stored state tokens (TTL 10m);
// session lookup and manual callback validate that state.
func RegisterOauthRoutes(r chi.Router, db *store.DB) {
	handler := &oauthHandler{db: db}

	r.Get("/api/oauth/providers", handler.listProviders)
	r.Post("/api/oauth/providers/{provider}/start", handler.startProviderFlow)
	r.Get("/api/oauth/sessions/{state}", handler.getSession)
	r.Post("/api/oauth/sessions/{state}/manual-callback", handler.manualCallback)
	r.Get("/api/oauth/connections", handler.listConnections)
	r.Post("/api/oauth/connections/{accountId}/rebind", handler.rebindConnection)
	r.Patch("/api/oauth/connections/{accountId}/proxy", handler.updateProxy)
	r.Delete("/api/oauth/connections/{accountId}", handler.deleteConnection)
	r.Post("/api/oauth/connections/{accountId}/quota/refresh", handler.refreshQuota)
	r.Post("/api/oauth/connections/quota/refresh-batch", handler.refreshQuotaBatch)
	r.Post("/api/oauth/import", handler.importConnections)
	r.Post("/api/oauth/route-units", handler.createRouteUnit)
	r.Patch("/api/oauth/route-units/{routeUnitId}", handler.updateRouteUnit)
	r.Delete("/api/oauth/route-units/{routeUnitId}", handler.deleteRouteUnit)
	r.Post("/api/oauth/route-units/{routeUnitId}/members/{memberId}/cooldown/clear", handler.clearMemberCooldown)
	r.Get("/api/oauth/callback/{provider}", handler.callback)
}

type oauthHandler struct {
	db *store.DB
}

func (h *oauthHandler) clearMemberCooldown(w http.ResponseWriter, r *http.Request) {
	unitID, unitErr := strconv.ParseInt(chi.URLParam(r, "routeUnitId"), 10, 64)
	memberID, memberErr := strconv.ParseInt(chi.URLParam(r, "memberId"), 10, 64)
	if unitErr != nil || memberErr != nil || unitID <= 0 || memberID <= 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "invalid route unit or member id")
		return
	}
	result, err := h.db.ExecContext(r.Context(), h.db.Rebind(`UPDATE oauth_route_unit_members SET
		cooldown_until = NULL, consecutive_fail_count = 0, cooldown_level = 0,
		cooldown_reason_code = NULL, cooldown_reason = NULL, cooldown_reason_at = NULL
		WHERE unit_id = ? AND id = ?`), unitID, memberID)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to clear member cooldown")
		return
	}
	affected, err := result.RowsAffected()
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to read member update result")
		return
	}
	if affected == 0 {
		writeErrorWithRequest(w, r, http.StatusNotFound, "route unit member not found")
		return
	}
	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "routeUnitId": unitID, "memberId": memberID})
}

// POST /api/oauth/import
func (h *oauthHandler) importConnections(w http.ResponseWriter, r *http.Request) {
	writeUnsupportedOAuthOperation(w)
}

// POST /api/oauth/route-units
func (h *oauthHandler) createRouteUnit(w http.ResponseWriter, r *http.Request) {
	writeUnsupportedOAuthOperation(w)
}

// PATCH /api/oauth/route-units/:routeUnitId
func (h *oauthHandler) updateRouteUnit(w http.ResponseWriter, r *http.Request) {
	writeUnsupportedOAuthOperation(w)
}

// DELETE /api/oauth/route-units/:routeUnitId
func (h *oauthHandler) deleteRouteUnit(w http.ResponseWriter, r *http.Request) {
	writeUnsupportedOAuthOperation(w)
}

// Keep the reserved paths explicit without pretending to persist a write.
func writeUnsupportedOAuthOperation(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"code":  "oauth_operation_not_supported",
		"error": "OAuth connection import and route-unit writes are not supported. Use provider authorization and managed routing instead.",
	})
}
