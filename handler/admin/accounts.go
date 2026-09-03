package admin

import (
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/singleflight"
)

// RegisterAccountsRoutes registers all /api/accounts routes.
func RegisterAccountsRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	handler := &accountsHandler{db: db, cfg: cfg}

	r.Get("/api/accounts", handler.listAccounts)
	r.Post("/api/accounts", handler.createAccount)
	r.Post("/api/accounts/login", handler.loginAccount)
	r.Post("/api/accounts/verify-token", handler.verifyToken)
	r.Post("/api/accounts/{id}/rebind-session", handler.rebindSession)
	r.Put("/api/accounts/{id}", handler.updateAccount)
	r.Delete("/api/accounts/{id}", handler.deleteAccount)
	r.Post("/api/accounts/batch", handler.batchAccounts)
	r.Post("/api/accounts/health/refresh", handler.healthRefresh)
	r.Get("/api/accounts/probe-history", handler.accountProbeHistory)
	r.Post("/api/accounts/{id}/balance", handler.refreshBalance)
	r.Get("/api/accounts/{id}/models", handler.getAccountModels)
	r.Post("/api/accounts/{id}/models/manual", handler.manualModels)
}

type accountsHandler struct {
	db  *sqlx.DB
	cfg *config.Config
}

// accountsSnapshotCache is an in-memory TTL cache for GET /api/accounts responses.
// Mirrors TS getAccountsSnapshot() behavior: cached response, ?refresh=true bypasses.
type accountsSnapshotCache struct {
	mu        sync.RWMutex
	data      []byte
	expiresAt time.Time
	ttl       time.Duration
	// flight deduplicates concurrent cache-miss computes so N admin sessions
	// polling an expired snapshot share one ListAccountsWithSites + per-account
	// metrics run instead of running it N× (thundering herd).
	flight singleflight.Group
}

func (c *accountsSnapshotCache) get() ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data != nil && time.Now().Before(c.expiresAt) {
		return c.data, true
	}
	return nil, false
}

func (c *accountsSnapshotCache) set(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = data
	c.expiresAt = time.Now().Add(c.ttl)
}

func (c *accountsSnapshotCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = nil
	c.expiresAt = time.Time{}
}

// getOrCompute returns the cached snapshot, or computes it via the supplied
// function under single-flight dedup so N concurrent admin sessions hitting a
// cold/expired cache share one compute instead of running it N×. Returns
// (data, hit, err): hit reports whether the fast-path cache served the bytes
// (for the x-accounts-snapshot-cache response header). Only successful
// computes are stored, so errors never poison the cache.
func (c *accountsSnapshotCache) getOrCompute(compute func() ([]byte, error)) ([]byte, bool, error) {
	if cached, hit := c.get(); hit {
		return cached, true, nil
	}
	result, err, _ := c.flight.Do("snapshot", func() (any, error) {
		// Re-check: a concurrent leader may have populated the cache while we
		// waited for the single-flight slot.
		if cached, hit := c.get(); hit {
			return cached, nil
		}
		data, err := compute()
		if err != nil {
			return nil, err
		}
		c.set(data)
		return data, nil
	})
	if err != nil {
		return nil, false, err
	}
	return result.([]byte), false, nil
}

var globalAccountsCache = &accountsSnapshotCache{ttl: 30 * time.Second}

func init() {
	// Site mutations invalidate process-local admin list cache via service hook.
	service.RegisterSiteProxyCacheInvalidator(func() {
		globalAccountsCache.clear()
	})
}
