package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type dashboardMetrics struct {
	fields map[string]any
	failed []string
}

func (m *dashboardMetrics) markFailed(field, operation string, err error) {
	slog.Error("dashboard stats query failed; degrading to partial", "operation", operation, "field", field, "error", err)
	m.failed = append(m.failed, field)
	m.fields[field] = nil
}

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
		metrics := dashboardMetrics{
			fields: map[string]any{"generatedAt": generatedAt},
			failed: make([]string, 0, 8),
		}
		if view == "summary" || view == "full" {
			h.collectDashboardSummary(&metrics, now)
		}
		if view == "insights" || view == "full" {
			h.collectDashboardInsights(&metrics)
		}

		// Aggregate per-metric health so the frontend can distinguish a fully
		// healthy response from one where individual queries degraded to null.
		dashboardStatus := "complete"
		if len(metrics.failed) > 0 {
			dashboardStatus = "partial"
		}
		metrics.fields["dashboardStatus"] = map[string]any{
			"status": dashboardStatus,
			"failed": metrics.failed,
		}

		// Truly fatal: every metric failed (e.g. DB connection lost, every table
		// missing). Return 500 and do NOT cache — the response has no useful
		// data and replaying it for 10s would hide a real outage. Individual
		// query failures (some succeed, some null) are NOT fatal and fall
		// through to the 200 + cache path below.
		hasRealData := false
		for key, value := range metrics.fields {
			if key == "generatedAt" || key == "dashboardStatus" || key == "todayMetricStatus" {
				continue
			}
			if value != nil {
				hasRealData = true
				break
			}
		}
		if !hasRealData {
			slog.Error("dashboard stats: all metrics failed; returning 500", "failedCount", len(metrics.failed))
			return nil, fmt.Errorf("all %d metric queries failed", len(metrics.failed))
		}

		// Marshal once; getOrCompute stores the bytes in the cache. Partial
		// responses (some fields null) are cached too — they are still HTTP 200
		// and better than a 500 blank. The 10s TTL keeps recovery quick; ?force=1
		// bypasses for a hard refresh. json.Encoder appends a newline, matching
		// shared.WriteJSON, so a later cache-hit response is byte-identical to
		// the miss that populated it.
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(metrics.fields); err != nil {
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
