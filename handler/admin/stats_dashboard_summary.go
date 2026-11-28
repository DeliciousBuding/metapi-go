package admin

import (
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	dailyservice "github.com/deliciousbuding/metapi-go/service/daily"
)

func (h *statsHandler) collectDashboardSummary(metrics *dashboardMetrics, now time.Time) {
	var siteCount, accountCount, activeAccounts int
	var upstreamChannelCount int
	if err := h.db.Get(&upstreamChannelCount, "SELECT COUNT(*) FROM upstream_channels"); err != nil {
		metrics.markFailed("upstreamChannelCount", "load independent upstream count", err)
	} else {
		metrics.fields["upstreamChannelCount"] = upstreamChannelCount
	}
	if err := h.db.Get(&siteCount, "SELECT COUNT(*) FROM sites"); err != nil {
		metrics.markFailed("siteCount", "load site count", err)
	} else {
		metrics.fields["siteCount"] = siteCount
	}
	if err := h.db.Get(&accountCount, "SELECT COUNT(*) FROM accounts"); err != nil {
		metrics.markFailed("accountCount", "load account count", err)
		metrics.fields["totalAccounts"] = nil
	} else {
		metrics.fields["accountCount"] = accountCount
		metrics.fields["totalAccounts"] = accountCount
	}
	if err := h.db.Get(&activeAccounts, `
			SELECT COUNT(*) FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE a.status = 'active' AND s.status = 'active'
		`); err != nil {
		metrics.markFailed("activeAccounts", "load active account count", err)
	} else {
		metrics.fields["activeAccounts"] = activeAccounts
	}

	var totalBalance, totalUsed float64
	if err := h.db.Get(&totalBalance, `
			SELECT COALESCE(SUM(COALESCE(a.balance, 0)), 0)
			FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
		metrics.markFailed("totalBalance", "load total balance", err)
	} else {
		metrics.fields["totalBalance"] = roundMicro(totalBalance)
	}
	if err := h.db.Get(&totalUsed, `
			SELECT COALESCE(SUM(COALESCE(a.balance_used, 0)), 0)
			FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
		metrics.markFailed("totalUsed", "load total used", err)
	} else {
		metrics.fields["totalUsed"] = roundMicro(totalUsed)
	}

	// 24h proxy window (UTC) — single-pass aggregate with effective tokens.
	since24h := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	var proxy24h struct {
		Total       int     `db:"total"`
		Success     int     `db:"success"`
		TotalTokens int64   `db:"total_tokens"`
		TotalCost   float64 `db:"total_cost"`
	}
	if err := h.db.Get(&proxy24h, `
			SELECT
				COUNT(*) AS total,
				COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END), 0) AS success,
				COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS total_tokens,
				COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS total_cost
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE pl.created_at >= ? AND s.status = 'active'
		`, since24h); err != nil {
		metrics.markFailed("proxy24h", "load 24h proxy metrics", err)
	} else {
		metrics.fields["proxy24h"] = map[string]any{
			"total":       proxy24h.Total,
			"success":     proxy24h.Success,
			"totalTokens": proxy24h.TotalTokens,
			"totalCost":   roundMicro(proxy24h.TotalCost),
		}
		// Legacy flat fields kept for older clients / tests.
		metrics.fields["totalTokens"] = proxy24h.TotalTokens
		metrics.fields["totalCost"] = roundMicro(proxy24h.TotalCost)
	}

	dailyMetrics, metricsErr := dailyservice.CollectDailySummaryMetrics(h.db.DB, now)
	if metricsErr != nil {
		metrics.markFailed("todayMetricStatus", "load local-day metrics", metricsErr)
		metrics.fields["todaySpend"] = nil
		metrics.fields["todayReward"] = nil
		metrics.fields["todayCheckin"] = nil
		metrics.fields["todayMetricStatus"] = map[string]any{
			"status": "partial",
			"reason": "metrics collection failed",
		}
	} else {
		metrics.fields["todaySpend"] = roundMicro(dailyMetrics.TodaySpend)
		metrics.fields["todayReward"] = roundMicro(dailyMetrics.TodayReward)
		metrics.fields["todayCheckin"] = map[string]any{
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
		metrics.fields["todayMetricStatus"] = map[string]any{
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
	}

	// Performance window: last 60s request/token rate from proxy_logs.
	windowSeconds := 60
	sincePerf := time.Now().UTC().Add(-time.Duration(windowSeconds) * time.Second).Format(time.RFC3339)
	var perf struct {
		Requests int64 `db:"requests"`
		Tokens   int64 `db:"tokens"`
	}
	if err := h.db.Get(&perf, `
			SELECT
				COUNT(*) AS requests,
				COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE pl.created_at >= ? AND s.status = 'active'
		`, sincePerf); err != nil {
		metrics.markFailed("performance", "load performance metrics", err)
	} else {
		rpm := float64(perf.Requests) * 60 / float64(windowSeconds)
		tpm := float64(perf.Tokens) * 60 / float64(windowSeconds)
		metrics.fields["performance"] = map[string]any{
			"windowSeconds":     windowSeconds,
			"requestsPerMinute": rpm,
			"tokensPerMinute":   tpm,
		}
	}
}
