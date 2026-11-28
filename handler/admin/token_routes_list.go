package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

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
		page = config.ClampInt(getQueryInt(r, "page", 1), 1, 1_000_000)
		pageSize = config.ClampInt(getQueryInt(r, "pageSize", 50), 1, 200)
		offset := (page - 1) * pageSize
		rows, queryErr = queryRowsErr(h.db,
			"SELECT * FROM token_routes ORDER BY sort_order ASC, id ASC LIMIT ? OFFSET ?",
			pageSize, offset)
		if queryErr != nil {
			writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load routes")
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
			writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load routes")
			return
		}
	}

	result, err := h.enrichRoutesWithChannels(rows, paginate)
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load route channels")
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
	accessTokenFrags := store.CredentialFragmentsSelect(h.db, "a.access_token", "access_token")
	apiTokenFrags := store.CredentialFragmentsSelect(h.db, "a.api_token", "api_token")
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

// routeChannelAccountPublic returns account fields for admin route channel lists
// without plaintext credentials. The masked form is rebuilt from the
// prefix/suffix/length fragments selected by store.CredentialFragmentsSelect() —
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

// routeChannelTokenPublic exposes only the display identity of a bound token.
// The route-channel list needs the token name to distinguish several keys on
// the same account; the secret itself stays in account_tokens and never
// enters this response.
func routeChannelTokenPublic(ch map[string]any) map[string]any {
	tokenID := coerceInt64(ch["tokenId"])
	if tokenID <= 0 {
		return nil
	}
	return map[string]any{
		"id":        tokenID,
		"name":      ch["tokenName"],
		"accountId": ch["accountId"],
		"enabled":   store.RowBool(ch["tokenEnabled"]),
		"isDefault": store.RowBool(ch["tokenIsDefault"]),
	}
}
