package scheduler

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// projectionLease holds the lease info for multi-instance coordination.
type projectionLease struct {
	Owner     string
	Token     string
	ExpiresAt string
}

// projectionCheckpoint mirrors the analytics_projection_checkpoints row.
type projectionCheckpoint struct {
	ProjectorKey         string
	TimeZone             string
	LastProxyLogID       int64
	RecomputeFromID      *int64
	RecomputeRequestedAt *string
}

func (s *UsageAggregationScheduler) buildLeaseOwner() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
}

func (s *UsageAggregationScheduler) generateLeaseToken() string {
	b := make([]byte, 16)
	// Go 1.24+ crypto/rand.Read never returns an error.
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func (s *UsageAggregationScheduler) tryAcquireLease(dbw *store.DB) (*projectionLease, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	expiresAt := time.Now().Add(time.Duration(usageProjectionLeaseMs) * time.Millisecond).UTC().Format(time.RFC3339)
	lease := &projectionLease{
		Owner:     s.buildLeaseOwner(),
		Token:     s.generateLeaseToken(),
		ExpiresAt: expiresAt,
	}

	// Ensure checkpoint row exists
	var ensureQuery string
	switch dbw.Dialect {
	case store.DialectPostgres:
		ensureQuery = `
			INSERT INTO analytics_projection_checkpoints
				(projector_key, time_zone, last_proxy_log_id, created_at, updated_at)
			VALUES (?, 'UTC', 0, ?, ?)
			ON CONFLICT (projector_key) DO NOTHING`
	default: // sqlite
		ensureQuery = `
			INSERT OR IGNORE INTO analytics_projection_checkpoints
				(projector_key, time_zone, last_proxy_log_id, created_at, updated_at)
			VALUES (?, 'UTC', 0, ?, ?)`
	}
	_, err := dbw.Exec(ensureQuery, usageProjectorKey, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure checkpoint: %w", err)
	}

	// Conditional update: acquire lease only if expired or null
	result, err := dbw.Exec(`
		UPDATE analytics_projection_checkpoints
		SET lease_owner = ?, lease_token = ?, lease_expires_at = ?, updated_at = ?
		WHERE projector_key = ?
		  AND (lease_expires_at IS NULL OR lease_expires_at <= ?)
	`, lease.Owner, lease.Token, lease.ExpiresAt, now, usageProjectorKey, now)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lease: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, nil // Another instance holds the lease
	}

	return lease, nil
}

func (s *UsageAggregationScheduler) releaseLease(dbw *store.DB, lease *projectionLease, err error) {
	now := time.Now().UTC().Format(time.RFC3339)
	var lastError *string
	if err != nil {
		msg := err.Error()
		lastError = &msg
	}
	if _, execErr := dbw.Exec(`
		UPDATE analytics_projection_checkpoints
		SET lease_owner = NULL, lease_token = NULL, lease_expires_at = NULL,
		    last_error = ?, updated_at = ?
		WHERE projector_key = ? AND lease_token = ?
	`, lastError, now, usageProjectorKey, lease.Token); execErr != nil {
		slog.Warn("usage-aggregation: failed to release lease", "error", execErr, "lease_token", lease.Token)
	}
}

func (s *UsageAggregationScheduler) readCheckpoint(dbw *store.DB) projectionCheckpoint {
	var cp projectionCheckpoint
	row := dbw.QueryRow(`
		SELECT projector_key, COALESCE(time_zone, 'UTC'), last_proxy_log_id,
		       recompute_from_id, recompute_requested_at
		FROM analytics_projection_checkpoints
		WHERE projector_key = ?
	`, usageProjectorKey)

	var tz, key string
	var recomputeFromID *int64
	var recomputeRequestedAt *string
	if err := row.Scan(&key, &tz, &cp.LastProxyLogID, &recomputeFromID, &recomputeRequestedAt); err != nil {
		return projectionCheckpoint{
			ProjectorKey:   usageProjectorKey,
			TimeZone:       "UTC",
			LastProxyLogID: 0,
		}
	}
	cp.ProjectorKey = key
	cp.TimeZone = tz
	cp.RecomputeFromID = recomputeFromID
	cp.RecomputeRequestedAt = recomputeRequestedAt
	return cp
}
