package admin

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- A3: income vs outcome balance analysis ----

// seedIncomeOutcomeFixture writes 3 daily snapshots for one account:

//	day1: balance 10.0, used 2.0 → initial income 12.0, outcome 0
//	day2: balance  8.0, used 5.0 → Δused 3.0 → outcome 3.0, income 1.0
//	day3: balance 11.0, used 5.0 → Δused 0.0 → outcome 0.0, income 3.0

// Totals: income 16.0, outcome 3.0, net 13.0.
func seedIncomeOutcomeFixture(t *testing.T, db *store.DB, days []string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, 'new-api', 'active', ?, ?)`, "income-site", "https://income.example.test", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", "income-site"); err != nil {
		t.Fatalf("site id: %v", err)
	}
	_, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, 'tok', 'active', TRUE, ?, ?)`, siteID, "income-user", now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = ?", "income-user"); err != nil {
		t.Fatalf("account id: %v", err)
	}

	balances := [3][2]float64{{10, 2}, {8, 5}, {11, 5}}
	for i, d := range days {
		if _, err := db.Exec(`INSERT INTO balance_history (account_id, balance, balance_used, quota, local_day, captured_at, created_at)
			VALUES (?, ?, ?, 20, ?, ?, ?)`, accountID, balances[i][0], balances[i][1], d, now, now); err != nil {
			t.Fatalf("insert balance_history %s: %v", d, err)
		}
	}
	return accountID
}

func TestStats_BalanceIncomeOutcome_IdentityHolds(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC()
	days := []string{
		now.AddDate(0, 0, -2).Format("2006-01-02"),
		now.AddDate(0, 0, -1).Format("2006-01-02"),
		now.Format("2006-01-02"),
	}
	accountID := seedIncomeOutcomeFixture(t, db, days)

	resp := doGet(t, r, "/api/stats/balance-income-outcome?days=7")
	if resp.Code != 200 {
		t.Fatalf("balance-income-outcome returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	points := body["points"].([]any)
	if len(points) != 3 {
		t.Fatalf("points = %#v, want 3 snapshot days", points)
	}
	byDay := make(map[string]map[string]any, len(points))
	for _, p := range points {
		pm := p.(map[string]any)
		byDay[pm["day"].(string)] = pm
	}
	d0, d1, d2 := days[0], days[1], days[2]

	p0 := byDay[d0]
	if p0["income"].(float64) != 12.0 || p0["outcome"].(float64) != 0.0 {
		t.Fatalf("day1 = %#v, want income 12 outcome 0 (initial credit)", p0)
	}
	p1 := byDay[d1]
	if p1["income"].(float64) != 1.0 || p1["outcome"].(float64) != 3.0 {
		t.Fatalf("day2 = %#v, want income 1 outcome 3", p1)
	}
	p2 := byDay[d2]
	if p2["income"].(float64) != 3.0 || p2["outcome"].(float64) != 0.0 {
		t.Fatalf("day3 = %#v, want income 3 outcome 0", p2)
	}

	summary := body["summary"].(map[string]any)
	if summary["totalIncome"].(float64) != 16.0 || summary["totalOutcome"].(float64) != 3.0 {
		t.Fatalf("summary = %#v, want income 16 outcome 3", summary)
	}
	if summary["net"].(float64) != 13.0 {
		t.Fatalf("net = %v, want 13.0 (identity income-outcome=Δbalance)", summary["net"])
	}
	if int64(summary["accounts"].(float64)) != 1 {
		t.Fatalf("accounts = %v, want 1", summary["accounts"])
	}

	// accountId filter returns the same series.
	resp = doGet(t, r, "/api/stats/balance-income-outcome?days=7&accountId="+itoa(accountID))
	if resp.Code != 200 {
		t.Fatalf("filtered returned %d", resp.Code)
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal filtered: %v", err)
	}
	if len(body["points"].([]any)) != 3 {
		t.Fatalf("filtered points = %#v, want 3", body["points"])
	}
}

func TestStats_BalanceIncomeOutcome_MultiAccountAggregates(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// Account A: yesterday (bal 5, used 0), today (bal 3, used 2).
	// Account B: today only (bal 7, used 1).
	accountA := seedIncomeOutcomeFixture(t, db, []string{yesterday, today})
	_ = accountA

	nowStr := now.Format(time.RFC3339)
	var siteID int64
	_, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, 'new-api', 'active', ?, ?)`, "income-site-b", "https://income-b.example.test", nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert site b: %v", err)
	}
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", "income-site-b"); err != nil {
		t.Fatalf("site b id: %v", err)
	}
	_, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, 'tok', 'active', TRUE, ?, ?)`, siteID, "income-user-b", nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert account b: %v", err)
	}
	var accountB int64
	if err := db.Get(&accountB, "SELECT id FROM accounts WHERE username = ?", "income-user-b"); err != nil {
		t.Fatalf("account b id: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO balance_history (account_id, balance, balance_used, quota, local_day, captured_at, created_at)
		VALUES (?, ?, ?, 20, ?, ?, ?)`, accountB, 7, 1, today, nowStr, nowStr); err != nil {
		t.Fatalf("insert balance_history b: %v", err)
	}

	resp := doGet(t, r, "/api/stats/balance-income-outcome?days=7")
	if resp.Code != 200 {
		t.Fatalf("returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	summary := body["summary"].(map[string]any)
	// Account A (2 days of the 3-day fixture): initial 12 (yesterday) +
	// today (Δbal -2 + Δused 3 = 1) = 13 income; outcome 3.
	// Account B (today only): initial 7 + 1 = 8 income; outcome 0.
	// Totals: income 21, outcome 3.
	if summary["totalIncome"].(float64) != 21.0 || summary["totalOutcome"].(float64) != 3.0 {
		t.Fatalf("summary = %#v, want income 21 outcome 3", summary)
	}
	if int64(summary["accounts"].(float64)) != 2 {
		t.Fatalf("accounts = %v, want 2", summary["accounts"])
	}
}

// Refund scenario (review fix): balance_used drops without balance changing —
// outcome goes negative, and the identity income - outcome = Δbalance still
// holds (clamping would break it).
func TestStats_BalanceIncomeOutcome_RefundKeepsIdentity(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC()
	day1 := now.AddDate(0, 0, -1).Format("2006-01-02")
	day2 := now.Format("2006-01-02")
	nowStr := now.Format(time.RFC3339)

	_, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, 'new-api', 'active', ?, ?)`, "refund-site", "https://refund.example.test", nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", "refund-site"); err != nil {
		t.Fatalf("site id: %v", err)
	}
	_, err = db.Exec(`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, 'tok', 'active', TRUE, ?, ?)`, siteID, "refund-user", nowStr, nowStr)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = ?", "refund-user"); err != nil {
		t.Fatalf("account id: %v", err)
	}
	// day1: balance 10, used 5 → initial income 15, outcome 0.
	// day2: balance 10, used 2 (refund of 3) → Δused -3 → outcome -3,
	// income = Δbal(0) + Δused(-3) = -3 → identity: -3 - (-3) = 0 = Δbalance ✓
	for i, d := range []struct {
		day   string
		bal   float64
		used  float64
	}{{day1, 10, 5}, {day2, 10, 2}} {
		if _, err := db.Exec(`INSERT INTO balance_history (account_id, balance, balance_used, quota, local_day, captured_at, created_at)
			VALUES (?, ?, ?, 20, ?, ?, ?)`, accountID, d.bal, d.used, d.day, nowStr, nowStr); err != nil {
			t.Fatalf("insert balance_history %d: %v", i, err)
		}
	}

	resp := doGet(t, r, "/api/stats/balance-income-outcome?days=7")
	if resp.Code != 200 {
		t.Fatalf("returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	points := body["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("points = %#v, want 2", points)
	}
	byDay := make(map[string]map[string]any, 2)
	for _, p := range points {
		pm := p.(map[string]any)
		byDay[pm["day"].(string)] = pm
	}
	d2 := byDay[day2]
	if d2["outcome"].(float64) != -3.0 {
		t.Fatalf("day2 outcome = %v, want -3 (refund reflected honestly)", d2["outcome"])
	}
	summary := body["summary"].(map[string]any)
	// Total: income 15 + (-3) = 12; outcome 0 + (-3) = -3; net 15.
	if summary["totalIncome"].(float64) != 12.0 || summary["totalOutcome"].(float64) != -3.0 {
		t.Fatalf("summary = %#v, want income 12 outcome -3", summary)
	}
	if summary["net"].(float64) != 15.0 {
		t.Fatalf("net = %v, want 15 (identity holds: 12 - (-3) = 15)", summary["net"])
	}
}

// PostgreSQL dialect parity for A3 (skipped without PG_TEST_DSN).
func TestStats_PostgresBalanceIncomeOutcome(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, r := setupStatsPostgresTest(t)
	now := time.Now().UTC()
	days := []string{
		now.AddDate(0, 0, -1).Format("2006-01-02"),
		now.Format("2006-01-02"),
	}
	seedIncomeOutcomeFixture(t, db, days)

	resp := doGet(t, r, "/api/stats/balance-income-outcome?days=7")
	if resp.Code != 200 {
		t.Fatalf("returned %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	points := body["points"].([]any)
	if len(points) != 2 {
		t.Fatalf("points = %#v, want 2", points)
	}
	summary := body["summary"].(map[string]any)
	// day1 initial 12 + day2 (Δbal -2 + Δused 3 = 1) = 13 income; outcome 3.
	if summary["totalIncome"].(float64) != 13.0 || summary["totalOutcome"].(float64) != 3.0 {
		t.Fatalf("summary = %#v, want income 13 outcome 3", summary)
	}
}
