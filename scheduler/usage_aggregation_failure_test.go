package scheduler

import (
	"database/sql"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// Flush-failure recovery tests: a failed batch flush must preserve the
// projected delta (source rows stay above the watermark) and must not
// advance the watermark, so the next pass re-projects and retries.

func TestUsageAggregationFlushFailurePreservesDeltaAndWatermark(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}

	const triggerName = "usage_fail_site_day_insert"
	armFailure := func() {
		if _, err := db.Exec(`CREATE TRIGGER ` + triggerName + `
			BEFORE INSERT ON site_day_usage
			FOR EACH ROW
			BEGIN
				SELECT RAISE(ABORT, 'simulated site_day_usage flush failure');
			END;`); err != nil {
			t.Fatalf("arm failure trigger: %v", err)
		}
	}
	disarmFailure := func() {
		if _, err := db.Exec("DROP TRIGGER IF EXISTS " + triggerName); err != nil {
			t.Fatalf("drop failure trigger: %v", err)
		}
	}
	t.Cleanup(disarmFailure)

	runUsageAggregationFlushFailureLifecycle(t, db, "sqlite-"+strings.ReplaceAll(t.Name(), "/", "-"), armFailure, disarmFailure)
}

func TestUsageAggregationFlushFailurePreservesDeltaAndWatermark_Postgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, err := store.Open(store.DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate postgres: %v", err)
	}

	idSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
	triggerName := "usage_fail_day_insert_trg_" + idSuffix
	functionName := "usage_fail_day_insert_" + idSuffix
	armFailure := func() {
		if _, err := db.Exec(`CREATE OR REPLACE FUNCTION ` + functionName + `() RETURNS trigger AS $$
			BEGIN
				RAISE EXCEPTION 'simulated site_day_usage flush failure';
			END;
			$$ LANGUAGE plpgsql`); err != nil {
			t.Fatalf("arm failure function: %v", err)
		}
		if _, err := db.Exec(`CREATE TRIGGER ` + triggerName + `
			BEFORE INSERT ON site_day_usage
			FOR EACH ROW EXECUTE FUNCTION ` + functionName + `()`); err != nil {
			t.Fatalf("arm failure trigger: %v", err)
		}
	}
	disarmFailure := func() {
		_, _ = db.Exec("DROP TRIGGER IF EXISTS " + triggerName + " ON site_day_usage")
		_, _ = db.Exec("DROP FUNCTION IF EXISTS " + functionName + "()")
	}
	t.Cleanup(disarmFailure)

	runUsageAggregationFlushFailureLifecycle(t, db, "pg-"+strings.ReplaceAll(t.Name(), "/", "-")+"-"+idSuffix, armFailure, disarmFailure)
}

// runUsageAggregationFlushFailureLifecycle drives the core failure-recovery
// assertions: failed flush → delta preserved, watermark unchanged, failure
// recorded; next pass retries successfully without double counting.
func runUsageAggregationFlushFailureLifecycle(t *testing.T, db *store.DB, suffix string, armFailure, disarmFailure func()) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339)
	seedProjectionCheckpoint(t, db, maxProxyLogID(t, db))

	siteID := insertProjectionSite(t, db, suffix, now)
	accountID := insertProjectionAccount(t, db, siteID, suffix, now)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM proxy_logs WHERE account_id = ?", accountID)
		_, _ = db.Exec("DELETE FROM model_day_usage WHERE site_id = ?", siteID)
		_, _ = db.Exec("DELETE FROM site_hour_usage WHERE site_id = ?", siteID)
		_, _ = db.Exec("DELETE FROM site_day_usage WHERE site_id = ?", siteID)
		_, _ = db.Exec("DELETE FROM sites WHERE id = ?", siteID)
		seedProjectionCheckpoint(t, db, maxProxyLogID(t, db))
	})

	logTime := time.Date(2026, 7, 6, 10, 37, 42, 0, time.UTC)
	logID := insertProjectionProxyLog(t, db, accountID, "success", "gpt-4.1", 120, 0.42, 230, logTime)

	previousDB := store.GetDB()
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(previousDB) })

	s := NewUsageAggregationScheduler(testConfig())

	// Pass 1: flush fails inside the batch transaction.
	armFailure()
	failed := s.RunProjectionPass()
	if failed != nil {
		t.Fatalf("failed projection pass = %+v, want nil", failed)
	}

	// Watermark must not advance past the failed batch, and the failure must
	// be recorded on the checkpoint for the next pass to retry.
	watermark, lastError := readProjectionCheckpointState(t, db)
	if watermark != 0 {
		t.Fatalf("watermark after failed flush = %d, want 0 (unchanged)", watermark)
	}
	if !lastError.Valid || lastError.String == "" {
		t.Fatalf("last_error after failed flush = %v, want recorded failure", lastError)
	}

	// The batch delta must not be partially applied: all three usage tables
	// stay empty for this site, and the source proxy_logs row survives.
	for _, table := range []string{"site_day_usage", "site_hour_usage", "model_day_usage"} {
		if count := countUsageRows(t, db, table, siteID); count != 0 {
			t.Fatalf("%s rows after failed flush = %d, want 0 (rolled back)", table, count)
		}
	}
	var remainingLogs int
	if err := db.Get(&remainingLogs, "SELECT COUNT(*) FROM proxy_logs WHERE id = ?", logID); err != nil {
		t.Fatalf("read proxy_logs: %v", err)
	}
	if remainingLogs != 1 {
		t.Fatalf("proxy_logs rows after failed flush = %d, want 1 (source delta preserved)", remainingLogs)
	}

	// Pass 2: retry succeeds and projects the preserved delta exactly once.
	disarmFailure()
	retried := s.RunProjectionPass()
	if retried == nil {
		t.Fatal("retry projection pass returned nil")
	}
	if retried.ProcessedLogs != 1 {
		t.Fatalf("retry ProcessedLogs = %d, want 1", retried.ProcessedLogs)
	}

	watermark, lastError = readProjectionCheckpointState(t, db)
	if watermark != logID {
		t.Fatalf("watermark after successful retry = %d, want %d", watermark, logID)
	}
	if lastError.Valid {
		t.Fatalf("last_error after successful retry = %q, want cleared", lastError.String)
	}

	day := logTime.Format("2006-01-02")
	var dayUsage struct {
		TotalCalls   int     `db:"total_calls"`
		SuccessCalls int     `db:"success_calls"`
		FailedCalls  int     `db:"failed_calls"`
		TotalTokens  int64   `db:"total_tokens"`
		Spend        float64 `db:"total_summary_spend"`
	}
	if err := db.Get(&dayUsage, `SELECT total_calls, success_calls, failed_calls, total_tokens, total_summary_spend
		FROM site_day_usage WHERE site_id = ? AND local_day = ?`, siteID, day); err != nil {
		t.Fatalf("read site_day_usage: %v", err)
	}
	if dayUsage.TotalCalls != 1 || dayUsage.SuccessCalls != 1 || dayUsage.FailedCalls != 0 {
		t.Fatalf("site_day_usage calls = %+v, want total=1 success=1 failed=0", dayUsage)
	}
	if dayUsage.TotalTokens != 120 {
		t.Fatalf("site_day_usage total_tokens = %d, want 120", dayUsage.TotalTokens)
	}
	if math.Abs(dayUsage.Spend-0.42) > 0.000001 {
		t.Fatalf("site_day_usage spend = %.8f, want 0.42", dayUsage.Spend)
	}

	var hourCalls int
	if err := db.Get(&hourCalls, `SELECT total_calls FROM site_hour_usage
		WHERE site_id = ? AND bucket_start_utc = ?`, siteID, logTime.Truncate(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("read site_hour_usage: %v", err)
	}
	if hourCalls != 1 {
		t.Fatalf("site_hour_usage total_calls = %d, want 1", hourCalls)
	}

	var modelCalls int
	if err := db.Get(&modelCalls, `SELECT total_calls FROM model_day_usage
		WHERE site_id = ? AND local_day = ? AND model = ?`, siteID, day, "gpt-4.1"); err != nil {
		t.Fatalf("read model_day_usage: %v", err)
	}
	if modelCalls != 1 {
		t.Fatalf("model_day_usage total_calls = %d, want 1", modelCalls)
	}

	// Pass 3: watermark advanced, nothing left to re-project (no double count).
	idle := s.RunProjectionPass()
	if idle == nil || idle.ProcessedLogs != 0 {
		t.Fatalf("idle pass = %+v, want ProcessedLogs=0", idle)
	}
}

// TestUsageAggregationFlushFailureKeepsWatermarkAtLastPersistedBatch covers
// the multi-batch case: when a later batch fails, earlier batches stay
// committed and the watermark must sit at the end of the last persisted
// batch, so only the failed range is re-projected on retry.
func TestUsageAggregationFlushFailureKeepsWatermarkAtLastPersistedBatch(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	seedProjectionCheckpoint(t, db, maxProxyLogID(t, db))
	suffix := "multibatch-" + strings.ReplaceAll(t.Name(), "/", "-")
	siteID := insertProjectionSite(t, db, suffix, now)
	accountID := insertProjectionAccount(t, db, siteID, suffix, now)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM proxy_logs WHERE account_id = ?", accountID)
		_, _ = db.Exec("DELETE FROM model_day_usage WHERE site_id = ?", siteID)
		_, _ = db.Exec("DELETE FROM site_hour_usage WHERE site_id = ?", siteID)
		_, _ = db.Exec("DELETE FROM site_day_usage WHERE site_id = ?", siteID)
		_, _ = db.Exec("DELETE FROM sites WHERE id = ?", siteID)
		seedProjectionCheckpoint(t, db, maxProxyLogID(t, db))
	})

	previousDB := store.GetDB()
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(previousDB) })

	// One full batch (1000 rows, single day/hour/model bucket) plus a small
	// follow-up batch that will fail.
	baseTime := time.Date(2026, 7, 6, 10, 0, 0, 0, time.UTC)
	insertProjectionProxyLog(t, db, accountID, "success", "gpt-4.1", 120, 0.42, 230, baseTime)
	for i := 1; i < usageProjectionBatchSize; i++ {
		insertProjectionProxyLog(t, db, accountID, "success", "gpt-4.1", 120, 0.42, 230, baseTime.Add(time.Duration(i)*time.Second))
	}

	s := NewUsageAggregationScheduler(testConfig())

	first := s.RunProjectionPass()
	if first == nil || first.ProcessedLogs != usageProjectionBatchSize {
		t.Fatalf("first pass = %+v, want ProcessedLogs=%d", first, usageProjectionBatchSize)
	}
	persistedWatermark := maxProxyLogID(t, db)

	// Second batch (3 rows) is armed to fail.
	const triggerName = "usage_fail_site_day_insert_multibatch"
	if _, err := db.Exec(`CREATE TRIGGER ` + triggerName + `
		BEFORE INSERT ON site_day_usage
		FOR EACH ROW
		BEGIN
			SELECT RAISE(ABORT, 'simulated site_day_usage flush failure');
		END;`); err != nil {
		t.Fatalf("arm failure trigger: %v", err)
	}
	defer func() { _, _ = db.Exec("DROP TRIGGER IF EXISTS " + triggerName) }()

	for i := 0; i < 3; i++ {
		insertProjectionProxyLog(t, db, accountID, "success", "gpt-4.1", 120, 0.42, 230, baseTime.Add(time.Duration(usageProjectionBatchSize+i)*time.Second))
	}

	failed := s.RunProjectionPass()
	if failed != nil {
		t.Fatalf("failed pass = %+v, want nil", failed)
	}

	// Watermark sits at the end of the last persisted batch, not past the
	// failed one.
	watermark, _ := readProjectionCheckpointState(t, db)
	if watermark != persistedWatermark {
		t.Fatalf("watermark after multi-batch failure = %d, want %d", watermark, persistedWatermark)
	}

	// Retry projects only the failed 3 rows and never re-projects the first
	// batch (no double counting).
	if _, err := db.Exec("DROP TRIGGER IF EXISTS " + triggerName); err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}
	retried := s.RunProjectionPass()
	if retried == nil || retried.ProcessedLogs != 3 {
		t.Fatalf("retry pass = %+v, want ProcessedLogs=3", retried)
	}

	day := baseTime.Format("2006-01-02")
	var dayUsage struct {
		TotalCalls  int   `db:"total_calls"`
		TotalTokens int64 `db:"total_tokens"`
	}
	if err := db.Get(&dayUsage, `SELECT total_calls, total_tokens
		FROM site_day_usage WHERE site_id = ? AND local_day = ?`, siteID, day); err != nil {
		t.Fatalf("read site_day_usage: %v", err)
	}
	wantCalls := usageProjectionBatchSize + 3
	if dayUsage.TotalCalls != wantCalls {
		t.Fatalf("site_day_usage total_calls = %d, want %d (no double count)", dayUsage.TotalCalls, wantCalls)
	}
	if wantTokens := int64(wantCalls) * 120; dayUsage.TotalTokens != wantTokens {
		t.Fatalf("site_day_usage total_tokens = %d, want %d", dayUsage.TotalTokens, wantTokens)
	}

	idle := s.RunProjectionPass()
	if idle == nil || idle.ProcessedLogs != 0 {
		t.Fatalf("idle pass = %+v, want ProcessedLogs=0", idle)
	}
}

func readProjectionCheckpointState(t *testing.T, db *store.DB) (watermark int64, lastError sql.NullString) {
	t.Helper()
	row := db.QueryRowx(`SELECT last_proxy_log_id, last_error
		FROM analytics_projection_checkpoints WHERE projector_key = ?`, usageProjectorKey)
	if err := row.Scan(&watermark, &lastError); err != nil {
		t.Fatalf("read checkpoint state: %v", err)
	}
	return watermark, lastError
}

func countUsageRows(t *testing.T, db *store.DB, table string, siteID int64) int {
	t.Helper()
	var count int
	if err := db.Get(&count, "SELECT COUNT(*) FROM "+table+" WHERE site_id = ?", siteID); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}
