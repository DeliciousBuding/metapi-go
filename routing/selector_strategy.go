package routing

import (
	"slices"
	"time"
)

func (s *ChannelSelector) pickCandidate(
	available []RouteChannelCandidate, strategy RouteRoutingStrategy, resolveModel func(RouteChannelCandidate) string,
	policy DownstreamRoutingPolicy, routeID int64, requestedModel string, nowMs int64, recordSelection bool,
) (selected *RouteChannelCandidate, rotationKey string, usedObservation bool) {
	var pick func([]RouteChannelCandidate) *RouteChannelCandidate
	switch strategy {
	case StrategyLeastBusy, StrategyLowestLatency, StrategyLowestCost:
		pool := softFilterWithFallback(available, resolveModel, nowMs, s.configuredMaxSec)
		switch strategy {
		case StrategyLeastBusy:
			return SelectLeastBusyCandidate(pool, s.channelLoadProvider), "", false
		case StrategyLowestLatency:
			return SelectLowestLatencyCandidate(pool, resolveModel), "", false
		default:
			return SelectLowestCostCandidate(pool, resolveModel, s.pricingFn, s.fallbackUnitCost), "", false
		}
	case StrategyRoundRobin:
		pick = SelectRoundRobinCandidate
	case StrategyStableFirst:
		rotationKey = BuildStableFirstRotationKey(routeID, requestedModel)
		pick = func(pool []RouteChannelCandidate) *RouteChannelCandidate {
			plan := BuildStableFirstPoolPlan(pool, resolveModel)
			observe := len(plan.ObservationCandidates) > 0 && (len(plan.PrimaryCandidates) == 0 ||
				(recordSelection && ShouldUseStableFirstObservationCandidate(rotationKey, plan.ObservationCandidates)))
			candidates, key := plan.PrimaryCandidates, rotationKey
			if observe {
				candidates, key = plan.ObservationCandidates, rotationKey+":observe"
			}
			if len(candidates) == 0 {
				return nil
			}
			candidate := s.pickWeighted(candidates, resolveModel, policy, StableFirstMode, key)
			if candidate != nil {
				usedObservation = observe
			}
			return candidate
		}
	default:
		pick = func(pool []RouteChannelCandidate) *RouteChannelCandidate {
			return s.pickWeighted(pool, resolveModel, policy, WeightedMode, "")
		}
	}
	selected = selectAcrossPriorityLayers(available, resolveModel, nowMs, s.configuredMaxSec, pick)
	return selected, rotationKey, usedObservation
}

// selectAcrossPriorityLayers walks priority layers from lowest number to highest.
// Only when every layer has failed does it use the full pool as a starvation fallback.
func selectAcrossPriorityLayers(
	available []RouteChannelCandidate,
	resolveModel func(RouteChannelCandidate) string,
	nowMs int64,
	configuredMaxSec int,
	selectFromPool func([]RouteChannelCandidate) *RouteChannelCandidate,
) *RouteChannelCandidate {
	if len(available) == 0 || selectFromPool == nil {
		return nil
	}

	layers := make(map[int64][]RouteChannelCandidate)
	for _, c := range available {
		layers[c.Channel.PriorityOrZero()] = append(layers[c.Channel.PriorityOrZero()], c)
	}

	priorities := make([]int64, 0, len(layers))
	for p := range layers {
		priorities = append(priorities, p)
	}
	slices.Sort(priorities)

	for _, priority := range priorities {
		rawLayer := layers[priority]
		filteredLayer := softFilterCandidatesStrict(rawLayer, resolveModel, nowMs, configuredMaxSec)
		if len(filteredLayer) == 0 {
			continue
		}
		if selected := selectFromPool(filteredLayer); selected != nil {
			return selected
		}
	}

	return selectFromPool(softFilterWithFallback(available, resolveModel, nowMs, configuredMaxSec))
}

func softFilterWithFallback(available []RouteChannelCandidate, resolveModel func(RouteChannelCandidate) string, nowMs int64, configuredMaxSec int) []RouteChannelCandidate {
	breakerHealthy, _ := FilterSiteRuntimeBrokenCandidatesByModelResolver(available, resolveModel)
	return FilterRecentlyFailedCandidates(breakerHealthy,
		func(c RouteChannelCandidate) (*int64, *string) { return c.Channel.FailCount, c.Channel.LastFailAt },
		nowMs, configuredMaxSec)
}

// softFilterCandidatesStrict applies site/model breaker then recent-failure
// filters without full-set fallback. An empty result means every candidate is
// soft-unhealthy and the caller should try the next priority layer (or global
// fallback). Unlike FilterRecentlyFailedCandidates / FilterSiteRuntimeBroken*,
// a single soft-unhealthy candidate still yields empty so a lone broken
// priority-0 channel cannot block demotion to priority>=1.
func softFilterCandidatesStrict(
	candidates []RouteChannelCandidate,
	resolveModel func(RouteChannelCandidate) string,
	nowMs int64,
	configuredMaxSec int,
) []RouteChannelCandidate {
	if len(candidates) == 0 {
		return nil
	}
	healthy := make([]RouteChannelCandidate, 0, len(candidates))
	for _, c := range candidates {
		modelName := ""
		if resolveModel != nil {
			modelName = resolveModel(c)
		}
		details := GetSiteRuntimeHealthDetails(c.Site.ID, modelName)
		if details.GlobalBreakerOpen || details.ModelBreakerOpen ||
			details.GlobalHalfOpen || details.ModelHalfOpen {
			continue
		}
		if IsChannelRecentlyFailed(c.Channel.FailCount, c.Channel.LastFailAt, nowMs, configuredMaxSec) {
			continue
		}
		healthy = append(healthy, c)
	}
	return healthy
}

func (s *ChannelSelector) pickWeighted(
	candidates []RouteChannelCandidate,
	modelResolver func(RouteChannelCandidate) string,
	policy DownstreamRoutingPolicy, mode WeightedSelectionMode, key string,
) *RouteChannelCandidate {
	result := CalculateWeightedSelection(
		candidates,
		modelResolver,
		s.routingWeights,
		policy.SiteWeightMultipliers,
		policy.KeyWeight,
		s.channelLoadProvider,
		time.Now().UnixMilli(),
		mode,
		key,
		s.pricingFn,
		s.fallbackUnitCost,
	)
	return result.Selected
}
