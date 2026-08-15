package admin

import (
	"log/slog"
	"net/http"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/routing"
)

// channelListRow is the flattened read-only projection for GET /api/channels.
// It aggregates route_channels across the account/token/oauth_unit dimensions
// and joins only the display fields the channels page needs (no credentials).
type channelListRow struct {
	ID                int64   `db:"id"`
	RouteID           int64   `db:"route_id"`
	AccountID         int64   `db:"account_id"`
	TokenID           *int64  `db:"token_id"`
	OAuthRouteUnitID  *int64  `db:"oauth_route_unit_id"`
	SourceModel       *string `db:"source_model"`
	Priority          int64   `db:"priority"`
	Weight            int64   `db:"weight"`
	Enabled           bool    `db:"enabled"`
	ManualOverride    bool    `db:"manual_override"`
	SuccessCount      int64   `db:"success_count"`
	TotalLatencyMs    int64   `db:"total_latency_ms"`
	CooldownUntil     *string `db:"cooldown_until"`
	Username          string  `db:"username"`
	SiteID            int64   `db:"site_id"`
	SiteName          string  `db:"site_name"`
	RouteModelPattern string  `db:"route_model_pattern"`
	OAuthUnitName     string  `db:"oauth_unit_name"`
	TokenName         string  `db:"token_name"`
}

// listChannels returns the aggregated read-only channel list (#622).
// Status is derived from routing in-memory breaker state + persisted cooldown
// so the UI never mutates routing health (read-only / soft isolation).
func (h *tokenRoutesHandler) listChannels(w http.ResponseWriter, r *http.Request) {
	var rows []channelListRow
	if err := h.db.SelectContext(r.Context(), &rows, h.db.Rebind(`
		SELECT rc.id, rc.route_id, rc.account_id, rc.token_id, rc.oauth_route_unit_id,
		       rc.source_model, rc.priority, rc.weight, rc.enabled, rc.manual_override,
		       rc.success_count, rc.total_latency_ms, rc.cooldown_until,
		       COALESCE(a.username, '') AS username,
		       a.site_id,
		       COALESCE(s.name, '') AS site_name,
		       COALESCE(tr.model_pattern, '') AS route_model_pattern,
		       COALESCE(oru.name, '') AS oauth_unit_name,
		       COALESCE(at.name, '') AS token_name
		FROM route_channels rc
		LEFT JOIN accounts a ON rc.account_id = a.id
		LEFT JOIN sites s ON a.site_id = s.id
		LEFT JOIN token_routes tr ON rc.route_id = tr.id
		LEFT JOIN oauth_route_units oru ON rc.oauth_route_unit_id = oru.id
		LEFT JOIN account_tokens at ON rc.token_id = at.id
		ORDER BY rc.id ASC`)); err != nil {
		slog.Error("channels list failed", "error", err)
		writeError(w, http.StatusInternalServerError, "加载通道列表失败")
		return
	}

	// Refresh the metapi_active_channels gauge so /metrics reflects the real
	// proxy channel count instead of a constant 0. Counted from the same rows
	// we already load for the list (enabled channels = active candidates).
	var enabledChannels int64
	for _, row := range rows {
		if row.Enabled {
			enabledChannels++
		}
	}
	shared.SetActiveChannels(enabledChannels)

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, channelListItem(row))
	}
	writeJSON(w, http.StatusOK, normalizeSlice(items))
}

func channelListItem(row channelListRow) map[string]any {
	sourceModel := ""
	if row.SourceModel != nil {
		sourceModel = *row.SourceModel
	}
	models := sourceModel
	if models == "" {
		models = row.RouteModelPattern
	}

	name := ""
	chType := "account"
	switch {
	case row.OAuthRouteUnitID != nil && *row.OAuthRouteUnitID > 0:
		chType = "oauth_unit"
		name = row.OAuthUnitName
	case row.TokenID != nil && *row.TokenID > 0:
		chType = "token"
		name = row.TokenName
	default:
		name = row.Username
	}
	if name == "" {
		name = sourceModel
	}
	if name == "" {
		name = row.RouteModelPattern
	}

	var responseMs *int64
	if row.SuccessCount > 0 && row.TotalLatencyMs > 0 {
		ms := row.TotalLatencyMs / row.SuccessCount
		responseMs = &ms
	}

	return map[string]any{
		"id":             row.ID,
		"routeId":        row.RouteID,
		"name":           name,
		"site":           map[string]any{"id": row.SiteID, "name": row.SiteName},
		"type":           chType,
		"status":         routing.ChannelRuntimeStatus(row.SiteID, sourceModel, row.Enabled, row.CooldownUntil),
		"models":         models,
		"priority":       row.Priority,
		"weight":         row.Weight,
		"responseMs":     responseMs,
		"cooldownUntil":  row.CooldownUntil,
		"enabled":        row.Enabled,
		"manualOverride": row.ManualOverride,
	}
}
