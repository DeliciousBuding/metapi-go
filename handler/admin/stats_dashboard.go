package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	dailyservice "github.com/deliciousbuding/metapi-go/service/daily"
	"golang.org/x/sync/singleflight"
)

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
	// flight deduplicates concurrent cache-miss computes for the same view so
	// N admin sessions polling an expired entry share one expensive
	// aggregation run instead of running it N× (thundering herd).
	flight singleflight.Group
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

// getOrCompute returns the cached payload for view, or computes it via the
// supplied function under single-flight dedup so N concurrent admin sessions
// hitting a cold/expired cache share one expensive aggregation run instead of
// running it N× (thundering herd). A leader that populates the cache mid-flight
// lets concurrent followers short-circuit on the re-check inside Do.
//
// Returns (data, hit, err): hit reports whether the fast-path cache served the
// bytes (for the x-dashboard-summary-cache response header). A single-flight-
// shared compute is reported as a miss since this request did not get an
// instant cached response — only successful computes are stored, so errors
// never poison the cache.
func (c *dashboardSummaryCache) getOrCompute(view string, compute func() ([]byte, error)) ([]byte, bool, error) {
	if cached, hit := c.get(view); hit {
		return cached, true, nil
	}
	result, err, _ := c.flight.Do(view, func() (any, error) {
		// Re-check: a concurrent leader may have populated the cache while we
		// waited for the single-flight slot.
		if cached, hit := c.get(view); hit {
			return cached, nil
		}
		data, err := compute()
		if err != nil {
			return nil, err
		}
		c.set(view, data)
		return data, nil
	})
	if err != nil {
		return nil, false, err
	}
	return result.([]byte), false, nil
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

	// Cache hit short-circuits the expensive aggregation queries; on a miss
	// the single-flight group deduplicates concurrent computes for the same
	// view so N admin sessions polling an expired entry share one aggregation
	// run instead of running it N× (thundering herd). ?force=1 cleared the
	// cache above, so a force request always enters the compute path.
	data, cacheHit, computeErr := globalDashboardCache.getOrCompute(view, func() ([]byte, error) {
		now := time.Now()
		generatedAt := now.UTC().Format(time.RFC3339)
		result := map[string]any{
			"generatedAt": generatedAt,
		}

		// Fail-open: a single aggregation query failing must not blank the whole
		// dashboard. Each sub-query is wrapped in individual error handling — on
		// failure the field gets a null marker, the error is logged, and the
		// field name is recorded in failedMetrics. Mirrors the todayMetricStatus
		// degrade-to-partial pattern from accounts.go (listAccounts).
		failedMetrics := make([]string, 0, 8)
		markFailed := func(field, operation string, err error) {
			slog.Error("dashboard stats query failed; degrading to partial", "operation", operation, "field", field, "error", err)
			failedMetrics = append(failedMetrics, field)
			result[field] = nil
		}

		if view == "summary" || view == "full" {
			var siteCount, accountCount, activeAccounts int
			var upstreamChannelCount int
			if err := h.db.Get(&upstreamChannelCount, "SELECT COUNT(*) FROM upstream_channels"); err != nil {
				markFailed("upstreamChannelCount", "load independent upstream count", err)
			} else {
				result["upstreamChannelCount"] = upstreamChannelCount
			}
			if err := h.db.Get(&siteCount, "SELECT COUNT(*) FROM sites"); err != nil {
				markFailed("siteCount", "load site count", err)
			} else {
				result["siteCount"] = siteCount
			}
			if err := h.db.Get(&accountCount, "SELECT COUNT(*) FROM accounts"); err != nil {
				markFailed("accountCount", "load account count", err)
				result["totalAccounts"] = nil
			} else {
				result["accountCount"] = accountCount
				result["totalAccounts"] = accountCount
			}
			if err := h.db.Get(&activeAccounts, `
			SELECT COUNT(*) FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE a.status = 'active' AND s.status = 'active'
		`); err != nil {
				markFailed("activeAccounts", "load active account count", err)
			} else {
				result["activeAccounts"] = activeAccounts
			}

			var totalBalance, totalUsed float64
			if err := h.db.Get(&totalBalance, `
			SELECT COALESCE(SUM(COALESCE(a.balance, 0)), 0)
			FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
				markFailed("totalBalance", "load total balance", err)
			} else {
				result["totalBalance"] = roundMicro(totalBalance)
			}
			if err := h.db.Get(&totalUsed, `
			SELECT COALESCE(SUM(COALESCE(a.balance_used, 0)), 0)
			FROM accounts a
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
				markFailed("totalUsed", "load total used", err)
			} else {
				result["totalUsed"] = roundMicro(totalUsed)
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
				markFailed("proxy24h", "load 24h proxy metrics", err)
			} else {
				result["proxy24h"] = map[string]any{
					"total":       proxy24h.Total,
					"success":     proxy24h.Success,
					"totalTokens": proxy24h.TotalTokens,
					"totalCost":   roundMicro(proxy24h.TotalCost),
				}
				// Legacy flat fields kept for older clients / tests.
				result["totalTokens"] = proxy24h.TotalTokens
				result["totalCost"] = roundMicro(proxy24h.TotalCost)
			}

			dailyMetrics, metricsErr := dailyservice.CollectDailySummaryMetrics(h.db, now)
			if metricsErr != nil {
				markFailed("todayMetricStatus", "load local-day metrics", metricsErr)
				result["todaySpend"] = nil
				result["todayReward"] = nil
				result["todayCheckin"] = nil
				result["todayMetricStatus"] = map[string]any{
					"status": "partial",
					"reason": "metrics collection failed",
				}
			} else {
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
				markFailed("performance", "load performance metrics", err)
			} else {
				rpm := float64(perf.Requests) * 60 / float64(windowSeconds)
				tpm := float64(perf.Tokens) * 60 / float64(windowSeconds)
				result["performance"] = map[string]any{
					"windowSeconds":     windowSeconds,
					"requestsPerMinute": rpm,
					"tokensPerMinute":   tpm,
				}
			}
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
				markFailed("totalTokens", "load total tokens", err)
			} else {
				result["totalTokens"] = totalTokens
			}
			var totalCost float64
			if err := h.db.Get(&totalCost, `
			SELECT COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0)
			FROM proxy_logs pl
			INNER JOIN accounts a ON a.id = pl.account_id
			INNER JOIN sites s ON s.id = a.site_id
			WHERE s.status = 'active'
		`); err != nil {
				markFailed("totalCost", "load total cost", err)
			} else {
				result["totalCost"] = roundMicro(totalCost)
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
				markFailed("siteAvailability", "load site availability", err)
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
				result["siteAvailability"] = siteAvailability
			}
		}

		// Aggregate per-metric health so the frontend can distinguish a fully
		// healthy response from one where individual queries degraded to null.
		dashboardStatus := "complete"
		if len(failedMetrics) > 0 {
			dashboardStatus = "partial"
		}
		result["dashboardStatus"] = map[string]any{
			"status": dashboardStatus,
			"failed": failedMetrics,
		}

		// Truly fatal: every metric failed (e.g. DB connection lost, every table
		// missing). Return 500 and do NOT cache — the response has no useful
		// data and replaying it for 10s would hide a real outage. Individual
		// query failures (some succeed, some null) are NOT fatal and fall
		// through to the 200 + cache path below.
		hasRealData := false
		for key, value := range result {
			if key == "generatedAt" || key == "dashboardStatus" || key == "todayMetricStatus" {
				continue
			}
			if value != nil {
				hasRealData = true
				break
			}
		}
		if !hasRealData {
			slog.Error("dashboard stats: all metrics failed; returning 500", "failedCount", len(failedMetrics))
			return nil, fmt.Errorf("all %d metric queries failed", len(failedMetrics))
		}

		// Marshal once; getOrCompute stores the bytes in the cache. Partial
		// responses (some fields null) are cached too — they are still HTTP 200
		// and better than a 500 blank. The 10s TTL keeps recovery quick; ?force=1
		// bypasses for a hard refresh. json.Encoder appends a newline, matching
		// shared.WriteJSON, so a later cache-hit response is byte-identical to
		// the miss that populated it.
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(result); err != nil {
			slog.Error("dashboard stats marshal failed", "error", err)
			return nil, fmt.Errorf("encode dashboard statistics: %w", err)
		}
		return buf.Bytes(), nil
	})

	if computeErr != nil {
		// Error responses still report "miss": the compute ran but produced no
		// cacheable payload, so the next request must recompute (errors are
		// never stored as cache hits).
		w.Header().Set("x-dashboard-summary-cache", "miss")
		w.Header().Set("x-dashboard-insights-cache", "miss")
		h.dashboardError(w, "load dashboard statistics", computeErr)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if cacheHit {
		w.Header().Set("x-dashboard-summary-cache", "hit")
	} else {
		w.Header().Set("x-dashboard-summary-cache", "miss")
	}
	w.Header().Set("x-dashboard-insights-cache", "miss")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
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
	writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "Failed to load dashboard statistics")
}
