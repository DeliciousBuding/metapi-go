package scheduler

import (
	"github.com/deliciousbuding/metapi-go/config"
)

// modelProbeResultRetentionDays bounds model_probe_results, the highest-volume
// table in a deployment that turns probing on: one row per probed
// (channel, model) per pass, which on a busy instance is tens of thousands of
// rows a day and, until now, never a single DELETE.
//
// Seven days is already generous relative to what anything reads:
//
//   - service.loadLatestProbeFailures (route rebuild's probe filter, #625)
//     reads only the newest row per (account_id, model_name), and those rows are
//     exempt from this job outright — see modelProbeResultKeepLatestWhere;
//   - handler/admin.queryProbeHistory shows the newest N rows per channel or
//     account with NO time filter, so extra days never reach the screen while
//     every extra row is paid for twice: the window scan that query performs
//     over the whole table, and the three indexes each insert maintains.
//
// The window is a constant rather than a new knob on purpose. Nobody has asked
// to tune it, RetentionSchedulerOptions already reads both values from config,
// and promoting it later is a two-line change here plus a docs/env-parity
// entry — cheaper than carrying an unexercised setting now.
const modelProbeResultRetentionDays = 7

// modelProbeResultPruneIntervalMin is the prune cadence. Hourly, matching a
// table that grows continuously rather than in daily batches, so the table
// never sits more than ~1/24 of a day above its steady-state size.
const modelProbeResultPruneIntervalMin = 60

// modelProbeResultKeepLatestWhere exempts the newest row of every
// (account_id, model_name) pair from the prune, however old it is.
//
// This is what makes the window safe to have at all: route rebuild's probe
// filter reads exactly that row set
//
//	WHERE id IN (SELECT MAX(id) FROM model_probe_results GROUP BY account_id, model_name)
//
// so a plain age-based DELETE would silently drop the filter's input for any
// model that has not been probed inside the window — probing paused, channel
// disabled, account idle — and routing would change behaviour because a cleanup
// job ran. MAX(id) is never NULL within a group (id is the primary key), so the
// NOT IN cannot degenerate into "delete nothing" the way a nullable subquery
// would.
const modelProbeResultKeepLatestWhere = " AND id NOT IN (SELECT MAX(id) FROM model_probe_results GROUP BY account_id, model_name)"

// ModelProbeResultRetentionScheduler periodically prunes aged
// model_probe_results rows. Implemented by the shared RetentionScheduler.
type ModelProbeResultRetentionScheduler = RetentionScheduler

// NewModelProbeResultRetentionScheduler creates the probe-result retention
// scheduler. It runs by default: there is no operator opt-out, because the rows
// it deletes are provably unread (the two consumers are listed above) while the
// rows it keeps are pinned by modelProbeResultKeepLatestWhere.
func NewModelProbeResultRetentionScheduler(cfg *config.Config) *RetentionScheduler {
	return NewRetentionScheduler(cfg, RetentionSchedulerOptions{
		Name:               "model-probe-result-retention",
		Table:              "model_probe_results",
		ExtraWhere:         modelProbeResultKeepLatestWhere,
		UseUTC:             true, // created_at is written as time.Now().UTC().RFC3339
		DefaultIntervalMin: modelProbeResultPruneIntervalMin,
		RetentionDaysFn:    func(*config.Config) int { return modelProbeResultRetentionDays },
		IntervalMinFn:      func(*config.Config) int { return modelProbeResultPruneIntervalMin },
		DisabledFn:         func(*config.Config) (bool, string) { return false, "" },
	})
}
