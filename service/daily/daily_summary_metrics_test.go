package daily

import (
	"testing"
	"time"

	"github.com/tokendancelab/metapi-go/store"
)

func setupDailyMetricsSQLite(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	return db
}

func seedDailyMetricsAccount(t *testing.T, db *store.DB, extraConfig *string) int64 {
	t.Helper()
	now := "2026-07-31T16:00:00Z"
	if _, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES ('daily-site', 'https://daily.example.test', 'openai', 'active', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	var siteID int64
	if err := db.Get(&siteID, "SELECT id FROM sites WHERE name = 'daily-site'"); err != nil {
		t.Fatalf("load site id: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO accounts
		(site_id, username, access_token, status, balance, checkin_enabled, extra_config, created_at, updated_at)
		VALUES (?, 'daily-user', 'sk-daily', 'active', 10, TRUE, ?, ?, ?)`, siteID, extraConfig, now, now); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	var accountID int64
	if err := db.Get(&accountID, "SELECT id FROM accounts WHERE username = 'daily-user'"); err != nil {
		t.Fatalf("load account id: %v", err)
	}
	return accountID
}

func TestCollectDailySummaryMetricsIncludesLegacySQLTimestampAtLocalDayStart(t *testing.T) {
	db := setupDailyMetricsSQLite(t)
	accountID := seedDailyMetricsAccount(t, db, nil)
	location := time.FixedZone("HKT", 8*60*60)
	now := time.Date(2026, 8, 1, 1, 0, 0, 0, location)

	// Check-in writers historically used UTC SQL text. This row is 00:30 HKT
	// and must remain inside the 2026-08-01 local-day window.
	if _, err := db.Exec(`INSERT INTO checkin_logs (account_id, status, reward, created_at)
		VALUES (?, 'success', '1.25', '2026-07-31 16:30:00')`, accountID); err != nil {
		t.Fatalf("insert in-window legacy checkin: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checkin_logs (account_id, status, reward, created_at)
		VALUES (?, 'success', '99', '2026-07-31 15:59:59')`, accountID); err != nil {
		t.Fatalf("insert out-of-window legacy checkin: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-daily', 'gpt-daily', 'success', 50, 0.2, '2026-07-31T16:30:00Z')`, accountID); err != nil {
		t.Fatalf("insert proxy log: %v", err)
	}

	metrics, err := CollectDailySummaryMetrics(db.DB, now)
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	if metrics.CheckinTotal != 1 || metrics.CheckinSuccess != 1 {
		t.Fatalf("checkin totals = %d/%d, want 1/1", metrics.CheckinTotal, metrics.CheckinSuccess)
	}
	if metrics.TodayReward != 1.25 {
		t.Fatalf("today reward = %v, want 1.25", metrics.TodayReward)
	}
	if metrics.TodaySpend != 0.2 || metrics.TodaySpendStatus != "complete" {
		t.Fatalf("today spend = %v status=%q, want 0.2 complete", metrics.TodaySpend, metrics.TodaySpendStatus)
	}
}

func TestCollectDailySummaryMetricsMarksIncompleteFallbackAndProxyCoverage(t *testing.T) {
	db := setupDailyMetricsSQLite(t)
	extraConfig := `{"todayIncomeSnapshot":{"day":"2026-08-01","baseline":1,"latest":1,"updatedAt":"2026-07-31T16:10:00Z"}}`
	accountID := seedDailyMetricsAccount(t, db, &extraConfig)
	location := time.FixedZone("HKT", 8*60*60)
	now := time.Date(2026, 8, 1, 2, 0, 0, 0, location)

	for _, reward := range []*string{stringPointer("2"), nil} {
		if _, err := db.Exec(`INSERT INTO checkin_logs (account_id, status, reward, created_at)
			VALUES (?, 'success', ?, '2026-07-31T17:00:00Z')`, accountID, reward); err != nil {
			t.Fatalf("insert checkin: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (?, 'gpt-unknown', 'gpt-unknown', NULL, 10, NULL, '2026-07-31T17:10:00Z')`, accountID); err != nil {
		t.Fatalf("insert unknown proxy log: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_logs
		(account_id, model_requested, model_actual, status, total_tokens, estimated_cost, created_at)
		VALUES (NULL, 'gpt-unattributed', 'gpt-unattributed', 'failed', 0, NULL, '2026-07-31T17:20:00Z')`); err != nil {
		t.Fatalf("insert unattributed proxy log: %v", err)
	}

	metrics, err := CollectDailySummaryMetrics(db.DB, now)
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	if metrics.TodayRewardStatus != "partial" || metrics.TodayRewardReason != "source_partial" {
		t.Fatalf("reward truth = %q/%q, want partial/source_partial", metrics.TodayRewardStatus, metrics.TodayRewardReason)
	}
	if metrics.TodayReward != 2 {
		t.Fatalf("today reward = %v, want parsed subtotal 2", metrics.TodayReward)
	}
	if metrics.ProxyUnknown != 1 || metrics.ProxyUnattributed != 1 || metrics.ProxyMissingCost != 1 {
		t.Fatalf("proxy coverage = unknown:%d unattributed:%d missingCost:%d", metrics.ProxyUnknown, metrics.ProxyUnattributed, metrics.ProxyMissingCost)
	}
	if metrics.ProxyMetricStatus != "partial" || metrics.ProxyMetricReason != "unattributed" {
		t.Fatalf("proxy truth = %q/%q, want partial/unattributed", metrics.ProxyMetricStatus, metrics.ProxyMetricReason)
	}
	if metrics.TodaySpendStatus != "partial" || metrics.TodaySpendReason != "unattributed" {
		t.Fatalf("spend truth = %q/%q, want partial/unattributed", metrics.TodaySpendStatus, metrics.TodaySpendReason)
	}
}

func stringPointer(value string) *string {
	return &value
}
