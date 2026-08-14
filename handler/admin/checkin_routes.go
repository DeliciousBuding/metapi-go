package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/deliciousbuding/metapi-go/config"
	checkinservice "github.com/deliciousbuding/metapi-go/service/checkin"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterCheckinRoutes registers all /api/checkin routes.
func RegisterCheckinRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	handler := &checkinHandler{db: db, cfg: cfg}

	r.Post("/api/checkin/trigger", handler.triggerAll)
	r.Post("/api/checkin/trigger/{id}", handler.triggerOne)
	r.Get("/api/checkin/logs", handler.getLogs)
	r.Put("/api/checkin/schedule", handler.updateSchedule)
}

type checkinHandler struct {
	db  *sqlx.DB
	cfg *config.Config
}

// POST /api/checkin/trigger
func (h *checkinHandler) triggerAll(w http.ResponseWriter, r *http.Request) {
	results := checkinservice.CheckinAll(h.cfg, h.db, nil, "manual")
	summary := summarizeCheckinResults(results)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": summary.Failed == 0,
		"queued":  false,
		"status":  "completed",
		"message": "签到执行完成",
		"summary": summary,
		"results": results,
	})
}

// POST /api/checkin/trigger/:id
func (h *checkinHandler) triggerOne(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	result := checkinservice.CheckinAccount(h.cfg, h.db, id, &checkinservice.CheckinOptions{
		ScheduleMode: "manual",
	})
	if result.Message == "account not found" {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false,
			"message": result.Message,
			"id":      id,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": result.Success,
		"status":  result.Status,
		"skipped": result.Skipped,
		"message": result.Message,
		"reward":  result.Reward,
		"id":      id,
	})
}

// GET /api/checkin/logs?limit=&offset=&accountId=
//
// Response shape is the nested TS-era form the frontend expects:
//
//	{
//	 "checkin_logs": { id, accountId, status, message, reward, createdAt },
//	 "accounts": { username, id },
//	 "sites": { name, url },
//	 "failureReason": { code, category, title, actionHint, detailHint } | null
//	}
//
// failureReason is lifted from the checkin_logs.failure_reason TEXT column
// (ClassifyFailureReason JSON) to a parsed object at the top level so the UI
// can read `log.failureReason.title` directly instead of a phantom always-`-`.
func (h *checkinHandler) getLogs(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, 50, 500)
	offset := max(0, getQueryInt(r, "offset", 0))
	accountIDStr := r.URL.Query().Get("accountId")

	query := `SELECT cl.*, a.username as account_username, s.name as site_name, s.url as site_url
		FROM checkin_logs cl
		INNER JOIN accounts a ON cl.account_id = a.id
		INNER JOIN sites s ON a.site_id = s.id`
	var args []any

	if accountIDStr != "" {
		if aid, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil && aid > 0 {
			query += " WHERE cl.account_id = ?"
			args = append(args, aid)
		}
	}

	query += " ORDER BY cl.created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows := queryRows(h.db, query, args...)
	writeJSON(w, http.StatusOK, reshapeCheckinLogs(rows))
}

// reshapeCheckinLogs converts the flat camelCased query rows into the nested
// {checkin_logs, accounts, sites, failureReason} shape and parses the stored
// failure_reason JSON string into a structured object (nil when absent/invalid).
func reshapeCheckinLogs(rows []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"checkin_logs": map[string]any{
				"id":        row["id"],
				"accountId": row["accountId"],
				"status":    row["status"],
				"message":   row["message"],
				"reward":    row["reward"],
				"createdAt": row["createdAt"],
			},
			"accounts": map[string]any{
				"username": row["accountUsername"],
				"id":       row["accountId"],
			},
			"sites": map[string]any{
				"name": row["siteName"],
				"url":  row["siteUrl"],
			},
			"failureReason": parseFailureReasonJSON(row["failureReason"]),
		})
	}
	return out
}

// parseFailureReasonJSON decodes the failure_reason TEXT column into a
// map[string]any. Returns nil for NULL/empty/garbage so the API yields a
// clean `null` rather than a raw string the UI cannot read `.title` from.
func parseFailureReasonJSON(raw any) any {
	if raw == nil {
		return nil
	}
	var encoded string
	switch v := raw.(type) {
	case string:
		encoded = v
	case []byte:
		encoded = string(v)
	default:
		return nil
	}
	if encoded == "" {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(encoded), &parsed); err != nil {
		return nil
	}
	return parsed
}

// PUT /api/checkin/schedule
func (h *checkinHandler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode          *string `json:"mode,omitempty"`
		Cron          *string `json:"cron,omitempty"`
		IntervalHours *int    `json:"intervalHours,omitempty"`
		// E1: random-window mode bounds (HH:mm, 24h).
		WindowStart *string `json:"windowStart,omitempty"`
		WindowEnd   *string `json:"windowEnd,omitempty"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid body"})
		return
	}

	state, err := applyCheckinScheduleSettings(h.db, h.cfg, checkinSchedulePatch{
		Mode:          body.Mode,
		Cron:          body.Cron,
		IntervalHours: body.IntervalHours,
		WindowStart:   body.WindowStart,
		WindowEnd:     body.WindowEnd,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"mode":          state.Mode,
		"cron":          state.Cron,
		"intervalHours": state.IntervalHours,
		"windowStart":   state.WindowStart,
		"windowEnd":     state.WindowEnd,
	})
}

type checkinSummary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

func summarizeCheckinResults(results []checkinservice.CheckinAllResult) checkinSummary {
	summary := checkinSummary{Total: len(results)}
	for _, item := range results {
		switch item.Result.Status {
		case checkinservice.CheckinSuccess:
			summary.Success++
		case checkinservice.CheckinSkipped:
			summary.Skipped++
		default:
			summary.Failed++
		}
	}
	return summary
}
