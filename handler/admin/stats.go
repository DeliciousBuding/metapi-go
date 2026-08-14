package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service"
	dailyservice "github.com/deliciousbuding/metapi-go/service/daily"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterStatsRoutes registers all /api/stats and /api/models routes.
func RegisterStatsRoutes(r chi.Router, db *sqlx.DB) {
	handler := &statsHandler{db: db}

	r.Get("/api/stats/dashboard", handler.dashboard)
	r.Get("/api/stats/proxy-logs", handler.proxyLogs)
	r.Get("/api/stats/proxy-logs/{id}", handler.proxyLogDetail)
	r.Get("/api/stats/proxy-debug/traces", handler.debugTraces)
	r.Get("/api/stats/proxy-debug/traces/{id}", handler.debugTraceDetail)
	r.Get("/api/stats/site-distribution", handler.siteDistribution)
	r.Get("/api/stats/site-trend", handler.siteTrend)
	r.Get("/api/stats/model-by-site", handler.modelBySite)
	// Usage density heatmap + slow-request ranking.
	r.Get("/api/stats/usage-heatmap", handler.usageHeatmap)
	r.Get("/api/stats/slow-requests", handler.slowRequests)
	// A1: per-account daily balance history series.
	r.Get("/api/stats/balance-history", handler.balanceHistory)
	// A3: income vs outcome balance analysis.
	r.Get("/api/stats/balance-income-outcome", handler.balanceIncomeOutcome)
	// B1: severity-ranked actionable attention items.
	r.Get("/api/stats/attention", handler.attention)
	// A2: model cost distribution + latency chart gallery.
	r.Get("/api/stats/model-cost-distribution", handler.modelCostDistribution)
	r.Get("/api/stats/latency-histogram", handler.latencyHistogram)
	r.Get("/api/stats/latency-trend", handler.latencyTrend)
	// Cross-site effective model price comparison (admin).
	// Both paths are registered for discoverability; they share one handler.
	r.Get("/api/stats/model-prices", handler.modelPriceCompare)

	// Model routes under /api/models
	r.Get("/api/models/marketplace", handler.marketplace)
	r.Get("/api/models/price-compare", handler.modelPriceCompare)
	r.Get("/api/models/token-candidates", handler.tokenCandidates)
	r.Post("/api/models/check/{accountId}", handler.modelCheck)
	r.Post("/api/models/probe", handler.modelProbe)
	// G1: batch model verification + history.
	r.Post("/api/models/verify-batch", handler.verifyBatch)
	r.Get("/api/models/verify-history", handler.verifyHistory)
}

// RegisterDownstreamPricingRoutes mounts the cross-site price catalog behind
// downstream-key (ProxyAuth) auth, NOT admin auth. N2 productization: a
// downstream consumer (中转站) holding a managed key can query effective
// cross-site model pricing for its own planning — the aggregator's
// transparent-pricing differentiator. Reuses modelPriceCompare so the data
// surface is identical to the admin view; no separate catalog to drift.

// Mounted under /v1/* so it inherits ProxyAuth + CORS from the /v1 route group.
func RegisterDownstreamPricingRoutes(r chi.Router, db *sqlx.DB) {
	handler := &statsHandler{db: db}
	r.Get("/v1/pricing", handler.modelPriceCompare)
	r.Get("/v1/models/price-compare", handler.modelPriceCompare)
}

type statsHandler struct {
	db *sqlx.DB
}

// dashboardSummaryCache is a short-TTL in-memory cache for the dashboard
// summary endpoint. Dashboard aggregation queries (COUNT/SUM over 24h of
// proxy_logs) are expensive enough to dedup rapid reloads but cheap enough
// that a 10s window keeps near-real-time semantics. Mirrors the
// accountsSnapshotCache pattern: RWMutex-guarded bytes + expiry timestamp.
//
// Keyed by view (summary|insights|full) so the three response shapes never
// collide. Only successful responses are cached; error paths bypass the cache.
// ?force=1 clears the cache (admin "force refresh").
type dashboardSummaryCache struct {
	mu      sync.RWMutex
	entries map[string]dashboardCacheEntry
	ttl     time.Duration
}

type dashboardCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

func (c *dashboardSummaryCache) get(view string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[view]
	if !ok || len(e.data) == 0 || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

func (c *dashboardSummaryCache) set(view string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]dashboardCacheEntry)
	}
	// Copy the slice so later buffer reuse by the caller cannot mutate the
	// cached payload (defensive against json.Marshal aliasing).
	stored := make([]byte, len(data))
	copy(stored, data)
	c.entries[view] = dashboardCacheEntry{data: stored, expiresAt: time.Now().Add(c.ttl)}
}

func (c *dashboardSummaryCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

var globalDashboardCache = &dashboardSummaryCache{ttl: 10 * time.Second}

// ---- Dashboard ----
// GET /api/stats/dashboard?force=&view=
func (h *statsHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	view := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("view")))
	if view != "summary" && view != "insights" {
		view = "full"
	}

	// ?force=1 (or true) is the admin "force refresh" hook: it invalidates the
	// snapshot cache so this request recomputes from the DB and repopulates it.
	forceRefresh := parseDashboardForceRefresh(r.URL.Query().Get("force"))
	if forceRefresh {
		globalDashboardCache.clear()
	}

	// Cache hit short-circuits the expensive aggregation queries. The cached
	// bytes are the exact JSON the miss path would have produced.
	if cached, hit := globalDashboardCache.get(view); hit {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-dashboard-summary-cache", "hit")
		w.Header().Set("x-dashboard-insights-cache", "miss")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cached)
		return
	}

	// Cache miss: report miss and compute. Both headers default to "miss";
	// the summary header reflects this cache's state.
	w.Header().Set("x-dashboard-summary-cache", "miss")
	w.Header().Set("x-dashboard-insights-cache", "miss")

	now := time.Now()
	generatedAt := now.UTC().Format(time.RFC3339)
	result := map[string]any{
		"generatedAt": generatedAt,
	}

	if view == "summary" || view == "full" {
		var siteCount, accountCount, activeAccounts int
		if err := h.db.Get(&siteCount, "SELECT COUNT(*) FROM sites"); err != nil {
			h.dashboardError(w, "load site count", err)
			return
		}
		if err := h.db.Get(&accountCount, "SELECT COUNT(*) FROM accounts"); err != nil {
			h.dashboardError(w, "load account count", err)
			return
		}
		if err := h.db.Get(&activeAccounts, `
			SELECT COUNT(*) FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE a.status = 'active' AND s.status = 'active'
		`); err != nil {
			h.dashboardError(w, "load active account count", err)
			return
		}

		var totalBalance, totalUsed float64
		if err := h.db.Get(&totalBalance, `
			SELECT COALESCE(SUM(COALESCE(a.balance, 0)), 0)
			FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
			h.dashboardError(w, "load total balance", err)
			return
		}
		if err := h.db.Get(&totalUsed, `
			SELECT COALESCE(SUM(COALESCE(a.balance_used, 0)), 0)
			FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
			h.dashboardError(w, "load total used", err)
			return
		}

		// 24h proxy window (UTC) — single-pass aggregate with effective tokens.
		since24h := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		var proxy24h struct {
			Total       int     `db:"total"`
			Success     int     `db:"success"`
			TotalTokens int64   `db:"total_tokens"`
			TotalCost   float64 `db:"total_cost"`
		}
		if err := h.db.Get(&proxy24h, rebindAdminQuery(h.db, `
			SELECT
				COUNT(*) AS total,
				COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END), 0) AS success,
				COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS total_tokens,
				COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS total_cost
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE pl.created_at >= ? AND s.status = 'active'
		`), since24h); err != nil {
			h.dashboardError(w, "load 24h proxy metrics", err)
			return
		}

		dailyMetrics, err := dailyservice.CollectDailySummaryMetrics(h.db, now)
		if err != nil {
			h.dashboardError(w, "load local-day metrics", err)
			return
		}

		// Performance window: last 60s request/token rate from proxy_logs.
		windowSeconds := 60
		sincePerf := time.Now().UTC().Add(-time.Duration(windowSeconds) * time.Second).Format(time.RFC3339)
		var perf struct {
			Requests int64 `db:"requests"`
			Tokens   int64 `db:"tokens"`
		}
		if err := h.db.Get(&perf, rebindAdminQuery(h.db, `
			SELECT
				COUNT(*) AS requests,
				COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE pl.created_at >= ? AND s.status = 'active'
		`), sincePerf); err != nil {
			h.dashboardError(w, "load performance metrics", err)
			return
		}

		rpm := float64(perf.Requests) * 60 / float64(windowSeconds)
		tpm := float64(perf.Tokens) * 60 / float64(windowSeconds)

		result["siteCount"] = siteCount
		result["accountCount"] = accountCount
		result["totalAccounts"] = accountCount
		result["activeAccounts"] = activeAccounts
		result["totalBalance"] = roundMicro(totalBalance)
		result["totalUsed"] = roundMicro(totalUsed)
		result["todaySpend"] = roundMicro(dailyMetrics.TodaySpend)
		result["todayReward"] = roundMicro(dailyMetrics.TodayReward)
		result["todayCheckin"] = map[string]any{
			"total":   dailyMetrics.CheckinTotal,
			"success": dailyMetrics.CheckinSuccess,
			"skipped": dailyMetrics.CheckinSkipped,
			"failed":  dailyMetrics.CheckinFailed,
		}
		overallTodayStatus := "complete"
		if dailyMetrics.TodayRewardStatus != "complete" ||
			dailyMetrics.TodaySpendStatus != "complete" ||
			dailyMetrics.ProxyMetricStatus != "complete" {
			overallTodayStatus = "partial"
		}
		result["todayMetricStatus"] = map[string]any{
			"status":      overallTodayStatus,
			"localDay":    dailyMetrics.LocalDay,
			"timeZone":    dailyMetrics.TimeZone,
			"windowStart": dailyMetrics.WindowStartUTC,
			"windowEnd":   dailyMetrics.WindowEndUTC,
			"metrics": map[string]any{
				"checkin": map[string]any{"status": "complete"},
				"spend": map[string]any{
					"status":            dailyMetrics.TodaySpendStatus,
					"reason":            dailyMetrics.TodaySpendReason,
					"missingCostCount":  dailyMetrics.ProxyMissingCost,
					"unattributedCount": dailyMetrics.ProxyUnattributed,
				},
				"reward": map[string]any{
					"status": dailyMetrics.TodayRewardStatus,
					"reason": dailyMetrics.TodayRewardReason,
				},
				"proxy": map[string]any{
					"status":             dailyMetrics.ProxyMetricStatus,
					"reason":             dailyMetrics.ProxyMetricReason,
					"unknownStatusCount": dailyMetrics.ProxyUnknown,
					"unattributedCount":  dailyMetrics.ProxyUnattributed,
				},
			},
		}
		result["proxy24h"] = map[string]any{
			"total":       proxy24h.Total,
			"success":     proxy24h.Success,
			"totalTokens": proxy24h.TotalTokens,
			"totalCost":   roundMicro(proxy24h.TotalCost),
		}
		result["performance"] = map[string]any{
			"windowSeconds":     windowSeconds,
			"requestsPerMinute": rpm,
			"tokensPerMinute":   tpm,
		}
		// Legacy flat fields kept for older clients / tests.
		result["totalTokens"] = proxy24h.TotalTokens
		result["totalCost"] = roundMicro(proxy24h.TotalCost)
	}

	if view == "insights" || view == "full" {
		// All-time totals with effective token expression (no double count).
		var totalTokens int64
		if err := h.db.Get(&totalTokens, rebindAdminQuery(h.db, `
			SELECT COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0)
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`)); err != nil {
			h.dashboardError(w, "load total tokens", err)
			return
		}
		var totalCost float64
		if err := h.db.Get(&totalCost, `
			SELECT COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0)
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
			h.dashboardError(w, "load total cost", err)
			return
		}
		result["totalTokens"] = totalTokens
		result["totalCost"] = roundMicro(totalCost)

		// Site availability over last 24h from proxy_logs join path.
		since24h := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		rows, err := queryRowsErr(h.db, `
			SELECT
				s.id AS site_id,
				s.name AS site_name,
				s.url AS site_url,
				s.platform AS platform,
				COUNT(pl.id) AS total_requests,
				COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END), 0) AS success_count,
				COALESCE(SUM(CASE WHEN pl.id IS NOT NULL AND COALESCE(pl.status, '') <> 'success' THEN 1 ELSE 0 END), 0) AS failed_count,
				CASE
					WHEN COUNT(pl.id) = 0 THEN NULL
					ELSE ROUND(100.0 * SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END) / COUNT(pl.id), 2)
				END AS availability_percent,
				CASE
					WHEN SUM(CASE WHEN COALESCE(pl.latency_ms, 0) > 0 THEN 1 ELSE 0 END) = 0 THEN NULL
					ELSE ROUND(1.0 * SUM(CASE WHEN COALESCE(pl.latency_ms, 0) > 0 THEN pl.latency_ms ELSE 0 END)
						/ SUM(CASE WHEN COALESCE(pl.latency_ms, 0) > 0 THEN 1 ELSE 0 END), 2)
				END AS average_latency_ms
			FROM sites s
			LEFT JOIN accounts a ON a.site_id = s.id
			LEFT JOIN proxy_logs pl ON pl.account_id = a.id AND pl.created_at >= ?
			WHERE s.status = 'active'
			GROUP BY s.id, s.name, s.url, s.platform
			ORDER BY total_requests DESC, s.name ASC
		`, since24h)
		if err != nil {
			h.dashboardError(w, "load site availability", err)
			return
		}

		siteAvailability := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			item := map[string]any{
				"siteId":              row["siteId"],
				"siteName":            row["siteName"],
				"siteUrl":             row["siteUrl"],
				"platform":            row["platform"],
				"totalRequests":       row["totalRequests"],
				"successCount":        row["successCount"],
				"failedCount":         row["failedCount"],
				"availabilityPercent": row["availabilityPercent"],
				"averageLatencyMs":    row["averageLatencyMs"],
				"buckets":             []any{},
			}
			siteAvailability = append(siteAvailability, item)
		}
		result["siteAvailability"] = siteAvailability
	}

	// Marshal once and cache the bytes on the success path only — error paths
	// returned earlier via dashboardError. Caching the exact encoded bytes
	// (json.Encoder appends a newline, matching shared.WriteJSON) guarantees a
	// later cache-hit response is byte-identical to the miss that populated it.
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(result); err != nil {
		slog.Error("dashboard stats marshal failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to encode dashboard statistics")
		return
	}
	cached := buf.Bytes()
	globalDashboardCache.set(view, cached)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cached)
}

// parseDashboardForceRefresh accepts ?force=1 / ?force=true (case-insensitive)
// as the admin "force refresh" signal. Empty and any other value miss the
// cache normally. Mirrors the accounts handler's refresh=true convention but
// uses the generic force name so it reads naturally for any cache-backed
// admin endpoint.
func parseDashboardForceRefresh(raw string) bool {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
}

func (h *statsHandler) dashboardError(w http.ResponseWriter, operation string, err error) {
	slog.Error("dashboard stats query failed", "operation", operation, "error", err)
	writeError(w, http.StatusInternalServerError, "Failed to load dashboard statistics")
}

// ---- Proxy Logs ----
// GET /api/stats/proxy-logs?view=&limit=&offset=&status=&search=&client=&siteId=&from=&to=
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

	var conditions []string
	var args []any

	if status == "success" {
		conditions = append(conditions, "pl.status = 'success'")
	} else if status == "failed" {
		conditions = append(conditions, "COALESCE(pl.status, '') <> 'success'")
	}
	if search != "" {
		conditions = append(conditions, "(LOWER(COALESCE(pl.model_requested, '')) LIKE ? OR LOWER(COALESCE(pl.model_actual, '')) LIKE ?)")
		like := "%" + search + "%"
		args = append(args, like, like)
	}
	if siteID > 0 {
		conditions = append(conditions, "s.id = ?")
		args = append(args, siteID)
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
		query := `SELECT pl.*, a.username, s.id as site_id, s.name as site_name, s.url as site_url
			FROM proxy_logs pl
			LEFT JOIN accounts a ON pl.account_id = a.id
			LEFT JOIN sites s ON a.site_id = s.id` + where +
			" ORDER BY pl.created_at DESC LIMIT ? OFFSET ?"
		qArgs := make([]any, len(args))
		copy(qArgs, args)
		qArgs = append(qArgs, limit, offset)
		items := queryRows(h.db, query, qArgs...)
		queryPayload["items"] = normalizeSlice(items)

		var total int
		countQuery := "SELECT COUNT(*) FROM proxy_logs pl LEFT JOIN accounts a ON pl.account_id = a.id LEFT JOIN sites s ON a.site_id = s.id" + where
		h.db.Get(&total, rebindAdminQuery(h.db, countQuery), args...)
		queryPayload["total"] = total
	}

	if view == "meta" || view == "full" {
		// Use effective token expression so partial logs are not under-counted,
		// and never sum prompt+completion on top of total_tokens.
		summaryQuery := `SELECT COUNT(*) as total_count,
			COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END), 0) as success_count,
			COALESCE(SUM(CASE WHEN COALESCE(pl.status, '') <> 'success' THEN 1 ELSE 0 END), 0) as failed_count,
			COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) as total_cost,
			COALESCE(SUM(` + service.EffectiveProxyTokensSQL + `), 0) as total_tokens_all
			FROM proxy_logs pl
			LEFT JOIN accounts a ON pl.account_id = a.id
			LEFT JOIN sites s ON a.site_id = s.id` + where
		metaArgs := make([]any, len(args))
		copy(metaArgs, args)

		var summary struct {
			TotalCount     int     `db:"total_count"`
			SuccessCount   int     `db:"success_count"`
			FailedCount    int     `db:"failed_count"`
			TotalCost      float64 `db:"total_cost"`
			TotalTokensAll int64   `db:"total_tokens_all"`
		}
		h.db.Get(&summary, rebindAdminQuery(h.db, summaryQuery), metaArgs...)
		metaPayload["summary"] = map[string]any{
			"totalCount":     summary.TotalCount,
			"successCount":   summary.SuccessCount,
			"failedCount":    summary.FailedCount,
			"totalCost":      roundMicro(summary.TotalCost),
			"totalTokensAll": summary.TotalTokensAll,
		}

		sites := queryRows(h.db, "SELECT id, name, status FROM sites")
		metaPayload["sites"] = normalizeSlice(sites)
	}

	if view == "query" {
		writeJSON(w, http.StatusOK, queryPayload)
	} else if view == "meta" {
		writeJSON(w, http.StatusOK, metaPayload)
	} else {
		result := queryPayload
		result["clientOptions"] = metaPayload["clientOptions"]
		result["summary"] = metaPayload["summary"]
		result["sites"] = metaPayload["sites"]
		writeJSON(w, http.StatusOK, result)
	}
}

// ---- Proxy Log Detail ----
// GET /api/stats/proxy-logs/:id
func (h *statsHandler) proxyLogDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	row := queryRow(h.db,
		`SELECT pl.*, a.username, s.id as site_id, s.name as site_name, s.url as site_url
		 FROM proxy_logs pl
		 LEFT JOIN accounts a ON pl.account_id = a.id
		 LEFT JOIN sites s ON a.site_id = s.id
		 WHERE pl.id = ?`, id)
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "proxy log not found"})
		return
	}

	// Parse billing details if present
	if bd, ok := row["billingDetails"]; ok {
		if bdStr, ok2 := bd.(string); ok2 && bdStr != "" {
			var parsed any
			if err := json.Unmarshal([]byte(bdStr), &parsed); err == nil {
				row["billingDetails"] = parsed
			}
		}
	}

	writeJSON(w, http.StatusOK, row)
}

// ---- Debug Traces ----
// GET /api/stats/proxy-debug/traces?limit=
func (h *statsHandler) debugTraces(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, 50, 100)

	rows := queryRows(h.db, "SELECT * FROM proxy_debug_traces ORDER BY created_at DESC LIMIT ?", limit)
	writeJSON(w, http.StatusOK, map[string]any{"items": normalizeSlice(rows)})
}

// ---- Debug Trace Detail ----
// GET /api/stats/proxy-debug/traces/:id
func (h *statsHandler) debugTraceDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	row := queryRow(h.db, "SELECT * FROM proxy_debug_traces WHERE id = ?", id)
	if row == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "proxy debug trace not found"})
		return
	}

	// Load related attempts
	attempts := queryRows(h.db, "SELECT * FROM proxy_debug_attempts WHERE trace_id = ? ORDER BY attempt_index ASC", id)
	row["attempts"] = normalizeSlice(attempts)

	writeJSON(w, http.StatusOK, row)
}

// ---- Site Distribution ----
// GET /api/stats/site-distribution?days=&refresh=
func (h *statsHandler) siteDistribution(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 7), 1, 365)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	// Prefer projected site_day_usage spend; include live account balances.
	rows := queryRows(h.db, `
		SELECT
			s.id AS site_id,
			s.name AS site_name,
			s.platform AS platform,
			COALESCE(bal.total_balance, 0) AS total_balance,
			COALESCE(usage.total_spend, 0) AS total_spend,
			COALESCE(bal.account_count, 0) AS account_count
		FROM sites s
		LEFT JOIN (
			SELECT site_id, COALESCE(SUM(COALESCE(balance, 0)), 0) AS total_balance, COUNT(*) AS account_count
			FROM accounts
			GROUP BY site_id
		) bal ON bal.site_id = s.id
		LEFT JOIN (
			SELECT site_id, COALESCE(SUM(COALESCE(total_summary_spend, 0)), 0) AS total_spend
			FROM site_day_usage
			WHERE local_day >= ?
			GROUP BY site_id
		) usage ON usage.site_id = s.id
		ORDER BY total_spend DESC, s.name ASC
	`, fromDay)

	distribution := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		distribution = append(distribution, map[string]any{
			"siteId":       row["siteId"],
			"siteName":     row["siteName"],
			"platform":     row["platform"],
			"totalBalance": coerceFloat(row["totalBalance"]),
			"totalSpend":   coerceFloat(row["totalSpend"]),
			"accountCount": coerceInt(row["accountCount"]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"distribution": distribution})
}

// ---- Site Trend ----
// GET /api/stats/site-trend?days=&refresh=
func (h *statsHandler) siteTrend(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 7), 1, 365)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	rows := queryRows(h.db, `
		SELECT
			u.local_day AS local_day,
			s.name AS site_name,
			COALESCE(SUM(u.total_summary_spend), 0) AS spend,
			COALESCE(SUM(u.total_calls), 0) AS calls
		FROM site_day_usage u
		INNER JOIN sites s ON s.id = u.site_id
		WHERE u.local_day >= ?
		GROUP BY u.local_day, s.name
		ORDER BY u.local_day ASC, s.name ASC
	`, fromDay)

	// Shape: [{ date, sites: { [siteName]: { spend, calls } } }]
	byDate := make(map[string]map[string]map[string]any)
	order := make([]string, 0)
	for _, row := range rows {
		day := coerceString(row["localDay"])
		siteName := coerceString(row["siteName"])
		if day == "" || siteName == "" {
			continue
		}
		if _, ok := byDate[day]; !ok {
			byDate[day] = make(map[string]map[string]any)
			order = append(order, day)
		}
		byDate[day][siteName] = map[string]any{
			"spend": coerceFloat(row["spend"]),
			"calls": coerceInt(row["calls"]),
		}
	}

	trend := make([]map[string]any, 0, len(order))
	for _, day := range order {
		trend = append(trend, map[string]any{
			"date":  day,
			"sites": byDate[day],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"trend": trend})
}

// ---- Balance History ----
// GET /api/stats/balance-history?accountId=&days=30
// Returns per-day balance snapshots for one account (latest-known of each day).
// If accountId is omitted, returns all accounts' series keyed by accountId.
func (h *statsHandler) modelBySite(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 7), 1, 365)
	siteID := getQueryInt(r, "siteId", 0)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	query := `SELECT model,
		COALESCE(SUM(total_calls), 0) AS calls,
		COALESCE(SUM(total_spend), 0) AS spend,
		COALESCE(SUM(total_tokens), 0) AS tokens
		FROM model_day_usage
		WHERE local_day >= ?`
	args := []any{fromDay}
	if siteID > 0 {
		query += " AND site_id = ?"
		args = append(args, siteID)
	}
	query += " GROUP BY model ORDER BY calls DESC"

	rows := queryRows(h.db, query, args...)
	models := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		models = append(models, map[string]any{
			"model":  coerceString(row["model"]),
			"calls":  coerceInt(row["calls"]),
			"spend":  coerceFloat(row["spend"]),
			"tokens": coerceInt64(row["tokens"]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

// usageHeatmapCellLimit caps density rows returned to admin clients.
// 31d * 24h * ~50 keys is more than enough for a dense operator view.
const usageHeatmapCellLimit = 2000

// ---- Usage Heatmap ----
// GET /api/stats/usage-heatmap?days=7&dimension=site|model

// Returns bounded hour-bucket density cells for admin analytics.
// Site dimension prefers projected site_hour_usage; model dimension aggregates
// proxy_logs with a hard LIMIT (no unbounded scans, no chat content).
func (h *statsHandler) marketplace(w http.ResponseWriter, r *http.Request) {
	refreshRequested := parseTruthyQuery(r.URL.Query().Get("refresh"))
	includePricing := parseTruthyQuery(r.URL.Query().Get("includePricing"))

	models := h.buildMarketplaceModels()
	meta := map[string]any{
		"refreshRequested": refreshRequested,
		// No background pricing/catalog job in this surface — DB-derived only.
		"refreshQueued":  false,
		"refreshReused":  false,
		"refreshRunning": false,
		"refreshJobId":   nil,
		"includePricing": includePricing,
		"source":         "db_availability",
	}
	if includePricing {
		// Explicit known limitation: full remote /api/pricing hydration is out of scope.
		meta["pricingStatus"] = "unavailable"
		meta["pricingNote"] = "Remote marketplace pricing catalog is not hydrated; use /api/models/price-compare for effective rates. pricingSources intentionally empty."
	}
	if refreshRequested {
		meta["refreshNote"] = "refresh=true acknowledged but no remote marketplace scrape is performed; response is rebuilt from local model_availability / token_model_availability / token_routes."
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"models": models,
		"meta":   meta,
	})
}

// ---- Token Candidates ----
// GET /api/models/token-candidates

// Structured maps for route configuration:
// - models: token-scoped availability (token_model_availability)
// - modelsWithoutToken: account-level availability with no matching token coverage
// - modelsMissingTokenGroups: accounts whose tokens lack group labels when groups are required
// - endpointTypesByModel: inferred endpoint types from site platforms
func (h *statsHandler) tokenCandidates(w http.ResponseWriter, r *http.Request) {
	allowed := h.loadGlobalAllowedModels()
	models := h.buildTokenCandidateModels(allowed)
	withoutToken := h.buildModelsWithoutToken(allowed)
	missingGroups := h.buildModelsMissingTokenGroups(allowed)
	endpointTypes := h.buildEndpointTypesByModel(allowed)

	writeJSON(w, http.StatusOK, map[string]any{
		"models":                   models,
		"modelsWithoutToken":       withoutToken,
		"modelsMissingTokenGroups": missingGroups,
		"endpointTypesByModel":     endpointTypes,
	})
}

// ---- Model Check ----
// POST /api/models/check/:accountId

// Real availability refresh for one account via platform.GetModels, then
// best-effort route rebuild. Never returns fake success when refresh fails.
func (h *statsHandler) modelCheck(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "accountId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   "Invalid account id",
		})
		return
	}

	result := accountModelRefresher(r.Context(), h.db, id, true)
	writeJSON(w, http.StatusOK, result)
}

// ---- Model Probe ----
// POST /api/models/probe
func (h *statsHandler) modelProbe(w http.ResponseWriter, r *http.Request) {
	sched := scheduler.GetGlobalModelProbeScheduler()
	if sched == nil {
		writeError(w, http.StatusServiceUnavailable, "model probe scheduler is not running (enable MODEL_AVAILABILITY_PROBE_ENABLED or start schedulers)")
		return
	}
	jobID := fmt.Sprintf("probe-%d", time.Now().UTC().UnixNano())
	go func() {
		sched.TriggerNow(true)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"queued":  true,
		"reused":  false,
		"jobId":   jobID,
		"status":  "pending",
		"message": "已开始模型可用性探测，请稍后查看任务列表或 LastRunSummary",
	})
}
