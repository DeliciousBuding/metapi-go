package admin

import (
	"net/http"
	"strconv"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/go-chi/chi/v5"
)

// clearChannelCooldown resets the cooldown of one route_channels row for
// POST /api/channels/:channelId/cooldown/clear in the route decision panel.
// Idempotent: a row that is not cooled is a successful no-op; unknown ids 404.
func (h *tokenRoutesHandler) clearChannelCooldown(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(chi.URLParam(r, "channelId"), 10, 64)
	if err != nil || channelID <= 0 {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeChannelNotFound, "channel not found")
		return
	}
	res, err := h.db.Exec(`UPDATE route_channels SET cooldown_until = NULL, consecutive_fail_count = 0, cooldown_level = 0, cooldown_reason_code = NULL, cooldown_reason = NULL, cooldown_reason_at = NULL WHERE id = ?`, channelID)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to clear channel cooldown: "+err.Error())
		return
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeChannelNotFound, "channel not found")
		return
	}
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "id": channelID})
}

// clearCooldown resets every channel of the route for POST /api/routes/:id/cooldown/clear.
func (h *tokenRoutesHandler) clearCooldown(w http.ResponseWriter, r *http.Request) {
	routeID, ok := pathID(w, r)
	if !ok {
		return
	}

	_, err := h.db.ExecContext(r.Context(), `UPDATE route_channels SET cooldown_until = NULL, consecutive_fail_count = 0, cooldown_level = 0, cooldown_reason_code = NULL, cooldown_reason = NULL, cooldown_reason_at = NULL WHERE route_id = ?`, routeID)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to clear route cooldown: "+err.Error())
		return
	}
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
