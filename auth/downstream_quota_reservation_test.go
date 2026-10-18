package auth

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

// insertQuotaKey creates one managed key carrying an accessPolicy quota and
// returns its ID.
func insertQuotaKey(t *testing.T, key string, quota store.DownstreamQuota) int64 {
	t.Helper()
	policy, err := json.Marshal(store.DownstreamAccessPolicy{Quota: &quota})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := testDB(t).QueryRowx(
		`INSERT INTO downstream_api_keys(name,key,supported_models,access_policy) VALUES (?,?,'["*"]',?) RETURNING id`,
		key, key, string(policy)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func admissionReservationResult(key string) DownstreamTokenAuthResult {
	return authorizeDownstreamToken(key, &config.RuntimeSettings{}, "", admissionOptions{reserveQuota: true})
}

// TestQuotaReservationConcurrentAdmission proves that concurrent live
// admissions cannot exceed the requests limit: exactly one of N requests gets
// the slot, the rest are denied with the unchanged over_quota reason.
func TestQuotaReservationConcurrentAdmission(t *testing.T) {
	setupTestDB(t)
	db := testDB(t)
	requests := int64(1)
	id := insertQuotaKey(t, "sk-quota-race", store.DownstreamQuota{
		Requests: &requests,
		Period:   store.DownstreamQuotaPeriod{Type: "all_time"},
	})

	const workers = 16
	start := make(chan struct{})
	results := make([]DownstreamTokenAuthResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = admissionReservationResult("sk-quota-race")
		}(i)
	}
	close(start)
	wg.Wait()

	var winner string
	allowed, denied := 0, 0
	for i, result := range results {
		if result.OK {
			allowed++
			winner = result.QuotaReservationID
			continue
		}
		if result.Reason != "over_quota" || result.StatusCode != 429 {
			t.Errorf("worker %d: status=%d reason=%q, want 429 over_quota", i, result.StatusCode, result.Reason)
		}
		denied++
	}
	if allowed != 1 || denied != workers-1 {
		t.Fatalf("concurrent admission allowed=%d denied=%d, want 1/%d", allowed, denied, workers-1)
	}
	if winner == "" {
		t.Fatal("admitted request has no reservation ID")
	}
	var reservations int
	if err := db.Get(&reservations, `SELECT COUNT(*) FROM downstream_quota_reservations WHERE key_id=?`, id); err != nil || reservations != 1 {
		t.Fatalf("reservation rows=%d err=%v, want 1", reservations, err)
	}

	// Settlement removes the hold and records the attempt in one transaction.
	tokens := int64(7)
	cost := 0.25
	RecordManagedKeyQuotaUsage(id, winner, &tokens, &cost)
	var usageRows int
	if err := db.Get(&usageRows, `SELECT COUNT(*) FROM downstream_quota_usage WHERE key_id=?`, id); err != nil || usageRows != 1 {
		t.Fatalf("usage rows=%d err=%v, want 1", usageRows, err)
	}
	if err := db.Get(&reservations, `SELECT COUNT(*) FROM downstream_quota_reservations WHERE key_id=?`, id); err != nil || reservations != 0 {
		t.Fatalf("reservation rows after settlement=%d err=%v, want 0", reservations, err)
	}
	if next := admissionReservationResult("sk-quota-race"); next.OK || next.Reason != "over_quota" {
		t.Fatalf("post-settlement admission OK=%v reason=%q, want over_quota", next.OK, next.Reason)
	}

	// A retry that reuses an already-settled reservation still records usage:
	// billing must not depend on admission bookkeeping.
	RecordManagedKeyQuotaUsage(id, winner, &tokens, &cost)
	if err := db.Get(&usageRows, `SELECT COUNT(*) FROM downstream_quota_usage WHERE key_id=?`, id); err != nil || usageRows != 2 {
		t.Fatalf("usage rows after retry=%d err=%v, want 2", usageRows, err)
	}
}

// TestQuotaReservationExpiresAfterTTL proves a crashed process cannot lock a
// key forever: a hold older than the TTL is swept by the next admission, while
// a fresh hold keeps blocking.
func TestQuotaReservationExpiresAfterTTL(t *testing.T) {
	setupTestDB(t)
	db := testDB(t)
	requests := int64(1)
	id := insertQuotaKey(t, "sk-quota-ttl", store.DownstreamQuota{
		Requests: &requests,
		Period:   store.DownstreamQuotaPeriod{Type: "all_time"},
	})
	now := time.Now()
	start, end := int64(0), now.UnixMilli()+1
	staleID := "stale-reservation"
	if _, err := db.Exec(`INSERT INTO downstream_quota_reservations
		(reservation_id,key_id,window_start,window_end,requests,tokens,cost,created_at)
		VALUES (?,?,?,?,1,NULL,NULL,?)`,
		staleID, id, start, end, now.Add(-quotaReservationTTL-time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	result := admissionReservationResult("sk-quota-ttl")
	if !result.OK {
		t.Fatalf("admission behind an expired hold denied: %s", result.Reason)
	}
	var stale int
	if err := db.Get(&stale, `SELECT COUNT(*) FROM downstream_quota_reservations WHERE reservation_id=?`, staleID); err != nil || stale != 0 {
		t.Fatalf("stale reservation rows=%d err=%v, want 0", stale, err)
	}
	// The fresh hold still occupies the only slot.
	if next := admissionReservationResult("sk-quota-ttl"); next.OK || next.Reason != "over_quota" {
		t.Fatalf("admission behind a fresh hold OK=%v reason=%q, want over_quota", next.OK, next.Reason)
	}
}

// TestQuotaReservationTokenHoldAndUnknownSettlement covers the token dimension:
// the estimated input is held at admission, and settling without observed
// tokens keeps the fail-closed quota_usage_unknown verdict.
func TestQuotaReservationTokenHoldAndUnknownSettlement(t *testing.T) {
	setupTestDB(t)
	db := testDB(t)
	requests := int64(100)
	tokensMax := int64(10)
	id := insertQuotaKey(t, "sk-quota-tokens", store.DownstreamQuota{
		Requests:    &requests,
		TotalTokens: &tokensMax,
		Period:      store.DownstreamQuotaPeriod{Type: "all_time"},
	})
	result := authorizeDownstreamToken("sk-quota-tokens", &config.RuntimeSettings{}, "", admissionOptions{
		reserveQuota:   true,
		estimateTokens: func() int64 { return 10 },
	})
	if !result.OK {
		t.Fatalf("first token admission denied: %s", result.Reason)
	}
	var held int64
	if err := db.Get(&held, `SELECT tokens FROM downstream_quota_reservations WHERE reservation_id=?`, result.QuotaReservationID); err != nil || held != 10 {
		t.Fatalf("held tokens=%d err=%v, want 10", held, err)
	}
	// The hold alone exhausts the token limit, so a concurrent request cannot
	// overrun it.
	if next := authorizeDownstreamToken("sk-quota-tokens", &config.RuntimeSettings{}, "", admissionOptions{
		reserveQuota:   true,
		estimateTokens: func() int64 { return 1 },
	}); next.OK || next.Reason != "over_quota" {
		t.Fatalf("second token admission OK=%v reason=%q, want over_quota", next.OK, next.Reason)
	}
	// Completion without observed tokens: hold removed, usage stays NULL.
	RecordManagedKeyQuotaUsage(id, result.QuotaReservationID, nil, nil)
	var reservations int
	if err := db.Get(&reservations, `SELECT COUNT(*) FROM downstream_quota_reservations WHERE key_id=?`, id); err != nil || reservations != 0 {
		t.Fatalf("reservation rows=%d err=%v, want 0", reservations, err)
	}
	if next := admissionReservationResult("sk-quota-tokens"); next.OK || next.Reason != "quota_usage_unknown" {
		t.Fatalf("unknown-usage admission OK=%v reason=%q, want quota_usage_unknown", next.OK, next.Reason)
	}
}

// TestQuotaReservationConcurrentAdmissionPostgres runs the same admission
// race against PostgreSQL, exercising the per-key SELECT ... FOR UPDATE lock
// and the settlement transaction through the dialect-rebound SQL. The shared
// PG_TEST_DSN schema requires serial package runs (CI passes -p 1).
func TestQuotaReservationConcurrentAdmissionPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	_ = store.CloseDatabase()
	cfg := &config.Config{DataDir: t.TempDir()}
	rt := &config.RuntimeSettings{DbType: "postgres", DbUrl: dsn}
	if err := store.EnsureRuntimeDatabase(cfg, rt); err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = store.CloseDatabase() })
	db := testDB(t)
	pgtest.Reset(t, db.DB)

	requests := int64(1)
	id := insertQuotaKey(t, "sk-quota-race-pg", store.DownstreamQuota{
		Requests: &requests,
		Period:   store.DownstreamQuotaPeriod{Type: "all_time"},
	})

	const workers = 16
	start := make(chan struct{})
	results := make([]DownstreamTokenAuthResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = admissionReservationResult("sk-quota-race-pg")
		}(i)
	}
	close(start)
	wg.Wait()

	var winner string
	allowed := 0
	for i, result := range results {
		if result.OK {
			allowed++
			winner = result.QuotaReservationID
			continue
		}
		if result.Reason != "over_quota" {
			t.Errorf("worker %d: status=%d reason=%q, want over_quota", i, result.StatusCode, result.Reason)
		}
	}
	if allowed != 1 || winner == "" {
		t.Fatalf("concurrent PostgreSQL admission allowed=%d, want 1", allowed)
	}
	tokens := int64(5)
	cost := 0.1
	RecordManagedKeyQuotaUsage(id, winner, &tokens, &cost)
	var reservations, usageRows int
	if err := db.Get(&reservations, `SELECT COUNT(*) FROM downstream_quota_reservations WHERE key_id=?`, id); err != nil || reservations != 0 {
		t.Fatalf("reservation rows=%d err=%v, want 0", reservations, err)
	}
	if err := db.Get(&usageRows, `SELECT COUNT(*) FROM downstream_quota_usage WHERE key_id=?`, id); err != nil || usageRows != 1 {
		t.Fatalf("usage rows=%d err=%v, want 1", usageRows, err)
	}
}

// TestQuotaReservationReleaseFreesSlot covers the release path for requests
// that end before any billable attempt is logged.
func TestQuotaReservationReleaseFreesSlot(t *testing.T) {
	setupTestDB(t)
	requests := int64(1)
	insertQuotaKey(t, "sk-quota-release", store.DownstreamQuota{
		Requests: &requests,
		Period:   store.DownstreamQuotaPeriod{Type: "all_time"},
	})
	first := admissionReservationResult("sk-quota-release")
	if !first.OK {
		t.Fatalf("first admission denied: %s", first.Reason)
	}
	ReleaseManagedKeyQuotaReservation(first.QuotaReservationID)
	if second := admissionReservationResult("sk-quota-release"); !second.OK {
		t.Fatalf("admission after release denied: %s", second.Reason)
	}
}
