package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/service"
)

type proxyLogFilter struct {
	fingerprint   string
	aggregateFrom string
	where         string
	args          []any
}

func parseProxyLogFilter(r *http.Request) proxyLogFilter {
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
	return proxyLogFilter{
		fingerprint:   fingerprint,
		aggregateFrom: proxyLogsAggregateFrom(siteID, search),
		where:         where,
		args:          args,
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
