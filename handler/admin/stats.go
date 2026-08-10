package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service"
	dailyservice "github.com/deliciousbuding/metapi-go/service/daily"
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

// ---- Dashboard ----
// GET /api/stats/dashboard?refresh=&view=
func (h *statsHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	view := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("view")))
	if view != "summary" && view != "insights" {
		view = "full"
	}

	// Set cache headers
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
					"missingCostCount": dailyMetrics.ProxyMissingCost,
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

	writeJSON(w, http.StatusOK, result)
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

	limit := clampInt(getQueryInt(r, "limit", 50), 1, 100)
	offset := maxInt(0, getQueryInt(r, "offset", 0))
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
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "proxy log id is invalid"})
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
	limit := clampInt(getQueryInt(r, "limit", 50), 1, 100)

	rows := queryRows(h.db, "SELECT * FROM proxy_debug_traces ORDER BY created_at DESC LIMIT ?", limit)
	writeJSON(w, http.StatusOK, map[string]any{"items": normalizeSlice(rows)})
}

// ---- Debug Trace Detail ----
// GET /api/stats/proxy-debug/traces/:id
func (h *statsHandler) debugTraceDetail(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "proxy debug trace id is invalid"})
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
func (h *statsHandler) balanceHistory(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 30), 1, 365)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	accountID := getQueryInt(r, "accountId", 0)

	args := []any{fromDay}
	q := `SELECT account_id, balance, balance_used, quota, local_day, captured_at
		FROM balance_history
		WHERE local_day >= ?`
	if accountID > 0 {
		q += ` AND account_id = ?`
		args = append(args, accountID)
	}
	q += ` ORDER BY local_day ASC, account_id ASC`

	rows := queryRows(h.db, q, args...)
	byAccount := make(map[int64][]map[string]any)
	for _, row := range rows {
		accID := coerceInt64(row["accountId"])
		byAccount[accID] = append(byAccount[accID], map[string]any{
			"day":         coerceString(row["localDay"]),
			"balance":     coerceFloat(row["balance"]),
			"balanceUsed": coerceFloat(row["balanceUsed"]),
			"quota":       coerceFloat(row["quota"]),
			"capturedAt":  coerceString(row["capturedAt"]),
		})
	}

	series := make([]map[string]any, 0, len(byAccount))
	for accID, points := range byAccount {
		series = append(series, map[string]any{
			"accountId": accID,
			"points":    points,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"series": series,
		"days":   days,
	})
}

// GET /api/stats/balance-income-outcome?days=30&accountId=
// A3: income vs outcome balance analysis, derived from
// the A1 snapshots via the accounting identity income - outcome = Δbalance:
// - outcome(day) = max(0, Δ balance_used) — consumption (chargeable spend);
// - income(day) = Δ balance + Δ balance_used — whatever refilled the
// balance (free quota top-ups, recharges), so the identity always holds;
// - the first snapshot day of an account has no previous value: its combined
// balance and balance_used is treated as initial income (no consumption before it).

// Only days with actual snapshots are emitted (missing day ≠ zero activity).
func (h *statsHandler) balanceIncomeOutcome(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 30), 1, 365)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	accountID := getQueryInt(r, "accountId", 0)

	args := []any{fromDay}
	q := `SELECT account_id, balance, balance_used, local_day
		FROM balance_history
		WHERE local_day >= ?`
	if accountID > 0 {
		q += ` AND account_id = ?`
		args = append(args, accountID)
	}
	q += ` ORDER BY account_id ASC, local_day ASC`

	rows := queryRows(h.db, q, args...)

	// Per-account chronological snapshots (already ordered).
	type snapshot struct {
		day         string
		balance     float64
		balanceUsed float64
	}
	byAccount := make(map[int64][]snapshot)
	accountOrder := make([]int64, 0, len(byAccount))
	for _, row := range rows {
		accID := coerceInt64(row["accountId"])
		if _, seen := byAccount[accID]; !seen {
			accountOrder = append(accountOrder, accID)
		}
		byAccount[accID] = append(byAccount[accID], snapshot{
			day:         coerceString(row["localDay"]),
			balance:     coerceFloat(row["balance"]),
			balanceUsed: coerceFloat(row["balanceUsed"]),
		})
	}

	byDay := make(map[string]map[string]float64) // day → {"income", "outcome"}
	for _, accID := range accountOrder {
		points := byAccount[accID]
		for i := range points {
			p := points[i]
			var income, outcome float64
			if i == 0 {
				// First snapshot: everything credited so far is initial income.
				income = p.balance + p.balanceUsed
			} else {
				prev := points[i-1]
				deltaUsed := p.balanceUsed - prev.balanceUsed
				// Keep negative deltas: a refund/remap that lowers balance_used
				// is negative consumption (outcome < 0) — clamping it to 0 would
				// break the accounting identity income - outcome = Δbalance.
				outcome = deltaUsed
				income = (p.balance - prev.balance) + deltaUsed
			}
			entry := byDay[p.day]
			if entry == nil {
				entry = map[string]float64{"income": 0, "outcome": 0}
				byDay[p.day] = entry
			}
			entry["income"] += income
			entry["outcome"] += outcome
		}
	}

	// Sort days ascending for a stable series.
	dayKeys := make([]string, 0, len(byDay))
	for day := range byDay {
		dayKeys = append(dayKeys, day)
	}
	sort.Strings(dayKeys)

	points := make([]map[string]any, 0, len(dayKeys))
	var totalIncome, totalOutcome float64
	for _, day := range dayKeys {
		entry := byDay[day]
		points = append(points, map[string]any{
			"day":     day,
			"income":  round4(entry["income"]),
			"outcome": round4(entry["outcome"]),
			"net":     round4(entry["income"] - entry["outcome"]),
		})
		totalIncome += entry["income"]
		totalOutcome += entry["outcome"]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"days":        days,
		"points":      points,
		"summary": map[string]any{
			"totalIncome":  round4(totalIncome),
			"totalOutcome": round4(totalOutcome),
			"net":          round4(totalIncome - totalOutcome),
			"accounts":     len(accountOrder),
		},
	})
}

// ---- Attention dashboard ----
// GET /api/stats/attention?limit=20
// Returns severity-ranked actionable items (deep-linkable) so the operator
// sees "what needs my eyes" in one place: expired accounts, low-balance
// accounts, disabled sites, recent warning/error events. Aggregates plain
// columns only (runtime health in extra_config JSON is already surfaced via
// the events table by alert.go, so we read events rather than json_extract).
type attentionItem struct {
	Severity  string `json:"severity"`  // critical | warning | info
	Category  string `json:"category"`  // expired_account | low_balance | disabled_site | event
	Label     string `json:"label"`     // human-readable
	Target    string `json:"target"`    // deep-link target (route + query)
	CreatedAt string `json:"createdAt"` // most recent signal time
}

func (h *statsHandler) attention(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(getQueryInt(r, "limit", 20), 1, 100)
	items := make([]attentionItem, 0, limit)

	// 1. Expired accounts — critical.
	expired := queryRows(h.db, rebindAdminQuery(h.db, `SELECT id, username, site_id, updated_at
		FROM accounts WHERE status = 'expired' ORDER BY updated_at DESC LIMIT ?`), limit)
	for _, row := range expired {
		items = append(items, attentionItem{
			Severity: "critical", Category: "expired_account",
			Label:     "账号已过期：" + coerceString(row["username"]),
			Target:    "/accounts?accountId=" + coerceString(row["id"]),
			CreatedAt: coerceString(row["updatedAt"]),
		})
		if len(items) >= limit {
			writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
			return
		}
	}

	// 2. Low-balance accounts (< 1.0) — warning. Matches G1 threshold.
	low := queryRows(h.db, rebindAdminQuery(h.db, `SELECT id, username, balance, site_id
		FROM accounts WHERE status = 'active' AND COALESCE(balance, 0) < 1.0
		ORDER BY balance ASC LIMIT ?`), limit)
	for _, row := range low {
		items = append(items, attentionItem{
			Severity: "warning", Category: "low_balance",
			Label:     fmt.Sprintf("余额不足：%s（%.2f）", coerceString(row["username"]), coerceFloat(row["balance"])),
			Target:    "/accounts?accountId=" + coerceString(row["id"]),
			CreatedAt: coerceString(row["updatedAt"]),
		})
		if len(items) >= limit {
			writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
			return
		}
	}

	// 3. Disabled sites — warning.
	disabledSites := queryRows(h.db, rebindAdminQuery(h.db, `SELECT id, name, updated_at
		FROM sites WHERE status = 'disabled' ORDER BY updated_at DESC LIMIT ?`), limit)
	for _, row := range disabledSites {
		items = append(items, attentionItem{
			Severity: "warning", Category: "disabled_site",
			Label:     "站点已禁用：" + coerceString(row["name"]),
			Target:    "/sites?siteId=" + coerceString(row["id"]),
			CreatedAt: coerceString(row["updatedAt"]),
		})
		if len(items) >= limit {
			writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
			return
		}
	}

	// 4. Recent unread warning/error events — info/warning (deep-link to events).
	since24h := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	evRows := queryRows(h.db, rebindAdminQuery(h.db, `SELECT type, title, level, related_id, related_type, created_at
		FROM events WHERE level IN ('warning', 'error') AND created_at >= ?
		ORDER BY created_at DESC LIMIT ?`), since24h, limit)
	for _, row := range evRows {
		severity := coerceString(row["level"])
		if severity == "error" {
			severity = "critical"
		}
		items = append(items, attentionItem{
			Severity:  severity,
			Category:  "event",
			Label:     coerceString(row["title"]),
			Target:    "/settings", // events surface lives in settings/notifications area
			CreatedAt: coerceString(row["createdAt"]),
		})
		if len(items) >= limit {
			break
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

// ---- Model by Site ----
// GET /api/stats/model-by-site?siteId=&days=
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
func (h *statsHandler) usageHeatmap(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 7), 1, 31)
	dimension := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("dimension")))
	if dimension != "model" {
		dimension = "site"
	}

	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour).Truncate(time.Hour).Format(time.RFC3339)
	source := "proxy_logs"
	var cells []map[string]any

	if dimension == "site" {
		// Prefer projected hour aggregates (cheap, already limited by projector).
		rows := queryRows(h.db, `
			SELECT
				u.bucket_start_utc AS bucket,
				CAST(u.site_id AS TEXT) AS key,
				COALESCE(s.name, '') AS label,
				COALESCE(u.total_calls, 0) AS calls,
				COALESCE(u.total_tokens, 0) AS tokens,
				COALESCE(u.total_summary_spend, 0) AS spend
			FROM site_hour_usage u
			LEFT JOIN sites s ON s.id = u.site_id
			WHERE u.bucket_start_utc >= ?
			ORDER BY u.bucket_start_utc ASC, u.total_calls DESC
			LIMIT ?
		`, since, usageHeatmapCellLimit)
		if len(rows) > 0 {
			source = "site_hour_usage"
			cells = makeUsageHeatmapCells(rows)
		} else {
			// Fallback: bounded live aggregate from proxy_logs when projection is empty.
			hourExpr := hourBucketSQLExpr(h.db, "pl.created_at")
			rows = queryRows(h.db, `
				SELECT
					`+hourExpr+` AS bucket,
					CAST(s.id AS TEXT) AS key,
					COALESCE(s.name, '') AS label,
					COUNT(*) AS calls,
					COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens,
					COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS spend
				FROM proxy_logs pl
				INNER JOIN accounts a ON a.id = pl.account_id
				INNER JOIN sites s ON s.id = a.site_id
				WHERE pl.created_at >= ?
				GROUP BY `+hourExpr+`, s.id, s.name
				ORDER BY bucket ASC, calls DESC
				LIMIT ?
			`, since, usageHeatmapCellLimit)
			cells = makeUsageHeatmapCells(rows)
		}
	} else {
		// Model density: no model_hour_usage table; aggregate proxy_logs with LIMIT.
		hourExpr := hourBucketSQLExpr(h.db, "pl.created_at")
		modelExpr := `COALESCE(NULLIF(pl.model_actual, ''), NULLIF(pl.model_requested, ''), 'unknown')`
		rows := queryRows(h.db, `
			SELECT
				`+hourExpr+` AS bucket,
				`+modelExpr+` AS key,
				`+modelExpr+` AS label,
				COUNT(*) AS calls,
				COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens,
				COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS spend
			FROM proxy_logs pl
			WHERE pl.created_at >= ?
			GROUP BY `+hourExpr+`, `+modelExpr+`
			ORDER BY bucket ASC, calls DESC
			LIMIT ?
		`, since, usageHeatmapCellLimit)
		cells = makeUsageHeatmapCells(rows)
	}

	if cells == nil {
		cells = []map[string]any{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"dimension": dimension,
		"days":      days,
		"since":     since,
		"source":    source,
		"cellLimit": usageHeatmapCellLimit,
		"count":     len(cells),
		"cells":     cells,
	})
}

// ---- Slow Requests ----
// GET /api/stats/slow-requests?limit=50&minLatencyMs=1000&hours=24

// Top proxy_logs by latency_ms within a bounded time window.
// Never returns request/response bodies or chat content.
func (h *statsHandler) slowRequests(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(getQueryInt(r, "limit", 50), 1, 200)
	minLatencyMs := clampInt(getQueryInt(r, "minLatencyMs", 1000), 0, 3_600_000)
	hours := clampInt(getQueryInt(r, "hours", 24), 1, 168)
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

	rows := queryRows(h.db, `
		SELECT
			pl.id AS id,
			COALESCE(NULLIF(pl.model_actual, ''), NULLIF(pl.model_requested, ''), '') AS model,
			COALESCE(pl.status, '') AS status,
			COALESCE(pl.latency_ms, 0) AS latency_ms,
			COALESCE(pl.first_byte_latency_ms, 0) AS first_byte_latency_ms,
			COALESCE(pl.http_status, 0) AS http_status,
			COALESCE(pl.request_id, '') AS request_id,
			pl.account_id AS account_id,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name,
			pl.created_at AS created_at
		FROM proxy_logs pl
		LEFT JOIN accounts a ON a.id = pl.account_id
		LEFT JOIN sites s ON s.id = a.site_id
		WHERE pl.created_at >= ?
			AND COALESCE(pl.latency_ms, 0) >= ?
		ORDER BY pl.latency_ms DESC, pl.created_at DESC
		LIMIT ?
	`, since, minLatencyMs, limit)

	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"id":                 coerceInt64(row["id"]),
			"model":              coerceString(row["model"]),
			"status":             coerceString(row["status"]),
			"latencyMs":          coerceInt64(row["latencyMs"]),
			"firstByteLatencyMs": coerceInt64(row["firstByteLatencyMs"]),
			"httpStatus":         coerceInt(row["httpStatus"]),
			"requestId":          coerceString(row["requestId"]),
			"accountId":          row["accountId"],
			"siteId":             row["siteId"],
			"siteName":           coerceString(row["siteName"]),
			"createdAt":          coerceString(row["createdAt"]),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"hours":        hours,
		"minLatencyMs": minLatencyMs,
		"limit":        limit,
		"since":        since,
		"count":        len(items),
		"items":        items,
	})
}

// ---- Model Cost Distribution ----
// GET /api/stats/model-cost-distribution?days=30&topN=8

// Top-N models by estimated cost with the remainder folded into an "Other"
// bucket (topN-with-Other pattern from UsageAnalytics). Model
// name preference: model_actual > model_requested > unknown (matches the
// usage heatmap expression). Data source: proxy_logs.
func (h *statsHandler) modelCostDistribution(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 30), 1, 90)
	topN := clampInt(getQueryInt(r, "topN", 8), 1, 20)
	since := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02T00:00:00Z")

	modelExpr := `COALESCE(NULLIF(pl.model_actual, ''), NULLIF(pl.model_requested, ''), 'unknown')`
	rows := queryRows(h.db, `
		SELECT `+modelExpr+` AS model,
			COUNT(*) AS calls,
			COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS cost,
			COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens
		FROM proxy_logs pl
		WHERE pl.created_at >= ?
		GROUP BY `+modelExpr+`
		ORDER BY cost DESC, model ASC
	`, since)

	items := make([]map[string]any, 0, topN+1)
	var totalCost, totalCalls, totalTokens float64
	var otherCost, otherCalls, otherTokens float64
	for i, row := range rows {
		model := coerceString(row["model"])
		cost := coerceFloat(row["cost"])
		calls := coerceFloat(row["calls"])
		tokens := coerceFloat(row["tokens"])
		totalCost += cost
		totalCalls += calls
		totalTokens += tokens
		if i < topN {
			items = append(items, map[string]any{
				"model":  model,
				"label":  model,
				"cost":   roundMicro(cost),
				"calls":  int64(calls),
				"tokens": int64(tokens),
			})
		} else {
			otherCost += cost
			otherCalls += calls
			otherTokens += tokens
		}
	}
	if otherCost > 0 || otherCalls > 0 {
		items = append(items, map[string]any{
			"model":  "other",
			"label":  "其他模型",
			"cost":   roundMicro(otherCost),
			"calls":  int64(otherCalls),
			"tokens": int64(otherTokens),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"days":  days,
		"since": since,
		"topN":  topN,
		"items": items,
		"totals": map[string]any{
			"cost":   roundMicro(totalCost),
			"calls":  int64(totalCalls),
			"tokens": int64(totalTokens),
		},
	})
}

// ---- Latency Histogram ----
// GET /api/stats/latency-histogram?days=7&bucketMs=500

// Request-count histogram over latency_ms buckets. Integer division is
// identical on SQLite and PostgreSQL, so the same expression drives both
// dialects. Buckets with zero requests are omitted; client renders the gaps.
func (h *statsHandler) latencyHistogram(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 7), 1, 90)
	bucketMs := clampInt(getQueryInt(r, "bucketMs", 500), 100, 60000)
	since := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02T00:00:00Z")

	rows := queryRows(h.db, `
		SELECT (COALESCE(pl.latency_ms, 0) / ?) * ? AS bucket_start,
			COUNT(*) AS count
		FROM proxy_logs pl
		WHERE pl.created_at >= ?
		GROUP BY bucket_start
		ORDER BY bucket_start ASC
	`, bucketMs, bucketMs, since)

	buckets := make([]map[string]any, 0, len(rows))
	var total int64
	for _, row := range rows {
		start := coerceInt64(row["bucketStart"])
		count := coerceInt64(row["count"])
		total += count
		buckets = append(buckets, map[string]any{
			"bucketStartMs": start,
			"bucketEndMs":   start + int64(bucketMs),
			"label":         fmt.Sprintf("%d–%dms", start, start+int64(bucketMs)),
			"count":         count,
		})
	}
	for _, b := range buckets {
		if total > 0 {
			b["percent"] = float64(b["count"].(int64)) * 100 / float64(total)
		} else {
			b["percent"] = 0.0
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"days":     days,
		"since":    since,
		"bucketMs": bucketMs,
		"total":    total,
		"buckets":  buckets,
	})
}

// ---- Latency Trend ----
// GET /api/stats/latency-trend?days=7

// Per-day latency profile from proxy_logs: request volume, average / max
// latency, average first-byte latency, success rate and p95. p95 is computed
// from a bounded descending sample per day (ORDER BY latency_ms DESC LIMIT);
// days exceeding the sample cap are flagged honestly via truncatedDays
// instead of silently under-reporting.
func (h *statsHandler) latencyTrend(w http.ResponseWriter, r *http.Request) {
	days := clampInt(getQueryInt(r, "days", 7), 1, 90)
	fromDay := time.Now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")

	dayExpr := dayBucketSQLExpr(h.db, "pl.created_at")
	rows := queryRows(h.db, `
		SELECT `+dayExpr+` AS day,
			COUNT(*) AS requests,
			AVG(CASE WHEN COALESCE(pl.latency_ms, 0) > 0 THEN pl.latency_ms END) AS avg_latency,
			MAX(pl.latency_ms) AS max_latency,
			AVG(CASE WHEN COALESCE(pl.first_byte_latency_ms, 0) > 0 THEN pl.first_byte_latency_ms END) AS avg_first_byte,
			COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END), 0) AS success_count
		FROM proxy_logs pl
		WHERE pl.created_at >= ?
		GROUP BY `+dayExpr+`
		ORDER BY day ASC
	`, fromDay)

	// p95 per day from a bounded descending sample. With LIMIT cap, the p95
	// index (floor(0.05*n)) stays inside the sample while n < 20*cap.
	const p95SampleCap = 10000
	truncatedDays := make([]string, 0)
	points := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		day := coerceString(row["day"])
		requests := coerceInt64(row["requests"])
		successCount := coerceInt64(row["successCount"])

		point := map[string]any{
			"date":           day,
			"requests":       requests,
			"avgLatencyMs":   round1(coerceFloat(row["avgLatency"])),
			"maxLatencyMs":   coerceInt64(row["maxLatency"]),
			"avgFirstByteMs": round1(coerceFloat(row["avgFirstByte"])),
			"p95LatencyMs":   nil,
			"successRate":    0.0,
		}
		if requests > 0 {
			point["successRate"] = round4(float64(successCount) / float64(requests))
		}

		dayStart := day + "T00:00:00Z"
		dayEnd := dayStart
		if next, err := time.Parse("2006-01-02", day); err == nil {
			dayEnd = next.AddDate(0, 0, 1).Format("2006-01-02") + "T00:00:00Z"
		}
		// COUNT(*) OVER () gives the true row count before LIMIT truncates,
		// so p95 sampling stays honest when a day exceeds the sample cap.
		samples := queryRows(h.db, `
			SELECT pl.latency_ms AS latency_ms, COUNT(*) OVER () AS total
			FROM proxy_logs pl
			WHERE pl.created_at >= ? AND pl.created_at < ?
				AND COALESCE(pl.latency_ms, 0) > 0
			ORDER BY pl.latency_ms DESC
			LIMIT ?
		`, dayStart, dayEnd, p95SampleCap)
		n := len(samples)
		if n > 0 {
			total := coerceInt64(samples[0]["total"])
			// Descending sample; p95 (ascending 0-based index ceil(0.95n)-1)
			// lands at descending 0-based index floor(0.05*n).
			p95Idx := int(math.Floor(float64(total) * 0.05))
			if p95Idx < n {
				point["p95LatencyMs"] = coerceInt64(samples[p95Idx]["latencyMs"])
			} else {
				truncatedDays = append(truncatedDays, day)
			}
		}
		points = append(points, point)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"days":          days,
		"points":        points,
		"p95SampleCap":  p95SampleCap,
		"truncatedDays": truncatedDays,
	})
}

// dayBucketSQLExpr returns a dual-dialect SQL expression that truncates a
// TEXT RFC3339 timestamp column to a date bucket string (YYYY-MM-DD).
func dayBucketSQLExpr(db *sqlx.DB, column string) string {
	driver := ""
	if db != nil {
		driver = strings.ToLower(strings.TrimSpace(db.DriverName()))
	}
	switch driver {
	case "pgx", "postgres", "postgresql":
		// created_at is stored as TEXT RFC3339; cast for date_trunc then re-format.
		return `to_char(date_trunc('day', (` + column + `)::timestamptz), 'YYYY-MM-DD')`
	default:
		// SQLite stores UTC RFC3339 without fractional seconds in our writers.
		return `substr(` + column + `, 1, 10)`
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// hourBucketSQLExpr returns a dual-dialect SQL expression that truncates a
// TEXT RFC3339 timestamp column to an hour bucket string (…T%H:00:00Z).
func hourBucketSQLExpr(db *sqlx.DB, column string) string {
	driver := ""
	if db != nil {
		driver = strings.ToLower(strings.TrimSpace(db.DriverName()))
	}
	switch driver {
	case "pgx", "postgres", "postgresql":
		// created_at is stored as TEXT RFC3339; cast for date_trunc then re-format.
		return `to_char(date_trunc('hour', (` + column + `)::timestamptz), 'YYYY-MM-DD"T"HH24:00:00"Z"')`
	default:
		// SQLite stores UTC RFC3339 without fractional seconds in our writers.
		// substr keeps the query portable and avoids full-table function scans
		// beyond the created_at range predicate + LIMIT.
		return `substr(` + column + `, 1, 13) || ':00:00Z'`
	}
}

func makeUsageHeatmapCells(rows []map[string]any) []map[string]any {
	cells := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		bucket := coerceString(row["bucket"])
		key := coerceString(row["key"])
		if bucket == "" || key == "" {
			continue
		}
		cells = append(cells, map[string]any{
			"bucket": bucket,
			"key":    key,
			"label":  coerceString(row["label"]),
			"calls":  coerceInt64(row["calls"]),
			"tokens": coerceInt64(row["tokens"]),
			"spend":  roundMicro(coerceFloat(row["spend"])),
		})
	}
	return cells
}

// ---- Model Marketplace ----
// GET /api/models/marketplace?refresh=&includePricing=

// Builds the operator marketplace view from local DB availability / routes.
// Does not scrape remote marketplace/pricing catalogs. When includePricing=1,
// pricingSources remain empty and meta.pricingStatus labels the gap
// (
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
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"success": false,
			"message": "model probe scheduler is not running (enable MODEL_AVAILABILITY_PROBE_ENABLED or start schedulers)",
		})
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
