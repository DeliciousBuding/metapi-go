package proxyhandler

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// The process-local video task cache is bounded by lazy eviction: a TTL aligned
// with PROXY_VIDEO_TASK_RETENTION_DAYS plus a hard capacity guardrail. These
// tests pin the eviction contract — including the throttle, so a future edit
// cannot quietly turn the insert path into a per-request full scan.

// videoTaskCacheFixture isolates the cache for one GC test: empty map, neutral
// sweep bookkeeping ("a sweep just ran at start"), pinned injectable clock and
// capacity guardrail, and a published retention-days config. Everything is
// restored on cleanup, so GC tests neither evict nor inherit lines owned by
// other tests in this package.
//
// The returned advance func moves the pinned clock; call it only from the test
// goroutine (never while cache goroutines are in flight).
func videoTaskCacheFixture(t *testing.T, start time.Time, retentionDays, maxEntries int) func(time.Duration) time.Time {
	t.Helper()

	prevNow, prevMaxEntries, prevCfg := videoTaskNow, videoTaskCacheMaxEntries, config.Get()
	now := start.UTC()
	videoTaskNow = func() time.Time { return now }
	videoTaskCacheMaxEntries = maxEntries
	cfgCopy := *prevCfg
	cfgCopy.ProxyVideoTaskRetentionDays = retentionDays
	config.Set(&cfgCopy)

	videoTaskStoreMu.Lock()
	prevStore := videoTaskStore
	prevInserts, prevSweepAt := videoTaskInsertsSinceSweep, videoTaskLastSweepAt
	videoTaskStore = make(map[string]*videoTaskEntry)
	videoTaskInsertsSinceSweep = 0
	videoTaskLastSweepAt = now
	videoTaskStoreMu.Unlock()

	t.Cleanup(func() {
		videoTaskStoreMu.Lock()
		videoTaskStore = prevStore
		videoTaskInsertsSinceSweep, videoTaskLastSweepAt = prevInserts, prevSweepAt
		videoTaskStoreMu.Unlock()
		videoTaskNow, videoTaskCacheMaxEntries = prevNow, prevMaxEntries
		config.Set(prevCfg)
	})

	return func(d time.Duration) time.Time {
		now = now.Add(d)
		return now
	}
}

// videoTaskCacheSnapshotForTest copies the cache keys with their storedAt plus
// the sweep bookkeeping, so assertions run without holding the lock.
func videoTaskCacheSnapshotForTest() (storedAt map[string]time.Time, insertsSinceSweep int, lastSweepAt time.Time) {
	videoTaskStoreMu.RLock()
	defer videoTaskStoreMu.RUnlock()
	storedAt = make(map[string]time.Time, len(videoTaskStore))
	for id, entry := range videoTaskStore {
		if entry != nil {
			storedAt[id] = entry.storedAt
		}
	}
	return storedAt, videoTaskInsertsSinceSweep, videoTaskLastSweepAt
}

// seedVideoTaskCacheLineForTest inserts a line with an arbitrary storedAt,
// bypassing the insert path (and its sweep accounting) to model a line that has
// been sitting in the cache since before the test started.
func seedVideoTaskCacheLineForTest(t *testing.T, publicID string, storedAt time.Time) {
	t.Helper()
	videoTaskStoreMu.Lock()
	videoTaskStore[publicID] = &videoTaskEntry{
		task:     &ProxyVideoTask{PublicID: publicID, UpstreamVideoID: "up_" + publicID},
		storedAt: storedAt.UTC(),
	}
	videoTaskStoreMu.Unlock()
}

// forceVideoTaskSweepForTest runs one sweep immediately, bypassing the
// throttle, and resets the bookkeeping the way a real sweep does.
func forceVideoTaskSweepForTest(t *testing.T) {
	t.Helper()
	videoTaskStoreMu.Lock()
	now := videoTaskNow().UTC()
	videoTaskInsertsSinceSweep = 0
	videoTaskLastSweepAt = now
	sweepVideoTaskCacheLocked(now)
	videoTaskStoreMu.Unlock()
}

// setVideoTaskSweepCounterForTest pretends n inserts already happened since the
// last sweep, so a test can trip the count threshold with a single real insert.
func setVideoTaskSweepCounterForTest(t *testing.T, n int) {
	t.Helper()
	videoTaskStoreMu.Lock()
	videoTaskInsertsSinceSweep = n
	videoTaskStoreMu.Unlock()
}

func TestVideoTaskCache_ExpiredLineIsLazyThenEvictedBySweep(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	advance := videoTaskCacheFixture(t, base, 7, videoTaskCacheMaxEntries)

	// A line stored 8 days ago is past the 7-day TTL but has not been swept yet.
	seedVideoTaskCacheLineForTest(t, "v_aged", base.Add(-8*24*time.Hour))
	// One insert inside the throttle window: no sweep may run.
	SaveProxyVideoTask(&ProxyVideoTask{PublicID: "v_fresh", UpstreamVideoID: "up_fresh"})

	storedAt, inserts, lastSweep := videoTaskCacheSnapshotForTest()
	if _, ok := storedAt["v_aged"]; !ok {
		t.Fatalf("expired line was removed without a sweep: %v", storedAt)
	}
	if len(storedAt) != 2 || inserts != 1 || !lastSweep.Equal(base) {
		t.Fatalf("throttled insert ran a sweep: len=%d inserts=%d lastSweep=%v", len(storedAt), inserts, lastSweep)
	}
	// Reads still refuse to serve the expired line while it waits for the sweep.
	if got := GetProxyVideoTaskByPublicID("v_aged"); got != nil {
		t.Fatalf("expired line served from cache: %+v", got)
	}
	if got := GetProxyVideoTaskByPublicID("v_fresh"); got == nil || got.UpstreamVideoID != "up_fresh" {
		t.Fatalf("fresh line = %+v", got)
	}

	forceVideoTaskSweepForTest(t)
	storedAt, _, _ = videoTaskCacheSnapshotForTest()
	if _, ok := storedAt["v_aged"]; ok {
		t.Fatalf("sweep kept the expired line: %v", storedAt)
	}
	if _, ok := storedAt["v_fresh"]; !ok {
		t.Fatalf("sweep evicted the in-window line: %v", storedAt)
	}

	// End to end: the next insert after the throttle window sweeps on its own.
	advance(8 * 24 * time.Hour)
	SaveProxyVideoTask(&ProxyVideoTask{PublicID: "v_newest", UpstreamVideoID: "up_newest"})
	storedAt, inserts, _ = videoTaskCacheSnapshotForTest()
	if _, ok := storedAt["v_fresh"]; ok {
		t.Fatalf("insert-path sweep did not evict the now-expired line: %v", storedAt)
	}
	if _, ok := storedAt["v_newest"]; !ok {
		t.Fatalf("insert-path sweep evicted the new line: %v", storedAt)
	}
	if inserts != 0 {
		t.Fatalf("sweep bookkeeping not reset: inserts=%d", inserts)
	}
}

func TestVideoTaskCache_RetentionDisabledKeepsLines(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	advance := videoTaskCacheFixture(t, base, 0, videoTaskCacheMaxEntries)

	seedVideoTaskCacheLineForTest(t, "v_kept", base.Add(-90*24*time.Hour))
	advance(90 * 24 * time.Hour)
	SaveProxyVideoTask(&ProxyVideoTask{PublicID: "v_new", UpstreamVideoID: "up_new"})
	forceVideoTaskSweepForTest(t)

	storedAt, _, _ := videoTaskCacheSnapshotForTest()
	if len(storedAt) != 2 {
		t.Fatalf("retention_days=0 must disable TTL eviction, got %v", storedAt)
	}
	// With TTL off the 90-day-old line is still served: "keep forever" is an
	// explicit operator choice, and memory stays bounded by the capacity guard.
	if got := GetProxyVideoTaskByPublicID("v_kept"); got == nil {
		t.Fatal("expected the retained line to be served")
	}
}

func TestVideoTaskCache_SweepIsThrottledBetweenThresholds(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	advance := videoTaskCacheFixture(t, base, 1, videoTaskCacheMaxEntries)

	seedVideoTaskCacheLineForTest(t, "v_aged", base.Add(-48*time.Hour))
	// 10 inserts inside the throttle window (1 s apart, counter below 256).
	for i := 0; i < 10; i++ {
		advance(time.Second)
		SaveProxyVideoTask(&ProxyVideoTask{PublicID: "v_throttle_" + strconv.FormatInt(int64(i), 10)})
	}

	storedAt, inserts, lastSweep := videoTaskCacheSnapshotForTest()
	if len(storedAt) != 11 || inserts != 10 || !lastSweep.Equal(base) {
		t.Fatalf("sweep ran inside the throttle window: len=%d inserts=%d lastSweep=%v",
			len(storedAt), inserts, lastSweep)
	}
	if _, ok := storedAt["v_aged"]; !ok {
		t.Fatalf("expired line swept early: %v", storedAt)
	}

	// The count threshold trips exactly one sweep on the next insert.
	setVideoTaskSweepCounterForTest(t, videoTaskSweepEveryInserts-1)
	advance(time.Second)
	SaveProxyVideoTask(&ProxyVideoTask{PublicID: "v_trigger"})

	storedAt, inserts, lastSweep = videoTaskCacheSnapshotForTest()
	if _, ok := storedAt["v_aged"]; ok {
		t.Fatalf("count threshold did not evict the expired line: %v", storedAt)
	}
	if len(storedAt) != 11 || inserts != 0 {
		t.Fatalf("post-sweep state len=%d inserts=%d lastSweep=%v", len(storedAt), inserts, lastSweep)
	}
	if want := base.Add(11 * time.Second); !lastSweep.Equal(want) {
		t.Fatalf("lastSweep = %v, want %v", lastSweep, want)
	}
}

func TestVideoTaskCache_CapacityGuardrailEvictsOldestFirst(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	// TTL disabled so only the capacity guardrail can evict; cap 4 keeps the
	// headroom target at exactly 4 (4 - 4/16).
	advance := videoTaskCacheFixture(t, base, 0, 4)

	for i := 1; i <= 6; i++ {
		advance(time.Second)
		SaveProxyVideoTask(&ProxyVideoTask{PublicID: "v_cap_" + strconv.FormatInt(int64(i), 10)})
	}
	// Throttled inserts may overshoot the guardrail; the bound is cap+256.
	if storedAt, _, _ := videoTaskCacheSnapshotForTest(); len(storedAt) != 6 {
		t.Fatalf("expected throttled overshoot to 6 lines, got %v", storedAt)
	}

	// Count threshold trips a trim down to the cap, oldest first.
	setVideoTaskSweepCounterForTest(t, videoTaskSweepEveryInserts-1)
	advance(time.Second)
	SaveProxyVideoTask(&ProxyVideoTask{PublicID: "v_cap_7"})
	assertVideoTaskCacheKeys(t, "count-triggered trim", []string{"v_cap_4", "v_cap_5", "v_cap_6", "v_cap_7"})

	// Interval threshold trips the next trim (counter is far from 256).
	advance(videoTaskSweepMinInterval + time.Minute)
	SaveProxyVideoTask(&ProxyVideoTask{PublicID: "v_cap_8"})
	assertVideoTaskCacheKeys(t, "interval-triggered trim", []string{"v_cap_5", "v_cap_6", "v_cap_7", "v_cap_8"})

	// A sweep resets the counter, including for the insert that triggered it.
	storedAt, inserts, lastSweep := videoTaskCacheSnapshotForTest()
	if want := base.Add(7*time.Second + videoTaskSweepMinInterval + time.Minute); inserts != 0 || !lastSweep.Equal(want) {
		t.Fatalf("post-sweep bookkeeping inserts=%d lastSweep=%v want %v (len %d)",
			inserts, lastSweep, want, len(storedAt))
	}
}

func assertVideoTaskCacheKeys(t *testing.T, label string, want []string) {
	t.Helper()
	storedAt, _, _ := videoTaskCacheSnapshotForTest()
	if len(storedAt) != len(want) {
		t.Fatalf("%s: cache = %v, want %v", label, storedAt, want)
	}
	for _, id := range want {
		if _, ok := storedAt[id]; !ok {
			t.Fatalf("%s: missing %q, cache = %v", label, id, storedAt)
		}
	}
}

func TestVideoTaskCache_WarmLoadStampsItsOwnStoredAt(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	advance := videoTaskCacheFixture(t, base, 7, videoTaskCacheMaxEntries)

	const publicID = "v_warm_load"
	SaveProxyVideoTask(&ProxyVideoTask{PublicID: publicID, UpstreamVideoID: "up_warm"})
	// Drop the cache line only (restart / other instance); the durable row stays.
	videoTaskStoreMu.Lock()
	delete(videoTaskStore, publicID)
	videoTaskStoreMu.Unlock()

	advance(2 * 24 * time.Hour)
	if got := GetProxyVideoTaskByPublicID(publicID); got == nil || got.UpstreamVideoID != "up_warm" {
		t.Fatalf("warm load = %+v", got)
	}
	// TTL counts from the warm load, not from the 2-day-old durable row.
	storedAt, _, _ := videoTaskCacheSnapshotForTest()
	if want := base.Add(2 * 24 * time.Hour); !storedAt[publicID].Equal(want) {
		t.Fatalf("warm-load storedAt = %v, want %v", storedAt[publicID], want)
	}

	// 6 more days: 8 days past the durable row, 6 past the warm load → kept.
	advance(6 * 24 * time.Hour)
	forceVideoTaskSweepForTest(t)
	if storedAt, _, _ := videoTaskCacheSnapshotForTest(); len(storedAt) != 1 {
		t.Fatalf("warm-loaded line evicted inside its TTL: %v", storedAt)
	}

	// 2 more days put the warm load past the TTL: the cache line goes, the
	// durable row stays for the retention scheduler to prune.
	advance(2 * 24 * time.Hour)
	forceVideoTaskSweepForTest(t)
	if storedAt, _, _ := videoTaskCacheSnapshotForTest(); len(storedAt) != 0 {
		t.Fatalf("expired warm-loaded line kept: %v", storedAt)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id = ?`, publicID).Scan(&rows); err != nil {
		t.Fatalf("count durable rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("cache sweep deleted durable rows: count=%d", rows)
	}
}

func TestVideoTaskCache_ConcurrentInsertTrimAndSweep(t *testing.T) {
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	// Frozen clock (advanced only after the goroutines join) + a small guardrail
	// so concurrent inserts keep tripping the capacity trim under -race.
	advance := videoTaskCacheFixture(t, base, 1, 32)

	const workers, iterations = 8, 40
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				id := "v_race_" + strconv.FormatInt(int64(w), 10) + "_" + strconv.FormatInt(int64(i), 10)
				SaveProxyVideoTask(&ProxyVideoTask{PublicID: id, UpstreamVideoID: "up_" + id})
				_ = GetProxyVideoTaskByPublicID(id)
				if i%4 == 0 {
					DeleteProxyVideoTaskByPublicID(id)
				}
			}
		}(w)
	}
	wg.Wait()

	storedAt, _, _ := videoTaskCacheSnapshotForTest()
	if len(storedAt) > videoTaskCacheMaxEntries+videoTaskSweepEveryInserts {
		t.Fatalf("cache overshot its documented bound: %d lines", len(storedAt))
	}
	forceVideoTaskSweepForTest(t)
	if storedAt, _, _ := videoTaskCacheSnapshotForTest(); len(storedAt) > videoTaskCacheMaxEntries {
		t.Fatalf("sweep left the cache over the guardrail: %d lines", len(storedAt))
	}

	// Everything ages out once the clock passes the TTL.
	advance(2 * 24 * time.Hour)
	forceVideoTaskSweepForTest(t)
	if storedAt, _, _ := videoTaskCacheSnapshotForTest(); len(storedAt) != 0 {
		t.Fatalf("expected an empty cache after TTL, got %d lines", len(storedAt))
	}
}
