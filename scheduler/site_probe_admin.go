package scheduler

import (
	"context"
	"fmt"
	"sync"

	"github.com/deliciousbuding/metapi-go/store"
)

// ProbeSiteResult is one model outcome for admin probe-now.
type ProbeSiteResult struct {
	ChannelID int64   `json:"channelId"`
	AccountID int64   `json:"accountId"`
	Model     string  `json:"model"`
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latencyMs"`
	Error     string  `json:"error,omitempty"`
}

// adminProbeConcurrency bounds parallel probes for admin probe-now /
// probe-stream. 8 workers turns a 32-target batch (~15s per-probe ceiling)
// from a sequential ~8min HTTP block into ~4 waves ≈ 60s worst case, while
// keeping upstream load bounded. The per-probe 15s timeout is a ceiling —
// real probes are sub-second, so typical batches finish in well under a
// second per wave.
const adminProbeConcurrency = 8

// ProbeSite runs probes for active channels on a site (bounded to 32) and
// returns the aggregated results. It is a thin wrapper around
// ProbeSiteIncremental that collects every result under a non-cancellable
// background context. Kept for callers that want a single batch response
// (app probe-wire tests, recovery paths).
func (s *ModelProbeScheduler) ProbeSite(siteID int64) (results []ProbeSiteResult, available, unavailable int) {
	_ = s.ProbeSiteIncremental(context.Background(), siteID, func(r ProbeSiteResult) {
		results = append(results, r)
		if r.Status == "success" {
			available++
		} else if r.Status == "failure" {
			unavailable++
		}
	})
	if results == nil {
		results = []ProbeSiteResult{}
	}
	return results, available, unavailable
}

// ProbeSiteIncremental runs probes for active channels on a site (bounded to
// 32 targets) with bounded concurrency and streams each result to onResult
// as soon as it completes. It respects ctx cancellation: on cancel it stops
// scheduling new probes and returns ctx.Err(). In-flight probes are allowed
// to finish (they carry their own per-target timeout via probeOne) but their
// results are not streamed to a cancelled caller.
//
// onResult may be nil. A nil scheduler, non-positive siteID, or missing DB
// yields a no-op (returns nil). A target-load error is delivered as a single
// ProbeSiteResult{Status:"error"} and also returned as err.
func (s *ModelProbeScheduler) ProbeSiteIncremental(ctx context.Context, siteID int64, onResult func(ProbeSiteResult)) error {
	if onResult == nil {
		onResult = func(ProbeSiteResult) {}
	}
	if s == nil || siteID <= 0 {
		return nil
	}
	dbw := store.GetDB()
	if dbw == nil {
		return nil
	}
	// siteProbeTargetsSQL selects up to 32 enabled route channels on the site
	// whose account/site is active and whose source model is non-empty. The
	// LIMIT 32 caps total work per admin probe pass so a single call cannot
	// explode into hundreds of upstream requests.
	const siteProbeTargetsSQL = "SELECT rc.id, rc.account_id, a.site_id, COALESCE(rc.source_model, '') AS source_model " +
		"FROM route_channels rc " +
		"INNER JOIN accounts a ON rc.account_id = a.id " +
		"INNER JOIN sites st ON a.site_id = st.id " +
		"WHERE rc.enabled = TRUE AND a.status = 'active' AND st.status = 'active' " +
		"AND a.site_id = ? AND COALESCE(rc.source_model, '') <> '' " +
		"ORDER BY rc.id ASC LIMIT 32"
	targets, err := queryProbeTargets(dbw, siteProbeTargetsSQL, siteID)
	if err != nil {
		onResult(ProbeSiteResult{Status: "error", Error: fmt.Sprintf("%v", err)})
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	timeoutMs := 15000
	if s.cfg != nil && s.cfg.ModelAvailabilityProbeTimeoutMs >= 3000 {
		timeoutMs = s.cfg.ModelAvailabilityProbeTimeoutMs
	}
	s.probeSiteTargets(ctx, targets, timeoutMs, onResult)
	return ctx.Err()
}

// probeSiteTargets runs probes for the given targets with bounded concurrency
// and streams each result to onResult as soon as it completes. It respects
// ctx cancellation: stops scheduling new probes on cancel; in-flight probes
// finish (bounded by their own per-target timeout) but their results are not
// streamed to a cancelled caller.
//
// This is split from ProbeSiteIncremental so tests can exercise the
// concurrency / cancellation contract directly with synthetic targets and
// a fake probe executor, without seeding a DB.
func (s *ModelProbeScheduler) probeSiteTargets(ctx context.Context, targets []ProbeTarget, timeoutMs int, onResult func(ProbeSiteResult)) {
	if onResult == nil {
		onResult = func(ProbeSiteResult) {}
	}
	if len(targets) == 0 {
		return
	}
	sem := make(chan struct{}, adminProbeConcurrency)
	var wg sync.WaitGroup
loop:
	for _, target := range targets {
		target := target
		// Stop scheduling new probes once the caller has cancelled. The
		// select also unblocks on ctx.Done() so we never block waiting for
		// a slot after cancellation.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break loop
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			// Boundary recovery: a panicking probe must not take the
			// process down; the wg/sem defers above still run.
			safeJob("site-probe-target", func() {
				outcome := s.probeOne(target, timeoutMs)
				// Don't stream to a cancelled caller — the client is gone and
				// writing would just block / error downstream. The probe still
				// ran to completion and any health mutation already happened.
				if ctx.Err() != nil {
					return
				}
				onResult(ProbeSiteResult{
					ChannelID: target.ChannelID,
					AccountID: target.AccountID,
					Model:     target.ModelName,
					Status:    outcome,
				})
			})
		}()
	}
	wg.Wait()
}
