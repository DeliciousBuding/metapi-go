package routing

import (
	"math"
	"math/rand"
	"sort"
)

// WeightedSelectionMode is either "weighted" or "stable_first".
type WeightedSelectionMode string

const (
	WeightedMode    WeightedSelectionMode = "weighted"
	StableFirstMode WeightedSelectionMode = "stable_first"
)

// WeightedSelectionResult holds the selection result and detailed info.
type WeightedSelectionResult struct {
	Selected        *RouteChannelCandidate
	Details         []WeightedDetail
	StableSiteCount int
}

// WeightedDetail describes a single candidate's score breakdown.
type WeightedDetail struct {
	Candidate   RouteChannelCandidate
	Probability float64
	Reason      string
}

// RoutingWeightsConfig mirrors config.RoutingWeights for the routing package.
type RoutingWeightsConfig struct {
	BaseWeightFactor float64
	ValueScoreFactor float64
	CostWeight       float64
	BalanceWeight    float64
	UsageWeight      float64
}

// CalculateWeightedSelection performs the full weighted random selection algorithm.
// Implements the TS calculateWeightedSelection exactly.
func CalculateWeightedSelection(
	candidates []RouteChannelCandidate,
	modelResolver func(RouteChannelCandidate) string,
	routingWeights RoutingWeightsConfig,
	siteWeightMultipliers map[int64]float64,
	keyWeight float64,
	channelLoadProvider ChannelLoadSnapshotProvider,
	nowMs int64,
	mode WeightedSelectionMode,
	stableFirstRotationKey string,
	pricingFn CatalogPricingResolver,
	fallbackUnitCost float64,
) WeightedSelectionResult {
	if len(candidates) == 0 {
		return WeightedSelectionResult{}
	}
	if len(candidates) == 1 {
		return WeightedSelectionResult{
			Selected:        &candidates[0],
			Details:         []WeightedDetail{{Candidate: candidates[0], Probability: 1}},
			StableSiteCount: 1,
		}
	}

	if modelResolver == nil {
		modelResolver = func(c RouteChannelCandidate) string { return "" }
	}

	n := len(candidates)

	// Step 1: Effective costs
	effectiveCosts := make([]CostSignal, n)
	for i, c := range candidates {
		effectiveCosts[i] = EffectiveUnitCost(c, modelResolver(c), pricingFn, fallbackUnitCost)
	}

	// Step 2: Runtime health details
	runtimeHealthDetails := make([]SiteRuntimeHealthDetails, n)
	for i, c := range candidates {
		runtimeHealthDetails[i] = GetSiteRuntimeHealthDetails(c.Site.ID, modelResolver(c))
	}

	// Step 3: Channel load snapshots
	var channelLoadSnapshots []ChannelLoadSnapshot
	if channelLoadProvider != nil {
		channelLoadSnapshots = make([]ChannelLoadSnapshot, n)
		for i, c := range candidates {
			channelLoadSnapshots[i] = channelLoadProvider.GetChannelLoadSnapshot(ChannelLoadParams{
				ChannelID:            c.Channel.ID,
				AccountExtraConfig:   c.Account.ExtraConfig,
				AccountOAuthProvider: c.Account.OAuthProvider,
			})
		}
	}

	// Step 4: Value scores
	valueScores := make([]float64, n)
	maxVS := math.Inf(-1)
	minVS := math.Inf(1)
	for i, c := range candidates {
		unitCost := effectiveCosts[i].UnitCost
		balance := c.Account.BalanceOrZero()
		totalUsed := float64(c.Channel.SuccessCountOrZero() + c.Channel.FailCountOrZero())
		recentUsage := math.Max(totalUsed, 1)
		valueScore := routingWeights.CostWeight*(1/unitCost) + routingWeights.BalanceWeight*balance + routingWeights.UsageWeight*(1/recentUsage)
		valueScores[i] = valueScore
		if valueScore > maxVS {
			maxVS = valueScore
		}
		if valueScore < minVS {
			minVS = valueScore
		}
	}

	// Step 5: Min-max normalization
	if maxVS < 0.001 {
		maxVS = 0.001
	}
	if minVS > 0 {
		minVS = 0
	}
	vsRange := maxVS - minVS
	if vsRange == 0 {
		vsRange = 1
	}

	// Step 7: Site channel counts
	siteChannelCounts := make(map[int64]int)
	for _, c := range candidates {
		siteChannelCounts[c.Site.ID]++
	}
	siteHistoricalHealthMetrics := BuildSiteHistoricalHealthMetrics(candidates)

	effectiveKeyWeight := 1.0
	if keyWeight > 0 && isFiniteFloat(keyWeight) {
		effectiveKeyWeight = keyWeight
	}

	// Step 8: Contributions
	contributions := make([]float64, n)
	for i, candidate := range candidates {
		siteChannels := math.Max(1, float64(siteChannelCounts[candidate.Site.ID]))
		runtimeMultiplier := runtimeHealthDetails[i].CombinedMultiplier
		runtimeLoadMultiplier := channelCapacityMultiplier(candidate)
		if channelLoadProvider != nil {
			runtimeLoadMultiplier *= ResolveChannelRuntimeLoadMultiplier(channelLoadSnapshots[i])
		}

		if mode == StableFirstMode {
			recentSuccessRate := ResolveStableFirstSuccessRate(
				runtimeHealthDetails[i],
				siteHistoricalHealthMetrics[candidate.Site.ID].SuccessRate,
			)
			contribution := math.Max(1e-4, recentSuccessRate*recentSuccessRate)
			contribution *= runtimeMultiplier
			contribution *= runtimeLoadMultiplier
			contributions[i] = contribution / siteChannels
			continue
		}

		normalizedVS := (valueScores[i] - minVS) / vsRange
		baseContribution := (float64(candidate.Channel.WeightOrZero())*effectiveKeyWeight + 10) *
			(routingWeights.BaseWeightFactor + normalizedVS*routingWeights.ValueScoreFactor)
		contribution := baseContribution / siteChannels

		// Site weight
		downstreamSiteMultiplier := 1.0
		if w, ok := siteWeightMultipliers[candidate.Site.ID]; ok && w > 0 && isFiniteFloat(w) {
			downstreamSiteMultiplier = w
		}
		siteGlobalWeight := 1.0
		if gw := candidate.Site.GlobalWeightOrZero(); gw > 0 && isFiniteFloat(gw) {
			siteGlobalWeight = gw
		}
		combinedSiteWeight := siteGlobalWeight * downstreamSiteMultiplier
		if combinedSiteWeight > 0 && isFiniteFloat(combinedSiteWeight) {
			contribution *= combinedSiteWeight
		}

		contribution *= runtimeMultiplier
		if m, ok := siteHistoricalHealthMetrics[candidate.Site.ID]; ok {
			contribution *= m.Multiplier
		}
		contribution *= runtimeLoadMultiplier

		// Fallback cost penalty
		if effectiveCosts[i].Source == "fallback" {
			contribution *= 1 / math.Max(1, effectiveCosts[i].UnitCost)
		}

		contributions[i] = contribution
	}

	// Total contribution
	totalContribution := 0.0
	for _, c := range contributions {
		totalContribution += c
	}

	// Rank by contribution
	rankedIndices := make([]int, n)
	for i := range rankedIndices {
		rankedIndices[i] = i
	}
	sort.SliceStable(rankedIndices, func(a, b int) bool {
		diff := contributions[rankedIndices[b]] - contributions[rankedIndices[a]]
		if math.Abs(diff) > 1e-9 {
			return diff > 0
		}
		return compareStableFirstCandidateOrder(candidates[rankedIndices[a]], candidates[rankedIndices[b]]) < 0
	})

	stableSiteLeaderIndices := getStableFirstSiteLeaderIndices(candidates, contributions, rankedIndices)

	var selected *RouteChannelCandidate
	if mode == WeightedMode {
		r := rand.Float64() * totalContribution
		selected = &candidates[len(candidates)-1]
		for i := 0; i < n; i++ {
			r -= contributions[i]
			if r <= 0 {
				selected = &candidates[i]
				break
			}
		}
	} else {
		// stable_first: deterministic selection with site rotation
		selected = selectStableFirstCandidate(candidates, rankedIndices, stableSiteLeaderIndices, stableFirstRotationKey)
	}

	result := WeightedSelectionResult{
		Selected:        selected,
		StableSiteCount: len(stableSiteLeaderIndices),
		Details:         make([]WeightedDetail, 0, n),
	}

	for i, candidate := range candidates {
		probability := 0.0
		if totalContribution > 0 {
			probability = contributions[i] / totalContribution
		}
		detail := WeightedDetail{
			Candidate:   candidate,
			Probability: probability,
		}
		result.Details = append(result.Details, detail)
	}

	return result
}
