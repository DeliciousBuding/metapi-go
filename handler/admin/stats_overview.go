package admin

import (
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
)

// overview gives every analytical panel one bounded window. Current resource
// balances and maintenance remain on their existing snapshot endpoints.
func (h *statsHandler) overview(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d"
	}
	now := time.Now().UTC()
	var from time.Time
	switch period {
	case "24h":
		from = now.Add(-24 * time.Hour)
	case "7d":
		from = now.AddDate(0, 0, -7)
	case "30d":
		from = now.AddDate(0, 0, -30)
	case "all":
	default:
		writeError(w, http.StatusBadRequest, "period must be 24h, 7d, 30d or all")
		return
	}
	window := map[string]string{"to": now.Format(time.RFC3339)}
	condition := "pl.created_at <= ?"
	args := []any{window["to"]}
	if !from.IsZero() {
		window["from"] = from.Format(time.RFC3339)
		condition += " AND pl.created_at >= ?"
		args = append(args, window["from"])
	}
	bucket := dayBucketSQLExpr(h.db, "pl.created_at")
	if period == "24h" {
		bucket = hourBucketSQLExpr(h.db, "pl.created_at")
	}
	requestBucket := dayBucketSQLExpr(h.db, "correlated.created_at")
	if period == "24h" {
		requestBucket = hourBucketSQLExpr(h.db, "correlated.created_at")
	}
	points, err := queryRowsErr(h.db, `SELECT `+bucket+` AS date,
  COUNT(*) AS requests,
  COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END),0) AS success_count,
  COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`),0) AS tokens,
  COALESCE(SUM(pl.estimated_cost),0) AS cost,
  COALESCE(SUM(CASE WHEN pl.latency_ms > 0 THEN pl.latency_ms ELSE 0 END),0) AS latency_sum,
  COALESCE(SUM(CASE WHEN pl.latency_ms > 0 THEN 1 ELSE 0 END),0) AS latency_count
  FROM proxy_logs pl WHERE `+condition+` GROUP BY `+bucket+` ORDER BY date`, args...)
	if err != nil {
		writeError(w, 500, "failed to load overview trend")
		return
	}
	requestPoints, err := queryRowsErr(h.db, `WITH correlated AS (
  SELECT pl.request_id, pl.status, pl.created_at,
         ROW_NUMBER() OVER (
           PARTITION BY pl.request_id
           ORDER BY COALESCE(pl.retry_count, 0) DESC, pl.id DESC
         ) AS terminal_rank
  FROM proxy_logs pl
  WHERE COALESCE(pl.request_id, '') <> '' AND `+condition+`
)
SELECT `+requestBucket+` AS date,
  COUNT(*) AS requests,
  COALESCE(SUM(CASE WHEN correlated.status = 'success' THEN 1 ELSE 0 END), 0) AS success_count
FROM correlated
WHERE terminal_rank = 1
GROUP BY `+requestBucket+`
ORDER BY date`, args...)
	if err != nil {
		writeError(w, 500, "failed to load request overview trend")
		return
	}
	var total, success, tokens, latencyCount int64
	var cost, latencySum float64
	for _, p := range points {
		count := coerceInt64(p["requests"])
		ok := coerceInt64(p["successCount"])
		total += count
		success += ok
		tokens += coerceInt64(p["tokens"])
		cost += coerceFloat(p["cost"])
		latencySum += coerceFloat(p["latencySum"])
		latencyCount += coerceInt64(p["latencyCount"])
		p["successRate"] = float64(ok) / float64(count)
		delete(p, "latencySum")
		delete(p, "latencyCount")
	}
	var requestTotal, requestSuccess int64
	for _, p := range requestPoints {
		count := coerceInt64(p["requests"])
		ok := coerceInt64(p["successCount"])
		requestTotal += count
		requestSuccess += ok
		p["successRate"] = float64(ok) / float64(count)
	}
	var legacyAttemptCount int64
	if err := h.db.Get(&legacyAttemptCount, rebindAdminQuery(h.db, `
		SELECT COUNT(*) FROM proxy_logs pl
		WHERE COALESCE(pl.request_id, '') = '' AND `+condition), args...); err != nil {
		writeError(w, 500, "failed to load request overview completeness")
		return
	}
	summary := map[string]any{"totalCount": total, "successCount": success, "failedCount": total - success, "totalTokensAll": tokens, "totalCost": roundMicro(cost), "averageLatencyMs": nil}
	if latencyCount > 0 {
		summary["averageLatencyMs"] = round1(latencySum / float64(latencyCount))
	}
	requestPartialReasons := []string{}
	if legacyAttemptCount > 0 {
		requestPartialReasons = append(requestPartialReasons, "requestIdMissing")
	}
	requestSummary := map[string]any{
		"totalCount":         requestTotal,
		"successCount":       requestSuccess,
		"failedCount":        requestTotal - requestSuccess,
		"legacyAttemptCount": legacyAttemptCount,
		"partial":            legacyAttemptCount > 0,
		"partialReasons":     requestPartialReasons,
	}
	sites, err := queryRowsErr(h.db, `SELECT s.id AS site_id, s.name AS site_name,
  COUNT(pl.id) AS total_requests,
  COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END),0) AS success_count,
  COALESCE(SUM(CASE WHEN pl.id IS NOT NULL AND COALESCE(pl.status,'') <> 'success' THEN 1 ELSE 0 END),0) AS failed_count,
  AVG(CASE WHEN pl.latency_ms > 0 THEN pl.latency_ms END) AS average_latency_ms
  FROM sites s LEFT JOIN accounts a ON a.site_id=s.id
  LEFT JOIN proxy_logs pl ON pl.account_id=a.id AND `+condition+`
  WHERE s.status='active' GROUP BY s.id,s.name
  ORDER BY failed_count DESC,total_requests DESC,s.name LIMIT 8`, args...)
	if err != nil {
		writeError(w, 500, "failed to load overview upstreams")
		return
	}
	model := `COALESCE(NULLIF(pl.model_actual,''),NULLIF(pl.model_requested,''),'unknown')`
	models, err := queryRowsErr(h.db, `SELECT `+model+` AS model, COUNT(*) AS calls,
  COALESCE(SUM(pl.estimated_cost),0) AS cost,
  COALESCE(SUM(`+service.EffectiveProxyTokensSQL+`),0) AS tokens
  FROM proxy_logs pl WHERE `+condition+` GROUP BY `+model+` ORDER BY cost DESC,model`, args...)
	if err != nil {
		writeError(w, 500, "failed to load overview models")
		return
	}
	if len(models) > 5 {
		var otherCalls, otherTokens int64
		var otherCost float64
		for _, m := range models[5:] {
			otherCalls += coerceInt64(m["calls"])
			otherTokens += coerceInt64(m["tokens"])
			otherCost += coerceFloat(m["cost"])
		}
		models = append(models[:5], map[string]any{"model": "other", "calls": otherCalls, "tokens": otherTokens, "cost": roundMicro(otherCost)})
	}
	if points == nil {
		points = []map[string]any{}
	}
	if requestPoints == nil {
		requestPoints = []map[string]any{}
	}
	if sites == nil {
		sites = []map[string]any{}
	}
	if models == nil {
		models = []map[string]any{}
	}
	gatewaySummary, gatewayPoints, err := h.gatewayRequestOverview(period, from, now)
	if err != nil {
		h.gatewayRequestLoadError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"gatewayRequestSummary": gatewaySummary,
		"gatewayRequestPoints":  gatewayPoints,
		"period":                period,
		"window":                window,
		"summary":               summary,
		"requestSummary":        requestSummary,
		"points":                points,
		"requestPoints":         requestPoints,
		"siteAvailability":      sites,
		"models":                models,
	})
}
