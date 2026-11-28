package admin

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// proxyLogsSummary is the five-way aggregate behind the proxy-logs summary
// strip (total/success/failed counts + cost + effective tokens). It is the
// most expensive query on the page — five SUM/CASE passes over the filtered
// proxy_logs table (~0.6-1.0s at 500k rows on the audit fixture) — so it is
// cached per filter fingerprint below.
type proxyLogsSummary struct {
	TotalCount     int     `db:"total_count"`
	SuccessCount   int     `db:"success_count"`
	FailedCount    int     `db:"failed_count"`
	TotalCost      float64 `db:"total_cost"`
	TotalTokensAll int64   `db:"total_tokens_all"`
}

// proxyLogsSummaryCacheMaxEntries bounds the fingerprint map. Entries expire
// after the TTL anyway; the cap only protects against a burst of distinct
// filter combinations inside one TTL window. On overflow the expired entries
// are pruned first, then the whole map is dropped (a 10s-TTL cache rebuilds
// quickly; bounded memory beats LRU bookkeeping here).
const proxyLogsSummaryCacheMaxEntries = 128

// proxyLogsSummaryCache is a short-TTL in-memory cache for the proxy-logs
// summary aggregate, keyed by the request's filter fingerprint (every query
// param that shapes the WHERE clause; limit/offset excluded). Mirrors the
// dashboardSummaryCache pattern: RWMutex-guarded entries + single-flight
// dedup so N admin sessions polling an expired fingerprint share one
// aggregation run instead of running it N× (thundering herd).
//
// Keying by fingerprint (not by view) is the point: pagination only changes
// offset, and the list page's split view=query + view=meta fetches carry the
// same filters, so one TTL window computes the summary once per filter set —
// the pre-fix page computed it twice per load (full + meta) plus once per
// page turn. Only successful computes are cached; errors bypass the cache.
// ?force=1 / ?refresh=1 clears it (admin "force refresh").
type proxyLogsSummaryCache struct {
	mu      sync.RWMutex
	entries map[string]proxyLogsSummaryEntry
	ttl     time.Duration
	// flight deduplicates concurrent cache-miss computes for the same
	// fingerprint (see dashboardSummaryCache.flight).
	flight singleflight.Group
}

type proxyLogsSummaryEntry struct {
	summary   proxyLogsSummary
	expiresAt time.Time
}

func (c *proxyLogsSummaryCache) get(key string) (proxyLogsSummary, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return proxyLogsSummary{}, false
	}
	return e.summary, true
}

func (c *proxyLogsSummaryCache) set(key string, summary proxyLogsSummary) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]proxyLogsSummaryEntry)
	}
	if len(c.entries) >= proxyLogsSummaryCacheMaxEntries {
		now := time.Now()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= proxyLogsSummaryCacheMaxEntries {
			// All entries still live: drop the lot rather than grow unbounded.
			c.entries = make(map[string]proxyLogsSummaryEntry)
		}
	}
	c.entries[key] = proxyLogsSummaryEntry{summary: summary, expiresAt: time.Now().Add(c.ttl)}
}

func (c *proxyLogsSummaryCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

// getOrCompute returns the cached summary for key, or computes it via the
// supplied function under single-flight dedup. Returns (summary, hit, err):
// hit reports whether the fast-path cache served the value (for the
// x-proxy-logs-summary-cache response header). A single-flight-shared
// compute is reported as a miss since this request did not get an instant
// cached response — only successful computes are stored, so errors never
// poison the cache.
func (c *proxyLogsSummaryCache) getOrCompute(key string, compute func() (proxyLogsSummary, error)) (proxyLogsSummary, bool, error) {
	if cached, hit := c.get(key); hit {
		return cached, true, nil
	}
	result, err, _ := c.flight.Do(key, func() (any, error) {
		// Re-check: a concurrent leader may have populated the cache while we
		// waited for the single-flight slot.
		if cached, hit := c.get(key); hit {
			return cached, nil
		}
		summary, err := compute()
		if err != nil {
			return proxyLogsSummary{}, err
		}
		c.set(key, summary)
		return summary, nil
	})
	if err != nil {
		return proxyLogsSummary{}, false, err
	}
	return result.(proxyLogsSummary), false, nil
}

var globalProxyLogsSummaryCache = &proxyLogsSummaryCache{ttl: 10 * time.Second}
