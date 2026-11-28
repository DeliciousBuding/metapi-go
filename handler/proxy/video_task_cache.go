package proxyhandler

import (
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// The process-local video task cache has two owners with two lifetimes:
//
//   - videoTaskStore (below) is a rewrite cache: publicId -> upstream video id
//     plus the channel/account pin used for sticky routing. It is not durable —
//     a restart starts empty and re-warms from the table on the first cold GET.
//   - proxy_video_tasks (DB) is the durable mapping shared across instances and
//     restarts. Its rows are pruned by
//     scheduler.NewProxyVideoTaskRetentionScheduler.
//
// Both age out on the same operator-visible dial
// (PROXY_VIDEO_TASK_RETENTION_DAYS). The cache never deletes DB rows — pruning
// durable state stays the scheduler's job, under its lease — and eviction here
// is lazy: it rides along on inserts, so there is no background goroutine and
// no per-request full scan.
//
// videoTaskEntry is one cache line. storedAt is when the mapping entered (or was
// refreshed in) *this process's* cache, not the durable row's created_at. Reads
// do not refresh it, so a mapping ages out on a fixed schedule whether or not
// clients keep polling it.
type videoTaskEntry struct {
	db       *store.DB
	task     *ProxyVideoTask
	storedAt time.Time
}

const (
	// videoTaskSweepEveryInserts bounds how many inserts may pass without a
	// sweep (burst guard): the map can overshoot the capacity guardrail by at
	// most this many lines between sweeps.
	videoTaskSweepEveryInserts = 256
	// videoTaskSweepMinInterval bounds how long an entry may sit past its TTL
	// in a low-traffic process (idle guard): the first insert after the window
	// sweeps even when the insert counter is nowhere near its threshold.
	videoTaskSweepMinInterval = 5 * time.Minute
	// videoTaskCacheHeadroomDivisor sets the batch-trim target: over the cap,
	// the sweep evicts oldest-first down to cap-cap/16 rather than exactly cap,
	// so the O(n log n) trim runs once per ~cap/16 inserts instead of on every
	// insert at the cap.
	videoTaskCacheHeadroomDivisor = 16
)

var (
	videoTaskStore   = make(map[string]*videoTaskEntry)
	videoTaskStoreMu sync.RWMutex

	// videoTaskCacheMaxEntries is the hard capacity guardrail on cache lines
	// (~200 B each, so ~4 MB at 20k). It holds even when TTL eviction is
	// explicitly disabled, because bounding process memory is not an
	// operator-optional behaviour. Fixed in production; tests lower it.
	videoTaskCacheMaxEntries = 20000
	// videoTaskNow is the cache clock, injectable so eviction tests do not
	// sleep. Production never reassigns it.
	videoTaskNow = time.Now

	// Sweep bookkeeping; both guarded by videoTaskStoreMu.
	videoTaskInsertsSinceSweep int
	videoTaskLastSweepAt       time.Time
)

// SaveProxyVideoTask saves a video task mapping to the process-local cache and
// dual-writes to proxy_video_tasks when a runtime DB is available.
// Both copies carry the same timestamp and both are bounded by
// PROXY_VIDEO_TASK_RETENTION_DAYS (cache: lazy sweep; DB: retention scheduler).
func SaveProxyVideoTask(db *store.DB, task *ProxyVideoTask) {
	if task == nil || strings.TrimSpace(task.PublicID) == "" {
		return
	}
	// Store a copy so callers cannot mutate the map entry after return.
	cp := *task
	cp.PublicID = strings.TrimSpace(cp.PublicID)
	cp.UpstreamVideoID = strings.TrimSpace(cp.UpstreamVideoID)
	cp.Owner = strings.TrimSpace(cp.Owner)

	now := videoTaskNow().UTC()
	videoTaskStoreMu.Lock()
	putVideoTaskEntryLocked(db, cp.PublicID, &cp, now)
	videoTaskStoreMu.Unlock()

	if err := upsertProxyVideoTaskDB(db, &cp, now); err != nil {
		slog.Warn("proxy video task: durable upsert failed (memory cache still set)",
			"public_id", cp.PublicID, "error", err)
	}
}

// GetProxyVideoTaskByPublicID retrieves a video task by publicId.
// Memory cache first; cold miss falls back to proxy_video_tasks.
// A cache line past the retention TTL counts as a miss even before the next
// (throttled) sweep deletes it; the durable table then decides exactly as it
// would for a cold process, so a row the scheduler has not pruned yet re-warms
// the line with a fresh storedAt.
func GetProxyVideoTaskByPublicID(db *store.DB, publicID string) *ProxyVideoTask {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return nil
	}
	videoTaskStoreMu.RLock()
	entry := videoTaskStore[publicID]
	if entry != nil && entry.db == db && entry.task != nil && !videoTaskCacheEntryExpired(entry) {
		cp := *entry.task
		videoTaskStoreMu.RUnlock()
		return &cp
	}
	videoTaskStoreMu.RUnlock()

	loaded, err := loadProxyVideoTaskDB(db, publicID)
	if err != nil {
		slog.Debug("proxy video task: durable load failed", "public_id", publicID, "error", err)
		return nil
	}
	if loaded == nil {
		return nil
	}
	// Warm process-local cache (a warm load is an insert, so it stamps its own
	// storedAt and takes part in sweep bookkeeping like a create does).
	now := videoTaskNow().UTC()
	videoTaskStoreMu.Lock()
	putVideoTaskEntryLocked(db, loaded.PublicID, loaded, now)
	videoTaskStoreMu.Unlock()
	cp := *loaded
	return &cp
}

// DeleteProxyVideoTaskByPublicID deletes a video task by publicId (memory + DB).
func DeleteProxyVideoTaskByPublicID(db *store.DB, publicID string) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return
	}
	videoTaskStoreMu.Lock()
	if entry := videoTaskStore[publicID]; entry != nil && entry.db == db {
		delete(videoTaskStore, publicID)
	}
	videoTaskStoreMu.Unlock()

	if err := deleteProxyVideoTaskDB(db, publicID); err != nil {
		slog.Warn("proxy video task: durable delete failed", "public_id", publicID, "error", err)
	}
}

// putVideoTaskEntryLocked inserts or refreshes a cache line and runs the
// amortized eviction check. Callers must hold videoTaskStoreMu for writing.
func putVideoTaskEntryLocked(db *store.DB, publicID string, task *ProxyVideoTask, now time.Time) {
	videoTaskStore[publicID] = &videoTaskEntry{db: db, task: task, storedAt: now}
	videoTaskInsertsSinceSweep++
	maybeSweepVideoTaskCacheLocked(now)
}

// maybeSweepVideoTaskCacheLocked runs at most one sweep per
// videoTaskSweepEveryInserts inserts or per videoTaskSweepMinInterval, which is
// what keeps the insert path amortized O(1): the two thresholds bound (a) how
// far the map can overshoot the cap during a burst and (b) how long an entry
// can sit past its TTL in an idle-ish process. Callers must hold the write lock.
func maybeSweepVideoTaskCacheLocked(now time.Time) {
	due := videoTaskInsertsSinceSweep >= videoTaskSweepEveryInserts ||
		now.Sub(videoTaskLastSweepAt) >= videoTaskSweepMinInterval
	if !due {
		return
	}
	videoTaskInsertsSinceSweep = 0
	videoTaskLastSweepAt = now
	sweepVideoTaskCacheLocked(now)
}

// sweepVideoTaskCacheLocked is the single eviction pass: one O(n) walk for TTL
// expiry plus a capacity trim when the guardrail is exceeded. It never touches
// the DB — proxy_video_tasks rows are the retention scheduler's to prune.
// Callers must hold videoTaskStoreMu for writing.
func sweepVideoTaskCacheLocked(now time.Time) {
	var expired int
	if ttl := videoTaskCacheTTL(); ttl > 0 {
		cutoff := now.Add(-ttl)
		for id, entry := range videoTaskStore {
			if entry == nil || entry.storedAt.Before(cutoff) {
				delete(videoTaskStore, id)
				expired++
			}
		}
	}
	trimmed := trimVideoTaskCacheLocked()
	if expired+trimmed > 0 {
		slog.Debug("proxy video task cache: sweep",
			"expired", expired,
			"over_cap_trimmed", trimmed,
			"remaining", len(videoTaskStore),
		)
	}
}

// trimVideoTaskCacheLocked enforces the hard capacity guardrail, evicting
// oldest-first down to the headroom target so a cache pinned at the cap does
// not pay for a trim on every insert. Returns the number of lines dropped.
// Callers must hold videoTaskStoreMu for writing.
func trimVideoTaskCacheLocked() int {
	if len(videoTaskStore) <= videoTaskCacheMaxEntries {
		return 0
	}
	target := videoTaskCacheMaxEntries - videoTaskCacheMaxEntries/videoTaskCacheHeadroomDivisor
	drop := len(videoTaskStore) - target
	if drop < 1 || drop > len(videoTaskStore) {
		// Only reachable for a nonsensical (<= 0) cap, which means "hold nothing".
		drop = len(videoTaskStore)
	}
	oldest := make([]videoTaskEntryRef, 0, len(videoTaskStore))
	for id, entry := range videoTaskStore {
		ref := videoTaskEntryRef{id: id}
		if entry != nil {
			ref.storedAt = entry.storedAt
		}
		oldest = append(oldest, ref)
	}
	slices.SortFunc(oldest, func(a, b videoTaskEntryRef) int { return a.storedAt.Compare(b.storedAt) })
	for _, ref := range oldest[:drop] {
		delete(videoTaskStore, ref.id)
	}
	return drop
}

// videoTaskEntryRef is a (id, storedAt) pair used to order lines for the
// capacity trim without holding pointers into the map while deleting from it.
type videoTaskEntryRef struct {
	id       string
	storedAt time.Time
}

// videoTaskCacheTTL reports how long a cache line may live, derived from the
// same PROXY_VIDEO_TASK_RETENTION_DAYS knob that prunes proxy_video_tasks.
// 0 means TTL eviction is off — either config is not loaded yet or an operator
// explicitly disabled retention — and only the capacity guardrail applies.
func videoTaskCacheTTL() time.Duration {
	cfg := config.GetSafe()
	if cfg == nil || cfg.ProxyVideoTaskRetentionDays <= 0 {
		return 0
	}
	return time.Duration(cfg.ProxyVideoTaskRetentionDays) * 24 * time.Hour
}

// videoTaskCacheEntryExpired reports whether a line is past its TTL. Reads use
// it to treat an expired line as a miss; deletion still belongs to the sweep,
// so the read path never needs the write lock.
func videoTaskCacheEntryExpired(entry *videoTaskEntry) bool {
	ttl := videoTaskCacheTTL()
	if ttl <= 0 || entry == nil {
		return false
	}
	return videoTaskNow().UTC().Sub(entry.storedAt) >= ttl
}
