package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/deliciousbuding/metapi-go/store"
)

func setupSchedulerStatusTest(t *testing.T) (*store.DB, chi.Router) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	r := chi.NewRouter()
	RegisterSchedulerStatusRoutes(r, db.DB)
	return db, r
}

// C1: unified scheduler run history aggregates the
// existing per-job signals (checkin_logs, accounts, events).
func TestSchedulerStatusReportsCheckinAndBalance(t *testing.T) {
	db, r := setupSchedulerStatusTest(t)
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	oldStr := now.Add(-48 * time.Hour).Format(time.RFC3339)

	// A recent checkin log (success) + an old one (failed).
	_, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?)`, "st-site", "https://st.example.test", "new-api", nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", "st-site"); err != nil {
		t.Fatalf("site id: %v", err)
	}
	_, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, last_balance_refresh, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?, ?)`, siteID, "st-user", "tok", true, nowStr, nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	var accID int64
	if err := db.Get(&accID, "SELECT id FROM accounts WHERE username = ?", "st-user"); err != nil {
		t.Fatalf("account id: %v", err)
	}
	_, err = db.Exec(`INSERT INTO checkin_logs (account_id, status, message, created_at)
		VALUES (?, 'success', 'ok', ?), (?, 'failed', 'bad', ?)`, accID, nowStr, accID, oldStr)
	if err != nil {
		t.Fatalf("insert checkin logs: %v", err)
	}

	resp := doGet(t, r, "/api/scheduler/status")
	if resp.Code != http.StatusOK {
		t.Fatalf("status returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) < 2 {
		t.Fatalf("items = %#v, want >= 2 jobs", body["items"])
	}

	byJob := map[string]map[string]any{}
	for _, it := range items {
		item := it.(map[string]any)
		byJob[item["job"].(string)] = item
	}
	checkin := byJob["checkin"]
	if checkin == nil {
		t.Fatalf("checkin job missing: %v", byJob)
	}
	if checkin["lastStatus"] != "success" {
		t.Fatalf("checkin lastStatus = %v, want success (latest log)", checkin["lastStatus"])
	}
	if checkin["runs24h"].(float64) != 1 {
		t.Fatalf("checkin runs24h = %v, want 1 (only the recent log)", checkin["runs24h"])
	}
	if checkin["success24h"].(float64) != 1 {
		t.Fatalf("checkin success24h = %v, want 1", checkin["success24h"])
	}
	balance := byJob["balance-refresh"]
	if balance == nil {
		t.Fatalf("balance-refresh job missing: %v", byJob)
	}
	if balance["runs24h"].(float64) != 1 {
		t.Fatalf("balance-refresh runs24h = %v, want 1 (last_balance_refresh within 24h)", balance["runs24h"])
	}
	// The job records no per-run status row, so the badge derives from its own
	// data: a last_balance_refresh stamped within the 24h window is success.
	if balance["lastStatus"] != "success" {
		t.Fatalf("balance-refresh lastStatus = %v, want success (fresh stamp = successful pass)", balance["lastStatus"])
	}
}
