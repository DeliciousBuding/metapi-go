package routing

import "math"

// EffectiveUnitCost resolves the best available unit cost for a candidate.
func EffectiveUnitCost(candidate RouteChannelCandidate, modelName string, pricingFn CatalogPricingResolver, fallbackUnitCost float64) CostSignal {
	minCost := 1e-6
	successCount := candidate.Channel.SuccessCountOrZero()
	if successCount < 0 {
		successCount = 0
	}
	totalCost := candidate.Channel.TotalCostOrZero()
	if totalCost < 0 {
		totalCost = 0
	}

	if successCount > 0 && totalCost > 0 {
		return CostSignal{
			UnitCost: math.Max(totalCost/float64(successCount), minCost),
			Source:   "observed",
		}
	}

	if candidate.Account.UnitCost != nil && *candidate.Account.UnitCost > 0 && isFiniteFloat(*candidate.Account.UnitCost) {
		return CostSignal{
			UnitCost: math.Max(*candidate.Account.UnitCost, minCost),
			Source:   "configured",
		}
	}

	if pricingFn != nil {
		catalogCost, catalogSource := pricingFn.ResolveCatalogPricing(candidate.Site.ID, candidate.Account.ID, modelName)
		if catalogCost != nil && *catalogCost > 0 && isFiniteFloat(*catalogCost) {
			source := catalogSource
			if source == "" {
				source = CatalogSourceOfficial
			}
			return CostSignal{
				UnitCost: math.Max(*catalogCost, minCost),
				Source:   source,
			}
		}
	}

	return CostSignal{
		UnitCost: math.Max(fallbackUnitCost, minCost),
		Source:   "fallback",
	}
}

// ResolveChannelRuntimeLoadMultiplier converts a load snapshot to a multiplier.
func ResolveChannelRuntimeLoadMultiplier(snapshot ChannelLoadSnapshot) float64 {
	if !snapshot.SessionScoped || snapshot.ConcurrencyLimit <= 0 {
		return 1
	}
	activeRatio := ClampNumber(float64(snapshot.ActiveLeaseCount)/math.Max(1, float64(snapshot.ConcurrencyLimit)), 0, 1.5)
	waitingRatio := ClampNumber(float64(snapshot.WaitingCount)/math.Max(1, float64(snapshot.ConcurrencyLimit)), 0, 3)
	activePenalty := activeRatio * 0.28
	waitingPenalty := waitingRatio * 0.32
	saturationPenalty := 0.0
	if snapshot.Saturated {
		saturationPenalty = 0.12
	}
	return ClampNumber(1-activePenalty-waitingPenalty-saturationPenalty, 0.18, 1)
}
