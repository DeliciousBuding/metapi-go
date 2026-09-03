package scheduler

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// The prune has to satisfy two claims at once, and only the second one is
// interesting: aged probe rows go away, and the row route rebuild reads never
// does — however old it is. A plain age-based DELETE passes the first and
// silently breaks the probe filter (#625) for any model that has not been
// probed inside the window, which is a routing behaviour change caused by a
// cleanup job.

func TestModelProbeResultRetention_SQLitePrunesHistoryButNeverTheRowRoutingReads(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}

	runModelProbeResultRetentionLifecycle(t, db, "sqlite-"+strings.ReplaceAll(t.Name(), "/", "-"))
}

func TestModelProbeResultRetention_PostgresPrunesHistoryButNeverTheRowRoutingReads(t *testing.T) {
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

	runModelProbeResultRetentionLifecycle(t, db, "pg-"+fmt.Sprint(time.Now().UnixNano()))
}

func runModelProbeResultRetentionLifecycle(t *testing.T, db *store.DB, suffix string) {
	t.Helper()

	now := time.Now().UTC()
	siteID := insertProbeRetentionSite(t, db, suffix, now)
	accountA := insertProbeRetentionAccount(t, db, siteID, suffix+"-a", now)
	accountB := insertProbeRetentionAccount(t, db, siteID, suffix+"-b", now)
	t.Cleanup(func() {
		for _, accountID := range []int64{accountA, accountB} {
			_, _ = db.Exec("DELETE FROM model_probe_results WHERE account_id = ?", accountID)
			_, _ = db.Exec("DELETE FROM accounts WHERE id = ?", accountID)
		}
		_, _ = db.Exec("DELETE FROM sites WHERE id = ?", siteID)
	})

	aged := func(days int) string {
		return now.Add(-time.Duration(days) * 24 * time.Hour).Format(time.RFC3339)
	}
	countMine := func() int {
		t.Helper()
		return countRows(t, db,
			`SELECT COUNT(*) FROM model_probe_results WHERE account_id IN (?, ?)`,
			accountA, accountB)
	}
	hasRow := func(accountID int64, model, createdAt string) bool {
		t.Helper()
		return countRows(t, db,
			`SELECT COUNT(*) FROM model_probe_results WHERE account_id = ? AND model_name = ? AND created_at = ?`,
			accountID, model, createdAt) == 1
	}

	// (account A, "gpt-stale"): three rows, ALL outside the 7-day window. The
	// newest of the three is the one route rebuild reads, so age alone must not
	// decide its fate.
	insertProbeResult(t, db, accountA, siteID, "gpt-stale", aged(30)) // pruned
	insertProbeResult(t, db, accountA, siteID, "gpt-stale", aged(20)) // pruned
	staleNewestA := aged(8)
	insertProbeResult(t, db, accountA, siteID, "gpt-stale", staleNewestA) // exempt
	// (account A, "gpt-fresh"): inside the window, kept for its age.
	freshA := aged(1)
	insertProbeResult(t, db, accountA, siteID, "gpt-fresh", freshA)
	// (account B, "gpt-stale"): the exemption is per (account, model), not one
	// global newest row — a second account's latest probe survives too.
	insertProbeResult(t, db, accountB, siteID, "gpt-stale", aged(40)) // pruned
	staleNewestB := aged(9)
	insertProbeResult(t, db, accountB, siteID, "gpt-stale", staleNewestB) // exempt

	// Anti-vacuity: prove the seed landed before asserting what the prune left.
	if got := countMine(); got != 6 {
		t.Fatalf("seeded %d probe rows, want 6 — the lifecycle would prove nothing", got)
	}

	s := NewModelProbeResultRetentionScheduler(&config.Config{})
	s.runCleanupLocked(db, modelProbeResultRetentionDays)

	if got := countMine(); got != 3 {
		t.Errorf("probe rows after prune = %d, want 3 (aged history gone, latest-per-pair kept)", got)
	}
	if !hasRow(accountA, "gpt-stale", staleNewestA) {
		t.Errorf("pruned the newest (account A, gpt-stale) row at %s: route rebuild's probe filter reads exactly this row", staleNewestA)
	}
	if !hasRow(accountB, "gpt-stale", staleNewestB) {
		t.Errorf("pruned the newest (account B, gpt-stale) row: the exemption must be per (account_id, model_name), not global")
	}
	if !hasRow(accountA, "gpt-fresh", freshA) {
		t.Errorf("pruned an in-window row at %s", freshA)
	}
	if hasRow(accountA, "gpt-stale", aged(30)) || hasRow(accountA, "gpt-stale", aged(20)) {
		t.Errorf("aged non-latest rows survived; the table would still grow without bound")
	}
	if hasRow(accountB, "gpt-stale", aged(40)) {
		t.Errorf("aged non-latest row for account B survived")
	}

	// The consumer's own query, not a paraphrase of it: after the prune, route
	// rebuild must still see one latest row per (account, model) pair.
	routingRows := countRows(t, db,
		`SELECT COUNT(*) FROM model_probe_results
		 WHERE account_id IN (?, ?)
		   AND id IN (SELECT MAX(id) FROM model_probe_results GROUP BY account_id, model_name)`,
		accountA, accountB)
	if routingRows != 3 {
		t.Errorf("route rebuild's latest-per-pair query returns %d rows for the seeded pairs, want 3", routingRows)
	}
}

// TestModelProbeResultRetentionJobRuns guards the failure mode
// app.TestBuildSchedulersRegistration was written for: a job that is
// constructed and registered but disabled by its own predicate, so the table
// keeps growing while everything looks wired.
func TestModelProbeResultRetentionJobRuns(t *testing.T) {
	cfg := &config.Config{}
	s := NewModelProbeResultRetentionScheduler(cfg)

	if got := s.Name(); got != "model-probe-result-retention" {
		t.Errorf("Name() = %q, want %q", got, "model-probe-result-retention")
	}
	if disabled, reason := s.opts.DisabledFn(cfg); disabled {
		t.Errorf("job is disabled by default (%s); model_probe_results would grow without bound", reason)
	}
	if got := s.opts.RetentionDaysFn(cfg); got <= 0 {
		t.Errorf("retention window = %d days; runCleanup treats <= 0 as \"do nothing\"", got)
	}
	if s.opts.Table != "model_probe_results" {
		t.Errorf("Table = %q, want model_probe_results", s.opts.Table)
	}
}

func insertProbeRetentionSite(t *testing.T, db *store.DB, suffix string, now time.Time) int64 {
	t.Helper()
	stamp := now.Format(time.RFC3339)
	query := `INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		VALUES (?, ?, 'openai', 'active', ?, ?)`
	args := []any{"probe-retention-" + suffix, "https://probe-retention-" + suffix + ".example.test", stamp, stamp}
	if db.Dialect == store.DialectPostgres {
		var id int64
		if err := db.QueryRow(query+" RETURNING id", args...).Scan(&id); err != nil {
			t.Fatalf("insert postgres site: %v", err)
		}
		return id
	}
	result, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("insert sqlite site: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("sqlite site LastInsertId: %v", err)
	}
	return id
}

func insertProbeRetentionAccount(t *testing.T, db *store.DB, siteID int64, suffix string, now time.Time) int64 {
	t.Helper()
	stamp := now.Format(time.RFC3339)
	query := `INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		VALUES (?, ?, ?, 'active', ?, ?, ?)`
	args := []any{siteID, "probe-retention-" + suffix, "sk-probe-retention-" + suffix, true, stamp, stamp}
	if db.Dialect == store.DialectPostgres {
		var id int64
		if err := db.QueryRow(query+" RETURNING id", args...).Scan(&id); err != nil {
			t.Fatalf("insert postgres account: %v", err)
		}
		return id
	}
	result, err := db.Exec(query, args...)
	if err != nil {
		t.Fatalf("insert sqlite account: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("sqlite account LastInsertId: %v", err)
	}
	return id
}

func insertProbeResult(t *testing.T, db *store.DB, accountID, siteID int64, model, createdAt string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO model_probe_results
			(channel_id, account_id, site_id, model_name, status, latency_ms, http_status, error_text, created_at)
		 VALUES (NULL, ?, ?, ?, 'success', 12, 200, NULL, ?)`,
		accountID, siteID, model, createdAt); err != nil {
		t.Fatalf("insert probe result (%d, %s, %s): %v", accountID, model, createdAt, err)
	}
}
