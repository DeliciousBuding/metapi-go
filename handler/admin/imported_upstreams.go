package admin

import (
	"fmt"
	"net/http"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// Imported upstreams are distinct from native account-backed route channels.
// These projections deliberately never select credentials or custom headers.
func RegisterImportedUpstreamRoutes(r chi.Router, db *sqlx.DB) {
	RegisterUpstreamCatalogRoutes(r, db)
	RegisterUpstreamLifecycleRoutes(r, db)
	RegisterUpstreamPresetRoutes(r)
	h := &importedUpstreamHandler{db: db}
	r.Get("/api/imported-upstreams", h.list)
	r.Get("/api/imported-upstreams/{id}", h.detail)
	r.Get("/api/imported-upstreams/{id}/request-config", h.requestConfig)
	r.Patch("/api/imported-upstreams/{id}", h.update)
	r.Patch("/api/imported-upstreams/members/{id}", h.updateMember)
	r.Post("/api/imported-upstreams/members/{id}/cooldown/clear", h.clearMemberCooldown)
	r.Get("/api/imported-upstreams/{id}/credentials", h.credentials)
	r.Patch("/api/imported-upstreams/credentials/{id}", h.updateCredential)
}

type importedUpstreamHandler struct{ db *sqlx.DB }

func (h *importedUpstreamHandler) list(w http.ResponseWriter, r *http.Request) {
	channels, err := queryRowsErr(h.db, `SELECT c.id, c.name, c.origin_key, c.dialect, c.provider, c.base_url, c.enabled,
  c.openai_chat_completion_path, c.openai_response_path, c.anthropic_message_path, c.endpoint_config,
  (SELECT COUNT(*) FROM upstream_credentials k WHERE k.channel_id=c.id) AS credential_count,
  (SELECT COUNT(*) FROM upstream_models m WHERE m.channel_id=c.id) AS model_count
  FROM upstream_channels c ORDER BY c.name, c.id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load imported upstreams")
		return
	}
	members, err := queryRowsErr(h.db, `SELECT i.id, i.group_id, i.priority, i.weight, i.protocol_order, g.name AS group_name,
  g.mode, g.active_item_id, rg.route_id, m.channel_id, m.name AS model_name,
	  k.id AS credential_id, k.name AS credential_name, k.kind AS credential_kind, k.enabled AS credential_enabled, grt.protocols,
    grt.id AS grant_id, grt.enabled AS grant_enabled, m.enabled AS model_enabled, g.enabled AS group_enabled,
    c.enabled AS channel_enabled, tr.enabled AS route_enabled,
    grt.cooldown_until,grt.cooldown_reason_code,grt.success_count,grt.fail_count
  FROM upstream_group_items i JOIN upstream_groups g ON g.id=i.group_id
  JOIN upstream_route_groups rg ON rg.group_id=g.id JOIN upstream_grants grt ON grt.id=i.grant_id
  JOIN upstream_models m ON m.id=grt.model_id JOIN upstream_credentials k ON k.id=grt.credential_id
  JOIN upstream_channels c ON c.id=m.channel_id JOIN token_routes tr ON tr.id=rg.route_id
  ORDER BY g.name, i.priority, i.id`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load imported routing members")
		return
	}
	for _, row := range channels {
		row["ownership"] = upstream.Ownership(fmt.Sprint(row["originKey"]))
		var endpoints store.DirectEndpoints
		if err := endpoints.Scan(row["endpointConfig"]); err != nil {
			writeError(w, http.StatusInternalServerError, "Invalid imported upstream endpoints")
			return
		}
		row["endpointConfig"] = endpoints
		row["baseUrl"] = importedDisplayURL(fmt.Sprint(row["baseUrl"]))
		row["enabled"] = coerceBool(row["enabled"])
	}
	for _, row := range members {
		var order store.DirectProtocolOrder
		if err := order.Scan(row["protocolOrder"]); err != nil {
			writeError(w, http.StatusInternalServerError, "Invalid imported protocol priority")
			return
		}
		if order == nil {
			order = store.DirectProtocolOrder{}
		}
		row["protocolOrder"] = order
		effective := true
		for _, key := range []string{"channelEnabled", "modelEnabled", "credentialEnabled", "grantEnabled", "groupEnabled", "routeEnabled"} {
			row[key] = coerceBool(row[key])
			effective = effective && row[key].(bool)
		}
		selected := fmt.Sprint(row["mode"]) != "manual" || fmt.Sprint(row["activeItemId"]) == fmt.Sprint(row["id"])
		row["selectedByGroup"] = selected
		row["effectiveEnabled"] = effective && selected
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": normalizeSlice(channels), "members": normalizeSlice(members)})
}
