package admin

import (
	"log/slog"
	"net/http"
	"strings"
)

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
	filter := parseProxyLogFilter(r)

	// ?force=1 / ?refresh=1 is the admin "force refresh" hook (the meta
	// client speaks `refresh`, the dashboard convention is `force`; both are
	// honored): invalidate every cached summary fingerprint so this request
	// recomputes from the DB.
	if parseDashboardForceRefresh(r.URL.Query().Get("force")) ||
		parseDashboardForceRefresh(r.URL.Query().Get("refresh")) {
		globalProxyLogsSummaryCache.clear()
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
			LEFT JOIN downstream_api_keys dk ON pl.downstream_api_key_id = dk.id` + filter.where +
			" ORDER BY pl.created_at DESC LIMIT ? OFFSET ?"
		qArgs := make([]any, len(filter.args))
		copy(qArgs, filter.args)
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
		countQuery := "SELECT COUNT(*) " + filter.aggregateFrom + filter.where
		h.db.Get(&total, countQuery, filter.args...)
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
		summary, cacheHit, summaryErr := globalProxyLogsSummaryCache.getOrCompute(filter.fingerprint, func() (proxyLogsSummary, error) {
			// Use effective token expression so partial logs are not under-counted,
			// and never sum prompt+completion on top of total_tokens.
			summaryQuery := proxyLogsSummaryQuerySQL(filter.aggregateFrom, filter.where)
			var s proxyLogsSummary
			if err := h.db.Get(&s, summaryQuery, filter.args...); err != nil {
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
