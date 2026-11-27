package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

const (
	// fallback for callers that build config.Config by hand (tests, embedders)
	// and leave UsageProjectionIntervalMs zero; config.Load always fills it in.
	defaultUsageProjectionIntervalMs = 5_000
	usageProjectionLeaseMs           = 600_000 // 10min lease
	usageProjectionBatchSize         = 1_000   // rows per batch
	usageProjectionMaxBatches        = 120     // max batches per pass
	usageProjectorKey                = "usage-aggregates-v1"
)

// UsageAggregationScheduler incrementally projects proxy_logs into
// site_day_usage, site_hour_usage, and model_day_usage tables.
// Uses a database lease for multi-instance safety.
type UsageAggregationScheduler struct {
	db                 *store.DB
	cfg                *config.Config
	mu                 sync.Mutex
	projectionInFlight bool
	runner             *intervalRunner
}

// NewUsageAggregationScheduler creates a new usage aggregation scheduler.
func NewUsageAggregationScheduler(db *store.DB, cfg *config.Config) *UsageAggregationScheduler {
	return &UsageAggregationScheduler{db: db, cfg: cfg, runner: &intervalRunner{}}
}

func (s *UsageAggregationScheduler) Name() string { return "usage-aggregation" }

func (s *UsageAggregationScheduler) Start(ctx context.Context) error {
	intervalMs := s.projectionIntervalMs()
	slog.Info("usage-aggregation scheduler started",
		"interval_ms", intervalMs,
		"lease_ms", usageProjectionLeaseMs,
	)
	return s.runner.start(ctx, time.Duration(intervalMs)*time.Millisecond, true, func() {
		s.runPass(ctx)
	})
}

// projectionIntervalMs resolves the pass cadence from USAGE_PROJECTION_INTERVAL_MS,
// falling back to the package default when the field is unset.
func (s *UsageAggregationScheduler) projectionIntervalMs() int {
	if s.cfg != nil && s.cfg.UsageProjectionIntervalMs > 0 {
		return s.cfg.UsageProjectionIntervalMs
	}
	return defaultUsageProjectionIntervalMs
}

func (s *UsageAggregationScheduler) Stop() error {
	return s.runner.stop()
}

// ProjectionPassResult is the result of a single projection pass.

// OrphanLogs counts proxy_logs without a site join (missing account/site).
// They still advance the watermark so projection does not stall, but they
// do not contribute to site/model aggregates (known limitation).
type ProjectionPassResult struct {
	ProcessedLogs int
	OrphanLogs    int
	WatermarkID   int64
	Recomputed    bool
}

// RunProjectionPass executes a single projection pass.
// Safe to call externally (e.g., from admin snapshot). De-duplication
// happens under the mutex inside runPass: reading projectionInFlight here
// without the lock raced the ticker path's writes (observed under -race).
// Returns nil when a pass is already in flight, same as before.
func (s *UsageAggregationScheduler) RunProjectionPass() *ProjectionPassResult {
	return s.runPass(context.Background())
}

func (s *UsageAggregationScheduler) runPass(ctx context.Context) *ProjectionPassResult {
	s.mu.Lock()
	if s.projectionInFlight {
		s.mu.Unlock()
		return nil
	}
	s.projectionInFlight = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.projectionInFlight = false
		s.mu.Unlock()
	}()

	dbw := s.db
	if dbw == nil {
		return nil
	}

	// Try to acquire lease
	lease, err := s.tryAcquireLease(dbw)
	if err != nil || lease == nil {
		if err != nil {
			slog.Error("usage-aggregation: failed to acquire lease", "error", err)
		}
		// Read checkpoint to return watermark
		cp := s.readCheckpoint(dbw)
		return &ProjectionPassResult{ProcessedLogs: 0, WatermarkID: cp.LastProxyLogID}
	}

	defer func() {
		if r := recover(); r != nil {
			slog.Error("usage aggregation pass panicked", "panic", r)
			s.releaseLease(dbw, lease, fmt.Errorf("panic: %v", r))
			// Do NOT re-panic — this runs inside a goroutine launched by runPass().
			// Re-panicking would crash the entire server.
		}
	}()

	var result *ProjectionPassResult
	var passErr error

	func() {
		defer func() {
			if passErr != nil {
				s.releaseLease(dbw, lease, passErr)
			} else {
				s.releaseLease(dbw, lease, nil)
			}
		}()

		cp := s.readCheckpoint(dbw)
		watermark := cp.LastProxyLogID

		// Recompute phase
		if cp.RecomputeFromID != nil && *cp.RecomputeFromID > 0 {
			cp, passErr = s.applyRecompute(ctx, dbw, cp)
			if passErr != nil {
				return
			}
			watermark = cp.LastProxyLogID
		}

		// Normal projection phase
		processedLogs := 0
		orphanLogs := 0
		for batch := 0; batch < usageProjectionMaxBatches; batch++ {
			rows, err := s.fetchBatch(dbw, watermark, usageProjectionBatchSize)
			if err != nil {
				passErr = err
				return
			}
			if len(rows) == 0 {
				break
			}

			batchOrphans, err := s.applyBatch(dbw, cp, rows)
			if err != nil {
				slog.Error("usage-aggregation: batch flush failed; increments preserved and watermark not advanced, retrying on next pass",
					"error", err,
					"batch", batch,
					"watermark_id", watermark,
					"batch_rows", len(rows),
					"batch_first_id", rows[0].id,
					"batch_last_id", rows[len(rows)-1].id,
				)
				passErr = err
				return
			}
			processedLogs += len(rows)
			orphanLogs += batchOrphans
			watermark = rows[len(rows)-1].id

			if len(rows) < usageProjectionBatchSize {
				break
			}
		}

		if orphanLogs > 0 {
			slog.Info("usage-aggregation: orphan proxy_logs skipped site buckets",
				"orphan_logs", orphanLogs,
				"processed_logs", processedLogs,
				"watermark_id", watermark,
			)
		}

		result = &ProjectionPassResult{
			ProcessedLogs: processedLogs,
			OrphanLogs:    orphanLogs,
			WatermarkID:   watermark,
			Recomputed:    cp.RecomputeFromID != nil && *cp.RecomputeFromID > 0,
		}
	}()

	return result
}
