package admin

import (
	"fmt"
	"net/http"
	"sort"
	"time"
)

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

	rows, err := queryRowsErr(h.db, q, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load balance history")
		return
	}
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

	rows, err := queryRowsErr(h.db, q, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load balance income/outcome")
		return
	}

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
	limit, _ := parseLimitOffset(r, 20, 100)
	items := make([]attentionItem, 0, limit)

	// 1. Expired accounts — critical.
	expired, err := queryRowsErr(h.db, `SELECT id, username, site_id, updated_at
		FROM accounts WHERE status = 'expired' ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load attention items")
		return
	}
	for _, row := range expired {
		items = append(items, attentionItem{
			Severity: "critical", Category: "expired_account",
			Label:     "Account expired: " + coerceString(row["username"]),
			Target:    "/accounts?accountId=" + coerceString(row["id"]),
			CreatedAt: coerceString(row["updatedAt"]),
		})
		if len(items) >= limit {
			writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
			return
		}
	}

	// 2. Low-balance accounts (< 1.0) — warning. Matches G1 threshold.
	low, err := queryRowsErr(h.db, `SELECT id, username, balance, site_id
		FROM accounts WHERE status = 'active' AND COALESCE(balance, 0) < 1.0
		ORDER BY balance ASC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load attention items")
		return
	}
	for _, row := range low {
		items = append(items, attentionItem{
			Severity: "warning", Category: "low_balance",
			Label:     fmt.Sprintf("Low balance: %s (%.2f)", coerceString(row["username"]), coerceFloat(row["balance"])),
			Target:    "/accounts?accountId=" + coerceString(row["id"]),
			CreatedAt: coerceString(row["updatedAt"]),
		})
		if len(items) >= limit {
			writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
			return
		}
	}

	// 3. Disabled sites — warning.
	disabledSites, err := queryRowsErr(h.db, `SELECT id, name, updated_at
		FROM sites WHERE status = 'disabled' ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load attention items")
		return
	}
	for _, row := range disabledSites {
		items = append(items, attentionItem{
			Severity: "warning", Category: "disabled_site",
			Label: "Site disabled: " + coerceString(row["name"]),
			// `/sites?edit=N` is the sites page's one-shot edit deep link
			// (opens the edit dialog for the referenced site then strips
			// the param); `siteId` is not part of the sites URL contract.
			Target:    "/sites?edit=" + coerceString(row["id"]),
			CreatedAt: coerceString(row["updatedAt"]),
		})
		if len(items) >= limit {
			writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
			return
		}
	}

	// 4. Recent unread warning/error events — info/warning (deep-link to events).
	since24h := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	evRows, err := queryRowsErr(h.db, `SELECT type, title, level, related_id, related_type, created_at
		FROM events WHERE level IN ('warning', 'error') AND created_at >= ?
		ORDER BY created_at DESC LIMIT ?`, since24h, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load attention items")
		return
	}
	for _, row := range evRows {
		severity := coerceString(row["level"])
		if severity == "error" {
			severity = "critical"
		}
		items = append(items, attentionItem{
			Severity: severity,
			Category: "event",
			Label:    coerceString(row["title"]),
			// The event log lives under settings → system-info → program-logs
			// (/settings/<subarea>/<section>); a bare /settings link landed on
			// the general section with no events in sight.
			Target:    "/settings/system-info/program-logs",
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
