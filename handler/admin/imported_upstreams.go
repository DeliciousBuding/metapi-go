package admin

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// Imported upstreams are distinct from native account-backed route channels.
// These projections deliberately never select credentials or custom headers.
func RegisterImportedUpstreamRoutes(r chi.Router, db *sqlx.DB) {
	h := &importedUpstreamHandler{db: db}
	r.Get("/api/imported-upstreams", h.list)
	r.Patch("/api/imported-upstreams/{id}", h.update)
}

type importedUpstreamHandler struct{ db *sqlx.DB }

func (h *importedUpstreamHandler) list(w http.ResponseWriter, r *http.Request) {
	channels, err := queryRowsErr(h.db, `SELECT c.id, c.name, c.origin_key, c.dialect, c.base_url, c.enabled,
  c.openai_chat_completion_path, c.openai_response_path, c.anthropic_message_path,
  (SELECT COUNT(*) FROM upstream_credentials k WHERE k.channel_id=c.id) AS credential_count,
  (SELECT COUNT(*) FROM upstream_models m WHERE m.channel_id=c.id) AS model_count
  FROM upstream_channels c ORDER BY c.name, c.id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load imported upstreams")
		return
	}
	members, err := queryRowsErr(h.db, `SELECT i.id, i.group_id, i.priority, i.weight, g.name AS group_name,
  g.mode, g.active_item_id, rg.route_id, m.channel_id, m.name AS model_name,
	  k.name AS credential_name, k.enabled AS credential_enabled, grt.protocols,
    grt.cooldown_until,grt.cooldown_reason_code,grt.success_count,grt.fail_count
  FROM upstream_group_items i JOIN upstream_groups g ON g.id=i.group_id
  JOIN upstream_route_groups rg ON rg.group_id=g.id JOIN upstream_grants grt ON grt.id=i.grant_id
  JOIN upstream_models m ON m.id=grt.model_id JOIN upstream_credentials k ON k.id=grt.credential_id
  ORDER BY g.name, i.priority, i.id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load imported routing members")
		return
	}
	for _, row := range channels {
		row["enabled"] = coerceBool(row["enabled"])
	}
	for _, row := range members {
		row["credentialEnabled"] = coerceBool(row["credentialEnabled"])
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": normalizeSlice(channels), "members": normalizeSlice(members)})
}

// Re-import remains the owner of the source graph. A disabled channel must
// disappear from cached selection immediately, without disabling its siblings.
func (h *importedUpstreamHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil || input.Enabled == nil {
		writeError(w, http.StatusBadRequest, "Expected an enabled boolean")
		return
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "Expected one JSON object")
		return
	}
	result, err := h.db.Exec(h.db.Rebind(`UPDATE upstream_channels SET enabled=? WHERE id=?`), *input.Enabled, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update imported upstream")
		return
	}
	count, err := result.RowsAffected()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to verify imported upstream update")
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "Imported upstream not found")
		return
	}
	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "id": id, "enabled": *input.Enabled})
}
