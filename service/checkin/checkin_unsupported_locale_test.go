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

// Observed failure (v0.19 stability window): a real New API upstream with the
// check-in feature switched off answers its own hardcoded 签到功能未启用. That
// string was in none of the three English-only "unsupported" lists, so binding
// an account wrote checkin_logs.status=failed with failure_reason
// code=unknown_error, published an error-level checkinFailed event, and set
// runtimeHealth=unhealthy with the upstream's raw text as the reason — for an
// account whose models, balance and relay all worked. The health entry only
// recovered when the next hourly balance refresh overwrote it. On the
// long-running testbed instance that added up to 71 failed check-in rows and 63
// error events over five days, while the sibling sub2api account — whose
// "not supported" text Metapi's own adapter writes in English — was classified
// correctly the whole time.
func TestCheckinAccount_UpstreamCheckinDisabledIsUnsupportedNotFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"data": map[string]any{
					"id": 7, "username": "checkin-disabled-user",
					"quota": 500000, "used_quota": 0,
				},
			})
		case "/api/user/checkin":
			// Exactly what New API's DoCheckin handler returns when the operator
			// left the feature off.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": false,
				"message": "签到功能未启用",
			})
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
		"checkin disabled upstream", server.URL, now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := siteRes.LastInsertId()
	acctRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, 'active', TRUE, ?, ?)",
		siteID, "checkin-disabled-user", "dashboard-pat", now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := acctRes.LastInsertId()

	// Events stay on: the event level is part of what the operator sees.
	result := CheckinAccount(&config.Config{}, db.DB, accountID, &CheckinOptions{ScheduleMode: "manual"})

	if !result.Success {
		t.Fatalf("result.Success = false, want true: an upstream without check-in is not a failed check-in")
	}
	if result.Status != CheckinSkipped || !result.Skipped {
		t.Fatalf("result.Status = %q (skipped=%v), want skipped", result.Status, result.Skipped)
	}

	var logStatus string
	var failureReason *string
	if err := db.QueryRow(
		"SELECT status, failure_reason FROM checkin_logs WHERE account_id = ? ORDER BY rowid DESC LIMIT 1", accountID,
	).Scan(&logStatus, &failureReason); err != nil {
		t.Fatalf("read checkin_logs: %v", err)
	}
	if logStatus != "skipped" {
		t.Fatalf("checkin_logs.status = %q, want skipped", logStatus)
	}
	if failureReason == nil {
		t.Fatal("checkin_logs.failure_reason = NULL, want the structured checkin_not_supported classification")
	}
	var reason struct {
		Code     string `json:"code"`
		Category string `json:"category"`
	}
	if err := json.Unmarshal([]byte(*failureReason), &reason); err != nil {
		t.Fatalf("decode failure_reason %q: %v", *failureReason, err)
	}
	if reason.Code != "checkin_not_supported" || reason.Category != "site" {
		t.Fatalf("failure_reason = (%q, %q), want (checkin_not_supported, site)", reason.Code, reason.Category)
	}

	var healthRaw *string
	if err := db.QueryRow("SELECT extra_config FROM accounts WHERE id = ?", accountID).Scan(&healthRaw); err != nil {
		t.Fatalf("read accounts.extra_config: %v", err)
	}
	if healthRaw == nil || !strings.Contains(*healthRaw, `"state":"degraded"`) {
		t.Fatalf("runtime health = %v, want degraded (unhealthy told the operator the account was broken)", healthRaw)
	}
	if !strings.Contains(*healthRaw, "does not support check-in") {
		t.Fatalf("runtime health reason = %v, want Metapi's own unsupported wording instead of the raw upstream text", healthRaw)
	}

	var errorEvents int
	var infoEvents int
	if err := db.QueryRow(
		"SELECT SUM(CASE WHEN level = 'error' THEN 1 ELSE 0 END), SUM(CASE WHEN level = 'info' THEN 1 ELSE 0 END) FROM events WHERE related_id = ? AND related_type = 'account'",
		accountID,
	).Scan(&errorEvents, &infoEvents); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if errorEvents != 0 {
		t.Fatalf("error-level events = %d, want 0: an upstream without check-in is not an error", errorEvents)
	}
	if infoEvents != 1 {
		t.Fatalf("info-level events = %d, want exactly the one checkinSkipped entry", infoEvents)
	}
}
