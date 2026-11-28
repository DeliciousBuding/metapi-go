package admin

import (
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	"golang.org/x/sync/singleflight"
)

// channelsSnapshotMaxEntries bounds how many distinct snapshot keys the cache
// keeps alive at once. See channelsSnapshotCache for the key space, the
// eviction rule and the memory arithmetic behind this number.
const channelsSnapshotMaxEntries = 16

func init() {
	service.RegisterSiteProxyCacheInvalidator(invalidateChannelsSnapshotCache)
}

// channelsSnapshotEntry is one cached page payload plus its deadline and its
// position in the insertion order (seq) that drives FIFO eviction.
type channelsSnapshotEntry struct {
	data      []byte
	expiresAt time.Time
	// seq is the value of channelsSnapshotCache.seq at insertion time: a
	// monotonic counter, so "oldest entry" is well defined without depending
	// on Go's deliberately randomized map iteration order.
	seq uint64
}

// channelsSnapshotCache is an in-memory TTL cache for GET /api/channels pages.
// Mirrors globalAccountsCache: a short-TTL snapshot so a fleet-wide 5-way JOIN
// list does not run on every dashboard poll. ?refresh=true bypasses it.
//
// Key space (built in listChannels): the dashboard's unbounded view is
// "unbounded"[:status,…] and every server-paged view is
// "page:pageSize:status,…", so the cache must hold several keys at once —
// a dashboard poll and an operator flipping pages or toggling a status filter
// happen concurrently. The previous single-slot implementation keyed by the
// same three-dimensional string but only ever remembered ONE key, so page=1
// and page=2 evicted each other and nearly every request missed: the 10s TTL
// never absorbed the polling it exists for, leaving only mutex + singleflight
// overhead in front of the JOIN.
//
// This is therefore a bounded map of at most channelsSnapshotMaxEntries (16)
// entries. 16 covers the hot subset of the key space with headroom — one
// unbounded key per status filter actually in use, plus the handful of
// page/pageSize combinations an operator browses — while keeping the footprint
// computable: pageSize is clamped to 200 and a channel item serializes to
// ~340 B of JSON, so a max-size page snapshot is ~66 KB and the paged slots
// cost at most 15 × 66 KB ≈ 1 MB. Unbounded entries carry no LIMIT and scale
// with fleet size (~340 KB per 1,000 channels, ~1.7 MB at 5,000) — a payload
// the single-slot cache already held, so the bound only multiplies the *paged*
// part. Worst case is ≲16 snapshots ≈ a few MB, never unbounded growth.
//
// Eviction is FIFO on insertion order: when a new key arrives and the map is
// full, the entry with the lowest seq (oldest insertion) is dropped. FIFO —
// not LRU — because it is deterministic and testable without touching
// read-path bookkeeping: a read hit must not reorder the cache, and a bounded
// snapshot cache gains little from recency tracking when every entry expires
// after 10s anyway. Re-setting an existing key overwrites it in place and
// stamps a fresh seq (it becomes the newest).
//
// clear() still empties the whole map: it is the "the data changed" invalidation
// hook (invalidateChannelsSnapshotCache), not an eviction path.
type channelsSnapshotCache struct {
	mu      sync.RWMutex
	entries map[string]channelsSnapshotEntry
	ttl     time.Duration
	// seq counts insertions monotonically; see channelsSnapshotEntry.seq.
	seq uint64
	// flight deduplicates concurrent cache-miss computes for the same page so
	// N admin sessions polling an expired entry share one 5-way JOIN run
	// instead of running it N× (thundering herd).
	flight singleflight.Group
}

func (c *channelsSnapshotCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[key]
	if !ok || entry.data == nil || !time.Now().Before(entry.expiresAt) {
		// Expired entries are left in place rather than deleted here (RLock):
		// they read as a miss and are overwritten by the next set, or evicted
		// once the map fills up. Bounded either way.
		return nil, false
	}
	return entry.data, true
}

func (c *channelsSnapshotCache) set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]channelsSnapshotEntry, channelsSnapshotMaxEntries)
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= channelsSnapshotMaxEntries {
		c.evictOldestLocked()
	}
	c.seq++
	c.entries[key] = channelsSnapshotEntry{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
		seq:       c.seq,
	}
}

// evictOldestLocked drops the entry with the lowest insertion seq (FIFO). The
// caller holds c.mu for writing. Scanning ≤16 entries is cheaper than keeping
// a second ordered structure, and picking the minimum of a unique monotonic
// counter is order-independent, so the randomized map iteration cannot change
// which entry goes. A seq tie (uint64 wrap, ~584 million years at 1 insert/ns)
// is broken by key so the choice stays deterministic.
func (c *channelsSnapshotCache) evictOldestLocked() {
	var (
		oldestKey string
		oldestSeq uint64
		found     bool
	)
	for key, entry := range c.entries {
		if !found || entry.seq < oldestSeq || (entry.seq == oldestSeq && key < oldestKey) {
			oldestKey, oldestSeq, found = key, entry.seq, true
		}
	}
	if found {
		delete(c.entries, oldestKey)
	}
}

func (c *channelsSnapshotCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

// getOrCompute returns the cached payload for key, or computes it via the
// supplied function under single-flight dedup so N concurrent admin sessions
// hitting a cold/expired entry share one 5-way JOIN run instead of running it
// N× (thundering herd). Returns (data, hit, err): hit reports whether the
// fast-path cache served the bytes (for the x-channels-snapshot-cache response
// header). Only successful computes are stored, so errors never poison the cache.
func (c *channelsSnapshotCache) getOrCompute(key string, compute func() ([]byte, error)) ([]byte, bool, error) {
	if cached, hit := c.get(key); hit {
		return cached, true, nil
	}
	result, err, _ := c.flight.Do(key, func() (any, error) {
		// Re-check: a concurrent leader may have populated the cache while we
		// waited for the single-flight slot.
		if cached, hit := c.get(key); hit {
			return cached, nil
		}
		data, err := compute()
		if err != nil {
			return nil, err
		}
		c.set(key, data)
		return data, nil
	})
	if err != nil {
		return nil, false, err
	}
	return result.([]byte), false, nil
}

var globalChannelsCache = &channelsSnapshotCache{ttl: 10 * time.Second}
var globalChannelsErrorSummaryCache = &channelsSnapshotCache{ttl: 10 * time.Second}

// invalidateChannelsSnapshotCache clears the channels list snapshot cache and
// the error-summary cache. Called from every route_channels mutation so a
// freshly edited fleet shows up immediately instead of up to ttl (10s) later.
func invalidateChannelsSnapshotCache() {
	globalChannelsCache.clear()
	globalChannelsErrorSummaryCache.clear()
}
