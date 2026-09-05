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

// Observed failure: the check-in runner sent one param map — {account, site,
// reason} — for all four check-in event keys, while the structured-event
// registry declares {account, site, reward} for checkinSuccess (its MessageEn is
// the historical "{{account}} @ {{site}}: {{reward}}"). WriteEvent rejects a
// param the definition does not declare, so every successful check-in lost its
// event and left "events: checkinSuccess does not declare param \"reason\"" in
// the log. The operator's feed showed failures and skips only — the one thing
// worth seeing, a check-in that worked, was the one row that never arrived.
func TestCheckinAccount_SuccessPersistsItsEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/self":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				// quota rises by 5000 (=0.01 in the 500k-per-unit convention) so the
				// reward is inferred the same way production infers it.
				"data": map[string]any{"id": 11, "username": "success-user", "quota": 505000, "used_quota": 0},
			})
		case "/api/user/checkin":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"success": true,
				"message": "签到成功",
				"data":    map[string]any{"quota_awarded": 5000, "checkin_date": "2026-09-06"},
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
		"checkin success upstream", server.URL, now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ := siteRes.LastInsertId()
	acctRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, balance, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, ?, 'active', TRUE, ?, ?)",
		siteID, "success-user", "dashboard-pat", 1.0, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := acctRes.LastInsertId()

	// Events stay on: the persisted event is the thing under test.
	result := CheckinAccount(&config.Config{}, db.DB, accountID, &CheckinOptions{ScheduleMode: "manual"})
	if !result.Success || result.Status != CheckinSuccess {
		t.Fatalf("result = %+v, want a successful check-in", result)
	}

	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM events WHERE related_id = ? AND related_type = 'account'", accountID); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("persisted events = %d, want exactly the one checkinSuccess row (0 means WriteEvent rejected the params)", count)
	}

	var titleKey, level, message string
	var params *string
	if err := db.QueryRow(
		"SELECT COALESCE(title_key,''), level, COALESCE(message,''), params FROM events WHERE related_id = ? AND related_type = 'account'", accountID,
	).Scan(&titleKey, &level, &message, &params); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if titleKey != "checkinSuccess" {
		t.Fatalf("title_key = %q, want checkinSuccess", titleKey)
	}
	if level != "info" {
		t.Fatalf("level = %q, want info", level)
	}
	if params == nil || strings.Contains(*params, `"reason"`) {
		t.Fatalf("params = %v, want the declared reward shape and no undeclared reason", params)
	}
	if !strings.Contains(message, "success-user") || !strings.Contains(message, "checkin success upstream") {
		t.Fatalf("rendered message = %q, want account and site", message)
	}
}
