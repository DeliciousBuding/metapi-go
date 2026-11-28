package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// ModelProbeScheduler periodically probes model/channel availability with a
// lightweight request so routing can avoid dead channels before user traffic fails.
// Supports account-level lease to prevent concurrent probes on the same account.
type ModelProbeScheduler struct {
	db            *store.DB
	cfg           *config.Config
	mu            sync.Mutex
	accountLeases map[int64]bool
	runner        *intervalRunner
	// ctx is the lifecycle context captured by Start so SetEnabled can
	// (re)start the ticker with the same cancellation semantics. Defaults to
	// context.Background() when SetEnabled runs before any Start.
	ctx context.Context

	probe    ChannelHealthProbe
	recorder ChannelHealthRecorder

	// lastRunSummary is retained for tests / operator diagnostics.
	lastRunSummary ProbeRunSummary
	// lastRuns is the ring buffer of recent completed passes (newest first,
	// capped at 10) so the scheduler-status surface can show honest history.
	lastRuns []ProbeRunSummary
}

// NewModelProbeScheduler creates a new model availability probe scheduler.
func NewModelProbeScheduler(db *store.DB, cfg *config.Config) *ModelProbeScheduler {
	return &ModelProbeScheduler{
		db:            db,
		cfg:           cfg,
		accountLeases: make(map[int64]bool),
		runner:        &intervalRunner{},
	}
}

// SetProbeExecutor injects the lightweight probe implementation.
func (s *ModelProbeScheduler) SetProbeExecutor(probe ChannelHealthProbe) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probe = probe
}

// SetHealthRecorder injects the routing health/cooldown recorder.
func (s *ModelProbeScheduler) SetHealthRecorder(recorder ChannelHealthRecorder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorder = recorder
}

func (s *ModelProbeScheduler) Name() string { return "model-probe" }

func (s *ModelProbeScheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Capture the lifecycle context so SetEnabled can (re)start the ticker
	// with the same cancellation semantics as the registry-driven Start.
	s.ctx = ctx

	// Always register for admin probe-now / recovery paths,
	// even when the background ticker is disabled.
	SetGlobalModelProbeScheduler(s)

	if !config.Runtime().ModelAvailabilityProbeEnabled {
		slog.Info("model-probe: disabled (probe not enabled)")
		return nil
	}

	// Hard floor of 60 seconds
	intervalMs := int64(config.MaxInt(s.cfg.ModelAvailabilityProbeIntervalMs, 60_000))
	interval := time.Duration(intervalMs) * time.Millisecond

	slog.Info("model-probe scheduler started",
		"interval_ms", intervalMs,
		"timeout_ms", s.cfg.ModelAvailabilityProbeTimeoutMs,
		"concurrency", s.cfg.ModelAvailabilityProbeConcurrency,
	)
	// No immediate first run: the original model-probe ticker only probes on
	// interval ticks (probe-now is driven by admin/global paths instead).
	return s.runner.start(ctx, interval, false, s.runProbe)
}

func (s *ModelProbeScheduler) Stop() error {
	return s.runner.stop()
}

// SetEnabled hot-toggles the model availability probe at runtime (#1027).
// Mirrors the boolean-feature-toggle contract: the caller has already
// persisted cfg.ModelAvailabilityProbeEnabled + the DB setting; this applies
// the change to the running ticker without a restart. Disabling stops the
// background probe loop; enabling restarts it (no immediate first run, same
// as Start). Admin probe-now paths stay available either way.
func (s *ModelProbeScheduler) SetEnabled(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.ModelAvailabilityProbeEnabled = enabled })
	if !enabled {
		slog.Info("model-probe: disabled at runtime (probe not enabled)")
		return s.runner.stop()
	}
	intervalMs := int64(config.MaxInt(s.cfg.ModelAvailabilityProbeIntervalMs, 60_000))
	interval := time.Duration(intervalMs) * time.Millisecond
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	slog.Info("model-probe scheduler enabled at runtime",
		"interval_ms", intervalMs,
		"timeout_ms", s.cfg.ModelAvailabilityProbeTimeoutMs,
		"concurrency", s.cfg.ModelAvailabilityProbeConcurrency,
	)
	return s.runner.start(ctx, interval, false, s.runProbe)
}

// TriggerNow runs one probe pass asynchronously (or sync if sync=true).
// Used by admin /api/models/probe and site probe-now.
func (s *ModelProbeScheduler) TriggerNow(sync bool) ProbeRunSummary {
	if s == nil {
		return ProbeRunSummary{}
	}
	if sync {
		safeJob("model-probe-now", s.runProbe)
		return s.LastRunSummary()
	}
	// Bare goroutine: the boundary wrapper protects the process from a
	// panicking probe pass (the sync path runs on the caller's stack).
	go safeJob("model-probe-now", s.runProbe)
	return s.LastRunSummary()
}

func (s *ModelProbeScheduler) runProbe() {
	dbw := s.db
	if dbw == nil {
		return
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), modelProbeJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runProbeLocked(dbw)
	})
}

// GlobalModelProbe is the process-wide scheduler instance (optional).
var globalModelProbeMu sync.Mutex
var globalModelProbe *ModelProbeScheduler

// SetGlobalModelProbeScheduler registers the running scheduler for admin triggers.
func SetGlobalModelProbeScheduler(s *ModelProbeScheduler) {
	globalModelProbeMu.Lock()
	defer globalModelProbeMu.Unlock()
	globalModelProbe = s
}

// GetGlobalModelProbeScheduler returns the registered scheduler (may be nil).
func GetGlobalModelProbeScheduler() *ModelProbeScheduler {
	globalModelProbeMu.Lock()
	defer globalModelProbeMu.Unlock()
	return globalModelProbe
}
