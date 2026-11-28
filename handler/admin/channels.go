package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
)

// listChannels returns the aggregated, paginated, read-only channel list (#622).
// Status is derived from routing in-memory breaker state + persisted cooldown
// so the UI never mutates routing health (read-only / soft isolation).
//
// Pagination: explicit ?page / ?pageSize opt into server-side paging
// (page default 1, pageSize default 50, max 200). When NEITHER param is
// present the endpoint returns the full channel list with no LIMIT — the
// channels page paginates client-side, so a hard-coded 50-row default would
// silently truncate fleets larger than 50. The total fleet size comes from
// COUNT(*) OVER () so the pager is accurate even on non-first pages. A 10s
// TTL snapshot cache (mirroring globalAccountsCache) absorbs dashboard
// polling; ?refresh=true bypasses it, and any route_channels mutation clears
// it via invalidateChannelsSnapshotCache().
//
// Response: {items:[...], total:N, page:int, pageSize:int} — same envelope
// shape as /api/checkin/logs and /api/stats/proxy-logs. In unbounded mode
// page=1 and pageSize=total (a single full page).
func (h *tokenRoutesHandler) listChannels(w http.ResponseWriter, r *http.Request) {
	queryParams := r.URL.Query()
	_, hasPage := queryParams["page"]
	_, hasPageSize := queryParams["pageSize"]
	unbounded := !hasPage && !hasPageSize

	page := config.ClampInt(getQueryInt(r, "page", 1), 1, 1_000_000)
	pageSize := config.ClampInt(getQueryInt(r, "pageSize", 50), 1, 200)
	forceRefresh := parseTruthyQuery(queryParams.Get("refresh"))
	statusFilter, err := parseChannelStatusFilter(queryParams.Get("status"))
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, err.Error())
		return
	}
	var cacheKey string
	if unbounded {
		cacheKey = "unbounded"
		if len(statusFilter) > 0 {
			cacheKey += ":" + strings.Join(statusFilter, ",")
		}
	} else {
		cacheKey = fmt.Sprintf("%d:%d:%s", page, pageSize, strings.Join(statusFilter, ","))
	}

	// Snapshot cache: a hit short-circuits; on a miss the single-flight group
	// deduplicates concurrent computes for the same page so N admin sessions
	// polling an expired entry share one 5-way JOIN run instead of running it
	// Nx (thundering herd). ?refresh=true clears the cache first so a force
	// request always recomputes and repopulates.
	if forceRefresh {
		globalChannelsCache.clear()
	}
	data, cacheHit, err := globalChannelsCache.getOrCompute(cacheKey, func() ([]byte, error) {
		return h.computeChannelsPage(r, page, pageSize, unbounded, statusFilter)
	})
	if err != nil {
		slog.Error("channels list failed", "error", err)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load channel list")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if cacheHit {
		w.Header().Set("x-channels-snapshot-cache", "hit")
	} else {
		w.Header().Set("x-channels-snapshot-cache", "miss")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

const channelListBaseQuery = `
	SELECT rc.id, rc.route_id, rc.account_id, rc.token_id, rc.oauth_route_unit_id,
	       rc.source_model, rc.priority, rc.weight, rc.enabled, rc.manual_override,
	       rc.success_count, rc.total_latency_ms, rc.cooldown_until,
	       rc.cooldown_reason_code, rc.cooldown_reason, rc.cooldown_reason_at,
	       COALESCE(a.username, '') AS username,
	       a.site_id,
	       COALESCE(s.name, '') AS site_name,
	       COALESCE(s.status, '') AS site_status,
	       COALESCE(a.status, '') AS account_status,
	       COALESCE(tr.model_pattern, '') AS route_model_pattern,
	       COALESCE(oru.name, '') AS oauth_unit_name,
	       COALESCE(at.name, '') AS token_name,
	       COUNT(*) OVER () AS total_count
	FROM route_channels rc
	LEFT JOIN accounts a ON rc.account_id = a.id
	LEFT JOIN sites s ON a.site_id = s.id
	LEFT JOIN token_routes tr ON rc.route_id = tr.id
	LEFT JOIN oauth_route_units oru ON rc.oauth_route_unit_id = oru.id
	LEFT JOIN account_tokens at ON rc.token_id = at.id
	ORDER BY rc.id ASC`

func (h *tokenRoutesHandler) loadAllChannelRows(r *http.Request) ([]channelListRow, error) {
	var rows []channelListRow
	err := h.db.SelectContext(r.Context(), &rows, channelListBaseQuery)
	return rows, err
}

// computeChannelsPage runs the 5-way JOIN channel list query (the cache-miss
// path). Extracted from listChannels so the single-flight group can
// deduplicate concurrent misses. Uses the caller's request context so a client
// disconnect cancels the in-flight query; under single-flight the leader's
// context is shared (followers arrive while the leader is already running, so
// their own context is not on the critical path). When unbounded is true no
// LIMIT/OFFSET is applied and total is the loaded row count. When a status
// filter is present, runtime status must be derived from in-memory routing
// health, so the complete row set is loaded first and pagination happens in
// memory after the filter. Returns the marshaled JSON bytes; the caller
// (getOrCompute) stores them in the cache.
func (h *tokenRoutesHandler) computeChannelsPage(r *http.Request, page, pageSize int, unbounded bool, statusFilter []string) ([]byte, error) {
	if len(statusFilter) > 0 {
		allRows, err := h.loadAllChannelRows(r)
		if err != nil {
			return nil, err
		}
		var enabledChannels int64
		filtered := make([]channelListRow, 0, len(allRows))
		for _, row := range allRows {
			if row.Enabled {
				enabledChannels++
			}
			if channelStatusMatches(row, statusFilter) {
				filtered = append(filtered, row)
			}
		}
		shared.SetActiveChannels(enabledChannels)

		total := int64(len(filtered))
		start := (page - 1) * pageSize
		if start < 0 {
			start = 0
		}
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + pageSize
		if end > len(filtered) {
			end = len(filtered)
		}
		items := make([]map[string]any, 0, end-start)
		for _, row := range filtered[start:end] {
			items = append(items, channelListItem(row))
		}
		respPageSize := pageSize
		if unbounded {
			// A single full filtered page: report the real row count instead
			// of the paginated default so the envelope never lies about
			// truncation.
			respPageSize = int(total)
		}
		return json.Marshal(map[string]any{
			"items":    normalizeSlice(items),
			"total":    total,
			"page":     page,
			"pageSize": respPageSize,
		})
	}

	var rows []channelListRow
	var err error
	if unbounded {
		rows, err = h.loadAllChannelRows(r)
	} else {
		offset := (page - 1) * pageSize
		err = h.db.SelectContext(r.Context(), &rows, channelListBaseQuery+`
		LIMIT ? OFFSET ?`, pageSize, offset)
	}
	if err != nil {
		return nil, err
	}

	// Refresh the metapi_active_channels gauge so /metrics reflects the real
	// proxy channel count instead of a constant 0. Counted from the same rows
	// we already load for the list (enabled channels = active candidates).
	var enabledChannels int64
	for _, row := range rows {
		if channelEffectivelyEnabled(row.Enabled, row.SiteStatus, row.AccountStatus) {
			enabledChannels++
		}
	}
	shared.SetActiveChannels(enabledChannels)

	items := make([]map[string]any, 0, len(rows))
	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalCount
	} else if unbounded {
		total = 0
	} else {
		// COUNT(*) OVER () is absent from an empty page; fall back to a plain
		// COUNT(*) so a pager past the last row still reports the true total.
		_ = h.db.Get(&total, "SELECT COUNT(*) FROM route_channels")
	}
	for _, row := range rows {
		items = append(items, channelListItem(row))
	}

	respPageSize := pageSize
	if unbounded {
		// A single full page: report the real row count instead of the
		// paginated default so the envelope never lies about truncation.
		respPageSize = int(total)
	}
	resp := map[string]any{
		"items":    normalizeSlice(items),
		"total":    total,
		"page":     page,
		"pageSize": respPageSize,
	}
	return json.Marshal(resp)
}
