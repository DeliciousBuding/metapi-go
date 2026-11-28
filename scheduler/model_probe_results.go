package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ChannelHealthRecorder applies probe outcomes to routing health/cooldown.
// Implementations wrap routing.TokenRouter.RecordProbeSuccess/RecordProbeFailure.
// Must never mark credentials/accounts expired.
type ChannelHealthRecorder interface {
	RecordProbeSuccess(ctx context.Context, channelID int64, latencyMs float64, modelName *string, actualAccountID *int64) error
	RecordProbeFailure(ctx context.Context, channelID int64, httpStatus *int, errorText *string, modelName *string, actualAccountID *int64) error
}

func resolveProbeResultDB(dbw []*store.DB) *store.DB {
	if len(dbw) == 0 {
		return nil
	}
	return dbw[0]
}

// persistProbeResult appends one model_probe_results row for a background
// probe outcome. It is best-effort: a failed history write must never block
// health/cooldown mutation or the probe pass itself.
func (s *ModelProbeScheduler) persistProbeResult(dbw *store.DB, target ProbeTarget, status string, outcome ProbeOutcome) {
	if dbw == nil {
		return
	}
	if status == "" {
		status = "inconclusive"
	}
	var latencyPtr *float64
	if outcome.LatencyMs > 0 {
		l := outcome.LatencyMs
		latencyPtr = &l
	}
	var httpPtr *int64
	if outcome.HTTPStatus > 0 {
		h := int64(outcome.HTTPStatus)
		httpPtr = &h
	}
	var errPtr *string
	if outcome.ErrorText != "" {
		e := outcome.ErrorText
		errPtr = &e
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := dbw.Exec(
		`INSERT INTO model_probe_results
			(channel_id, account_id, site_id, model_name, status, latency_ms, http_status, error_text, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		target.ChannelID, target.AccountID, target.SiteID, target.ModelName, status, latencyPtr, httpPtr, errPtr, now,
	); err != nil {
		// Best-effort history persistence: probing must continue regardless,
		// but the write failure must not be invisible. Debug level because
		// probe runs are high-frequency and the row is non-critical.
		slog.Debug("model-probe: failed to persist probe result",
			"channel_id", target.ChannelID, "model", target.ModelName, "error", err)
	}
}

// Recovery probes may initially know only the channel. Pin history and health
// to the identity used on the wire even if the channel binding has changed.
func measuredProbeTarget(target ProbeTarget, outcome ProbeOutcome) ProbeTarget {
	if outcome.AccountID > 0 {
		target.AccountID = outcome.AccountID
		target.SiteID = outcome.SiteID
	}
	return target
}

// ApplyProbeOutcome is a pure helper used by tests and optional direct callers.
// It maps a ProbeOutcome onto the recorder without requiring a live DB target load.
func ApplyProbeOutcome(
	ctx context.Context,
	recorder ChannelHealthRecorder,
	target ProbeTarget,
	outcome ProbeOutcome,
) (string, error) {
	if recorder == nil {
		return "skipped", nil
	}
	modelName := target.ModelName
	var modelPtr *string
	if modelName != "" {
		modelPtr = &modelName
	}
	accountID := measuredProbeTarget(target, outcome).AccountID
	if accountID <= 0 && (outcome.Status == "success" || outcome.Status == "failure") {
		return "inconclusive", fmt.Errorf("missing account identity for channel %d probe", target.ChannelID)
	}
	accountPtr := &accountID

	switch outcome.Status {
	case "success":
		if err := recorder.RecordProbeSuccess(ctx, target.ChannelID, outcome.LatencyMs, modelPtr, accountPtr); err != nil {
			return "inconclusive", err
		}
		return "success", nil
	case "failure":
		var statusPtr *int
		if outcome.HTTPStatus > 0 {
			st := outcome.HTTPStatus
			statusPtr = &st
		}
		var errPtr *string
		if outcome.ErrorText != "" {
			e := outcome.ErrorText
			errPtr = &e
		}
		if err := recorder.RecordProbeFailure(ctx, target.ChannelID, statusPtr, errPtr, modelPtr, accountPtr); err != nil {
			return "inconclusive", err
		}
		return "failure", nil
	case "skipped":
		return "skipped", nil
	default:
		return "inconclusive", nil
	}
}
