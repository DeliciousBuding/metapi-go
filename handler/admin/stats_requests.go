package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

type gatewayRequestListItem = store.GatewayRequest

type gatewayRequestSummary struct {
	TotalCount     int64 `db:"total_count" json:"totalCount"`
	CompletedCount int64 `db:"completed_count" json:"completedCount"`
	FailedCount    int64 `db:"failed_count" json:"failedCount"`
	CancelledCount int64 `db:"cancelled_count" json:"cancelledCount"`
	RejectedCount  int64 `db:"rejected_count" json:"rejectedCount"`
}

const gatewayRequestAggregateSQL = `COUNT(*) AS total_count,
	COALESCE(SUM(CASE WHEN gr.terminal_status = 'completed' THEN 1 ELSE 0 END),0) AS completed_count,
	COALESCE(SUM(CASE WHEN gr.terminal_status IN ('upstream_failed','output_interrupted','gateway_failed') THEN 1 ELSE 0 END),0) AS failed_count,
	COALESCE(SUM(CASE WHEN gr.terminal_status = 'client_cancelled' THEN 1 ELSE 0 END),0) AS cancelled_count,
	COALESCE(SUM(CASE WHEN gr.terminal_status IN ('rejected_no_route','rejected_policy') THEN 1 ELSE 0 END),0) AS rejected_count`

// gatewayRequestFilter is shared by rows and summary: filtering one must not
// silently leave another panel showing an unfiltered denominator.
func gatewayRequestFilter(r *http.Request) (string, []any, int, int, error) {
	q := r.URL.Query()
	limit, offset := 50, 0
	for _, spec := range []struct {
		name     string
		min, max int
		target   *int
	}{{"limit", 1, 100, &limit}, {"offset", 0, 0, &offset}} {
		if raw, present := q[spec.name]; present {
			if len(raw) != 1 {
				return "", nil, 0, 0, fmt.Errorf("%s must be a single integer", spec.name)
			}
			value, err := strconv.Atoi(raw[0])
			if err != nil || value < spec.min || (spec.max > 0 && value > spec.max) {
				return "", nil, 0, 0, fmt.Errorf("%s is out of range", spec.name)
			}
			*spec.target = value
		}
	}
	conditions := []string{"1=1"}
	args := []any{}
	if status := q.Get("status"); status != "" {
		switch status {
		case "completed", "upstream_failed", "output_interrupted", "client_cancelled", "rejected_no_route", "rejected_policy", "gateway_failed":
			conditions = append(conditions, "gr.terminal_status = ?")
			args = append(args, status)
		default:
			return "", nil, 0, 0, fmt.Errorf("invalid request status")
		}
	}
	if search := strings.TrimSpace(q.Get("search")); search != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(search)) + "%"
		conditions = append(conditions, `(LOWER(COALESCE(gr.model_requested,'')) LIKE ? ESCAPE '\' OR LOWER(gr.id) LIKE ? ESCAPE '\' OR LOWER(COALESCE(gr.external_request_id,'')) LIKE ? ESCAPE '\' OR LOWER(COALESCE(gr.reason_code,'')) LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like, like)
	}
	if raw := q.Get("downstreamKeyId"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return "", nil, 0, 0, fmt.Errorf("downstreamKeyId must be a positive integer")
		}
		conditions = append(conditions, "gr.downstream_api_key_id = ?")
		args = append(args, id)
	}
	var bounds [2]time.Time
	for i, name := range []string{"from", "to"} {
		if raw := q.Get(name); raw != "" {
			value, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				return "", nil, 0, 0, fmt.Errorf("%s must be RFC3339", name)
			}
			bounds[i] = value
			operator := ">="
			if i == 1 {
				operator = "<="
			}
			conditions = append(conditions, "gr.started_at "+operator+" ?")
			args = append(args, value.UTC().Format(store.GatewayRequestTimeLayout))
		}
	}
	if !bounds[0].IsZero() && !bounds[1].IsZero() && bounds[0].After(bounds[1]) {
		return "", nil, 0, 0, fmt.Errorf("from must not be after to")
	}
	return strings.Join(conditions, " AND "), args, limit, offset, nil
}

func (h *statsHandler) gatewayRequests(w http.ResponseWriter, r *http.Request) {
	where, args, limit, offset, err := gatewayRequestFilter(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	var summary gatewayRequestSummary
	if err := h.db.GetContext(r.Context(), &summary, rebindAdminQuery(h.db, `SELECT `+gatewayRequestAggregateSQL+` FROM gateway_requests gr WHERE `+where), args...); err != nil {
		h.gatewayRequestLoadError(w, r, err)
		return
	}
	var coverage *string
	if err := h.db.GetContext(r.Context(), &coverage, `SELECT MIN(started_at) FROM gateway_requests`); err != nil {
		h.gatewayRequestLoadError(w, r, err)
		return
	}
	items := []gatewayRequestListItem{}
	pageArgs := append(append([]any{}, args...), limit, offset)
	if err := h.db.SelectContext(r.Context(), &items, rebindAdminQuery(h.db, `SELECT gr.*
		FROM gateway_requests gr WHERE `+where+` ORDER BY gr.started_at DESC,gr.id DESC LIMIT ? OFFSET ?`), pageArgs...); err != nil {
		h.gatewayRequestLoadError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": summary.TotalCount, "page": offset/limit + 1, "pageSize": limit, "summary": summary, "coverageStartedAt": coverage})
}

func (h *statsHandler) gatewayRequestDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" || len(id) > 256 {
		writeError(w, 400, "invalid request ID")
		return
	}
	var entry store.GatewayRequest
	err := h.db.GetContext(r.Context(), &entry, rebindAdminQuery(h.db, `SELECT * FROM gateway_requests WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "request not found")
		return
	}
	if err != nil {
		h.gatewayRequestLoadError(w, r, err)
		return
	}
	attempts, err := queryRowsErr(h.db, `SELECT pl.*, a.username, s.id AS site_id,s.name AS site_name,
		dk.name AS downstream_key_name FROM proxy_logs pl
		LEFT JOIN accounts a ON a.id = pl.account_id LEFT JOIN sites s ON s.id = a.site_id
		LEFT JOIN downstream_api_keys dk ON dk.id = pl.downstream_api_key_id
		WHERE pl.gateway_request_id = ? ORDER BY pl.retry_count,pl.id`, id)
	if err != nil {
		h.gatewayRequestLoadError(w, r, err)
		return
	}
	if attempts == nil {
		attempts = []map[string]any{}
	}
	for _, attempt := range attempts {
		attempt["billingDetails"] = decodeProxyLogBilling(attempt["billingDetails"])
		normalizeProxyLogRow(attempt)
	}
	writeJSON(w, 200, struct {
		store.GatewayRequest
		Partial  bool             `json:"partial"`
		Attempts []map[string]any `json:"attempts"`
	}{entry, len(attempts) != entry.AttemptCount, attempts})
}

func (h *statsHandler) gatewayRequestLoadError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("gateway request query failed", "error", err)
	writeErrorCodeWithRequest(w, r, 500, ErrorCodeResourceLoadFailed, "failed to load gateway requests")
}

func (h *statsHandler) gatewayRequestOverview(period string, from, to time.Time) (map[string]any, []map[string]any, error) {
	where := "gr.started_at <= ?"
	args := []any{to.UTC().Format(store.GatewayRequestTimeLayout)}
	if !from.IsZero() {
		where += " AND gr.started_at >= ?"
		args = append(args, from.UTC().Format(store.GatewayRequestTimeLayout))
	}
	summary, err := queryRow(h.db, `SELECT `+gatewayRequestAggregateSQL+` FROM gateway_requests gr WHERE `+where, args...)
	if err != nil {
		return nil, nil, err
	}
	var coverage *string
	if err := h.db.Get(&coverage, `SELECT MIN(started_at) FROM gateway_requests`); err != nil {
		return nil, nil, err
	}
	summary["coverageStartedAt"] = coverage
	bucket := dayBucketSQLExpr(h.db, "gr.started_at")
	if period == "24h" {
		bucket = hourBucketSQLExpr(h.db, "gr.started_at")
	}
	points, err := queryRowsErr(h.db, `SELECT `+bucket+` AS date, `+gatewayRequestAggregateSQL+` FROM gateway_requests gr WHERE `+where+` GROUP BY `+bucket+` ORDER BY date`, args...)
	if err != nil {
		return nil, nil, err
	}
	if points == nil {
		points = []map[string]any{}
	}
	for _, point := range points {
		point["requests"] = point["totalCount"]
		delete(point, "totalCount")
	}
	return summary, points, nil
}
