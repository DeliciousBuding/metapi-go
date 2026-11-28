package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ---- List Lite ----
// GET /api/routes/lite
func (h *tokenRoutesHandler) listLite(w http.ResponseWriter, r *http.Request) {
	rows, err := queryRowsErr(h.db, "SELECT id, model_pattern, display_name, display_icon, route_mode, routing_strategy, trace_sticky_mode, enabled, context_length FROM token_routes ORDER BY sort_order ASC, id ASC")
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load routes")
		return
	}
	type srcRow struct {
		GroupRouteID  int64 `db:"group_route_id"`
		SourceRouteID int64 `db:"source_route_id"`
	}
	var srcRows []srcRow
	h.db.Select(&srcRows, "SELECT group_route_id, source_route_id FROM route_group_sources")
	sourceIdsByRoute := map[int64][]int64{}
	for _, sr := range srcRows {
		sourceIdsByRoute[sr.GroupRouteID] = append(sourceIdsByRoute[sr.GroupRouteID], sr.SourceRouteID)
	}
	for _, row := range rows {
		rid := coerceInt64(row["id"])
		if ids, ok := sourceIdsByRoute[rid]; ok {
			row["sourceRouteIds"] = ids
		} else {
			row["sourceRouteIds"] = []int64{}
		}
	}
	writeJSON(w, http.StatusOK, normalizeSlice(rows))
}

// ---- List Summary ----
// GET /api/routes/summary
func (h *tokenRoutesHandler) listSummary(w http.ResponseWriter, r *http.Request) {
	rows, err := queryRowsErr(h.db, "SELECT * FROM token_routes ORDER BY sort_order ASC, id ASC")
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load routes")
		return
	}

	// Batch-load per-route channel counts in a single GROUP BY query instead of
	// firing two COUNT queries per route (the previous N+1 shape). A route with
	// no channels simply has no row in route_channels, so it falls through to
	// the zero value of routeChannelCounts — same as the old per-route COUNT.
	type routeChannelCounts struct {
		RouteID      int64 `db:"route_id"`
		Total        int64 `db:"total"`
		EnabledCount int64 `db:"enabled_count"`
	}
	var counts []routeChannelCounts
	if err := h.db.Select(&counts, `
		SELECT route_id, COUNT(*) AS total,
		       SUM(CASE WHEN enabled THEN 1 ELSE 0 END) AS enabled_count
		FROM route_channels GROUP BY route_id`); err != nil {
		slog.Warn("listSummary: batch channel counts failed", "err", err)
	}
	countsByRoute := make(map[int64]routeChannelCounts, len(counts))
	for _, c := range counts {
		countsByRoute[c.RouteID] = c
	}

	// Direct-upstream grant targets linked via upstream_route_groups count as
	// route channels too: without them an imported route that proxies fine
	// still renders as "0 channels, needs channels" in the summary.
	var directCounts []routeChannelCounts
	if err := h.db.Select(&directCounts, `
		SELECT urg.route_id AS route_id, COUNT(*) AS total,
		       SUM(CASE WHEN g.enabled AND cr.enabled AND ch.enabled THEN 1 ELSE 0 END) AS enabled_count
		FROM upstream_route_groups urg
		JOIN upstream_group_items gi ON gi.group_id = urg.group_id
		JOIN upstream_grants g ON g.id = gi.grant_id
		JOIN upstream_credentials cr ON cr.id = g.credential_id
		JOIN upstream_channels ch ON ch.id = cr.channel_id
		GROUP BY urg.route_id`); err != nil {
		slog.Warn("listSummary: batch direct grant counts failed", "err", err)
	}
	for _, c := range directCounts {
		existing := countsByRoute[c.RouteID]
		existing.RouteID = c.RouteID
		existing.Total += c.Total
		existing.EnabledCount += c.EnabledCount
		countsByRoute[c.RouteID] = existing
	}

	// Batch-load distinct site names per route via a single GROUP BY query
	// (route_channels → accounts → sites). A route with no channels has no
	// row in route_channels and falls through to a non-nil empty slice —
	// matching the frontend's `route.siteNames ?? []` render. GROUP BY on
	// (route_id, site_name) collapses multiple channels that share a site
	// so each site name appears at most once per route. No bind params means
	// the dual-dialect rebind gate is a no-op here (same as the counts query).
	type routeSiteName struct {
		RouteID  int64  `db:"route_id"`
		SiteName string `db:"site_name"`
	}
	var siteNameRows []routeSiteName
	if err := h.db.Select(&siteNameRows, `
		SELECT rc.route_id AS route_id, s.name AS site_name
		FROM route_channels rc
		JOIN accounts a ON rc.account_id = a.id
		JOIN sites s ON a.site_id = s.id
		WHERE s.name IS NOT NULL AND s.name <> ''
		GROUP BY rc.route_id, s.name`); err != nil {
		slog.Warn("listSummary: batch site names failed", "err", err)
	}
	siteNamesByRoute := make(map[int64][]string)
	for _, sn := range siteNameRows {
		siteNamesByRoute[sn.RouteID] = append(siteNamesByRoute[sn.RouteID], sn.SiteName)
	}

	result := make([]map[string]any, 0)
	for _, route := range rows {
		routeID := coerceInt64(route["id"])
		c := countsByRoute[routeID]
		siteNames := siteNamesByRoute[routeID]
		if siteNames == nil {
			siteNames = []string{}
		}

		item := map[string]any{
			"id":                  route["id"],
			"modelPattern":        route["modelPattern"],
			"displayName":         route["displayName"],
			"displayIcon":         route["displayIcon"],
			"routeMode":           route["routeMode"],
			"sourceRouteIds":      []int64{},
			"modelMapping":        route["modelMapping"],
			"routingStrategy":     route["routingStrategy"],
			"contextLength":       route["contextLength"],
			"enabled":             route["enabled"],
			"channelCount":        c.Total,
			"enabledChannelCount": c.EnabledCount,
			"siteNames":           siteNames,
			"decisionSnapshot":    nil,
			"decisionRefreshedAt": route["decisionRefreshedAt"],
		}
		// Parse decision snapshot
		if ds, ok := route["decisionSnapshot"].(string); ok && ds != "" {
			var parsed any
			if json.Unmarshal([]byte(ds), &parsed) == nil {
				item["decisionSnapshot"] = parsed
			}
		}
		result = append(result, item)
	}
	writeJSON(w, http.StatusOK, result)
}
