package admin

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	"golang.org/x/sync/singleflight"
)

// proxyLogsSummary is the five-way aggregate behind the proxy-logs summary
// strip (total/success/failed counts + cost + effective tokens). It is the
// most expensive query on the page — five SUM/CASE passes over the filtered
// proxy_logs table (~0.6-1.0s at 500k rows on the audit fixture) — so it is
// cached per filter fingerprint below.
type proxyLogsSummary struct {
	TotalCount     int     `db:"total_count"`
	SuccessCount   int     `db:"success_count"`
	FailedCount    int     `db:"failed_count"`
	TotalCost      float64 `db:"total_cost"`
	TotalTokensAll int64   `db:"total_tokens_all"`
}

// proxyLogsSummaryCacheMaxEntries bounds the fingerprint map. Entries expire
// after the TTL anyway; the cap only protects against a burst of distinct
// filter combinations inside one TTL window. On overflow the expired entries
// are pruned first, then the whole map is dropped (a 10s-TTL cache rebuilds
// quickly; bounded memory beats LRU bookkeeping here).
const proxyLogsSummaryCacheMaxEntries = 128

// proxyLogsSummaryCache is a short-TTL in-memory cache for the proxy-logs
// summary aggregate, keyed by the request's filter fingerprint (every query
// param that shapes the WHERE clause; limit/offset excluded). Mirrors the
// dashboardSummaryCache pattern: RWMutex-guarded entries + single-flight
// dedup so N admin sessions polling an expired fingerprint share one
// aggregation run instead of running it N× (thundering herd).
//
// Keying by fingerprint (not by view) is the point: pagination only changes
// offset, and the list page's split view=query + view=meta fetches carry the
// same filters, so one TTL window computes the summary once per filter set —
// the pre-fix page computed it twice per load (full + meta) plus once per
// page turn. Only successful computes are cached; errors bypass the cache.
// ?force=1 / ?refresh=1 clears it (admin "force refresh").
type proxyLogsSummaryCache struct {
	mu      sync.RWMutex
	entries map[string]proxyLogsSummaryEntry
	ttl     time.Duration
	// flight deduplicates concurrent cache-miss computes for the same
	// fingerprint (see dashboardSummaryCache.flight).
	flight singleflight.Group
}

type proxyLogsSummaryEntry struct {
	summary   proxyLogsSummary
	expiresAt time.Time
}

func (c *proxyLogsSummaryCache) get(key string) (proxyLogsSummary, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return proxyLogsSummary{}, false
	}
	return e.summary, true
}

func (c *proxyLogsSummaryCache) set(key string, summary proxyLogsSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]proxyLogsSummaryEntry)
	}
	if len(c.entries) >= proxyLogsSummaryCacheMaxEntries {
		now := time.Now()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= proxyLogsSummaryCacheMaxEntries {
			// All entries still live: drop the lot rather than grow unbounded.
			c.entries = make(map[string]proxyLogsSummaryEntry)
		}
	}
	c.entries[key] = proxyLogsSummaryEntry{summary: summary, expiresAt: time.Now().Add(c.ttl)}
}

func (c *proxyLogsSummaryCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

// getOrCompute returns the cached summary for key, or computes it via the
// supplied function under single-flight dedup. Returns (summary, hit, err):
// hit reports whether the fast-path cache served the value (for the
// x-proxy-logs-summary-cache response header). A single-flight-shared
// compute is reported as a miss since this request did not get an instant
// cached response — only successful computes are stored, so errors never
// poison the cache.
func (c *proxyLogsSummaryCache) getOrCompute(key string, compute func() (proxyLogsSummary, error)) (proxyLogsSummary, bool, error) {
	if cached, hit := c.get(key); hit {
		return cached, true, nil
	}
	result, err, _ := c.flight.Do(key, func() (any, error) {
		// Re-check: a concurrent leader may have populated the cache while we
		// waited for the single-flight slot.
		if cached, hit := c.get(key); hit {
			return cached, nil
		}
		summary, err := compute()
		if err != nil {
			return proxyLogsSummary{}, err
		}
		c.set(key, summary)
		return summary, nil
	})
	if err != nil {
		return proxyLogsSummary{}, false, err
	}
	return result.(proxyLogsSummary), false, nil
}

var globalProxyLogsSummaryCache = &proxyLogsSummaryCache{ttl: 10 * time.Second}

// ---- Proxy Logs ----
// GET /api/stats/proxy-logs?view=&limit=&offset=&status=&search=&client=&siteId=&channelId=&downstreamKeyId=&from=&to=&latencyMin=&latencyMax=
//
// All list filters (status/search/client/siteId/channelId/from/to/latencyMin/
// latencyMax) are applied SERVER-SIDE so that items, total, and summary share
// one `where`/`args` slice and never disagree (the previous client-side latency
// filter produced a wrong `total` + broken manualPagination). The `client`
// param matches `client_family` exactly; `from`/`to` compare against
// `created_at` (RFC3339 TEXT, lexicographic compare is correct); latency
// bounds compare against `latency_ms` (INTEGER); `channelId` matches the
// nullable `channel_id` (INTEGER) of the route_channels row that served the
// request, which lets operators answer "why is this channel failing" without
// a global search.
func (h *statsHandler) proxyLogs(w http.ResponseWriter, r *http.Request) {
	view := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("view")))
	if view != "query" && view != "meta" {
		view = "full"
	}

	limit, _ := parseLimitOffset(r, 50, 100)
	offset := max(0, getQueryInt(r, "offset", 0))
	status := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	search := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("search")))
	siteID := getQueryInt(r, "siteId", 0)
	channelID := getQueryInt(r, "channelId", 0)
	downstreamKeyID := getQueryInt(r, "downstreamKeyId", 0)
	client := strings.TrimSpace(r.URL.Query().Get("client"))
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	latencyMin := getQueryInt(r, "latencyMin", 0)
	latencyMax := getQueryInt(r, "latencyMax", 0)

	// ?force=1 / ?refresh=1 is the admin "force refresh" hook (the meta
	// client speaks `refresh`, the dashboard convention is `force`; both are
	// honored): invalidate every cached summary fingerprint so this request
	// recomputes from the DB.
	if parseDashboardForceRefresh(r.URL.Query().Get("force")) ||
		parseDashboardForceRefresh(r.URL.Query().Get("refresh")) {
		globalProxyLogsSummaryCache.clear()
	}

	// Fingerprint keys the summary cache: every filter that shapes the WHERE
	// clause, spelled the same way the conditions below parse them. limit and
	// offset are deliberately excluded — the summary is page-independent, so
	// pagination and the split view=query + view=meta fetches all reuse one
	// cached aggregate per filter set per TTL window.
	fingerprint := strings.Join([]string{
		"status=" + status,
		"search=" + search,
		"siteId=" + strconv.Itoa(siteID),
		"channelId=" + strconv.Itoa(channelID),
		"downstreamKeyId=" + strconv.Itoa(downstreamKeyID),
		"client=" + client,
		"from=" + from,
		"to=" + to,
		"latencyMin=" + strconv.Itoa(latencyMin),
		"latencyMax=" + strconv.Itoa(latencyMax),
	}, "&")

	var conditions []string
	var args []any

	if status == "success" {
		conditions = append(conditions, "pl.status = 'success'")
	} else if status == "failed" {
		conditions = append(conditions, "COALESCE(pl.status, '') <> 'success'")
	}
	if search != "" {
		// Search is the only free-text slice across models / account username /
		// downstream token (value or name). All three columns are joined at the
		// query level so items, total and summary stay in agreement; the
		// fingerprint already carries `search` so the summary cache is keyed
		// per search string exactly like the other filters.
		conditions = append(conditions, "(LOWER(COALESCE(pl.model_requested, '')) LIKE ? OR LOWER(COALESCE(pl.model_actual, '')) LIKE ? OR LOWER(COALESCE(a.username, '')) LIKE ? OR LOWER(COALESCE(dk.key, '')) LIKE ? OR LOWER(COALESCE(dk.name, '')) LIKE ?)")
		like := "%" + search + "%"
		args = append(args, like, like, like, like, like)
	}
	if siteID > 0 {
		conditions = append(conditions, "s.id = ?")
		args = append(args, siteID)
	}
	// channelId narrows to the route_channels row that served the request
	// ("why is this channel failing"). `channel_id` is a nullable INTEGER, so
	// an equality match on a positive id naturally excludes the NULL rows
	// written before channel attribution existed. Integer comparison is
	// dialect-neutral (no boolean/COALESCE literal), so SQLite and PostgreSQL
	// agree without a dialect branch.
	if channelID > 0 {
		conditions = append(conditions, "pl.channel_id = ?")
		args = append(args, channelID)
	}
	// downstreamKeyId narrows to the downstream key that made the request —
	// the "this key's traffic" drill-down linked from the keys page and the
	// connect dialog's verify outcome. Same nullable-INTEGER semantics as
	// channelId: rows without key attribution (legacy rows, admin traffic)
	// carry NULL and are naturally excluded.
	if downstreamKeyID > 0 {
		conditions = append(conditions, "pl.downstream_api_key_id = ?")
		args = append(args, downstreamKeyID)
	}
	// client filters on the upstream client family detected from the
	// User-Agent (e.g. "openai-node"). Exact match: the dropdown sends a
	// discrete value from clientOptions, not a free-text substring.
	if client != "" {
		conditions = append(conditions, "pl.client_family = ?")
		args = append(args, client)
	}
	// from/to bound created_at (RFC3339 TEXT). Lexicographic ordering of
	// RFC3339 strings is chronological, so `>=` / `<=` compare correctly
	// without parsing to a datetime type — matches how rows are inserted
	// (time.Now().UTC().Format(time.RFC3339) in handler/proxy/proxy_log.go).
	if from != "" {
		conditions = append(conditions, "pl.created_at >= ?")
		args = append(args, from)
	}
	if to != "" {
		conditions = append(conditions, "pl.created_at <= ?")
		args = append(args, to)
	}
	// latency_ms bounds for the "Slow only" filter. 0 means unset (the
	// frontend sends null/undefined → omitted from the query string).
	if latencyMin > 0 {
		conditions = append(conditions, "pl.latency_ms >= ?")
		args = append(args, latencyMin)
	}
	if latencyMax > 0 {
		conditions = append(conditions, "pl.latency_ms <= ?")
		args = append(args, latencyMax)
	}

	var where string
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	queryPayload := map[string]any{
		"items":    []any{},
		"total":    0,
		"page":     (offset / limit) + 1,
		"pageSize": limit,
	}
	metaPayload := map[string]any{
		"clientOptions": []any{},
		"summary": map[string]any{
			"totalCount":     0,
			"successCount":   0,
			"failedCount":    0,
			"totalCost":      0.0,
			"totalTokensAll": 0,
		},
		"sites": []any{},
	}

	if view == "query" || view == "full" {
		// The downstream-key join is unconditional: beyond the search
		// condition it feeds the token column and the key drill-down chip
		// (downstreamKeyId/downstreamKeyName on every row). LEFT JOIN keeps
		// unattributed rows intact; the aggregate COUNT path still picks its
		// join shape per active filter via proxyLogsAggregateFrom.
		query := `SELECT pl.*, a.username, s.id as site_id, s.name as site_name, s.url as site_url,
			 dk.id AS downstream_key_id, dk.name AS downstream_key_name
			FROM proxy_logs pl
			LEFT JOIN accounts a ON pl.account_id = a.id
			LEFT JOIN sites s ON a.site_id = s.id
			LEFT JOIN downstream_api_keys dk ON pl.downstream_api_key_id = dk.id` + where +
			" ORDER BY pl.created_at DESC LIMIT ? OFFSET ?"
		qArgs := make([]any, len(args))
		copy(qArgs, args)
		qArgs = append(qArgs, limit, offset)
		items, err := queryRowsErr(h.db, query, qArgs...)
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load proxy logs")
			return
		}
		for _, row := range items {
			normalizeProxyLogRow(row)
			omitProxyAttemptDetail(row)
		}
		queryPayload["items"] = normalizeSlice(items)

		var total int
		// The aggregate COUNT runs off the conditional FROM (no accounts/sites
		// join unless the site filter needs s.id) — at 500k rows the join
		// roughly doubled the count time on the audit fixture.
		countQuery := "SELECT COUNT(*) " + proxyLogsAggregateFrom(siteID, search) + where
		h.db.Get(&total, countQuery, args...)
		queryPayload["total"] = total
	}

	// summaryCacheHit is reported via the x-proxy-logs-summary-cache response
	// header for meta/full views (the views that compute the summary).
	summaryCacheHit := false
	if view == "meta" || view == "full" {
		// The five-way summary aggregate is the expensive part of this
		// endpoint (~0.6-1.0s at 500k rows): it is cached per filter
		// fingerprint with single-flight dedup, mirroring the dashboard
		// summary cache. The list page's view=query + view=meta split carries
		// the same fingerprint, so the summary computes once per filter set
		// per TTL window instead of twice per page load.
		summary, cacheHit, summaryErr := globalProxyLogsSummaryCache.getOrCompute(fingerprint, func() (proxyLogsSummary, error) {
			// Use effective token expression so partial logs are not under-counted,
			// and never sum prompt+completion on top of total_tokens.
			summaryQuery := proxyLogsSummaryQuerySQL(proxyLogsAggregateFrom(siteID, search), where)
			var s proxyLogsSummary
			if err := h.db.Get(&s, summaryQuery, args...); err != nil {
				return proxyLogsSummary{}, err
			}
			return s, nil
		})
		if summaryErr != nil {
			// Fail-open preserves the pre-cache behavior: a failed summary
			// degraded silently to zeroes rather than blanking the whole list
			// page. Errors are never cached, so the next request retries the DB.
			slog.Error("proxy logs summary query failed; degrading to zero summary", "error", summaryErr)
			summary = proxyLogsSummary{}
		}
		summaryCacheHit = cacheHit
		metaPayload["summary"] = map[string]any{
			"totalCount":     summary.TotalCount,
			"successCount":   summary.SuccessCount,
			"failedCount":    summary.FailedCount,
			"totalCost":      roundMicro(summary.TotalCost),
			"totalTokensAll": summary.TotalTokensAll,
		}

		sites, err := queryRowsErr(h.db, "SELECT id, name, status FROM sites")
		if err != nil {
			writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load sites")
			return
		}
		metaPayload["sites"] = normalizeSlice(sites)
	}

	if view == "query" {
		writeJSON(w, http.StatusOK, queryPayload)
		return
	}

	if summaryCacheHit {
		w.Header().Set("x-proxy-logs-summary-cache", "hit")
	} else {
		w.Header().Set("x-proxy-logs-summary-cache", "miss")
	}
	if view == "meta" {
		writeJSON(w, http.StatusOK, metaPayload)
	} else {
		result := queryPayload
		result["clientOptions"] = metaPayload["clientOptions"]
		result["summary"] = metaPayload["summary"]
		result["sites"] = metaPayload["sites"]
		writeJSON(w, http.StatusOK, result)
	}
}

// proxyLogsAggregateFrom returns the FROM clause for the proxy-logs COUNT and
// summary aggregates. The only conditions that reference a joined table are
// the site filter (s.id) and the search filter (a.username / dk.key / dk.name),
// so without either the aggregates run straight off proxy_logs — at 500k rows
// the audit measured the LEFT JOINs adding ~90ms to the summary and ~100ms to
// the COUNT. With either active the joins stay (and the summary query's
// CASE/COALESCE never needs the join for row preservation: it is a LEFT JOIN
// on primary keys).
//
// Site filter (siteID > 0): accounts and sites join as INNER, not LEFT. The
// WHERE clause `s.id = ?` already discards any row whose account/site join did
// not match, so INNER returns the identical row set while freeing the planner
// to reorder the join. LEFT pinned the scan to proxy_logs first (a full scan of
// the largest table, then join); INNER lets SQLite drive from the site's small
// account set (accounts_site_id_idx) into proxy_logs_account_created_at_idx —
// ~10x faster for the COUNT/summary on the 300k-row audit fixture. The
// downstream_api_keys join stays LEFT because only the search filter reads it
// and it must not drop rows for the row-preserving summary CASEs.
func proxyLogsAggregateFrom(siteID int, search string) string {
	if siteID > 0 {
		return "FROM proxy_logs pl INNER JOIN accounts a ON pl.account_id = a.id INNER JOIN sites s ON a.site_id = s.id LEFT JOIN downstream_api_keys dk ON pl.downstream_api_key_id = dk.id"
	}
	if search != "" {
		return "FROM proxy_logs pl LEFT JOIN accounts a ON pl.account_id = a.id LEFT JOIN sites s ON a.site_id = s.id LEFT JOIN downstream_api_keys dk ON pl.downstream_api_key_id = dk.id"
	}
	return "FROM proxy_logs pl"
}

// proxyLogsSummaryQuerySQL builds the five-way summary aggregate over the
// given FROM clause + WHERE suffix. Kept a pure string builder (no db handle)
// so tests can assert the join-free shape without a live database.
func proxyLogsSummaryQuerySQL(from, where string) string {
	return `SELECT COUNT(*) as total_count,
		COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END), 0) as success_count,
		COALESCE(SUM(CASE WHEN COALESCE(pl.status, '') <> 'success' THEN 1 ELSE 0 END), 0) as failed_count,
		COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) as total_cost,
		COALESCE(SUM(` + service.EffectiveProxyTokensSQL + `), 0) as total_tokens_all
		` + from + where
}
