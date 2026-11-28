package admin

import (
	"time"

	"github.com/deliciousbuding/metapi-go/service"
)

func (h *statsHandler) collectDashboardInsights(metrics *dashboardMetrics) {
	// All-time totals with effective token expression (no double count).
	var totalTokens int64
	if err := h.db.Get(&totalTokens, `
			SELECT COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0)
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
		metrics.markFailed("totalTokens", "load total tokens", err)
	} else {
		metrics.fields["totalTokens"] = totalTokens
	}
	var totalCost float64
	if err := h.db.Get(&totalCost, `
			SELECT COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0)
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
		metrics.markFailed("totalCost", "load total cost", err)
	} else {
		metrics.fields["totalCost"] = roundMicro(totalCost)
	}

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
		metrics.markFailed("siteAvailability", "load site availability", err)
	} else {
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
		metrics.fields["siteAvailability"] = siteAvailability
	}
}
