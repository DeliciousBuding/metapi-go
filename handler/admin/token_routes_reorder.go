package admin

import (
	"net/http"
	"time"
)

// PUT /api/routes/reorder
// Body: { "items": [ { "id": 1, "sortOrder": 0 },... ] }
// Assigns explicit sort_order for admin drag-and-drop route lists.
func (h *tokenRoutesHandler) reorderRoutes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			ID        int64 `json:"id"`
			SortOrder int64 `json:"sortOrder"`
		} `json:"items"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}
	if len(body.Items) == 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "items is required")
		return
	}
	if len(body.Items) > 1000 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "too many items (max 1000)")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var successIDs []int64
	var failedItems []map[string]any
	seen := map[int64]bool{}

	for _, item := range body.Items {
		if item.ID <= 0 {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": "invalid id"})
			continue
		}
		if item.SortOrder < 0 {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": "sortOrder must be >= 0"})
			continue
		}
		if seen[item.ID] {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": "duplicate id in payload"})
			continue
		}
		seen[item.ID] = true

		res, err := h.db.Exec(`
			UPDATE token_routes SET sort_order = ?, updated_at = ? WHERE id = ?
		`, item.SortOrder, now, item.ID)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": err.Error()})
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": "route not found"})
			continue
		}
		successIDs = append(successIDs, item.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     len(failedItems) == 0,
		"successIds":  successIDs,
		"failedItems": failedItems,
	})
}
