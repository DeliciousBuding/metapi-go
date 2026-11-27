package routing

import (
	"math"
	"sort"
)

const (
	StableFirstPrimarySuccessRateRatio    = 0.92
	StableFirstTrustedRecentConfidence    = 0.5
	StableFirstTrustedHistoricalCalls     = 8
	StableFirstObservationRequestInterval = 24
	StableFirstObservationSiteCooldownMs  = 30 * 60 * 1000
)

// BuildStableFirstPoolPlan splits candidates into primary and observation pools.
func BuildStableFirstPoolPlan(candidates []RouteChannelCandidate, modelResolver func(RouteChannelCandidate) string) StableFirstPoolPlan {
	if len(candidates) == 0 {
		return StableFirstPoolPlan{}
	}

	historicalBySiteID := BuildSiteHistoricalHealthMetrics(candidates)
	leaderBySiteID := make(map[int64]RouteChannelCandidate)
	siteStateByID := make(map[int64]StableFirstSitePoolState)

	for _, c := range candidates {
		siteID := c.Site.ID
		currentLeader, ok := leaderBySiteID[siteID]
		if !ok || compareStableFirstCandidateOrder(c, currentLeader) < 0 {
			leaderBySiteID[siteID] = c
		}
	}

	for siteID, leader := range leaderBySiteID {
		resolvedModel := ""
		if modelResolver != nil {
			resolvedModel = modelResolver(leader)
		}
		healthDetails := GetSiteRuntimeHealthDetails(siteID, resolvedModel)
		historical := historicalBySiteID[siteID]
		historicalTotalCalls := historical.TotalCalls
		effectiveSuccessRate := ResolveStableFirstSuccessRate(healthDetails, historical.SuccessRate)
		trusted := healthDetails.RecentConfidence >= StableFirstTrustedRecentConfidence || historicalTotalCalls >= StableFirstTrustedHistoricalCalls
		siteStateByID[siteID] = StableFirstSitePoolState{
			SiteID:               siteID,
			Leader:               leader,
			EffectiveSuccessRate: effectiveSuccessRate,
			Trusted:              trusted,
		}
	}

	// Sort all site states by effective success rate descending
	type siteStateEntry struct {
		siteID int64
		state  StableFirstSitePoolState
	}
	var allEntries []siteStateEntry
	for siteID, state := range siteStateByID {
		allEntries = append(allEntries, siteStateEntry{siteID, state})
	}
	sort.SliceStable(allEntries, func(a, b int) bool {
		diff := allEntries[b].state.EffectiveSuccessRate - allEntries[a].state.EffectiveSuccessRate
		if math.Abs(diff) > 1e-9 {
			return diff > 0
		}
		return compareStableFirstCandidateOrder(allEntries[a].state.Leader, allEntries[b].state.Leader) < 0
	})

	// Build trusted leader pool
	var trustedEntries []siteStateEntry
	for _, e := range allEntries {
		if e.state.Trusted {
			trustedEntries = append(trustedEntries, e)
		}
	}
	leaderPool := trustedEntries
	if len(leaderPool) == 0 {
		leaderPool = allEntries
	}

	thresholdRate := 0.0
	if len(leaderPool) > 0 && leaderPool[0].state.EffectiveSuccessRate > 0 {
		thresholdRate = leaderPool[0].state.EffectiveSuccessRate * StableFirstPrimarySuccessRateRatio
	}

	plan := StableFirstPoolPlan{}

	for _, e := range allEntries {
		inPrimary := false
		if len(leaderPool) > 0 {
			for _, le := range leaderPool {
				if le.siteID == e.siteID && e.state.EffectiveSuccessRate >= thresholdRate {
					inPrimary = true
					break
				}
			}
		}
		if inPrimary {
			plan.PrimarySiteIDs = append(plan.PrimarySiteIDs, e.siteID)
		} else {
			plan.ObservationSiteIDs = append(plan.ObservationSiteIDs, e.siteID)
			state := e.state
			if state.Trusted {
				state.ObservationReason = "观察池：近期成功率暂时落后，仅灰度真实流量会命中"
			} else {
				state.ObservationReason = "观察池：近期样本不足，仅灰度真实流量会命中"
			}
			siteStateByID[e.siteID] = state
		}
	}

	// If primary pool is empty, promote top site
	if len(plan.PrimarySiteIDs) == 0 && len(allEntries) > 0 {
		plan.PrimarySiteIDs = append(plan.PrimarySiteIDs, allEntries[0].siteID)
		// Remove from observation
		filtered := make([]int64, 0)
		for _, sid := range plan.ObservationSiteIDs {
			if sid != allEntries[0].siteID {
				filtered = append(filtered, sid)
			}
		}
		plan.ObservationSiteIDs = filtered
	}

	// Collect candidates
	primarySet := make(map[int64]bool)
	for _, sid := range plan.PrimarySiteIDs {
		primarySet[sid] = true
	}
	observationSet := make(map[int64]bool)
	for _, sid := range plan.ObservationSiteIDs {
		observationSet[sid] = true
	}
	for _, c := range candidates {
		if primarySet[c.Site.ID] {
			plan.PrimaryCandidates = append(plan.PrimaryCandidates, c)
		} else if observationSet[c.Site.ID] {
			plan.ObservationCandidates = append(plan.ObservationCandidates, c)
		}
	}
	plan.SiteStateByID = siteStateByID

	return plan
}

// StableFirstPoolPlan is the result of splitting candidates into pools.
type StableFirstPoolPlan struct {
	PrimaryCandidates     []RouteChannelCandidate
	ObservationCandidates []RouteChannelCandidate
	PrimarySiteIDs        []int64
	ObservationSiteIDs    []int64
	SiteStateByID         map[int64]StableFirstSitePoolState
}

// StableFirstSitePoolState is per-site state in a pool plan.
type StableFirstSitePoolState struct {
	SiteID               int64
	Leader               RouteChannelCandidate
	EffectiveSuccessRate float64
	Trusted              bool
	ObservationReason    string
}
