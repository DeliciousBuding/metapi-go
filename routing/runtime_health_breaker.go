package routing

// ---- Breaker filter helpers ----

// TryAdmitSiteModelRuntimeRequest gates one dispatch decision through the
// three-state recovery semantics for (site, model), mirroring the octopus
// circuit breaker (internal/relay/balancer/circuit.go):
//
//   - open (cooldown not elapsed): reject — matches octopus StateOpen.
//   - half-open (cooldown elapsed, single recovery probe in flight): reject —
//     matches octopus StateHalfOpen.
//   - expired (cooldown elapsed, no probe granted yet): this request becomes
//     the recovery probe; the states are marked half-open and the request is
//     admitted. Exactly one probe passes; the rest keep being rejected until
//     the probe outcome is recorded or the probe window times out.
//   - closed: admit normally.
//
// Note the gate hard-rejects open breakers, so the pre-existing fail-open
// starvation guard (single-candidate short-circuit / all-broken full-set pool
// in the filter layer) no longer dispatches against hard-open sites: open now
// means no traffic until the cooldown expires and the probe recovers the
// breaker. This is the deliberate three-state semantic change.
func TryAdmitSiteModelRuntimeRequest(siteID int64, modelName string) bool {
	healthStateMu.Lock()
	defer healthStateMu.Unlock()

	n := nowMs()
	globalState := siteRuntimeHealthStates[siteID]
	modelState := getSiteModelRuntimeHealthState(siteID, modelName)

	globalBlocked, globalExpired := evaluateHalfOpenAdmission(globalState, n)
	modelBlocked, modelExpired := evaluateHalfOpenAdmission(modelState, n)

	if !globalBlocked && !modelBlocked {
		return true
	}
	// A hard-open breaker or an in-flight probe refuses new requests. Only when
	// every blocking state is expired (waiting for its first probe) does this
	// request become the recovery probe.
	if (globalBlocked && !globalExpired) || (modelBlocked && !modelExpired) {
		return false
	}
	if globalExpired {
		globalState.HalfOpenSinceMs = &n
	}
	if modelExpired {
		modelState.HalfOpenSinceMs = &n
	}
	scheduleSiteRuntimeHealthPersistence()
	return true
}

// evaluateHalfOpenAdmission classifies a single breaker state for the admission
// gate. blocked=true keeps new requests away; expired=true additionally means
// the blocking state is a cooldown that already elapsed with no probe granted,
// so the state is waiting for its first post-cooldown request.
func evaluateHalfOpenAdmission(state *SiteRuntimeHealthState, n int64) (blocked bool, expired bool) {
	if state == nil {
		return false, false
	}
	if state.HalfOpenSinceMs != nil {
		if n-*state.HalfOpenSinceMs > SiteRuntimeHalfOpenProbeTimeoutMs {
			// Probe outcome never arrived: release back to the expired state so
			// the next request retries the probe instead of staying isolated.
			state.HalfOpenSinceMs = nil
		} else {
			return true, false
		}
	}
	if state.BreakerUntilMs == nil {
		return false, false
	}
	if *state.BreakerUntilMs > n {
		return true, false
	}
	return true, true
}

// FilterSiteRuntimeBrokenCandidatesByModel filters by model-level breaker.
func FilterSiteRuntimeBrokenCandidatesByModel(
	candidates []RouteChannelCandidate,
	modelName string,
) (healthy []RouteChannelCandidate, avoided []struct {
	Candidate RouteChannelCandidate
	Reason    string
}) {
	return FilterSiteRuntimeBrokenCandidatesByModelResolver(candidates, func(RouteChannelCandidate) string {
		return modelName
	})
}

// FilterSiteRuntimeBrokenCandidatesByModelResolver filters by model-level breaker,
// resolving the model name per-candidate via the provided function.
// This is needed for display-name-matched routes where each candidate may have a different source model.
func FilterSiteRuntimeBrokenCandidatesByModelResolver(
	candidates []RouteChannelCandidate,
	resolveModel func(RouteChannelCandidate) string,
) (healthy []RouteChannelCandidate, avoided []struct {
	Candidate RouteChannelCandidate
	Reason    string
}) {
	if len(candidates) <= 1 {
		return candidates, nil
	}

	for _, candidate := range candidates {
		modelName := resolveModel(candidate)
		details := GetSiteRuntimeHealthDetails(candidate.Site.ID, modelName)
		blocked := details.GlobalBreakerOpen || details.ModelBreakerOpen ||
			details.GlobalHalfOpen || details.ModelHalfOpen
		if blocked {
			reason := buildRuntimeBreakerReason(details)
			avoided = append(avoided, struct {
				Candidate RouteChannelCandidate
				Reason    string
			}{candidate, reason})
		} else {
			healthy = append(healthy, candidate)
		}
	}

	if len(healthy) > 0 {
		return healthy, avoided
	}
	return candidates, nil
}

func buildRuntimeBreakerReason(details SiteRuntimeHealthDetails) string {
	if details.GlobalBreakerOpen && details.ModelBreakerOpen {
		return "站点熔断中，模型熔断中，优先避让"
	}
	if details.GlobalBreakerOpen {
		return "站点熔断中，优先避让"
	}
	if details.ModelBreakerOpen {
		return "模型熔断中，优先避让"
	}
	if details.GlobalHalfOpen && details.ModelHalfOpen {
		return "站点半开探测中，模型半开探测中，优先避让"
	}
	if details.GlobalHalfOpen {
		return "站点半开探测中，优先避让"
	}
	if details.ModelHalfOpen {
		return "模型半开探测中，优先避让"
	}
	return "运行时熔断中，优先避让"
}


type ProbeStatus struct {
	Status      string // success | failure | inconclusive | ""
	AtMs        int64
	LatencyMs   *float64
	ErrorText   *string
	ModelName   string
	ChannelID   *int64
	BreakerOpen bool
	Multiplier  float64
}

// RecordSiteProbeOutcome stamps the last background probe result on site health
// without treating the event as user traffic. Success/failure still go through
// the normal runtime health counters via RecordSiteRuntime* helpers.
func RecordSiteProbeOutcome(siteID int64, status string, latencyMs float64, modelName *string, channelID *int64, errText *string) {
	if siteID <= 0 {
		return
	}
	healthStateMu.Lock()
	defer healthStateMu.Unlock()

	n := nowMs()
	state := getOrCreateSiteRuntimeHealthState(siteID)
	state.LastProbeAtMs = &n
	state.LastProbeStatus = status
	state.LastUpdatedAtMs = n
	if latencyMs > 0 {
		v := latencyMs
		state.LastProbeLatencyMs = &v
	} else {
		state.LastProbeLatencyMs = nil
	}
	if errText != nil && *errText != "" {
		e := *errText
		state.LastProbeError = &e
	} else {
		state.LastProbeError = nil
	}
	if modelName != nil {
		state.LastProbeModel = *modelName
	} else {
		state.LastProbeModel = ""
	}
	if channelID != nil && *channelID > 0 {
		id := *channelID
		state.LastProbeChannelID = &id
	} else {
		state.LastProbeChannelID = nil
	}

	if modelName != nil && *modelName != "" {
		if modelState := getOrCreateSiteModelRuntimeHealthState(siteID, *modelName); modelState != nil {
			modelState.LastProbeAtMs = &n
			modelState.LastProbeStatus = status
			modelState.LastUpdatedAtMs = n
			if latencyMs > 0 {
				v := latencyMs
				modelState.LastProbeLatencyMs = &v
			} else {
				modelState.LastProbeLatencyMs = nil
			}
			if errText != nil && *errText != "" {
				e := *errText
				modelState.LastProbeError = &e
			} else {
				modelState.LastProbeError = nil
			}
			modelState.LastProbeModel = *modelName
			if channelID != nil && *channelID > 0 {
				id := *channelID
				modelState.LastProbeChannelID = &id
			} else {
				modelState.LastProbeChannelID = nil
			}
		}
	}
	scheduleSiteRuntimeHealthPersistence()
}

// GetSiteProbeStatus returns the last background probe stamp for a site.
func GetSiteProbeStatus(siteID int64) ProbeStatus {
	healthStateMu.RLock()
	defer healthStateMu.RUnlock()

	state := siteRuntimeHealthStates[siteID]
	if state == nil {
		return ProbeStatus{Multiplier: 1}
	}
	out := ProbeStatus{
		Status:      state.LastProbeStatus,
		ModelName:   state.LastProbeModel,
		BreakerOpen: isRuntimeHealthBreakerOpen(state),
		Multiplier:  GetRuntimeHealthMultiplier(state),
	}
	if state.LastProbeAtMs != nil {
		out.AtMs = *state.LastProbeAtMs
	}
	if state.LastProbeLatencyMs != nil {
		v := *state.LastProbeLatencyMs
		out.LatencyMs = &v
	}
	if state.LastProbeError != nil {
		e := *state.LastProbeError
		out.ErrorText = &e
	}
	if state.LastProbeChannelID != nil {
		id := *state.LastProbeChannelID
		out.ChannelID = &id
	}
	return out
}

