package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
)

// ---- Batch Routes ----
// POST /api/routes/batch
func (h *tokenRoutesHandler) batchRoutes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string  `json:"action"`
		IDs    []int64 `json:"ids"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	if len(body.IDs) == 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "ids must be a non-empty array")
		return
	}

	action := strings.TrimSpace(body.Action)
	if action != "enable" && action != "disable" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "action must be enable or disable")
		return
	}

	enabled := action == "enable"
	now := time.Now().UTC().Format(time.RFC3339)
	updated := 0
	for _, id := range body.IDs {
		res, err := h.db.Exec("UPDATE token_routes SET enabled = ?, updated_at = ? WHERE id = ?", enabled, now, id)
		if err == nil {
			n, _ := res.RowsAffected()
			updated += int(n)
		}
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"updatedCount": updated,
	})
}
