package routing

import "math"

const (
	SiteRecentOutcomeHalfLifeMs        = 30 * 60 * 1000
	SiteRecentSuccessConfidenceSamples = 12
	SiteRecentSuccessPriorSuccesses    = 1
	SiteRecentSuccessPriorFailures     = 1
	SiteRecentSuccessFallbackRate      = 0.5
	SiteRecentModelWeight              = 0.65

	SiteHistoricalHealthMinMultiplier = 0.45
	SiteHistoricalHealthMaxSample     = 24
	SiteHistoricalLatencyBaselineMs   = 2000
	SiteHistoricalLatencyWindowMs     = 20000
	SiteHistoricalMaxLatencyPenalty   = 0.18
)

// SiteRuntimeHealthDetails is the resolved health for selection.
type SiteRuntimeHealthDetails struct {
	GlobalMultiplier   float64
	ModelMultiplier    float64
	CombinedMultiplier float64
	GlobalBreakerOpen  bool
	ModelBreakerOpen   bool
	// Half-open flags: the cooldown elapsed and the single recovery probe has
	// been admitted but not resolved yet. New requests keep being rejected.
	GlobalHalfOpen    bool
	ModelHalfOpen     bool
	ModelKey          string
	RecentSuccessRate float64
	RecentSampleCount float64
	RecentConfidence  float64
}

// RecentOutcomeSnapshot is a snapshot of recent success/failure counts.
type RecentOutcomeSnapshot struct {
	SuccessCount float64
	FailureCount float64
	SampleCount  float64
	SuccessRate  float64
	Confidence   float64
}

// SiteHistoricalHealthMetrics tracks historical health per site.
type SiteHistoricalHealthMetrics struct {
	Multiplier   float64
	TotalCalls   int64
	SuccessRate  *float64
	AvgLatencyMs *int64
}

func getDecayedSiteRuntimePenalty(state *SiteRuntimeHealthState) float64 {
	if state.PenaltyScore <= 0 || !isFiniteFloat(state.PenaltyScore) {
		return 0
	}
	elapsedMs := float64(nowMs() - state.LastUpdatedAtMs)
	if elapsedMs <= 0 {
		return state.PenaltyScore
	}
	decayFactor := math.Pow(0.5, elapsedMs/float64(SiteRuntimeHealthDecayHalfLifeMs))
	return state.PenaltyScore * decayFactor
}

// GetRuntimeHealthMultiplier returns the health multiplier for a state.
func GetRuntimeHealthMultiplier(state *SiteRuntimeHealthState) float64 {
	if state == nil {
		return 1
	}
	if isRuntimeHealthBreakerOpen(state) || isRuntimeHealthHalfOpenActive(state) {
		return SiteRuntimeMinMultiplier
	}
	penaltyScore := getDecayedSiteRuntimePenalty(state)
	failurePenaltyFactor := 1.0 / (1.0 + penaltyScore)

	// Prefer first-byte/TTFT EMA when available; fall back to total latency EMA.
	latencyMsForPenalty := 0.0
	if state.FirstByteEMAMs != nil {
		latencyMsForPenalty = *state.FirstByteEMAMs
	} else if state.LatencyEMAMs != nil {
		latencyMsForPenalty = *state.LatencyEMAMs
	}
	latencyPenaltyRatio := 0.0
	if latencyMsForPenalty > 0 {
		latencyPenaltyRatio = ClampNumber(
			(latencyMsForPenalty-SiteRuntimeLatencyBaselineMs)/SiteRuntimeLatencyWindowMs,
			0, 1,
		)
	}
	latencyFactor := 1.0 - (latencyPenaltyRatio * SiteRuntimeMaxLatencyPenalty)
	return ClampNumber(failurePenaltyFactor*latencyFactor, SiteRuntimeMinMultiplier, 1)
}

// GetSiteRuntimeHealthDetails returns combined health details for a site and model.
func GetSiteRuntimeHealthDetails(siteID int64, modelName string) SiteRuntimeHealthDetails {
	healthStateMu.RLock()
	defer healthStateMu.RUnlock()

	modelKey := NormalizeModelAlias(modelName)
	globalState := siteRuntimeHealthStates[siteID]
	var modelState *SiteRuntimeHealthState
	if modelKey != "" {
		modelState = getSiteModelRuntimeHealthState(siteID, modelKey)
	}
	displayKey := modelKey
	if len(displayKey) > 256 {
		displayKey = displayKey[:256] + "…"
	}
	globalMultiplier := GetRuntimeHealthMultiplier(globalState)
	modelMultiplier := 1.0
	if modelState != nil {
		modelMultiplier = GetRuntimeHealthMultiplier(modelState)
	}
	globalRecentSnapshot := getRecentOutcomeSnapshot(globalState)
	var modelRecentSnapshot *RecentOutcomeSnapshot
	if modelState != nil {
		snap := getRecentOutcomeSnapshot(modelState)
		modelRecentSnapshot = &snap
	}
	recentSnapshot := blendRecentOutcomeSnapshots(globalRecentSnapshot, modelRecentSnapshot)
	return SiteRuntimeHealthDetails{
		GlobalMultiplier:   globalMultiplier,
		ModelMultiplier:    modelMultiplier,
		CombinedMultiplier: ClampNumber(globalMultiplier*modelMultiplier, SiteRuntimeMinMultiplier*SiteRuntimeMinMultiplier, 1),
		GlobalBreakerOpen:  isRuntimeHealthBreakerOpen(globalState),
		ModelBreakerOpen:   isRuntimeHealthBreakerOpen(modelState),
		GlobalHalfOpen:     isRuntimeHealthHalfOpenActive(globalState),
		ModelHalfOpen:      isRuntimeHealthHalfOpenActive(modelState),
		ModelKey:           displayKey,
		RecentSuccessRate:  recentSnapshot.SuccessRate,
		RecentSampleCount:  recentSnapshot.SampleCount,
		RecentConfidence:   recentSnapshot.Confidence,
	}
}

// GetSiteRuntimeHealthMultiplier is a convenience wrapper.
func GetSiteRuntimeHealthMultiplier(siteID int64) float64 {
	healthStateMu.RLock()
	defer healthStateMu.RUnlock()

	state := siteRuntimeHealthStates[siteID]
	return GetRuntimeHealthMultiplier(state)
}

// IsSiteRuntimeBreakerOpen checks if a site's global breaker is open.
func IsSiteRuntimeBreakerOpen(siteID int64) bool {
	healthStateMu.RLock()
	defer healthStateMu.RUnlock()

	state := siteRuntimeHealthStates[siteID]
	return isRuntimeHealthBreakerOpen(state)
}

// ---- Outcome tracking ----

func decayRecentOutcomeCount(value float64, elapsedMs float64) float64 {
	if value <= 0 || !isFiniteFloat(value) {
		return 0
	}
	if elapsedMs <= 0 {
		return value
	}
	return value * math.Pow(0.5, elapsedMs/float64(SiteRecentOutcomeHalfLifeMs))
}

func buildRecentOutcomeSnapshot(successCount, failureCount float64) RecentOutcomeSnapshot {
	sc := math.Max(0, successCount)
	fc := math.Max(0, failureCount)
	sampleCount := sc + fc
	successRate := (sc + SiteRecentSuccessPriorSuccesses) / (sampleCount + SiteRecentSuccessPriorSuccesses + SiteRecentSuccessPriorFailures)
	return RecentOutcomeSnapshot{
		SuccessCount: sc,
		FailureCount: fc,
		SampleCount:  sampleCount,
		SuccessRate:  successRate,
		Confidence:   ClampNumber(sampleCount/SiteRecentSuccessConfidenceSamples, 0, 1),
	}
}

func getRecentOutcomeSnapshot(state *SiteRuntimeHealthState) RecentOutcomeSnapshot {
	if state == nil {
		return buildRecentOutcomeSnapshot(0, 0)
	}
	n := nowMs()
	updatedAtMs := state.RecentWindowUpdatedAtMs
	if updatedAtMs <= 0 {
		updatedAtMs = state.LastUpdatedAtMs
	}
	elapsedMs := float64(max(0, n-updatedAtMs))
	return buildRecentOutcomeSnapshot(
		decayRecentOutcomeCount(state.RecentSuccessCount, elapsedMs),
		decayRecentOutcomeCount(state.RecentFailureCount, elapsedMs),
	)
}

func refreshRecentOutcomeWindow(state *SiteRuntimeHealthState) {
	snapshot := getRecentOutcomeSnapshot(state)
	state.RecentSuccessCount = snapshot.SuccessCount
	state.RecentFailureCount = snapshot.FailureCount
	state.RecentWindowUpdatedAtMs = nowMs()
}

func blendRecentOutcomeSnapshots(globalSnapshot RecentOutcomeSnapshot, modelSnapshot *RecentOutcomeSnapshot) RecentOutcomeSnapshot {
	if modelSnapshot == nil || modelSnapshot.SampleCount <= 0 {
		return globalSnapshot
	}
	modelWeight := SiteRecentModelWeight
	globalWeight := 1.0 - modelWeight
	return buildRecentOutcomeSnapshot(
		(globalSnapshot.SuccessCount*globalWeight)+(modelSnapshot.SuccessCount*modelWeight),
		(globalSnapshot.FailureCount*globalWeight)+(modelSnapshot.FailureCount*modelWeight),
	)
}

// ResolveStableFirstSuccessRate blends recent runtime success rate with historical rate.
func ResolveStableFirstSuccessRate(details SiteRuntimeHealthDetails, historicalSuccessRate *float64) float64 {
	fallbackRate := SiteRecentSuccessFallbackRate
	if historicalSuccessRate != nil {
		fallbackRate = *historicalSuccessRate
	}
	return (details.RecentSuccessRate * details.RecentConfidence) + (fallbackRate * (1 - details.RecentConfidence))
}

// BuildSiteHistoricalHealthMetrics aggregates historical health per site.
func BuildSiteHistoricalHealthMetrics(candidates []RouteChannelCandidate) map[int64]SiteHistoricalHealthMetrics {
	type siteTotal struct {
		totalCalls     int64
		successCount   int64
		failCount      int64
		totalLatencyMs int64
		latencySamples int64
	}
	totals := make(map[int64]*siteTotal)
	for _, c := range candidates {
		siteID := c.Site.ID
		st, ok := totals[siteID]
		if !ok {
			st = &siteTotal{}
			totals[siteID] = st
		}
		sc := c.Channel.SuccessCountOrZero()
		fc := c.Channel.FailCountOrZero()
		if sc < 0 {
			sc = 0
		}
		if fc < 0 {
			fc = 0
		}
		st.successCount += sc
		st.failCount += fc
		st.totalCalls += sc + fc
		if sc > 0 {
			st.totalLatencyMs += max(0, c.Channel.TotalLatencyMsOrZero())
			st.latencySamples += sc
		}
	}

	metrics := make(map[int64]SiteHistoricalHealthMetrics)
	for siteID, st := range totals {
		if st.totalCalls <= 0 {
			metrics[siteID] = SiteHistoricalHealthMetrics{Multiplier: 1, TotalCalls: 0}
			continue
		}
		sampleFactor := ClampNumber(float64(st.totalCalls)/SiteHistoricalHealthMaxSample, 0, 1)
		successRate := float64(st.successCount) / float64(st.totalCalls)
		successPenaltyFactor := 1.0 - ((1.0 - successRate) * 0.55 * sampleFactor)

		var avgLatencyMs *int64
		if st.latencySamples > 0 {
			avg := int64(math.Round(float64(st.totalLatencyMs) / float64(st.latencySamples)))
			avgLatencyMs = &avg
		}

		latencyPenaltyRatio := 0.0
		if avgLatencyMs != nil {
			latencyPenaltyRatio = ClampNumber(
				(float64(*avgLatencyMs)-SiteHistoricalLatencyBaselineMs)/SiteHistoricalLatencyWindowMs,
				0, 1,
			) * sampleFactor
		}
		latencyFactor := 1.0 - (latencyPenaltyRatio * SiteHistoricalMaxLatencyPenalty)

		metrics[siteID] = SiteHistoricalHealthMetrics{
			Multiplier:   ClampNumber(successPenaltyFactor*latencyFactor, SiteHistoricalHealthMinMultiplier, 1),
			TotalCalls:   st.totalCalls,
			SuccessRate:  &successRate,
			AvgLatencyMs: avgLatencyMs,
		}
	}
	return metrics
}
