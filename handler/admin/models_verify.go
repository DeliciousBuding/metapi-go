package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
)

// nullableInt64 / nullableFloat64 map zero values to SQL NULL (optional cols).
func nullableInt64(v int64) any {
	if v <= 0 {
		return nil
	}
	return v
}

func nullableFloat64(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}

// ---- Batch Model Verification ----

// Operator-initiated verification of N models/channels in one pass, with a
// durable per-row history. Reuses the injected model-probe executor via
// ModelProbeScheduler.ProbeBatch — same lightweight probe the background
// scheduler uses — so results reflect real upstream reachability, and health
// outcomes are applied to routing when a recorder is wired.

// POST /api/models/verify-batch
// body: {"models": ["gpt-5"], "accountId": 0, "limit": 50}
// models empty = all enabled route channels; accountId > 0 narrows to one account.
func (h *statsHandler) verifyBatch(w http.ResponseWriter, r *http.Request) {
	sched := scheduler.GetGlobalModelProbeScheduler()
	if sched == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, ErrorCodeResourceDisabled, "model probe scheduler is not running (start schedulers)")
		return
	}

	var body struct {
		Models    []string `json:"models"`
		AccountID int64    `json:"accountId"`
		Limit     int      `json:"limit"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	limit := body.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = config.ClampInt(limit, 1, 200)

	models := make([]string, 0, len(body.Models))
	seen := make(map[string]struct{}, len(body.Models))
	for _, m := range body.Models {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		models = append(models, m)
	}

	// Load enabled route-channel targets (accounts/sites active). Filter by
	// explicit model list and/or accountId; capped by limit.
	q := `SELECT rc.id AS channel_id, rc.account_id, a.site_id, COALESCE(rc.source_model, '') AS model_name
		FROM route_channels rc
		INNER JOIN accounts a ON rc.account_id = a.id
		INNER JOIN sites st ON a.site_id = st.id
		WHERE rc.enabled = TRUE
		  AND a.status = 'active'
		  AND st.status = 'active'
		  AND COALESCE(rc.source_model, '') <> ''`
	var args []any
	if body.AccountID > 0 {
		q += ` AND rc.account_id = ?`
		args = append(args, body.AccountID)
	}
	if len(models) > 0 {
		placeholders := make([]string, 0, len(models))
		for _, m := range models {
			placeholders = append(placeholders, "?")
			args = append(args, m)
		}
		q += ` AND COALESCE(rc.source_model, '') IN (` + strings.Join(placeholders, ",") + `)`
	}
	q += ` ORDER BY rc.id ASC LIMIT ?`
	args = append(args, limit)

	rows, err := queryRowsErr(h.db, q, args...)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load verification targets")
		return
	}
	targets := make([]scheduler.ProbeTarget, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, scheduler.ProbeTarget{
			ChannelID: coerceInt64(row["channelId"]),
			AccountID: coerceInt64(row["accountId"]),
			SiteID:    coerceInt64(row["siteId"]),
			ModelName: coerceString(row["modelName"]),
		})
	}
	if len(targets) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"batchId": "",
			"probed":  0,
			"summary": map[string]any{"success": 0, "failure": 0, "inconclusive": 0, "skipped": 0},
			"items":   []any{},
			"note":    "no enabled route channels match the filter",
		})
		return
	}

	results := sched.ProbeBatch(r.Context(), targets, 0)
	batchID := fmt.Sprintf("vb-%d", time.Now().UTC().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)

	summary := map[string]int{"success": 0, "failure": 0, "inconclusive": 0, "skipped": 0}
	items := make([]map[string]any, 0, len(results))
	for _, res := range results {
		summary[res.Outcome.Status]++
		status := res.Outcome.Status
		if status == "" {
			status = "inconclusive"
		}
		items = append(items, map[string]any{
			"model":         res.Target.ModelName,
			"channelId":     res.Target.ChannelID,
			"accountId":     res.Target.AccountID,
			"siteId":        res.Target.SiteID,
			"status":        status,
			"latencyMs":     res.Outcome.LatencyMs,
			"httpStatus":    res.Outcome.HTTPStatus,
			"errorText":     res.Outcome.ErrorText,
			"healthApplied": res.HealthApplied,
		})
		// Durable per-row history (best-effort; verification is not blocked
		// by a failed history write).
		var errText *string
		if res.Outcome.ErrorText != "" {
			e := res.Outcome.ErrorText
			errText = &e
		}
		_, _ = h.db.Exec(rebindAdminQuery(h.db, `
			INSERT INTO model_verify_history
				(batch_id, model_name, channel_id, account_id, site_id, status, latency_ms, http_status, error_text, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			batchID,
			res.Target.ModelName,
			nullableInt64(res.Target.ChannelID),
			nullableInt64(res.Target.AccountID),
			nullableInt64(res.Target.SiteID),
			status,
			nullableFloat64(res.Outcome.LatencyMs),
			nullableInt64(int64(res.Outcome.HTTPStatus)),
			errText,
			now,
		)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"batchId": batchID,
		"probed":  len(results),
		"summary": summary,
		"items":   items,
	})
}

// GET /api/models/verify-history?limit=&model=
// Returns recent per-row verification history, newest first, with site names.
func (h *statsHandler) verifyHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, 50, 200)
	model := strings.TrimSpace(r.URL.Query().Get("model"))

	q := `SELECT v.id, v.batch_id, v.model_name, v.channel_id, v.account_id, v.site_id,
		v.status, v.latency_ms, v.http_status, v.error_text, v.created_at,
		COALESCE(s.name, '') AS site_name
		FROM model_verify_history v
		LEFT JOIN sites s ON s.id = v.site_id`
	var args []any
	if model != "" {
		q += ` WHERE v.model_name = ?`
		args = append(args, model)
	}
	q += ` ORDER BY v.created_at DESC, v.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := queryRowsErr(h.db, q, args...)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load verification history")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"id":         coerceInt64(row["id"]),
			"batchId":    coerceString(row["batchId"]),
			"model":      coerceString(row["modelName"]),
			"channelId":  row["channelId"],
			"accountId":  row["accountId"],
			"siteId":     row["siteId"],
			"siteName":   coerceString(row["siteName"]),
			"status":     coerceString(row["status"]),
			"latencyMs":  row["latencyMs"],
			"httpStatus": row["httpStatus"],
			"errorText":  row["errorText"],
			"createdAt":  coerceString(row["createdAt"]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
