package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

const (
	routeDecisionBatchMaxItems     = 500
	routeDecisionRefreshTaskType   = "route-decision.refresh"
	routeDecisionRefreshTaskTitle  = "Refresh route selection probabilities"
	routeDecisionRefreshDedupeKey  = "route-decision-refresh"
	routeDecisionRouterUnavailable = "route decision engine is not configured"
)

// RouteDecisionExplainer is the router surface used by decision admin APIs.
// Tests inject fakes; production wires routing.TokenRouter.
type RouteDecisionExplainer interface {
	ExplainSelection(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error)
	ExplainSelectionForRoute(ctx context.Context, routeID int64, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error)
	ExplainSelectionRouteWide(ctx context.Context, routeID int64, policy routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error)
}

// RouteDecisionRefresher refreshes persisted route decision snapshots.
type RouteDecisionRefresher interface {
	RefreshAllRouteDecisionSnapshots(ctx context.Context, refreshPricingCatalog bool) (exactModelCount int, wildcardRouteCount int, err error)
}

// TokenRoutesDeps are optional dependencies for route decision endpoints.
type TokenRoutesDeps struct {
	Router    RouteDecisionExplainer
	Decisions RouteDecisionRefresher
}

// RegisterTokenRoutesWithDeps registers routes with optional decision engine deps.
func RegisterTokenRoutesWithDeps(r chi.Router, db *sqlx.DB, deps TokenRoutesDeps) {
	handler := &tokenRoutesHandler{db: db, router: deps.Router, decisions: deps.Decisions}

	// Route list
	r.Get("/api/routes/lite", handler.listLite)
	r.Get("/api/routes/summary", handler.listSummary)
	r.Get("/api/routes", handler.listRoutes)

	// Route CRUD
	r.Post("/api/routes", handler.createRoute)
	r.Post("/api/routes/batch", handler.batchRoutes)
	r.Put("/api/routes/reorder", handler.reorderRoutes) // static path before /{id}
	r.Post("/api/routes/rebuild", handler.rebuildRoutes)
	r.Put("/api/routes/{id}", handler.updateRoute)
	r.Delete("/api/routes/{id}", handler.deleteRoute)

	// Route channels
	r.Get("/api/routes/{id}/channels", handler.getRouteChannels)
	r.Post("/api/routes/{id}/channels", handler.addChannel)
	r.Post("/api/routes/{id}/channels/batch", handler.batchAddChannels)
	r.Post("/api/routes/{id}/cooldown/clear", handler.clearCooldown)

	// Channel operations
	r.Get("/api/channels", handler.listChannels)
	r.Get("/api/channels/error-summary", handler.channelErrorSummary)
	r.Get("/api/channels/probe-history", handler.channelProbeHistory)
	r.Put("/api/channels/batch", handler.batchUpdateChannels)
	r.Put("/api/channels/{channelId}", handler.updateChannel)
	r.Delete("/api/channels/{channelId}", handler.deleteChannel)

	// Route decisions
	r.Get("/api/routes/decision", handler.routeDecision)
	r.Post("/api/routes/decision/batch", handler.routeDecisionBatch)
	r.Post("/api/routes/decision/by-route/batch", handler.routeDecisionByRouteBatch)
	r.Post("/api/routes/decision/route-wide/batch", handler.routeDecisionRouteWideBatch)
	r.Post("/api/routes/decision/refresh", handler.routeDecisionRefresh)
}

type tokenRoutesHandler struct {
	db        *sqlx.DB
	router    RouteDecisionExplainer
	decisions RouteDecisionRefresher
}

// ---- List Lite ----
// GET /api/routes/lite
func (h *tokenRoutesHandler) listLite(w http.ResponseWriter, r *http.Request) {
	rows, err := queryRowsErr(h.db, "SELECT id, model_pattern, display_name, display_icon, route_mode, routing_strategy, enabled, context_length FROM token_routes ORDER BY sort_order ASC, id ASC")
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to load routes")
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
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to load routes")
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

// ---- List Routes ----
// GET /api/routes
//
// Defensive pagination (#719/#711 parity): the endpoint is unbounded by
// default, but operators can opt into server-side paging with ?page/&pageSize.
// When ?page is absent the behavior and response shape are byte-identical to
// the legacy surface (a bare JSON array of routes with batched channels) so
// existing frontend code that does not paginate keeps working untouched. Only
// when a non-empty ?page is supplied does the handler apply LIMIT/OFFSET and
// return the {items,total,page,pageSize} envelope used by /api/channels.
func (h *tokenRoutesHandler) listRoutes(w http.ResponseWriter, r *http.Request) {
	pageStr := strings.TrimSpace(r.URL.Query().Get("page"))
	paginate := pageStr != ""

	var rows []map[string]any
	var total int64
	var queryErr error
	page, pageSize := 1, 50
	if paginate {
		page = clampInt(getQueryInt(r, "page", 1), 1, 1_000_000)
		pageSize = clampInt(getQueryInt(r, "pageSize", 50), 1, 200)
		offset := (page - 1) * pageSize
		rows, queryErr = queryRowsErr(h.db,
			"SELECT * FROM token_routes ORDER BY sort_order ASC, id ASC LIMIT ? OFFSET ?",
			pageSize, offset)
		if queryErr != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to load routes")
			return
		}
		// Separate COUNT keeps the row maps free of a window-function column
		// (queryRows MapScans into map[string]any and would echo total_count
		// into every route item otherwise).
		if err := h.db.Get(&total, h.db.Rebind("SELECT COUNT(*) FROM token_routes")); err != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to count routes")
			return
		}
	} else {
		rows, queryErr = queryRowsErr(h.db, "SELECT * FROM token_routes ORDER BY sort_order ASC, id ASC")
		if queryErr != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to load routes")
			return
		}
	}

	result, err := h.enrichRoutesWithChannels(rows, paginate)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to load route channels")
		return
	}

	if paginate {
		writeJSON(w, http.StatusOK, map[string]any{
			"items":    normalizeSlice(result),
			"total":    total,
			"page":     page,
			"pageSize": pageSize,
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// enrichRoutesWithChannels batch-loads channels for the given route rows and
// attaches the enriched channel list to each route. When scopeToPage is true
// (paginated call) the channel query is filtered to the route IDs on the
// current page so a 50-row page does not pull the entire fleet's channels;
// when false (legacy unpaginated call) it loads every channel, matching the
// pre-pagination behavior exactly.
func (h *tokenRoutesHandler) enrichRoutesWithChannels(rows []map[string]any, scopeToPage bool) ([]map[string]any, error) {
	// Select only credential fragments (first4/last4/length) instead of the
	// plaintext access_token/api_token — the full secret never crosses the
	// DB→Go boundary, so a stray slog/metrics call cannot leak it. The masked
	// form is rebuilt in Go by routeChannelAccountPublic().
	accessTokenFrags := credentialFragmentsSelect(h.db, "a.access_token", "access_token")
	apiTokenFrags := credentialFragmentsSelect(h.db, "a.api_token", "api_token")
	channelQuery := `SELECT rc.*, a.username, ` + accessTokenFrags + `, ` + apiTokenFrags + `,
		       a.balance, a.status as account_status,
		        s.id as site_id, s.name as site_name, s.url as site_url, s.platform as site_platform, s.status as site_status
		 FROM route_channels rc
		 LEFT JOIN accounts a ON rc.account_id = a.id
		 LEFT JOIN sites s ON a.site_id = s.id`

	// Collect route IDs to scope the channel batch load to the current page.
	// Scoping only happens on paginated calls; the legacy call keeps the
	// unfiltered load so its result set is identical to the pre-pagination
	// behavior (an IN list of every route ID could also exceed SQLite's
	// bind-parameter ceiling on very large fleets).
	var allChannelRows []map[string]any
	var channelErr error
	if scopeToPage {
		routeIDs := make([]int64, 0, len(rows))
		for _, route := range rows {
			if rid := coerceInt64(route["id"]); rid > 0 {
				routeIDs = append(routeIDs, rid)
			}
		}
		if len(routeIDs) > 0 {
			placeholders := make([]string, len(routeIDs))
			args := make([]any, len(routeIDs))
			for i, id := range routeIDs {
				placeholders[i] = "?"
				args[i] = id
			}
			channelQuery += " WHERE rc.route_id IN (" + strings.Join(placeholders, ",") + ")"
			channelQuery += " ORDER BY rc.route_id ASC, rc.priority ASC, rc.id ASC"
			allChannelRows, channelErr = queryRowsErr(h.db, channelQuery, args...)
		} else {
			channelQuery += " ORDER BY rc.route_id ASC, rc.priority ASC, rc.id ASC"
			allChannelRows, channelErr = queryRowsErr(h.db, channelQuery)
		}
	} else {
		channelQuery += " ORDER BY rc.route_id ASC, rc.priority ASC, rc.id ASC"
		allChannelRows, channelErr = queryRowsErr(h.db, channelQuery)
	}
	if channelErr != nil {
		return nil, channelErr
	}

	channelsByRoute := make(map[int64][]map[string]any)
	for _, ch := range allChannelRows {
		routeID := coerceInt64(ch["routeId"])
		if routeID == 0 {
			routeID = coerceInt64(ch["route_id"])
		}
		enriched := map[string]any{
			"id":               ch["id"],
			"routeId":          ch["routeId"],
			"accountId":        ch["accountId"],
			"tokenId":          ch["tokenId"],
			"oauthRouteUnitId": ch["oauthRouteUnitId"],
			"sourceModel":      ch["sourceModel"],
			"priority":         ch["priority"],
			"weight":           ch["weight"],
			"enabled":          ch["enabled"],
			"manualOverride":   ch["manualOverride"],
			// Same RouteChannel wire contract as GET /api/routes/{id}/channels:
			// runtime hit counts + cooldown must not be dropped from the
			// embedded list.
			"successCount":  ch["successCount"],
			"failCount":     ch["failCount"],
			"cooldownUntil": ch["cooldownUntil"],
			"account":       routeChannelAccountPublic(ch),
			"site": map[string]any{
				"id":       ch["siteId"],
				"name":     ch["siteName"],
				"url":      ch["siteUrl"],
				"platform": ch["sitePlatform"],
				"status":   ch["siteStatus"],
			},
		}
		channelsByRoute[routeID] = append(channelsByRoute[routeID], enriched)
	}

	result := make([]map[string]any, 0, len(rows))
	for _, route := range rows {
		routeID := coerceInt64(route["id"])
		enrichedChannels := channelsByRoute[routeID]
		if enrichedChannels == nil {
			enrichedChannels = []map[string]any{}
		}
		item := route
		item["channels"] = enrichedChannels
		if ds, ok := route["decisionSnapshot"].(string); ok && ds != "" {
			var parsed any
			if json.Unmarshal([]byte(ds), &parsed) == nil {
				item["decisionSnapshot"] = parsed
			} else {
				item["decisionSnapshot"] = nil
			}
		}
		result = append(result, item)
	}
	return result, nil
}

// ---- Create Route ----
// POST /api/routes
func (h *tokenRoutesHandler) createRoute(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ModelPattern    string  `json:"modelPattern"`
		RouteMode       *string `json:"routeMode"`
		DisplayName     *string `json:"displayName"`
		DisplayIcon     *string `json:"displayIcon"`
		SourceRouteIds  []int64 `json:"sourceRouteIds"`
		RoutingStrategy *string `json:"routingStrategy"`
		// ContextLength is optional route metadata (tokens). NULL/omit/0 means unknown;
		// not enforced at proxy runtime in currently (admin surface + /v1/models known limitation).
		ContextLength any   `json:"contextLength"`
		ModelMapping  any   `json:"modelMapping"`
		Enabled       *bool `json:"enabled"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	routeMode := "pattern"
	if body.RouteMode != nil {
		routeMode = strings.TrimSpace(*body.RouteMode)
	}

	modelPattern := strings.TrimSpace(body.ModelPattern)
	displayName := ""
	if body.DisplayName != nil {
		displayName = strings.TrimSpace(*body.DisplayName)
	}

	if routeMode == "explicit_group" {
		modelPattern = displayName
	}

	if routeMode != "explicit_group" && modelPattern == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "model match pattern is required")
		return
	}

	if routeMode == "explicit_group" && displayName == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "explicit groups must specify an external model name")
		return
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	routingStrategy := "weighted"
	if body.RoutingStrategy != nil {
		routingStrategy = *body.RoutingStrategy
	}

	now := time.Now().UTC().Format(time.RFC3339)

	var modelMapping any
	if body.ModelMapping != nil {
		if b, err := json.Marshal(body.ModelMapping); err == nil {
			modelMapping = string(b)
		}
	}

	contextLength := normalizeContextLengthOrNull(body.ContextLength)

	id, err := execInsertID(h.db,
		`INSERT INTO token_routes (model_pattern, display_name, display_icon, route_mode, model_mapping, routing_strategy, context_length, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		modelPattern, strOrNull(&displayName), strOrNull(body.DisplayIcon), routeMode,
		modelMapping, routingStrategy, contextLength, enabled, now, now,
	)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to create route")
		return
	}

	// For explicit_group, insert source route references
	if routeMode == "explicit_group" && len(body.SourceRouteIds) > 0 {
		for _, srcID := range body.SourceRouteIds {
			if _, err := h.db.Exec(h.db.Rebind("INSERT INTO route_group_sources (group_route_id, source_route_id) VALUES (?, ?)"), id, srcID); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to create route group source")
				return
			}
		}
	}

	// Pattern routes: auto-populate channels from exact-model routes + model availability.
	if routeMode != "explicit_group" && modelPattern != "" {
		if _, err := service.PopulateRouteChannelsByModelPattern(r.Context(), h.db, id, modelPattern); err != nil {
			slog.Warn("route create: channel auto-populate failed", "routeId", id, "error", err)
		}
	}

	created := queryRow(h.db, "SELECT * FROM token_routes WHERE id = ?", id)
	if created == nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to create route")
		return
	}
	created["sourceRouteIds"] = body.SourceRouteIds
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, created)
}

// ---- Update Route ----
// PUT /api/routes/:id
func (h *tokenRoutesHandler) updateRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	existing := queryRow(h.db, "SELECT * FROM token_routes WHERE id = ?", id)
	if existing == nil {
		writeErrorWithRequest(w, r, http.StatusNotFound, "route not found")
		return
	}

	var body map[string]any
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate the resulting mode configuration before writing anything —
	// mirrors the createRoute invariants (pattern → modelPattern required,
	// explicit_group → displayName required) so a mode switch can never
	// persist an inconsistent route.
	nextMode, _ := existing["routeMode"].(string)
	if v, ok := body["routeMode"]; ok {
		if s, ok2 := v.(string); ok2 {
			nextMode = strings.TrimSpace(s)
		}
	}
	nextPattern, _ := existing["modelPattern"].(string)
	if v, ok := body["modelPattern"]; ok {
		if s, ok2 := v.(string); ok2 {
			nextPattern = strings.TrimSpace(s)
		}
	}
	nextDisplayName, _ := existing["displayName"].(string)
	if v, ok := body["displayName"]; ok {
		if s, ok2 := v.(string); ok2 {
			nextDisplayName = strings.TrimSpace(s)
		}
	}
	if nextMode != "explicit_group" && nextPattern == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "model match pattern is required")
		return
	}
	if nextMode == "explicit_group" && nextDisplayName == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "explicit groups must specify an external model name")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := h.db.Beginx()
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	if v, ok := body["modelPattern"]; ok {
		if s, ok2 := v.(string); ok2 {
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET model_pattern = ?, updated_at = ? WHERE id = ?"), strings.TrimSpace(s), now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["displayName"]; ok {
		if s, ok2 := v.(string); ok2 {
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET display_name = ?, updated_at = ? WHERE id = ?"), s, now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["displayIcon"]; ok {
		if s, ok2 := v.(string); ok2 {
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET display_icon = ?, updated_at = ? WHERE id = ?"), s, now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["enabled"]; ok {
		if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET enabled = ?, updated_at = ? WHERE id = ?"), toBool(v), now, id); err != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
			return
		}
	}
	if v, ok := body["routeMode"]; ok {
		if s, ok2 := v.(string); ok2 {
			mode := strings.TrimSpace(s)
			if mode != "pattern" && mode != "explicit_group" {
				writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid routeMode. Expected pattern or explicit_group.")
				return
			}
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET route_mode = ?, updated_at = ? WHERE id = ?"), mode, now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["routingStrategy"]; ok {
		if s, ok2 := v.(string); ok2 {
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET routing_strategy = ?, updated_at = ? WHERE id = ?"), s, now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["modelMapping"]; ok {
		mappingJSON, _ := json.Marshal(v)
		if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET model_mapping = ?, updated_at = ? WHERE id = ?"), string(mappingJSON), now, id); err != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
			return
		}
	}
	// contextLength: present key updates (including explicit null/0 clear → NULL).
	// Metadata only — no proxy max-token enforcement is wired from this field yet.
	if v, ok := body["contextLength"]; ok {
		if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET context_length = ?, updated_at = ? WHERE id = ?"), normalizeContextLengthOrNull(v), now, id); err != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
			return
		}
	}

	// Update source route IDs for explicit_group
	if v, ok := body["sourceRouteIds"]; ok {
		if ids, ok2 := v.([]any); ok2 {
			if _, err := tx.Exec(tx.Rebind("DELETE FROM route_group_sources WHERE group_route_id = ?"), id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route group source")
				return
			}
			for _, rawID := range ids {
				switch rid := rawID.(type) {
				case float64:
					if _, err := tx.Exec(tx.Rebind("INSERT INTO route_group_sources (group_route_id, source_route_id) VALUES (?, ?)"), id, int64(rid)); err != nil {
						writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route group source")
						return
					}
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	// Pattern change: recompose automatic channels while preserving manual overrides.
	// Unrelated field updates (displayName, routingStrategy, modelMapping, enabled)
	// must not wipe intentional in-route channel configuration.
	if v, ok := body["modelPattern"]; ok {
		if s, ok2 := v.(string); ok2 {
			nextPattern := strings.TrimSpace(s)
			prevPattern, _ := existing["modelPattern"].(string)
			mode, _ := existing["routeMode"].(string)
			if nextPattern != prevPattern && !routing.IsExplicitGroupRoute(mode) {
				if _, err := service.RebuildTokenRoutesFromAvailability(r.Context(), h.db); err != nil {
					slog.Warn("route update: rebuild after modelPattern change failed", "routeId", id, "error", err)
				}
			}
		}
	}

	updated := queryRow(h.db, "SELECT * FROM token_routes WHERE id = ?", id)
	var srcIDs []int64
	h.db.Select(&srcIDs, h.db.Rebind("SELECT source_route_id FROM route_group_sources WHERE group_route_id = ?"), id)
	updated["sourceRouteIds"] = srcIDs
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, updated)
}

// ---- Delete Route ----
// DELETE /api/routes/:id
func (h *tokenRoutesHandler) deleteRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	tx, err := h.db.Beginx()
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(tx.Rebind("DELETE FROM route_group_sources WHERE group_route_id = ?"), id); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to delete route")
		return
	}
	if _, err := tx.Exec(tx.Rebind("DELETE FROM route_group_sources WHERE source_route_id = ?"), id); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to delete route")
		return
	}
	if _, err := tx.Exec(tx.Rebind("DELETE FROM route_channels WHERE route_id = ?"), id); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to delete route")
		return
	}
	if _, err := tx.Exec(tx.Rebind("DELETE FROM token_routes WHERE id = ?"), id); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to delete route")
		return
	}

	if err := tx.Commit(); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

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
		res, err := h.db.Exec(h.db.Rebind("UPDATE token_routes SET enabled = ?, updated_at = ? WHERE id = ?"), enabled, now, id)
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

// ---- Rebuild Routes ----
// POST /api/routes/rebuild
// Synchronously recomposes pattern-route channels from model availability and
// invalidates the in-process route cache. Response must stay truthful: do not
// claim a background job was queued.
//
// Request body (both fields optional; the frontend always sends it, curl users
// may omit it): {"refreshModels": true, "wait": true}. refreshModels defaults
// to true and first refreshes the upstream model list of every active account
// (same batch semantics as the periodic model-sync: per-account failures are
// counted, never fatal), then rebuilds channels from the refreshed
// availability — this is what the UI's "auto rebuild" promises operators
// (#1024: previously the field was silently ignored, so rebuilds ran against
// stale/empty model lists). wait is accepted for TS contract compatibility;
// this handler has always been synchronous, so queued is always false.
func (h *tokenRoutesHandler) rebuildRoutes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshModels *bool `json:"refreshModels"`
		Wait          bool  `json:"wait"`
	}
	if r.Body != nil {
		body, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if readErr == nil && len(bytes.TrimSpace(body)) > 0 {
			if jsonErr := json.Unmarshal(body, &req); jsonErr != nil {
				slog.Warn("routes rebuild: malformed body, using defaults", "error", jsonErr)
			}
		}
	}
	refreshModels := true
	if req.RefreshModels != nil {
		refreshModels = *req.RefreshModels
	}

	var stats service.RouteRebuildStats
	if refreshModels {
		summary := service.SyncAllAccountModels(r.Context(), h.db)
		if summary.RebuildRan {
			stats = summary.Rebuild
			if summary.RebuildErr != nil {
				writeRebuildFailure(w, summary.RebuildErr)
				return
			}
		} else {
			// No account was refreshed (no candidates, or every refresh
			// failed): still rebuild from whatever availability rows already
			// exist (manual models, previous syncs) — the pass stays useful.
			rebuildStats, err := service.RebuildTokenRoutesFromAvailability(r.Context(), h.db)
			if err != nil {
				writeRebuildFailure(w, err)
				return
			}
			stats = rebuildStats
		}
	} else {
		rebuildStats, err := service.RebuildTokenRoutesFromAvailability(r.Context(), h.db)
		if err != nil {
			writeRebuildFailure(w, err)
			return
		}
		stats = rebuildStats
	}
	shared.RecordRouteRebuildCompleted()
	// Rebuild recomposes route_channels rows; drop the list snapshot so the
	// next GET /api/channels reflects the rebuilt fleet immediately.
	invalidateChannelsSnapshotCache()
	slog.Info("routes rebuild completed",
		"queued", false,
		"status", "completed",
		"routesConsidered", stats.RoutesConsidered,
		"patternRoutes", stats.PatternRoutes,
		"groupRoutes", stats.GroupRoutes,
		"channelsInserted", stats.ChannelsInserted,
		"channelsRemoved", stats.ChannelsRemoved,
	)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":          true,
		"queued":           false,
		"reused":           false,
		"status":           "completed",
		"message":          "route channels rebuilt and cache refreshed",
		"routesConsidered": stats.RoutesConsidered,
		"patternRoutes":    stats.PatternRoutes,
		"groupRoutes":      stats.GroupRoutes,
		"channelsInserted": stats.ChannelsInserted,
		"channelsRemoved":  stats.ChannelsRemoved,
		"channelsKept":     stats.ChannelsKept,
		"changed":          stats.Changed,
	})
}

// writeRebuildFailure emits the truthful failure envelope for rebuild passes.
func writeRebuildFailure(w http.ResponseWriter, err error) {
	slog.Error("routes rebuild failed", "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"success": false,
		"queued":  false,
		"reused":  false,
		"status":  "failed",
		"message": "route rebuild failed",
	})
}

// ---- Get Route Channels ----
// GET /api/routes/:id/channels
func routeDecisionToMap(d routing.RouteDecisionExplanation) map[string]any {
	candidates := make([]map[string]any, 0, len(d.Candidates))
	for _, c := range d.Candidates {
		candidates = append(candidates, map[string]any{
			"channelId":              c.ChannelID,
			"accountId":              c.AccountID,
			"username":               c.Username,
			"siteName":               c.SiteName,
			"tokenName":              c.TokenName,
			"priority":               c.Priority,
			"weight":                 c.Weight,
			"eligible":               c.Eligible,
			"recentlyFailed":         c.RecentlyFailed,
			"avoidedByRecentFailure": c.AvoidedByRecentFailure,
			"probability":            c.Probability,
			"reason":                 c.Reason,
		})
	}
	out := map[string]any{
		"requestedModel": d.RequestedModel,
		"actualModel":    d.ActualModel,
		"matched":        d.Matched,
		"modelPattern":   d.ModelPattern,
		"selectedLabel":  d.SelectedLabel,
		"summary":        d.Summary,
		"candidates":     candidates,
	}
	if d.RouteID != nil {
		out["routeId"] = *d.RouteID
	}
	if d.SelectedChannelID != nil {
		out["selectedChannelId"] = *d.SelectedChannelID
	}
	if d.SelectedAccountID != nil {
		out["selectedAccountId"] = *d.SelectedAccountID
	}
	if d.Summary == nil {
		out["summary"] = []string{}
	}
	return out
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func uniquePositiveInt64(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, v := range values {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// normalizeContextLengthOrNull parses admin camelCase contextLength.
// Contract: null / omit (caller) / "" / 0 / negative → NULL (unknown, no enforcement).
// Positive integers (or numeric strings) are stored as token window metadata only.
func normalizeContextLengthOrNull(input any) any {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case float64:
		if v <= 0 {
			return nil
		}
		return int64(v)
	case float32:
		if v <= 0 {
			return nil
		}
		return int64(v)
	case int:
		if v <= 0 {
			return nil
		}
		return int64(v)
	case int64:
		if v <= 0 {
			return nil
		}
		return v
	case json.Number:
		i, err := v.Int64()
		if err != nil || i <= 0 {
			return nil
		}
		return i
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil || i <= 0 {
			return nil
		}
		return i
	default:
		return nil
	}
}

func strOrNull(s *string) any {
	if s == nil || *s == "" {
		return nil
	}
	return *s
}

func execInsertID(db *sqlx.DB, query string, args ...any) (int64, error) {
	if db.DriverName() == "pgx" {
		var id int64
		err := db.QueryRowx(db.Rebind(query+" RETURNING id"), args...).Scan(&id)
		return id, err
	}

	result, err := db.Exec(db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// routeChannelAccountPublic returns account fields for admin route channel lists
// without plaintext credentials. The masked form is rebuilt from the
// prefix/suffix/length fragments selected by credentialFragmentsSelect() —
// the plaintext access_token/api_token are never pulled from the DB.
func routeChannelAccountPublic(ch map[string]any) map[string]any {
	out := map[string]any{
		"id":       ch["accountId"],
		"username": ch["username"],
		"balance":  ch["balance"],
		"status":   ch["accountStatus"],
	}
	if accessTokenLen := coerceInt64(ch["accessTokenLen"]); accessTokenLen > 0 {
		out["accessTokenMasked"] = maskSecretFromFragments(ch["accessTokenPrefix"], ch["accessTokenSuffix"], accessTokenLen)
	}
	if apiTokenLen := coerceInt64(ch["apiTokenLen"]); apiTokenLen > 0 {
		out["apiTokenMasked"] = maskSecretFromFragments(ch["apiTokenPrefix"], ch["apiTokenSuffix"], apiTokenLen)
	}
	return out
}
