package routing

// ---- Breaker filter helpers ----

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
		blocked := details.GlobalBreakerOpen || details.ModelBreakerOpen
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

