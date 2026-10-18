package proxyhandler

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"golang.org/x/sync/singleflight"
)

const (
	// modelsListCacheTTL bounds how long a policy-filtered model list may lag
	// route/runtime changes that do not alter the exposed catalog itself.
	// Catalog content changes produce a new cache key immediately.
	modelsListCacheTTL = 15 * time.Second
	// modelsListCacheMaxEntries caps process memory: one entry per
	// (policy fingerprint, exposed catalog) view. Distinct managed-key
	// policies are admin-controlled, so this only guards misconfiguration.
	modelsListCacheMaxEntries = 256
	// modelsListCacheKeyVersion invalidates entries when the fingerprint
	// encoding changes across releases.
	modelsListCacheKeyVersion = "models-list-v1"
)

type modelsListCacheEntry struct {
	models    []string
	expiresAt time.Time
}

// modelsListCache reuses the expensive per-policy visibility evaluation
// (IsModelAllowedByPolicy + one ExplainSelection call per catalog model) for
// callers whose routing policy and exposed catalog both fingerprint the same.
//
// Entries are process-local and never shared with unrestricted requests: the
// policy fingerprint includes AccessPolicy and AllowedRouteIDs, and only the
// restricted listing path uses this cache. Entries expire after a short TTL,
// and inserts lazily evict expired/oldest entries once the capacity cap is hit.
type modelsListCache struct {
	mu         sync.Mutex
	entries    map[string]modelsListCacheEntry
	flight     singleflight.Group
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

func newModelsListCache(ttl time.Duration, maxEntries int) *modelsListCache {
	if ttl <= 0 {
		ttl = modelsListCacheTTL
	}
	if maxEntries <= 0 {
		maxEntries = modelsListCacheMaxEntries
	}
	return &modelsListCache{
		entries:    make(map[string]modelsListCacheEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

var globalModelsListCache = newModelsListCache(modelsListCacheTTL, modelsListCacheMaxEntries)

// get returns a copy of the cached list so callers cannot mutate the stored
// snapshot. Expired entries are dropped on access.
func (c *modelsListCache) get(key string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return copyModelList(entry.models), true
}

func (c *modelsListCache) put(key string, models []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.evictLocked(now)
	c.entries[key] = modelsListCacheEntry{
		models:    copyModelList(models),
		expiresAt: now.Add(c.ttl),
	}
}

// evictLocked drops expired entries first, then the entries closest to expiry
// until there is room for one more. Lazy eviction keeps the steady-state map
// bounded without a background sweeper.
func (c *modelsListCache) evictLocked(now time.Time) {
	if len(c.entries) < c.maxEntries {
		return
	}
	for key, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
	for len(c.entries) >= c.maxEntries {
		oldestKey := ""
		var oldestAt time.Time
		first := true
		for key, entry := range c.entries {
			if first || entry.expiresAt.Before(oldestAt) {
				oldestKey, oldestAt, first = key, entry.expiresAt, false
			}
		}
		if first {
			return
		}
		delete(c.entries, oldestKey)
	}
}

// getOrCompute serves fresh cached lists and deduplicates concurrent misses for
// the same key through singleflight. Expiry is re-checked inside the flight so
// followers short-circuit when the leader already populated the entry.
func (c *modelsListCache) getOrCompute(key string, compute func() []string) []string {
	if models, ok := c.get(key); ok {
		return models
	}
	result, _, _ := c.flight.Do(key, func() (any, error) {
		if models, ok := c.get(key); ok {
			return models, nil
		}
		models := compute()
		c.put(key, models)
		return copyModelList(models), nil
	})
	return copyModelList(result.([]string))
}

func (c *modelsListCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]modelsListCacheEntry)
}

func (c *modelsListCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func copyModelList(models []string) []string {
	if len(models) == 0 {
		return []string{}
	}
	out := make([]string, len(models))
	copy(out, models)
	return out
}

// modelsListCacheKey fingerprints the full routing policy plus the exposed
// catalog. Any policy edit (supported models, route grants, access policy,
// site/credential filters, weights) or catalog change misses the old entry, so
// a restricted key can never read another policy's visible-model list.
func modelsListCacheKey(policy auth.DownstreamRoutingPolicy, catalog []string) (string, bool) {
	payload, err := json.Marshal(policy)
	if err != nil {
		return "", false
	}
	policySum := sha256.Sum256(payload)
	return modelsListCacheKeyVersion + "|" + hex.EncodeToString(policySum[:]) + "|" + modelsCatalogFingerprint(catalog), true
}

func modelsCatalogFingerprint(catalog []string) string {
	h := sha256.New()
	var length [8]byte
	for _, model := range catalog {
		binary.LittleEndian.PutUint64(length[:], uint64(len(model)))
		_, _ = h.Write(length[:])
		_, _ = h.Write([]byte(model))
	}
	return hex.EncodeToString(h.Sum(nil))
}
