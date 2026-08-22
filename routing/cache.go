package routing

import (
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// globalCache is the package-level cache reference, set during router initialization.
// It allows admin handlers to invalidate the cache without holding a *TokenRouter reference.
var (
	globalCache   *RouteCache
	globalCacheMu sync.RWMutex
)

// SetGlobalCache sets the global route cache. Called once during router initialization.
func SetGlobalCache(c *RouteCache) {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	globalCache = c
}

// InvalidateCache invalidates the global route cache. Safe to call even if the cache
// has not been initialized (no-op).
func InvalidateCache() {
	globalCacheMu.RLock()
	c := globalCache
	globalCacheMu.RUnlock()
	if c != nil {
		c.InvalidateAll()
	} else {
		slog.Debug("routing.InvalidateCache: global cache not yet initialized, skipping")
	}
}

// RouteCache caches the routes list and per-route matches with TTL. The
// selector serves both from cache on the hot path and falls back to the DB on
// miss/expiry. Cached matches are immutable snapshots: patching clones (see
// PatchCachedChannel), so lock-free readers never race the patcher.
type RouteCache struct {
	mu           sync.RWMutex
	routesLoaded bool
	routesAt     int64
	routes       []store.TokenRoute
	matchCache   map[int64]*routeMatchEntry
	ttlMs        int64
}

type routeMatchEntry struct {
	loadedAt int64
	match    *RouteMatch
}

// NewRouteCache creates a new route cache with the given TTL in milliseconds.
func NewRouteCache(ttlMs int64) *RouteCache {
	if ttlMs < 100 {
		ttlMs = 100
	}
	return &RouteCache{
		matchCache: make(map[int64]*routeMatchEntry),
		ttlMs:      ttlMs,
	}
}

// IsRoutesFresh checks if the routes list is still fresh.
func (c *RouteCache) IsRoutesFresh() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.routesLoaded && (time.Now().UnixMilli()-c.routesAt < c.ttlMs)
}

// GetRoutes returns cached routes if fresh, nil otherwise.
func (c *RouteCache) GetRoutes() []store.TokenRoute {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.routesLoaded && (time.Now().UnixMilli()-c.routesAt < c.ttlMs) {
		return c.routes
	}
	return nil
}

// SetRoutes sets the cached routes.
func (c *RouteCache) SetRoutes(routes []store.TokenRoute) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.routes = routes
	c.routesAt = time.Now().UnixMilli()
	c.routesLoaded = true
}

// GetMatch returns a cached route match if fresh.
func (c *RouteCache) GetMatch(routeID int64) *RouteMatch {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.matchCache[routeID]
	if !ok {
		return nil
	}
	if time.Now().UnixMilli()-entry.loadedAt >= c.ttlMs {
		return nil
	}
	return entry.match
}

// SetMatch caches a route match.
func (c *RouteCache) SetMatch(routeID int64, match *RouteMatch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.matchCache[routeID] = &routeMatchEntry{
		loadedAt: time.Now().UnixMilli(),
		match:    match,
	}
}

// PatchCachedChannel applies a mutation to a channel across all cached
// matches. Cached matches are treated as immutable snapshots: the mutation is
// applied to a clone that atomically replaces the cached entry, so goroutines
// holding a previously returned snapshot (GetMatch readers use the match
// outside the cache lock) keep reading consistent data instead of racing the
// patcher.
func (c *RouteCache) PatchCachedChannel(channelID int64, apply func(ch *store.RouteChannel)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for routeID, entry := range c.matchCache {
		if entry.match == nil {
			continue
		}
		idx := -1
		for i := range entry.match.Channels {
			if entry.match.Channels[i].Channel.ID == channelID {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		cloned := &RouteMatch{Route: entry.match.Route}
		cloned.Channels = make([]RouteChannelCandidate, len(entry.match.Channels))
		copy(cloned.Channels, entry.match.Channels)
		apply(&cloned.Channels[idx].Channel)
		c.matchCache[routeID] = &routeMatchEntry{loadedAt: entry.loadedAt, match: cloned}
	}
}

// InvalidateRouteScopedCache clears the cache for a specific route.
func (c *RouteCache) InvalidateRouteScopedCache(routeID int64) {
	if routeID <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.matchCache, routeID)
	ClearStableFirstCachesForRoute(routeID)
}

// InvalidateAll clears all caches.
func (c *RouteCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.routesLoaded = false
	c.routes = nil
	c.routesAt = 0
	c.matchCache = make(map[int64]*routeMatchEntry)
	// Clear stable first global state
	clearAllStableFirstCaches()
}

func clearAllStableFirstCaches() {
	stableFirstStateMu.Lock()
	defer stableFirstStateMu.Unlock()
	for k := range stableFirstLastSelectedSiteByKey {
		delete(stableFirstLastSelectedSiteByKey, k)
	}
	for k := range stableFirstObservationProgressByKey {
		delete(stableFirstObservationProgressByKey, k)
	}
	for k := range stableFirstObservationSiteCooldownByKey {
		delete(stableFirstObservationSiteCooldownByKey, k)
	}
}
