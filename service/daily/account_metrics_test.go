package daily

import (
	"testing"
	"time"

	"github.com/tokendancelab/metapi-go/store"
)

func seedPerAccountSite(t *testing.T, db *store.DB, name, status string, now string) int64 {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, 'openai', ?, ?, ?)`, name, "https://"+name+".example.test", status, now, now); err != nil {
		t.Fatalf("insert site %s: %v", name, err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = ?", name); err != nil {
		t.Fatalf("load site id %s: %v", name, err)
	}
	return siteID
}

func seedPerAccountAccount(t *testing.T, db *store.DB, siteID int64, username string, now string) int64 {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO accounts
		(site_id, username, access_token, status, balance, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, 'sk-'+?, 'active', 10, TRUE, ?, ?)`, siteID, username, username, now, now); err != nil {
		t.Fatalf("insert account %s: %v", username, err)
	}
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = ?", username); err != nil {
		t.Fatalf("load account id %s: %v", username, err)
	}
	return accountID
}

// Per-account metrics share the daily summary's window and truth semantics:
// active sites only, legacy SQL timestamps normalized, real-zero for accounts
// with no rows, partial when rows are missing reward/cost fields.
func TestCollectPerAccountTodayMetrics(t *testing.T) {
	db := setupDailyMetricsSQLite(t)
	location := time.FixedZone("HKT", 8*60*60)
	now := time.Date(2026, 8, 1, 1, 0, 0, 0, location)

	siteID := seedPerAccountSite(t, db, "per-account-site", "active", "2026-07-31T16:00:00Z")
	disabledSiteID := seedPerAccountSite(t, db, "per-account-disabled", "disabled", "2026-07-31T16:00:00Z")
	rewardAccount := seedPerAccountAccount(t, db, siteID, "reward-user", "2026-07-31T16:00:00Z")
	spendAccount := seedPerAccountAccount(t, db, siteID, "spend-user", "2026-07-31T16:00:00Z")
	cleanAccount := seedPerAccountAccount(t, db, siteID, "clean-user", "2026-07-31T16:00:00Z")
	disabledAccount := seedPerAccountAccount(t, db, disabledSiteID, "disabled-user", "2026-07-31T16:00:00Z")

	// reward-user: two successful checkins inside the local day, one legacy
	// SQL timestamp, one RFC 3339 — both must count.
	if _, err := db.Exec(`INSERT INTO checkin_logs (account_id, status, reward, created_at)
		VALUES (?, 'success', '1.25', '2026-07-31 16:30:00')`, rewardAccount); err != nil {
		t.Fatalf("insert legacy checkin: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checkin_logs (account_id, status, reward, created_at)
		VALUES (?, 'success', '0.75', '2026-07-31T17:05:00Z')`, rewardAccount); err != nil {
		t.Fatalf("insert rfc3339 checkin: %v", err)
	}
	// Out-of-window checkin (previous local day) must not leak in.
	if _, err := db.Exec(`INSERT INTO checkin_logs (account_id, status, reward, created_at)
		VALUES (?, 'success', '99', '2026-07-31 15:59:59')`, rewardAccount); err != nil {
		t.Fatalf("insert out-of-window checkin: %v", err)
	}

	// reward-user: one proxy success with cost; one success with tokens but no
	// cost (missing cost → spend partial).
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-1', 'gpt-1', 'success', 50, 0.2, '2026-07-31T16:45:00Z')`, rewardAccount); err != nil {
		t.Fatalf("insert proxy with cost: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-2', 'gpt-2', 'success', 30, NULL, '2026-07-31T17:10:00Z')`, rewardAccount); err != nil {
		t.Fatalf("insert proxy missing cost: %v", err)
	}

	// spend-user: proxy rows with NULL status (legacy unknown) and a failed row.
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-3', 'gpt-3', NULL, 10, 0.05, '2026-07-31T17:00:00Z')`, spendAccount); err != nil {
		t.Fatalf("insert proxy unknown status: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-4', 'gpt-4', 'failed', 5, 0.01, '2026-07-31T17:01:00Z')`, spendAccount); err != nil {
		t.Fatalf("insert proxy failed: %v", err)
	}

	// Disabled-site account: checkin and proxy rows must be excluded entirely.
	if _, err := db.Exec(`INSERT INTO checkin_logs (account_id, status, reward, created_at)
		VALUES (?, 'success', '9.99', '2026-07-31T16:30:00')`, disabledAccount); err != nil {
		t.Fatalf("insert disabled-site checkin: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-5', 'gpt-5', 'success', 500, 5.0, '2026-07-31T16:40:00Z')`, disabledAccount); err != nil {
		t.Fatalf("insert disabled-site proxy: %v", err)
	}

	// Unattributed proxy row (NULL account_id) must not pollute any account.
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (NULL, 'gpt-6', 'gpt-6', 'success', 999, 9.99, '2026-07-31T16:50:00Z')`); err != nil {
		t.Fatalf("insert unattributed proxy: %v", err)
	}

	metrics, err := CollectPerAccountTodayMetrics(db.DB, now)
	if err != nil {
		t.Fatalf("collect per-account metrics: %v", err)
	}

	reward := metrics[rewardAccount]
	if reward == nil {
		t.Fatal("reward-user missing from metrics")
	}
	if reward.Reward != 2.0 || reward.RewardStatus != "complete" {
		t.Fatalf("reward-user reward = %v/%q, want 2/complete", reward.Reward, reward.RewardStatus)
	}
	if reward.Spend != 0.2 || reward.SpendStatus != "partial" || reward.SpendReason != "source_partial" {
		t.Fatalf("reward-user spend = %v/%q/%q, want 0.2/partial/source_partial",
			reward.Spend, reward.SpendStatus, reward.SpendReason)
	}
	if reward.ProxyTotal != 2 || reward.ProxySuccess != 2 {
		t.Fatalf("reward-user proxy = %d/%d, want 2/2", reward.ProxyTotal, reward.ProxySuccess)
	}
	if reward.Tokens != 80 {
		t.Fatalf("reward-user tokens = %d, want 80", reward.Tokens)
	}

	spend := metrics[spendAccount]
	if spend == nil {
		t.Fatal("spend-user missing from metrics")
	}
	if spend.Spend != 0.06 || spend.SpendStatus != "partial" || spend.SpendReason != "legacy_unknown" {
		t.Fatalf("spend-user spend = %v/%q/%q, want 0.06/partial/legacy_unknown",
			spend.Spend, spend.SpendStatus, spend.SpendReason)
	}
	if spend.ProxyTotal != 2 || spend.ProxySuccess != 0 || spend.ProxyFailed != 1 || spend.ProxyUnknown != 1 {
		t.Fatalf("spend-user proxy = %d/%d/%d/%d, want 2/0/1/1",
			spend.ProxyTotal, spend.ProxySuccess, spend.ProxyFailed, spend.ProxyUnknown)
	}
	if spend.Reward != 0 || spend.RewardStatus != "complete" {
		t.Fatalf("spend-user reward = %v/%q, want 0/complete", spend.Reward, spend.RewardStatus)
	}

	if _, exists := metrics[cleanAccount]; exists {
		t.Fatal("clean-user with no rows must not appear in metrics map")
	}
	if _, exists := metrics[disabledAccount]; exists {
		t.Fatal("disabled-site account must be excluded")
	}
}

// A successful checkin whose reward cannot be parsed (no message fallback)
// must surface as partial, not silently zero.
func TestCollectPerAccountTodayMetricsRewardPartial(t *testing.T) {
	db := setupDailyMetricsSQLite(t)
	location := time.FixedZone("HKT", 8*60*60)
	now := time.Date(2026, 8, 1, 1, 0, 0, 0, location)

	siteID := seedPerAccountSite(t, db, "partial-site", "active", "2026-07-31T16:00:00Z")
	accountID := seedPerAccountAccount(t, db, siteID, "partial-user", "2026-07-31T16:00:00Z")
	if _, err := db.Exec(`INSERT INTO checkin_logs (account_id, status, reward, created_at)
		VALUES (?, 'success', NULL, '2026-07-31T16:30:00Z')`, accountID); err != nil {
		t.Fatalf("insert unparsable checkin: %v", err)
	}

	metrics, err := CollectPerAccountTodayMetrics(db.DB, now)
	if err != nil {
		t.Fatalf("collect per-account metrics: %v", err)
	}
	m := metrics[accountID]
	if m == nil {
		t.Fatal("account missing from metrics")
	}
	if m.RewardStatus != "partial" || m.RewardReason != "source_partial" {
		t.Fatalf("reward = %q/%q, want partial/source_partial", m.RewardStatus, m.RewardReason)
	}
}

func TestLegacyCreatedAtExpr(t *testing.T) {
	expr := legacyCreatedAtExpr("cl")
	want := "(CASE WHEN SUBSTR(cl.created_at, 11, 1) = ' ' " +
		"THEN REPLACE(SUBSTR(cl.created_at, 1, 19), ' ', 'T') || 'Z' " +
		"ELSE cl.created_at END)"
	if expr != want {
		t.Fatalf("legacyCreatedAtExpr = %q, want %q", expr, want)
	}
}
