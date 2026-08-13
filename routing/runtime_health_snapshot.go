package routing

// Read-only aggregation of the in-memory site/model runtime health state.
//
// The runtime breaker lives only in memory (persisted to settings on an
// interval), so a projection endpoint cannot read it from the DB. This
// snapshot copies counts + open-breaker rows under the same mutex the proxy
// path uses, without mutating anything — soft isolation only.

// RuntimeHealthBreaker is one open site-level or site+model breaker row.
type RuntimeHealthBreaker struct {
	SiteID         int64   `json:"siteId"`
	Model          string  `json:"model"` // empty = site-level breaker
	BreakerLevel   int64   `json:"breakerLevel"`
	BreakerUntilMs *int64  `json:"breakerUntilMs,omitempty"`
	PenaltyScore   float64 `json:"penaltyScore"`
}

// RuntimeHealthSnapshot is a read-only aggregate of runtime health state.
type RuntimeHealthSnapshot struct {
	SitesTracked      int                    `json:"sitesTracked"`
	SitesBreakerOpen  int                    `json:"sitesBreakerOpen"`
	ModelsTracked     int                    `json:"modelsTracked"`
	ModelsBreakerOpen int                    `json:"modelsBreakerOpen"`
	OpenBreakers      []RuntimeHealthBreaker `json:"openBreakers"`
}

// SnapshotRuntimeHealth returns a read-only projection of the in-memory
// site/model runtime breaker state. Never hard-disables anything.
func SnapshotRuntimeHealth() RuntimeHealthSnapshot {
	healthStateMu.RLock()
	defer healthStateMu.RUnlock()

	snapshot := RuntimeHealthSnapshot{
		SitesTracked: len(siteRuntimeHealthStates),
		OpenBreakers: []RuntimeHealthBreaker{},
	}

	for siteID, state := range siteRuntimeHealthStates {
		if isRuntimeHealthBreakerOpen(state) {
			snapshot.SitesBreakerOpen++
			snapshot.OpenBreakers = append(snapshot.OpenBreakers, RuntimeHealthBreaker{
				SiteID:         siteID,
				BreakerLevel:   state.BreakerLevel,
				BreakerUntilMs: state.BreakerUntilMs,
				PenaltyScore:   getDecayedSiteRuntimePenalty(state),
			})
		}
	}

	for siteID, models := range siteModelRuntimeHealthStates {
		snapshot.ModelsTracked += len(models)
		for model, state := range models {
			if isRuntimeHealthBreakerOpen(state) {
				snapshot.ModelsBreakerOpen++
				snapshot.OpenBreakers = append(snapshot.OpenBreakers, RuntimeHealthBreaker{
					SiteID:         siteID,
					Model:          model,
					BreakerLevel:   state.BreakerLevel,
					BreakerUntilMs: state.BreakerUntilMs,
					PenaltyScore:   getDecayedSiteRuntimePenalty(state),
				})
			}
		}
	}

	return snapshot
}
