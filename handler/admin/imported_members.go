package admin

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func (h *importedUpstreamHandler) updateMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input struct {
		Priority *int64                     `json:"priority"`
		Weight   *int64                     `json:"weight"`
		Order    *store.DirectProtocolOrder `json:"protocolOrder"`
	}
	if decodeImportedJSON(r, &input) != nil || input.Priority == nil && input.Weight == nil && input.Order == nil {
		writeError(w, 400, "Invalid member update")
		return
	}
	if input.Priority != nil && (*input.Priority < -2147483648 || *input.Priority > 2147483647) || input.Weight != nil && (*input.Weight <= 0 || *input.Weight > 2147483647) {
		writeError(w, 400, "Priority must fit int32; weight must be a positive int32")
		return
	}
	tx, err := h.db.BeginTxx(r.Context(), nil)
	if err != nil {
		importedWriteError(w, err)
		return
	}
	defer tx.Rollback()
	query := `SELECT i.priority,i.weight,i.protocol_order,g.protocols FROM upstream_group_items i JOIN upstream_grants g ON g.id=i.grant_id WHERE i.id=?`
	if h.db.DriverName() == "pgx" {
		query += " FOR UPDATE OF i,g"
	}
	var priority, weight int64
	var order store.DirectProtocolOrder
	var protocols int
	if err = tx.QueryRowxContext(r.Context(), tx.Rebind(query), id).Scan(&priority, &weight, &order, &protocols); err != nil {
		importedReadError(w, err)
		return
	}
	if input.Priority != nil {
		priority = *input.Priority
	}
	if input.Weight != nil {
		weight = *input.Weight
	}
	if input.Order != nil {
		order = *input.Order
		encoded, _ := json.Marshal(order)
		var validated store.DirectProtocolOrder
		if validated.Scan(string(encoded)) != nil {
			writeError(w, 400, "Invalid protocol order")
			return
		}
		for _, bit := range order {
			if bit&protocols == 0 {
				writeError(w, 400, "Protocol order must be a subset of the shared grant")
				return
			}
		}
	}
	if _, err = tx.ExecContext(r.Context(), tx.Rebind(`UPDATE upstream_group_items SET priority=?,weight=?,protocol_order=? WHERE id=?`), priority, weight, order, id); err != nil {
		importedWriteError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		importedWriteError(w, err)
		return
	}
	routing.InvalidateCache()
	writeJSON(w, 200, map[string]any{"success": true, "id": id})
}

func (h *importedUpstreamHandler) clearMemberCooldown(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var grantID int64
	err := h.db.QueryRowxContext(r.Context(), h.db.Rebind(`UPDATE upstream_grants SET cooldown_until=NULL,cooldown_reason_code=NULL
WHERE id=(SELECT grant_id FROM upstream_group_items WHERE id=?) RETURNING id`), id).Scan(&grantID)
	if err != nil {
		if err == sql.ErrNoRows {
			writeError(w, 404, "Imported member not found")
		} else {
			importedWriteError(w, err)
		}
		return
	}
	routing.InvalidateCache()
	writeJSON(w, 200, map[string]any{"success": true, "id": id, "grantId": grantID})
}
