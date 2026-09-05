package checkin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// The other half of the same family as #1267: an upstream that answers "today's
// check-in already happened" is not a failure either. The runner knew that (16
// wordings, 今日已签到 among them) and recorded success with a healthy
// runtime-health entry, while the structured classifier kept a five-wording copy
// of the same list. Both read one vocabulary now.
//
// What this test pins is the state a second same-day check-in leaves behind:
// success (not failed), a healthy health entry in the product's own words,
// last_checkin_at advanced for a manual trigger, and no failure row — because
// the runner normalizes "already checked in" to status=success, which is also
// why checkin_logs.failure_reason stays NULL here: the structured reason column
// is only written for non-success statuses.
func TestCheckinAccount_AlreadyCheckedInIsRecordedAsSuccessNotFailure(t *testing.T) {
	// Three of the wordings the shared vocabulary carries: the one New API
	// actually answers, one the retired runner-private list also had, and one
	// only the English branch covers. A re-introduced private copy with fewer
	// wordings fails on the second and third.
	for _, answer := range []string{"今日已签到", "已签到", "reward already claimed"} {
		t.Run(answer, func(t *testing.T) {
			checkinAlreadyAnsweredOnce(t, answer)
		})
	}
}

func checkinAlreadyAnsweredOnce(t *testing.T, upstreamAnswer string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data":    map[string]any{"id": 9, "username": "already-user", "quota": 500000, "used_quota": 0},
			})
		case "/api/user/checkin":
			// New API's model/checkin.go answers 今日已签到 for a second same-day attempt.
			_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "message": upstreamAnswer})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	siteRes, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'new-api', 'active', ?, ?)",
		"already checked in upstream", server.URL, now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := siteRes.LastInsertId()
	acctRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, 'active', TRUE, ?, ?)",
		siteID, "already-user", "dashboard-pat", now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := acctRes.LastInsertId()

	result := CheckinAccount(&config.Config{}, db.DB, accountID, &CheckinOptions{ScheduleMode: "manual"})
	if !result.Success {
		t.Fatalf("result.Success = false, want true: today's check-in already happening is not a failure")
	}
	if result.Status != CheckinSuccess || result.Skipped {
		t.Fatalf("result.Status = %q (skipped=%v), want success/not-skipped", result.Status, result.Skipped)
	}

	var logStatus string
	var failureReason *string
	if err := db.QueryRow(
		"SELECT status, failure_reason FROM checkin_logs WHERE account_id = ? ORDER BY rowid DESC LIMIT 1", accountID,
	).Scan(&logStatus, &failureReason); err != nil {
		t.Fatalf("read checkin_logs: %v", err)
	}
	if logStatus != "success" {
		t.Fatalf("checkin_logs.status = %q, want success", logStatus)
	}
	if failureReason != nil {
		t.Fatalf("checkin_logs.failure_reason = %s, want NULL for a success row", *failureReason)
	}

	var extraConfig *string
	var lastCheckinAt *string
	if err := db.QueryRow("SELECT extra_config, last_checkin_at FROM accounts WHERE id = ?", accountID).Scan(&extraConfig, &lastCheckinAt); err != nil {
		t.Fatalf("read account: %v", err)
	}
	if extraConfig == nil || !strings.Contains(*extraConfig, `"state":"healthy"`) {
		t.Fatalf("runtime health = %v, want healthy with the runner's own wording", extraConfig)
	}
	// The check-in path writes healthy/"already checked in today" first; because
	// an already-checked-in answer also triggers a balance refresh, the fresher
	// balance entry is what ends up persisted. Both say healthy, which is the
	// point: a normal second attempt the same day leaves no failure state behind.
	if lastCheckinAt == nil {
		t.Fatal("last_checkin_at was not advanced for a manual-mode already-checked-in answer")
	}

	var errorEvents int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM events WHERE related_id = ? AND related_type = 'account' AND level = 'error'", accountID,
	).Scan(&errorEvents); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if errorEvents != 0 {
		t.Fatalf("error-level events = %d, want 0", errorEvents)
	}
}
