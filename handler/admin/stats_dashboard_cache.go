package admin

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// dashboardSummaryCache is a short-TTL in-memory cache for the dashboard
// summary endpoint. Dashboard aggregation queries (COUNT/SUM over 24h of
// proxy_logs) are expensive enough to dedup rapid reloads but cheap enough
// that a 10s window keeps near-real-time semantics. Mirrors the
// accountsSnapshotCache pattern: RWMutex-guarded bytes + expiry timestamp.
//
// Keyed by view (summary|insights|full) so the three response shapes never
// collide. Only successful responses are cached; error paths bypass the cache.
// ?force=1 clears the cache (admin "force refresh").
type dashboardSummaryCache struct {
	mu      sync.RWMutex
	entries map[string]dashboardCacheEntry
	ttl     time.Duration
	// flight deduplicates concurrent cache-miss computes for the same view so
	// N admin sessions polling an expired entry share one expensive
	// aggregation run instead of running it N× (thundering herd).
	flight singleflight.Group
}

type dashboardCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

func (c *dashboardSummaryCache) get(view string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[view]
	if !ok || len(e.data) == 0 || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.data, true
}

func (c *dashboardSummaryCache) set(view string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]dashboardCacheEntry)
	}
	// Copy the slice so later buffer reuse by the caller cannot mutate the
	// cached payload (defensive against json.Marshal aliasing).
	stored := make([]byte, len(data))
	copy(stored, data)
	c.entries[view] = dashboardCacheEntry{data: stored, expiresAt: time.Now().Add(c.ttl)}
}

func (c *dashboardSummaryCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

// getOrCompute returns the cached payload for view, or computes it via the
// supplied function under single-flight dedup so N concurrent admin sessions
// hitting a cold/expired cache share one expensive aggregation run instead of
// running it N× (thundering herd). A leader that populates the cache mid-flight
// lets concurrent followers short-circuit on the re-check inside Do.
//
// Returns (data, hit, err): hit reports whether the fast-path cache served the
// bytes (for the x-dashboard-summary-cache response header). A single-flight-
// shared compute is reported as a miss since this request did not get an
// instant cached response — only successful computes are stored, so errors
// never poison the cache.
func (c *dashboardSummaryCache) getOrCompute(view string, compute func() ([]byte, error)) ([]byte, bool, error) {
	if cached, hit := c.get(view); hit {
		return cached, true, nil
	}
	result, err, _ := c.flight.Do(view, func() (any, error) {
		// Re-check: a concurrent leader may have populated the cache while we
		// waited for the single-flight slot.
		if cached, hit := c.get(view); hit {
			return cached, nil
		}
		data, err := compute()
		if err != nil {
			return nil, err
		}
		c.set(view, data)
		return data, nil
	})
	if err != nil {
		return nil, false, err
	}
	return result.([]byte), false, nil
}

var globalDashboardCache = &dashboardSummaryCache{ttl: 10 * time.Second}
