package routing

import (
	"sort"
	"strconv"
	"sync"
)

func compareStableFirstCandidateOrder(left, right RouteChannelCandidate) int {
	// Compare by lastSelectedAt || lastUsedAt
	lo := left.Channel.LastSelectedAt
	if lo == nil {
		lo = left.Channel.LastUsedAt
	}
	ro := right.Channel.LastSelectedAt
	if ro == nil {
		ro = right.Channel.LastUsedAt
	}
	cmp := CompareNullableTimeAsc(lo, ro)
	if cmp != 0 {
		return cmp
	}
	cmp = CompareNullableTimeAsc(left.Channel.LastUsedAt, right.Channel.LastUsedAt)
	if cmp != 0 {
		return cmp
	}
	return int(left.Channel.ID - right.Channel.ID)
}

var (
	stableFirstLastSelectedSiteByKey        = make(map[string]int64)
	stableFirstObservationProgressByKey     = make(map[string]StableFirstObservationProgressState)
	stableFirstObservationSiteCooldownByKey = make(map[string]int64)
	stableFirstStateMu                      sync.RWMutex
)

const (
	MaxStableFirstRotationKeys                = 1024
	MaxStableFirstObservationProgressKeys     = 1024
	MaxStableFirstObservationSiteCooldownKeys = 4096
)

// StableFirstObservationProgressState tracks observation gating progress.
type StableFirstObservationProgressState struct {
	RequestCount        int64
	LastObservationAtMs *int64
}

func rememberStableFirstSiteSelectionForKey(rotationKey string, siteID int64) {
	if rotationKey == "" || siteID <= 0 {
		return
	}
	stableFirstStateMu.Lock()
	defer stableFirstStateMu.Unlock()
	rememberStableFirstSiteSelectionForKeyLocked(rotationKey, siteID)
}

func rememberStableFirstSiteSelectionForKeyLocked(rotationKey string, siteID int64) {
	delete(stableFirstLastSelectedSiteByKey, rotationKey)
	stableFirstLastSelectedSiteByKey[rotationKey] = siteID
	for len(stableFirstLastSelectedSiteByKey) > MaxStableFirstRotationKeys {
		for k := range stableFirstLastSelectedSiteByKey {
			delete(stableFirstLastSelectedSiteByKey, k)
			break
		}
	}
}

func rememberStableFirstObservationProgressForKeyLocked(rotationKey string, state StableFirstObservationProgressState) {
	delete(stableFirstObservationProgressByKey, rotationKey)
	stableFirstObservationProgressByKey[rotationKey] = state
	for len(stableFirstObservationProgressByKey) > MaxStableFirstObservationProgressKeys {
		for k := range stableFirstObservationProgressByKey {
			delete(stableFirstObservationProgressByKey, k)
			break
		}
	}
}

func rememberStableFirstObservationSiteCooldownLocked(rotationKey string, siteID int64, observedAtMs int64) {
	scopedKey := rotationKey + ":" + strconv.FormatInt(siteID, 10)
	delete(stableFirstObservationSiteCooldownByKey, scopedKey)
	stableFirstObservationSiteCooldownByKey[scopedKey] = observedAtMs
	for len(stableFirstObservationSiteCooldownByKey) > MaxStableFirstObservationSiteCooldownKeys {
		for k := range stableFirstObservationSiteCooldownByKey {
			delete(stableFirstObservationSiteCooldownByKey, k)
			break
		}
	}
}

func getStableFirstSiteOrder(candidates []RouteChannelCandidate, siteID int64) int64 {
	order := int64(1<<63 - 1) // max int64
	for _, c := range candidates {
		if c.Site.ID != siteID {
			continue
		}
		if p := c.Channel.PriorityOrZero(); p < order {
			order = p
		}
	}
	if order == (1<<63 - 1) {
		return 0
	}
	return order
}

func getStableFirstOrderedSiteLeaderIndices(candidates []RouteChannelCandidate, stableSiteLeaderIndices []int) []int {
	sorted := make([]int, len(stableSiteLeaderIndices))
	copy(sorted, stableSiteLeaderIndices)
	sort.SliceStable(sorted, func(a, b int) bool {
		leftSiteID := candidates[sorted[a]].Site.ID
		rightSiteID := candidates[sorted[b]].Site.ID
		ordDiff := getStableFirstSiteOrder(candidates, leftSiteID) - getStableFirstSiteOrder(candidates, rightSiteID)
		if ordDiff != 0 {
			return ordDiff < 0
		}
		return candidates[sorted[a]].Channel.ID < candidates[sorted[b]].Channel.ID
	})
	return sorted
}

func getStableFirstSiteLeaderIndices(candidates []RouteChannelCandidate, contributions []float64, rankedIndices []int) []int {
	if len(rankedIndices) <= 1 {
		return rankedIndices
	}

	// Collect site leaders
	var siteLeaderIndices []int
	seenSiteIDs := make(map[int64]bool)
	for _, idx := range rankedIndices {
		siteID := candidates[idx].Site.ID
		if seenSiteIDs[siteID] {
			continue
		}
		seenSiteIDs[siteID] = true
		siteLeaderIndices = append(siteLeaderIndices, idx)
	}

	if len(siteLeaderIndices) <= 1 {
		return siteLeaderIndices
	}

	// Filter to leaders close to best
	bestIdx := siteLeaderIndices[0]
	if len(rankedIndices) > 0 {
		bestIdx = rankedIndices[0]
	}
	bestContribution := contributions[bestIdx]
	var stableLeaderIndices []int
	for _, idx := range siteLeaderIndices {
		if IsContributionCloseToBest(contributions[idx], bestContribution, StableFirstSiteScoreRatio) {
			stableLeaderIndices = append(stableLeaderIndices, idx)
		}
	}
	if len(stableLeaderIndices) > 0 {
		return stableLeaderIndices
	}
	return siteLeaderIndices
}

const StableFirstSiteScoreRatio = 0.92

func selectStableFirstCandidate(candidates []RouteChannelCandidate, rankedIndices, stableSiteLeaderIndices []int, rotationKey string) *RouteChannelCandidate {
	if len(stableSiteLeaderIndices) == 0 {
		if len(rankedIndices) > 0 {
			return &candidates[rankedIndices[0]]
		}
		return nil
	}

	orderedSiteLeaderIndices := getStableFirstOrderedSiteLeaderIndices(candidates, stableSiteLeaderIndices)

	stableFirstStateMu.RLock()
	lastSelectedSiteID := stableFirstLastSelectedSiteByKey[rotationKey]
	stableFirstStateMu.RUnlock()
	lastSelectedIndex := -1
	for i, idx := range orderedSiteLeaderIndices {
		if candidates[idx].Site.ID == lastSelectedSiteID {
			lastSelectedIndex = i
			break
		}
	}

	nextIndex := 0
	if lastSelectedIndex >= 0 {
		nextIndex = (lastSelectedIndex + 1) % len(orderedSiteLeaderIndices)
	}
	selectedSiteLeader := orderedSiteLeaderIndices[nextIndex]
	selectedSiteID := candidates[selectedSiteLeader].Site.ID

	// Pick the highest-ranked candidate on that site
	for _, idx := range rankedIndices {
		if candidates[idx].Site.ID == selectedSiteID {
			return &candidates[idx]
		}
	}
	return &candidates[selectedSiteLeader]
}

// BuildStableFirstRotationKey creates a rotation key from route ID and model.
func BuildStableFirstRotationKey(routeID int64, requestedModel string) string {
	normalizedModel := NormalizeModelAlias(requestedModel)
	if normalizedModel == "" {
		normalizedModel = NormalizeRouteDisplayName(nil) // won't work, fallback
		if normalizedModel == "" {
			normalizedModel = strconv.FormatInt(routeID, 10)
		}
	}
	return strconv.FormatInt(routeID, 10) + ":" + normalizedModel
}

// ClearStableFirstCachesForRoute clears rotation/progress/cooldown for a route.
func ClearStableFirstCachesForRoute(routeID int64) {
	stableFirstStateMu.Lock()
	defer stableFirstStateMu.Unlock()
	routePrefix := strconv.FormatInt(routeID, 10) + ":"
	for k := range stableFirstLastSelectedSiteByKey {
		if len(k) >= len(routePrefix) && k[:len(routePrefix)] == routePrefix {
			delete(stableFirstLastSelectedSiteByKey, k)
		}
	}
	for k := range stableFirstObservationProgressByKey {
		if len(k) >= len(routePrefix) && k[:len(routePrefix)] == routePrefix {
			delete(stableFirstObservationProgressByKey, k)
		}
	}
	for k := range stableFirstObservationSiteCooldownByKey {
		if len(k) >= len(routePrefix) && k[:len(routePrefix)] == routePrefix {
			delete(stableFirstObservationSiteCooldownByKey, k)
		}
	}
}

// ShouldUseStableFirstObservationCandidate checks if it's time to probe observation pool.
func ShouldUseStableFirstObservationCandidate(rotationKey string, observationCandidates []RouteChannelCandidate) bool {
	if rotationKey == "" || len(observationCandidates) == 0 {
		return false
	}
	stableFirstStateMu.RLock()
	defer stableFirstStateMu.RUnlock()
	state, ok := stableFirstObservationProgressByKey[rotationKey]
	if !ok {
		state = StableFirstObservationProgressState{}
	}
	if (state.RequestCount + 1) < StableFirstObservationRequestInterval {
		return false
	}
	n := nowMs()
	for _, c := range observationCandidates {
		scopedKey := rotationKey + ":" + strconv.FormatInt(c.Site.ID, 10)
		observedAtMs, exists := stableFirstObservationSiteCooldownByKey[scopedKey]
		if !exists || (n-observedAtMs) >= StableFirstObservationSiteCooldownMs {
			return true
		}
	}
	return false
}

// UpdateStableFirstObservationProgress updates observation progress after a selection.
// The full read-modify-write of progress (and optional cooldown) is held under a
// single exclusive lock to avoid TOCTOU races on the shared maps.
func UpdateStableFirstObservationProgress(rotationKey string, usedObservation bool, selectedSiteID int64) {
	if rotationKey == "" {
		return
	}
	n := nowMs()
	stableFirstStateMu.Lock()
	defer stableFirstStateMu.Unlock()

	previous, ok := stableFirstObservationProgressByKey[rotationKey]
	if !ok {
		previous = StableFirstObservationProgressState{}
	}
	if usedObservation {
		rememberStableFirstObservationProgressForKeyLocked(rotationKey, StableFirstObservationProgressState{
			RequestCount:        0,
			LastObservationAtMs: &n,
		})
		if selectedSiteID > 0 {
			rememberStableFirstObservationSiteCooldownLocked(rotationKey, selectedSiteID, n)
		}
		return
	}
	rememberStableFirstObservationProgressForKeyLocked(rotationKey, StableFirstObservationProgressState{
		RequestCount:        previous.RequestCount + 1,
		LastObservationAtMs: previous.LastObservationAtMs,
	})
}
