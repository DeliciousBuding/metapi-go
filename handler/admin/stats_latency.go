package admin

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
)

func (h *statsHandler) slowRequests(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, 50, 200)
	minLatencyMs := clampInt(getQueryInt(r, "minLatencyMs", 1000), 0, 3_600_000)
	hours := clampInt(getQueryInt(r, "hours", 24), 1, 168)
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Format(time.RFC3339)

	rows, err := queryRowsErr(h.db, `
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
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load slow requests")
		return
	}

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
	rows, err := queryRowsErr(h.db, `
		SELECT `+modelExpr+` AS model,
			COUNT(*) AS calls,
			COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS cost,
			COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`), 0) AS tokens
		FROM proxy_logs pl
		WHERE pl.created_at >= ?
		GROUP BY `+modelExpr+`
		ORDER BY cost DESC, model ASC
	`, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load model cost distribution")
		return
	}

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

	rows, err := queryRowsErr(h.db, `
		SELECT (COALESCE(pl.latency_ms, 0) / ?) * ? AS bucket_start,
			COUNT(*) AS count
		FROM proxy_logs pl
		WHERE pl.created_at >= ?
		GROUP BY bucket_start
		ORDER BY bucket_start ASC
	`, bucketMs, bucketMs, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load latency histogram")
		return
	}

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
	rows, err := queryRowsErr(h.db, `
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
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load latency trend")
		return
	}

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
		samples, err := queryRowsErr(h.db, `
			SELECT pl.latency_ms AS latency_ms, COUNT(*) OVER () AS total
			FROM proxy_logs pl
			WHERE pl.created_at >= ? AND pl.created_at < ?
				AND COALESCE(pl.latency_ms, 0) > 0
			ORDER BY pl.latency_ms DESC
			LIMIT ?
		`, dayStart, dayEnd, p95SampleCap)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load latency trend samples")
			return
		}
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
