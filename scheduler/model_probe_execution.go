package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

const probeRecordTimeout = 5 * time.Second

// ProbeTarget is one lightweight channel/model pair the background probe may hit.
type ProbeTarget struct {
	ChannelID int64
	AccountID int64
	SiteID    int64
	ModelName string
}

// ProbeOutcome is the result of a single lightweight probe attempt.
// Status is one of: success | failure | inconclusive | skipped.
type ProbeOutcome struct {
	AccountID int64
	SiteID    int64
	Status    string
	LatencyMs float64
	// HTTPStatus is optional; 0 means unknown / transport error.
	HTTPStatus int
	ErrorText  string
}

// ChannelHealthProbe executes one lightweight request against a channel/model.
// Implementations live outside scheduler (proxy/platform) and are injected at boot.
// Returning an error is treated as an inconclusive outcome (no health mutation).
type ChannelHealthProbe interface {
	ProbeChannel(ctx context.Context, target ProbeTarget) (ProbeOutcome, error)
}

// BatchProbeResult is one row of an operator-initiated batch verification.
// HealthApplied reports whether the recorder updated routing health/cooldown.
type BatchProbeResult struct {
	Target        ProbeTarget
	Outcome       ProbeOutcome
	HealthApplied bool
}

// ProbeBatch runs immediate lightweight probes for explicit admin targets.
// It reuses the injected
// probe executor and, when a recorder is wired, applies health outcomes —
// but never acquires account leases: this is a one-shot operator action,
// not a background pass. A nil scheduler or missing executor yields skipped
// results with no health mutation.
func (s *ModelProbeScheduler) ProbeBatch(ctx context.Context, targets []ProbeTarget, timeoutMs int) []BatchProbeResult {
	if s == nil {
		out := make([]BatchProbeResult, 0, len(targets))
		for _, t := range targets {
			out = append(out, BatchProbeResult{Target: t, Outcome: ProbeOutcome{Status: "skipped"}})
		}
		return out
	}
	s.mu.Lock()
	probe := s.probe
	recorder := s.recorder
	s.mu.Unlock()

	if timeoutMs < 3000 {
		timeoutMs = 3000
	}

	out := make([]BatchProbeResult, 0, len(targets))
	for _, target := range targets {
		res := BatchProbeResult{Target: target, Outcome: ProbeOutcome{Status: "skipped"}}
		if probe == nil {
			out = append(out, res)
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		outcome, err := probe.ProbeChannel(probeCtx, target)
		cancel()
		if err != nil {
			res.Outcome = ProbeOutcome{Status: "inconclusive", ErrorText: err.Error()}
			out = append(out, res)
			continue
		}
		target = measuredProbeTarget(target, outcome)
		res.Target = target
		res.Outcome = outcome
		if recorder != nil {
			status, applyErr := ApplyProbeOutcome(ctx, recorder, target, outcome)
			if applyErr != nil {
				slog.Warn("model-probe: failed to record health", "channel_id", target.ChannelID, "account_id", target.AccountID, "error", applyErr)
			} else {
				res.HealthApplied = status == "success" || status == "failure"
			}
		}
		out = append(out, res)
	}
	return out
}

func (s *ModelProbeScheduler) probeOne(target ProbeTarget, timeoutMs int, dbw ...*store.DB) string {
	s.mu.Lock()
	probe := s.probe
	recorder := s.recorder
	s.mu.Unlock()

	if probe == nil || recorder == nil {
		// Without an injected executor/recorder we cannot safely touch health.
		// Keep the pass no-op so operators can enable the flag without crash.
		slog.Debug("model-probe: probe executor or recorder not configured; skipping target",
			"channel_id", target.ChannelID,
			"model", target.ModelName,
		)
		return "skipped"
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	outcome, err := probe.ProbeChannel(ctx, target)
	if err != nil {
		slog.Warn("model-probe: probe executor error (inconclusive)",
			"channel_id", target.ChannelID,
			"model", target.ModelName,
			"error", err,
		)
		s.persistProbeResult(resolveProbeResultDB(dbw), target, "inconclusive", ProbeOutcome{Status: "inconclusive", ErrorText: err.Error()})
		return "inconclusive"
	}

	target = measuredProbeTarget(target, outcome)
	// The probe context may already be expired when a probe times out. Health
	// bookkeeping is a separate database write and must not inherit that
	// deadline, otherwise a timed-out channel is never cooled down.
	recordCtx, recordCancel := context.WithTimeout(context.Background(), probeRecordTimeout)
	defer recordCancel()

	status, recordErr := ApplyProbeOutcome(recordCtx, recorder, target, outcome)
	if recordErr != nil {
		slog.Warn("model-probe: failed to record health", "channel_id", target.ChannelID, "account_id", target.AccountID, "error", recordErr)
	}

	// Record the raw probe outcome for the route-rebuild probe filter (#625).
	// Persisted status uses the measured outcome so connectivity filtering sees
	// the actual upstream signal, not the recorder-applied result.
	s.persistProbeResult(resolveProbeResultDB(dbw), target, status, outcome)
	return status
}
