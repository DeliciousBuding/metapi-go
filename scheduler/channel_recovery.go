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
	channelRecoverySweepIntervalMs   = 30_000 // 30s sweep interval
	channelRecoveryMinIntervalMs     = 10_000 // 10s floor
	channelRecoveryMaxBatch          = 4      // max candidates per sweep
	channelRecoveryCooldownRecheckMs = 30_000 // 30s for cooldown
)

// ChannelRecoveryScheduler probes cooling channels to recover routing state.
// Healthy-channel monitoring belongs to ModelProbeScheduler. Normal app wiring
// never supplied active lease IDs, so recovery must not start scanning healthy
// channels when the unused lease plumbing is removed.
type ChannelRecoveryScheduler struct {
	db                 *store.DB
	cfg                *config.Config
	mu                 sync.Mutex
	inFlightKeys       map[string]bool  // "channelId:modelName" -> in flight
	lastStartedAtByKey map[string]int64 // "channelId:modelName" -> start timestamp ms
	sweepInFlight      bool
	runner             *intervalRunner
}

// NewChannelRecoveryScheduler creates a new channel recovery probe scheduler.
func NewChannelRecoveryScheduler(db *store.DB, cfg *config.Config) *ChannelRecoveryScheduler {
	return &ChannelRecoveryScheduler{
		db:                 db,
		cfg:                cfg,
		inFlightKeys:       make(map[string]bool),
		lastStartedAtByKey: make(map[string]int64),
		runner:             &intervalRunner{},
	}
}

func (s *ChannelRecoveryScheduler) Name() string { return "channel-recovery" }

func (s *ChannelRecoveryScheduler) Start(ctx context.Context) error {
	intervalMs := int64(config.MaxInt(channelRecoverySweepIntervalMs, channelRecoveryMinIntervalMs))
	interval := time.Duration(intervalMs) * time.Millisecond

	slog.Info("channel-recovery scheduler started",
		"interval_ms", intervalMs,
		"max_batch", channelRecoveryMaxBatch,
	)
	return s.runner.start(ctx, interval, true, s.runSweep)
}

func (s *ChannelRecoveryScheduler) Stop() error {
	return s.runner.stop()
}

// runSweep performs a single recovery probe sweep.
// Serially: if a previous sweep is in flight, it waits.
func (s *ChannelRecoveryScheduler) runSweep() {
	s.mu.Lock()
	if s.sweepInFlight {
		s.mu.Unlock()
		return
	}
	s.sweepInFlight = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.sweepInFlight = false
		s.mu.Unlock()
	}()

	dbw := s.db
	if dbw == nil {
		return
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), channelRecoveryJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runSweepLocked(dbw)
	})
}

func (s *ChannelRecoveryScheduler) runSweepLocked(dbw *store.DB) {
	nowMs := time.Now().UnixMilli()

	// Query cooling channels (SQL-backed; real candidate list).
	coolingCandidates := s.loadCoolingCandidates(dbw)
	due := s.filterDue(coolingCandidates, nowMs)

	if len(due) == 0 {
		return
	}

	// Sort by priority and limit to max batch
	s.prioritize(due)
	if len(due) > channelRecoveryMaxBatch {
		due = due[:channelRecoveryMaxBatch]
	}

	slog.Info("channel-recovery: running sweep",
		"cooling", len(coolingCandidates),
		"due", len(due),
	)

	for _, c := range due {
		s.probeCandidate(dbw, c, nowMs)
	}
}

type recoveryCandidate struct {
	channelID int64
	modelName string
}

func (s *ChannelRecoveryScheduler) loadCoolingCandidates(dbw *store.DB) []recoveryCandidate {
	nowISO := time.Now().UTC().Format(time.RFC3339)
	var candidates []recoveryCandidate

	rows, err := dbw.Query(`
		SELECT rc.id, COALESCE(rc.source_model, '') as source_model
		FROM route_channels rc
		INNER JOIN accounts a ON rc.account_id = a.id
		INNER JOIN sites st ON a.site_id = st.id
		WHERE rc.enabled = TRUE
		  AND a.status = 'active'
		  AND st.status = 'active'
		  AND rc.cooldown_until IS NOT NULL
		  AND rc.cooldown_until > ?
	`, nowISO)
	if err != nil {
		slog.Error("channel-recovery: failed to load cooling candidates", "error", err)
		return nil
	}
	defer rows.Close()

	var scanErr error
	for rows.Next() {
		var c recoveryCandidate
		if err := rows.Scan(&c.channelID, &c.modelName); err != nil {
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		// Skip provider-directed cooldown (failCount<=0 && consecutiveFailCount<=0 && cooldownLevel<=0)
		// For simplicity, we don't filter here - the TS does deeper filtering
		if c.modelName != "" {
			candidates = append(candidates, c)
		}
	}
	if err := rows.Err(); err != nil && scanErr == nil {
		scanErr = err
	}
	if scanErr != nil {
		slog.Warn("channel-recovery: cooling candidate rows degraded", "error", scanErr)
	}
	return candidates
}

func (s *ChannelRecoveryScheduler) filterDue(candidates []recoveryCandidate, nowMs int64) []recoveryCandidate {
	s.mu.Lock()
	defer s.mu.Unlock()

	var due []recoveryCandidate
	for _, c := range candidates {
		key := fmt.Sprintf("%d:%s", c.channelID, c.modelName)
		if s.inFlightKeys[key] {
			continue
		}
		lastStarted, exists := s.lastStartedAtByKey[key]
		if exists && (nowMs-lastStarted) < channelRecoveryCooldownRecheckMs {
			continue
		}
		due = append(due, c)
	}
	return due
}

func (s *ChannelRecoveryScheduler) prioritize(candidates []recoveryCandidate) {
	// Sort: never-probed first, then earliest-probed first
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			ki := fmt.Sprintf("%d:%s", candidates[i].channelID, candidates[i].modelName)
			kj := fmt.Sprintf("%d:%s", candidates[j].channelID, candidates[j].modelName)
			li, hasI := s.lastStartedAtByKey[ki]
			lj, hasJ := s.lastStartedAtByKey[kj]

			swap := false
			if !hasI && hasJ {
				swap = false // never-probed i stays before probed j
			} else if hasI && !hasJ {
				swap = true // probed i should move behind never-probed j
			} else if !hasI && !hasJ {
				swap = candidates[i].channelID > candidates[j].channelID
			} else {
				if li != lj {
					swap = li > lj
				} else {
					swap = candidates[i].channelID > candidates[j].channelID
				}
			}
			if swap {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}
}

func (s *ChannelRecoveryScheduler) probeCandidate(dbw *store.DB, candidate recoveryCandidate, nowMs int64) {
	key := fmt.Sprintf("%d:%s", candidate.channelID, candidate.modelName)

	s.mu.Lock()
	s.inFlightKeys[key] = true
	s.lastStartedAtByKey[key] = nowMs
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.inFlightKeys, key)
		s.mu.Unlock()
	}()

	// Prefer shared model probe executor when registered.
	if mp := GetGlobalModelProbeScheduler(); mp != nil {
		target := ProbeTarget{
			ChannelID: candidate.channelID,
			ModelName: candidate.modelName,
		}
		timeoutMs := 15000
		if s.cfg != nil && s.cfg.ModelAvailabilityProbeTimeoutMs >= 3000 {
			timeoutMs = s.cfg.ModelAvailabilityProbeTimeoutMs
		}
		outcome := mp.probeOne(target, timeoutMs)
		slog.Debug("channel-recovery: probe complete",
			"channel_id", candidate.channelID,
			"model", candidate.modelName,
			"source", "cooldown",
			"outcome", outcome,
		)
		return
	}
	slog.Debug("channel-recovery: no model probe scheduler registered; skip live probe",
		"channel_id", candidate.channelID,
		"model", candidate.modelName,
		"source", "cooldown",
	)
}
