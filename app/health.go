package app

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

var readinessDraining atomic.Bool

// readinessPingTTL bounds how often the unauthenticated /ready endpoint may
// reach the database. SQLite runs with MaxOpenConns=1, so an uncached ping can
// be used to contend for the only connection; PostgreSQL can be drained the
// same way (audit L15). Failure results are cached too: readiness stays 503
// until the TTL expires and a fresh probe succeeds.
const readinessPingTTL = time.Second

// readinessProbeCache memoises a dependency probe and merges concurrent
// callers behind one execution. The database pointer is part of the key so a
// database switch (tests, runtime reload) never reuses another handle's result.
type readinessProbeCache struct {
	mu    sync.Mutex
	key   any
	at    time.Time
	err   error
	valid bool
}

var readinessDatabaseProbe readinessProbeCache

// probe returns the cached result when it is younger than the TTL. Otherwise
// it runs fn while holding the lock, so concurrent callers wait for the same
// probe instead of each taking a connection.
func (c *readinessProbeCache) probe(key any, ttl time.Duration, fn func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.valid && c.key == key && now.Sub(c.at) < ttl {
		return c.err
	}
	c.err = fn()
	c.key = key
	c.at = now
	c.valid = true
	return c.err
}

type HealthStatus struct {
	Status   string `json:"status"`
	Database string `json:"database,omitempty"`
}

// Health handles GET /health as a liveness endpoint.
// It intentionally returns 200 without touching dependencies: orchestrators
// should use /ready when they need dependency-aware routing decisions.
func Health(w http.ResponseWriter, r *http.Request) {
	writeHealthJSON(w, http.StatusOK, HealthStatus{Status: "ok"})
}

// Ready handles GET /ready as a readiness endpoint. It returns 503 when the
// database is not initialized or cannot be pinged.
func Ready(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	db := store.GetDB()
	if db == nil {
		slog.Warn("readiness: database not initialized")
		writeHealthJSON(w, http.StatusServiceUnavailable, HealthStatus{Status: "degraded", Database: "error"})
		return
	}

	if err := readinessDatabaseProbe.probe(db, readinessPingTTL, db.Ping); err != nil {
		slog.Warn("readiness: database ping failed", "error", err)
		writeHealthJSON(w, http.StatusServiceUnavailable, HealthStatus{Status: "degraded", Database: "error"})
		return
	}

	if readinessDraining.Load() {
		writeHealthJSON(w, http.StatusServiceUnavailable, HealthStatus{Status: "draining", Database: "ok"})
		return
	}

	writeHealthJSON(w, http.StatusOK, HealthStatus{Status: "ok", Database: "ok"})
}

func markReadinessDraining(draining bool) {
	readinessDraining.Store(draining)
}

func writeHealthJSON(w http.ResponseWriter, statusCode int, status HealthStatus) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(status); err != nil {
		slog.Warn("health: failed to write response", "error", err)
	}
}
