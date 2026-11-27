package admin

import (
	"net/http"
	"strconv"
	"time"

	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterSchedulerStatusRoutes exposes a unified run-history view of the
// recurring schedulers. Each entry reports the job's
// last-run signal and a coarse activity window (24h) so operators can see at
// a glance which automations are healthy without opening per-job pages.

// Data sources are existing signals only — no scheduler code changes:
// - checkin: checkin_logs aggregate (status counts + latest timestamp)
// - balance-refresh: accounts.last_balance_refresh / last_checkin_at
// - oauth-refresh / sub2api-refresh: extra_config is JSON — we do not parse
// it here (cross-dialect); the events table carries token/balance alerts
// - model-probe: in-memory ProbeRunSummary (single-instance honest)
// - announcements: site_announcements.last_seen_at
// - daily-summary / log-cleanup / usage-aggregation: recent events rows
func RegisterSchedulerStatusRoutes(r chi.Router, db *store.DB) {
	h := &schedulerStatusHandler{db: db}
	r.Get("/api/scheduler/status", h.status)
}

type schedulerStatusHandler struct {
	db *store.DB
}

type schedulerRunStatus struct {
	Job        string `json:"job"`
	Enabled    bool   `json:"enabled"`
	LastRunAt  string `json:"lastRunAt,omitempty"`  // RFC3339 or ""
	LastStatus string `json:"lastStatus,omitempty"` // success | failed | running | never
	Runs24h    int64  `json:"runs24h"`
	Success24h int64  `json:"success24h"`
	Note       string `json:"note,omitempty"`
	// RecentRuns is populated only by model-probe (in-memory ring buffer).
	// Other jobs keep it empty — their history surfaces via per-job pages.
	RecentRuns []probeRunSummaryJSON `json:"recentRuns,omitempty"`
}

// probeRunSummaryJSON is the wire form of one scheduler.ProbeRunSummary pass.
type probeRunSummaryJSON struct {
	StartedAt          string `json:"startedAt,omitempty"`
	CompletedAt        string `json:"completedAt,omitempty"`
	AccountsConsidered int    `json:"accountsConsidered"`
	AccountsProbed     int    `json:"accountsProbed"`
	TargetsScanned     int    `json:"targetsScanned"`
	Success            int    `json:"success"`
	Failed             int    `json:"failed"`
	Inconclusive       int    `json:"inconclusive"`
	Skipped            int    `json:"skipped"`
}

func (h *schedulerStatusHandler) status(w http.ResponseWriter, r *http.Request) {
	items := []schedulerRunStatus{
		h.checkinStatus(),
		h.balanceRefreshStatus(),
		h.modelProbeStatus(),
		h.announcementsStatus(),
		h.eventsStatus("daily-summary"),
		h.eventsStatus("log-cleanup"),
		h.eventsStatus("usage-aggregation"),
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
	})
}

// since24h returns an RFC3339 timestamp 24h ago (UTC).
func since24h() string {
	return time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
}

// checkinStatus aggregates checkin_logs for the last 24h.
func (h *schedulerStatusHandler) checkinStatus() schedulerRunStatus {
	signal := h.db.CheckinActivity(since24h())

	status := "never"
	if signal.Latest != "" {
		status = mapRunStatus(signal.Status)
	}
	return schedulerRunStatus{
		Job:        "checkin",
		Enabled:    true,
		LastRunAt:  signal.Latest,
		LastStatus: status,
		Runs24h:    signal.Runs,
		Success24h: signal.Success,
	}
}

// balanceRefreshStatus uses accounts.last_balance_refresh as the run signal.
func (h *schedulerStatusHandler) balanceRefreshStatus() schedulerRunStatus {
	signal, total := h.db.BalanceRefreshActivity(since24h())

	// The job records no per-run status row, so derive the last-status badge
	// from its own data sources instead of reporting "never": a non-empty
	// last_balance_refresh written within the 24h window is the success
	// signal (the timestamp is only stamped after a successful fetch).
	status := mapRunStatus("")
	if signal.Latest != "" && signal.Runs > 0 {
		status = "success"
	}
	return schedulerRunStatus{
		Job:        "balance-refresh",
		Enabled:    true,
		LastRunAt:  signal.Latest,
		LastStatus: status,
		Runs24h:    signal.Runs,
		Success24h: signal.Runs,
		Note:       "activeAccounts=" + numStr(total),
	}
}

// modelProbeStatus reads the in-memory probe summary (single-instance honest).
func (h *schedulerStatusHandler) modelProbeStatus() schedulerRunStatus {
	probe := scheduler.GetGlobalModelProbeScheduler()
	if probe == nil {
		return schedulerRunStatus{Job: "model-probe", Enabled: false, LastStatus: "never", Note: "not enabled (MODEL_AVAILABILITY_PROBE_ENABLED)"}
	}
	summary := probe.LastRunSummary()
	status := "never"
	if summary.CompletedAtMs > 0 {
		status = "success"
		if summary.Failed > 0 {
			status = "failed"
		}
	}
	runs := probe.RecentRunSummaries()
	recent := make([]probeRunSummaryJSON, 0, len(runs))
	for _, r := range runs {
		recent = append(recent, probeRunSummaryJSON{
			StartedAt:          msToRFC3339(r.StartedAtMs),
			CompletedAt:        msToRFC3339(r.CompletedAtMs),
			AccountsConsidered: r.AccountsConsidered,
			AccountsProbed:     r.AccountsProbed,
			TargetsScanned:     r.TargetsScanned,
			Success:            r.Success,
			Failed:             r.Failed,
			Inconclusive:       r.Inconclusive,
			Skipped:            r.Skipped,
		})
	}
	return schedulerRunStatus{
		Job:        "model-probe",
		Enabled:    true,
		LastRunAt:  msToRFC3339(summary.CompletedAtMs),
		LastStatus: status,
		Note:       "success=" + numStr(int64(summary.Success)) + " failed=" + numStr(int64(summary.Failed)),
		RecentRuns: recent,
	}
}

// announcementsStatus uses the latest site_announcements.last_seen_at.
func (h *schedulerStatusHandler) announcementsStatus() schedulerRunStatus {
	latest := h.db.LatestSiteAnnouncementTime()
	return schedulerRunStatus{
		Job:        "site-announcements",
		Enabled:    true,
		LastRunAt:  latest,
		LastStatus: mapRunStatus(""),
	}
}

// eventsStatus reports a job whose runs land in the events table (type =
// eventType). Used for daily-summary / log-cleanup / usage-aggregation.
func (h *schedulerStatusHandler) eventsStatus(eventType string) schedulerRunStatus {
	signal := h.db.EventActivity(eventType, since24h())
	return schedulerRunStatus{
		Job:        eventType,
		Enabled:    true,
		LastRunAt:  signal.Latest,
		LastStatus: mapRunStatus(""),
		Runs24h:    signal.Runs,
	}
}

// mapRunStatus maps a DB status string to the uniform status vocabulary.
// Empty status means no run recorded yet → "never".
func mapRunStatus(status string) string {
	switch status {
	case "success", "succeeded":
		return "success"
	case "failed", "error":
		return "failed"
	case "pending", "running":
		return "running"
	default:
		if status == "" {
			return "never"
		}
		return status
	}
}

func msToRFC3339(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func numStr(v int64) string {
	return strconv.FormatInt(v, 10)
}
