package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/routing"
	"golang.org/x/sync/singleflight"
)

// channelListRow is the flattened read-only projection for GET /api/channels.
// It aggregates route_channels across the account/token/oauth_unit dimensions
// and joins only the display fields the channels page needs (no credentials).
type channelListRow struct {
	ID               int64   `db:"id"`
	RouteID          int64   `db:"route_id"`
	AccountID        int64   `db:"account_id"`
	TokenID          *int64  `db:"token_id"`
	OAuthRouteUnitID *int64  `db:"oauth_route_unit_id"`
	SourceModel      *string `db:"source_model"`
	// Priority/Weight/SuccessCount/TotalLatencyMs are nullable in route_channels
	// (DEFAULT without NOT NULL): NULL must scan to nil, not crash the list
	// with "converting NULL to int64" (#933 family).
	Priority       *int64  `db:"priority"`
	Weight         *int64  `db:"weight"`
	Enabled        bool    `db:"enabled"`
	ManualOverride bool    `db:"manual_override"`
	SuccessCount   *int64  `db:"success_count"`
	TotalLatencyMs *int64  `db:"total_latency_ms"`
	CooldownUntil  *string `db:"cooldown_until"`
	// Structured cooldown reason (P0-3): why the channel cooled down. NULL on
	// rows cooled before the reason columns existed.
	CooldownReasonCode *string `db:"cooldown_reason_code"`
	CooldownReason     *string `db:"cooldown_reason"`
	CooldownReasonAt   *string `db:"cooldown_reason_at"`
	Username           string  `db:"username"`
	SiteID             int64   `db:"site_id"`
	SiteName           string  `db:"site_name"`
	RouteModelPattern  string  `db:"route_model_pattern"`
	OAuthUnitName      string  `db:"oauth_unit_name"`
	TokenName          string  `db:"token_name"`
	// TotalCount is the COUNT(*) OVER () window result: the total number of
	// route_channels rows matching the FROM/JOINs ignoring LIMIT/OFFSET, so the
	// pager can show the true fleet size on every page. Identical across all
	// rows in a page; read from the first row (or a fallback COUNT when empty).
	TotalCount int64 `db:"total_count"`
}

// channelsSnapshotCache is an in-memory TTL cache for GET /api/channels pages.
// Mirrors globalAccountsCache: a short-TTL snapshot so a fleet-wide 5-way JOIN
// list does not run on every dashboard poll. Entries are keyed by "page:pageSize"
// so distinct page requests don't shadow each other. ?refresh=true bypasses.
type channelsSnapshotCache struct {
	mu        sync.RWMutex
	key       string
	data      []byte
	expiresAt time.Time
	ttl       time.Duration
	// flight deduplicates concurrent cache-miss computes for the same page so
	// N admin sessions polling an expired entry share one 5-way JOIN run
	// instead of running it N× (thundering herd).
	flight singleflight.Group
}

func (c *channelsSnapshotCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data != nil && c.key == key && time.Now().Before(c.expiresAt) {
		return c.data, true
	}
	return nil, false
}

func (c *channelsSnapshotCache) set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.key = key
	c.data = data
	c.expiresAt = time.Now().Add(c.ttl)
}

func (c *channelsSnapshotCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = nil
	c.key = ""
	c.expiresAt = time.Time{}
}

// getOrCompute returns the cached payload for key, or computes it via the
// supplied function under single-flight dedup so N concurrent admin sessions
// hitting a cold/expired entry share one 5-way JOIN run instead of running it
// N× (thundering herd). Returns (data, hit, err): hit reports whether the
// fast-path cache served the bytes (for the x-channels-snapshot-cache response
// header). Only successful computes are stored, so errors never poison the cache.
func (c *channelsSnapshotCache) getOrCompute(key string, compute func() ([]byte, error)) ([]byte, bool, error) {
	if cached, hit := c.get(key); hit {
		return cached, true, nil
	}
	result, err, _ := c.flight.Do(key, func() (any, error) {
		// Re-check: a concurrent leader may have populated the cache while we
		// waited for the single-flight slot.
		if cached, hit := c.get(key); hit {
			return cached, nil
		}
		data, err := compute()
		if err != nil {
			return nil, err
		}
		c.set(key, data)
		return data, nil
	})
	if err != nil {
		return nil, false, err
	}
	return result.([]byte), false, nil
}

var globalChannelsCache = &channelsSnapshotCache{ttl: 10 * time.Second}
var globalChannelsErrorSummaryCache = &channelsSnapshotCache{ttl: 10 * time.Second}

// invalidateChannelsSnapshotCache clears the channels list snapshot cache and
// the error-summary cache. Called from every route_channels mutation so a
// freshly edited fleet shows up immediately instead of up to ttl (10s) later.
func invalidateChannelsSnapshotCache() {
	globalChannelsCache.clear()
	globalChannelsErrorSummaryCache.clear()
}

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

	page := clampInt(getQueryInt(r, "page", 1), 1, 1_000_000)
	pageSize := clampInt(getQueryInt(r, "pageSize", 50), 1, 200)
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
	err := h.db.SelectContext(r.Context(), &rows, h.db.Rebind(channelListBaseQuery))
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
		err = h.db.SelectContext(r.Context(), &rows, h.db.Rebind(channelListBaseQuery+`
		LIMIT ? OFFSET ?`), pageSize, offset)
	}
	if err != nil {
		return nil, err
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
	var total int64
	if len(rows) > 0 {
		total = rows[0].TotalCount
	} else if unbounded {
		total = 0
	} else {
		// COUNT(*) OVER () is absent from an empty page; fall back to a plain
		// COUNT(*) so a pager past the last row still reports the true total.
		_ = h.db.Get(&total, h.db.Rebind("SELECT COUNT(*) FROM route_channels"))
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

var validChannelStatuses = map[string]struct{}{
	routing.ChannelStatusEnabled:          {},
	routing.ChannelStatusCooldown:         {},
	routing.ChannelStatusBreakerOpen:      {},
	routing.ChannelStatusManuallyDisabled: {},
}

func parseChannelStatusFilter(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	statuses := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("invalid channel status filter")
		}
		if _, ok := validChannelStatuses[part]; !ok {
			return nil, fmt.Errorf("invalid channel status filter: %q", part)
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		statuses = append(statuses, part)
	}
	return statuses, nil
}

func channelStatusForRow(row channelListRow) string {
	sourceModel := ""
	if row.SourceModel != nil {
		sourceModel = *row.SourceModel
	}
	return routing.ChannelRuntimeStatus(row.SiteID, sourceModel, row.Enabled, row.CooldownUntil)
}

func channelStatusMatches(row channelListRow, statusFilter []string) bool {
	status := channelStatusForRow(row)
	for _, wanted := range statusFilter {
		if status == wanted {
			return true
		}
	}
	return false
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
	if row.SuccessCount != nil && *row.SuccessCount > 0 && row.TotalLatencyMs != nil && *row.TotalLatencyMs > 0 {
		ms := *row.TotalLatencyMs / *row.SuccessCount
		responseMs = &ms
	}

	return map[string]any{
		"id":                 row.ID,
		"routeId":            row.RouteID,
		"name":               name,
		"site":               map[string]any{"id": row.SiteID, "name": row.SiteName},
		"type":               chType,
		"status":             routing.ChannelRuntimeStatus(row.SiteID, sourceModel, row.Enabled, row.CooldownUntil),
		"models":             models,
		"priority":           row.Priority,
		"weight":             row.Weight,
		"responseMs":         responseMs,
		"cooldownUntil":      row.CooldownUntil,
		"cooldownReasonCode": row.CooldownReasonCode,
		"cooldownReason":     row.CooldownReason,
		"cooldownReasonAt":   row.CooldownReasonAt,
		"enabled":            row.Enabled,
		"manualOverride":     row.ManualOverride,
	}
}

type channelSummaryRow struct {
	SiteID            int64   `db:"site_id"`
	SourceModel       *string `db:"source_model"`
	RouteModelPattern string  `db:"route_model_pattern"`
	Enabled           bool    `db:"enabled"`
	CooldownUntil     *string `db:"cooldown_until"`
}

// channelErrorSummary handles GET /api/channels/error-summary. The summary is
// read-only and deliberately separate from the list page: it computes the
// fleet-wide runtime status counts that cannot be derived from a SQL aggregate
// because breaker state lives in the routing in-memory health maps.
func (h *tokenRoutesHandler) channelErrorSummary(w http.ResponseWriter, r *http.Request) {
	if parseTruthyQuery(r.URL.Query().Get("refresh")) {
		globalChannelsErrorSummaryCache.clear()
	}
	data, cacheHit, err := globalChannelsErrorSummaryCache.getOrCompute("summary", func() ([]byte, error) {
		return h.computeChannelErrorSummary(r)
	})
	if err != nil {
		slog.Error("channels error summary failed", "error", err)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load channel error summary")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if cacheHit {
		w.Header().Set("x-channels-error-summary-cache", "hit")
	} else {
		w.Header().Set("x-channels-error-summary-cache", "miss")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *tokenRoutesHandler) computeChannelErrorSummary(r *http.Request) ([]byte, error) {
	var rows []channelSummaryRow
	err := h.db.SelectContext(r.Context(), &rows, h.db.Rebind(`
		SELECT a.site_id, rc.source_model,
		       COALESCE(tr.model_pattern, '') AS route_model_pattern,
		       rc.enabled, rc.cooldown_until
		FROM route_channels rc
		LEFT JOIN accounts a ON rc.account_id = a.id
		LEFT JOIN token_routes tr ON rc.route_id = tr.id`))
	if err != nil {
		return nil, err
	}

	byStatus := map[string]int64{
		routing.ChannelStatusEnabled:          0,
		routing.ChannelStatusCooldown:         0,
		routing.ChannelStatusBreakerOpen:      0,
		routing.ChannelStatusManuallyDisabled: 0,
	}
	var total int64
	var errorCount int64
	for _, row := range rows {
		modelName := ""
		if row.SourceModel != nil {
			modelName = *row.SourceModel
		}
		status := routing.ChannelRuntimeStatus(row.SiteID, modelName, row.Enabled, row.CooldownUntil)
		total++
		byStatus[status]++
		if status == routing.ChannelStatusCooldown || status == routing.ChannelStatusBreakerOpen {
			errorCount++
		}
	}
	return json.Marshal(map[string]any{
		"total":      total,
		"errorCount": errorCount,
		"byStatus":   byStatus,
	})
}
