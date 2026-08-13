package admin

import (
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterMonitorHealthRoute mounts the read-only observability health
// projection. It never hard-disables anything — the response only aggregates
// the in-memory runtime breaker state and the DB-backed channel cooldown /
// site+account health counters.
func RegisterMonitorHealthRoute(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	h := &monitorHealthHandler{
		db:               db,
		configuredMaxSec: cfg.TokenRouterFailureCooldownMaxSec,
	}
	r.Get("/api/monitor/health", h.health)
}

type monitorHealthHandler struct {
	db               *sqlx.DB
	configuredMaxSec int
}

// health is the read-only projection endpoint backing the observability
// Health section (site/account health + breaker/cooldown aggregation).
func (h *monitorHealthHandler) health(w http.ResponseWriter, r *http.Request) {
	runtimeHealth := routing.SnapshotRuntimeHealth()

	writeJSON(w, http.StatusOK, map[string]any{
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"runtimeHealth": runtimeHealth,
		"cooldown":      h.aggregateCooldown(),
		"sites":         h.statusCounts("sites"),
		"accounts":      h.statusCounts("accounts"),
	})
}

// aggregateCooldown projects route_channels cooldown/failure state without
// writing anything. Cooling means cooldownUntil is still in the future;
// recently-failed means failCount>0 with a lastFailAt inside the configured
// Fibonacci backoff window.
func (h *monitorHealthHandler) aggregateCooldown() map[string]any {
	rows := queryRows(h.db, `
		SELECT
			rc.id AS id,
			rc.account_id AS account_id,
			rc.fail_count AS fail_count,
			rc.last_fail_at AS last_fail_at,
			rc.cooldown_until AS cooldown_until,
			s.id AS site_id,
			COALESCE(s.name, '') AS site_name
		FROM route_channels rc
		LEFT JOIN accounts a ON a.id = rc.account_id
		LEFT JOIN sites s ON s.id = a.site_id
		WHERE rc.cooldown_until IS NOT NULL OR rc.fail_count > 0
		ORDER BY rc.cooldown_until DESC
		LIMIT 10000
	`)

	nowISO := time.Now().UTC().Format(time.RFC3339)
	nowMs := time.Now().UnixMilli()

	channelsCooling := 0
	channelsWithFailures := 0
	channelsRecentlyFailed := 0
	cooling := make([]map[string]any, 0)

	for _, row := range rows {
		failCount := coerceInt64(row["failCount"])
		cooldownUntil := nullableString(row["cooldownUntil"])
		lastFailAt := nullableString(row["lastFailAt"])

		if failCount > 0 {
			channelsWithFailures++
		}
		if routing.IsChannelRecentlyFailed(&failCount, lastFailAt, nowMs, h.configuredMaxSec) {
			channelsRecentlyFailed++
		}
		if routing.IsCooldownActive(cooldownUntil, nowISO) {
			channelsCooling++
			cooling = append(cooling, map[string]any{
				"channelId":     coerceInt64(row["id"]),
				"accountId":     coerceInt64(row["accountId"]),
				"siteId":        coerceInt64(row["siteId"]),
				"siteName":      coerceString(row["siteName"]),
				"failCount":     failCount,
				"cooldownUntil": coerceString(row["cooldownUntil"]),
			})
		}
	}

	return map[string]any{
		"channelsCooling":        channelsCooling,
		"channelsWithFailures":   channelsWithFailures,
		"channelsRecentlyFailed": channelsRecentlyFailed,
		"cooling":                cooling,
	}
}

// statusCounts aggregates a table's `status` column into total/active/
// disabled/other buckets. `table` is always a literal in this file.
func (h *monitorHealthHandler) statusCounts(table string) map[string]any {
	rows := queryRows(h.db, "SELECT status, COUNT(*) AS count FROM "+table+" GROUP BY status")
	result := map[string]any{
		"total":    0,
		"active":   0,
		"disabled": 0,
		"other":    0,
	}
	var total int64
	for _, row := range rows {
		count := coerceInt64(row["count"])
		total += count
		switch coerceString(row["status"]) {
		case "active":
			result["active"] = count
		case "disabled":
			result["disabled"] = count
		default:
			result["other"] = coerceInt64(result["other"]) + count
		}
	}
	result["total"] = total
	return result
}

// nullableString converts a nullable MapScan value into *string, treating nil
// and empty as nil so cooldown helpers see a genuinely absent timestamp.
func nullableString(v any) *string {
	if v == nil {
		return nil
	}
	s := coerceString(v)
	if s == "" {
		return nil
	}
	return &s
}
