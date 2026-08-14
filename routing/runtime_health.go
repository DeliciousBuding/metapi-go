package routing

import (
	"math"
	"regexp"
	"sync"
	"time"
)

// ---- Site runtime health constants ----

const (
	SiteRuntimeHealthDecayHalfLifeMs   = 10 * 60 * 1000
	SiteRuntimeMinMultiplier           = 0.08
	SiteRuntimeLatencyBaselineMs       = 2500
	SiteRuntimeLatencyWindowMs         = 30000
	SiteRuntimeMaxLatencyPenalty       = 0.35
	SiteRuntimeLatencyEMAAlpha         = 0.3
	SiteRuntimeBreakerStreakThreshold  = 3
	SiteTransientStreakWindowMs        = 5 * 60 * 1000
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

	SiteRuntimeHealthSettingKey        = "token_router_site_runtime_health_v1"
	SiteRuntimeHealthPersistDebounceMs = 500
	SiteRuntimeHealthPersistStaleTTLMs = 7 * 24 * 60 * 60 * 1000
	SiteRuntimeHealthPersistIdleTTLMs  = 12 * 60 * 60 * 1000
	SiteRuntimeHealthPersistMinPenalty = 0.02

	StableFirstPrimarySuccessRateRatio    = 0.92
	StableFirstTrustedRecentConfidence    = 0.5
	StableFirstTrustedHistoricalCalls     = 8
	StableFirstObservationRequestInterval = 24
	StableFirstObservationSiteCooldownMs  = 30 * 60 * 1000
)

// SiteRuntimeBreakerLevelsMs defines breaker durations: [0ms, 60s, 5min, 30min].
var SiteRuntimeBreakerLevelsMs = []int64{0, 60_000, 5 * 60_000, 30 * 60 * 1000}

// ---- Failure classification patterns ----

var siteTransientFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bad\s+gateway`),
	regexp.MustCompile(`(?i)gateway\s+time-?out`),
	regexp.MustCompile(`(?i)service\s+unavailable`),
	regexp.MustCompile(`(?i)temporar(?:y|ily)\s+unavailable`),
	regexp.MustCompile(`(?i)cpu\s+overloaded`),
	regexp.MustCompile(`(?i)overloaded`),
	regexp.MustCompile(`(?i)connection\s+reset`),
	regexp.MustCompile(`(?i)connection\s+refused`),
	regexp.MustCompile(`(?i)econnreset`),
	regexp.MustCompile(`(?i)econnrefused`),
	regexp.MustCompile(`(?i)timeout`),
	regexp.MustCompile(`(?i)timed\s*out`),
}

var siteModelFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)unsupported\s+model`),
	regexp.MustCompile(`(?i)model\s+not\s+supported`),
	regexp.MustCompile(`(?i)does\s+not\s+support(?:\s+the)?\s+model`),
	regexp.MustCompile(`(?i)no\s+such\s+model`),
	regexp.MustCompile(`(?i)unknown\s+model`),
	regexp.MustCompile(`(?i)unknown\s+provider\s+for\s+model`),
	regexp.MustCompile(`(?i)invalid\s+model`),
	regexp.MustCompile(`(?i)model.*does\s+not\s+exist`),
}

var siteProtocolFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)unsupported\s+legacy\s+protocol`),
	regexp.MustCompile(`(?i)please\s+use\s+/v1/responses`),
	regexp.MustCompile(`(?i)please\s+use\s+/v1/messages`),
	regexp.MustCompile(`(?i)please\s+use\s+/v1/chat/completions`),
	regexp.MustCompile(`(?i)does\s+not\s+allow\s+/v1/`),
	regexp.MustCompile(`(?i)unsupported\s+endpoint`),
	regexp.MustCompile(`(?i)unsupported\s+path`),
	regexp.MustCompile(`(?i)unknown\s+endpoint`),
	regexp.MustCompile(`(?i)unrecognized\s+request\s+url`),
	regexp.MustCompile(`(?i)no\s+route\s+matched`),
}

var siteValidationFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)invalid\s+request\s+body`),
	regexp.MustCompile(`(?i)validation`),
	regexp.MustCompile(`(?i)missing\s+required`),
	regexp.MustCompile(`(?i)required\s+parameter`),
	regexp.MustCompile(`(?i)unknown\s+parameter`),
	regexp.MustCompile(`(?i)unrecognized\s+(?:field|key|parameter)`),
	regexp.MustCompile(`(?i)malformed`),
	regexp.MustCompile(`(?i)invalid\s+json`),
	regexp.MustCompile(`(?i)cannot\s+parse`),
	regexp.MustCompile(`(?i)unsupported\s+media\s+type`),
}

var usageLimitRateLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)usage_limit_reached`),
	regexp.MustCompile(`(?i)usage\s+limit\s+has\s+been\s+reached`),
	regexp.MustCompile(`(?i)quota\s+exceeded`),
	regexp.MustCompile(`(?i)rate\s+limit`),
	regexp.MustCompile(`(?i)\blimit\b`),
}

// ---- State types ----

// SiteRuntimeHealthState tracks runtime health for a site or (site, model).
type SiteRuntimeHealthState struct {
	PenaltyScore             float64  `json:"penaltyScore"`
	LatencyEMAMs             *float64 `json:"latencyEmaMs,omitempty"`
	FirstByteEMAMs           *float64 `json:"firstByteEmaMs,omitempty"`
	TransientFailureStreak   int64    `json:"transientFailureStreak"`
	LastTransientFailureAtMs *int64   `json:"lastTransientFailureAtMs,omitempty"`
	RecentSuccessCount       float64  `json:"recentSuccessCount"`
	RecentFailureCount       float64  `json:"recentFailureCount"`
	RecentWindowUpdatedAtMs  int64    `json:"recentWindowUpdatedAtMs"`
	BreakerLevel             int64    `json:"breakerLevel"`
	BreakerUntilMs           *int64   `json:"breakerUntilMs,omitempty"`
	LastUpdatedAtMs          int64    `json:"lastUpdatedAtMs"`
	LastFailureAtMs          *int64   `json:"lastFailureAtMs,omitempty"`
	LastSuccessAtMs          *int64   `json:"lastSuccessAtMs,omitempty"`
	// Background probe status (). Soft operator signal; never marks keys expired.
	LastProbeAtMs      *int64   `json:"lastProbeAtMs,omitempty"`
	LastProbeStatus    string   `json:"lastProbeStatus,omitempty"` // success | failure | inconclusive
	LastProbeLatencyMs *float64 `json:"lastProbeLatencyMs,omitempty"`
	LastProbeError     *string  `json:"lastProbeError,omitempty"`
	LastProbeModel     string   `json:"lastProbeModel,omitempty"`
	LastProbeChannelID *int64   `json:"lastProbeChannelId,omitempty"`
}

// SiteRuntimeHealthDetails is the resolved health for selection.
type SiteRuntimeHealthDetails struct {
	GlobalMultiplier   float64
	ModelMultiplier    float64
	CombinedMultiplier float64
	GlobalBreakerOpen  bool
	ModelBreakerOpen   bool
	ModelKey           string
	RecentSuccessRate  float64
	RecentSampleCount  float64
	RecentConfidence   float64
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

// ---- Global state ----

var (
	siteRuntimeHealthStates      = make(map[int64]*SiteRuntimeHealthState)
	siteModelRuntimeHealthStates = make(map[int64]map[string]*SiteRuntimeHealthState)
	healthStateMu                sync.RWMutex
	siteRuntimeHealthLoaded      bool
	healthPersistInFlight        bool
	healthPersistTimer           *time.Timer
)

// ---- Classification helpers ----

func matchesAnyPattern(patterns []*regexp.Regexp, text string) bool {
	if text == "" {
		return false
	}
	for _, p := range patterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

// IsUsageLimitRateLimitFailure checks if a 429 is from a usage/rate limit.
func IsUsageLimitRateLimitFailure(ctx SiteRuntimeFailureContext) bool {
	status := 0
	if ctx.Status != nil {
		status = *ctx.Status
	}
	if status != 429 {
		return false
	}
	errorText := ""
	if ctx.ErrorText != nil {
		errorText = *ctx.ErrorText
	}
	return matchesAnyPattern(usageLimitRateLimitPatterns, errorText)
}

func isModelScopedRuntimeFailure(ctx SiteRuntimeFailureContext) bool {
	text := ""
	if ctx.ErrorText != nil {
		text = *ctx.ErrorText
	}
	return matchesAnyPattern(siteModelFailurePatterns, text)
}

func isProtocolRuntimeFailure(ctx SiteRuntimeFailureContext) bool {
	text := ""
	if ctx.ErrorText != nil {
		text = *ctx.ErrorText
	}
	return matchesAnyPattern(siteProtocolFailurePatterns, text)
}

func isValidationRuntimeFailure(ctx SiteRuntimeFailureContext) bool {
	text := ""
	if ctx.ErrorText != nil {
		text = *ctx.ErrorText
	}
	return matchesAnyPattern(siteValidationFailurePatterns, text)
}

// ResolveSiteRuntimeFailurePenalty assigns a penalty score based on failure context.
func ResolveSiteRuntimeFailurePenalty(ctx SiteRuntimeFailureContext) float64 {
	status := 0
	if ctx.Status != nil {
		status = *ctx.Status
	}
	errorText := ""
	if ctx.ErrorText != nil {
		errorText = *ctx.ErrorText
	}

	if IsUsageLimitRateLimitFailure(ctx) {
		return 0.4
	}
	if isModelScopedRuntimeFailure(ctx) {
		return 0.9
	}
	if isProtocolRuntimeFailure(ctx) {
		return 0.6
	}
	if isValidationRuntimeFailure(ctx) {
		return 0.25
	}
	if status >= 500 || matchesAnyPattern(siteTransientFailurePatterns, errorText) {
		return 2.5
	}
	if status == 429 {
		return 2.2
	}
	if status == 401 || status == 403 {
		return 1.8
	}
	if status >= 400 && status < 500 {
		return 0.9
	}
	return 1.2
}

// IsTransientSiteRuntimeFailure checks if a failure is transient.
func IsTransientSiteRuntimeFailure(ctx SiteRuntimeFailureContext) bool {
	status := 0
	if ctx.Status != nil {
		status = *ctx.Status
	}
	errorText := ""
	if ctx.ErrorText != nil {
		errorText = *ctx.ErrorText
	}

	if IsUsageLimitRateLimitFailure(ctx) {
		return false
	}
	if isModelScopedRuntimeFailure(ctx) {
		return false
	}
	if isProtocolRuntimeFailure(ctx) {
		return false
	}
	if isValidationRuntimeFailure(ctx) {
		return false
	}
	return status >= 500 || status == 429 || matchesAnyPattern(siteTransientFailurePatterns, errorText)
}

// ---- State management ----

func nowMs() int64 {
	return time.Now().UnixMilli()
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

func getOrCreateRuntimeHealthState(states map[int64]*SiteRuntimeHealthState, siteID int64) *SiteRuntimeHealthState {
	if state, ok := states[siteID]; ok {
		nextPenalty := getDecayedSiteRuntimePenalty(state)
		now := nowMs()
		if nextPenalty != state.PenaltyScore || state.LastUpdatedAtMs != now {
			state.PenaltyScore = nextPenalty
			state.LastUpdatedAtMs = now
		}
		return state
	}
	n := nowMs()
	s := &SiteRuntimeHealthState{
		PenaltyScore:             0,
		LatencyEMAMs:             nil,
		TransientFailureStreak:   0,
		LastTransientFailureAtMs: nil,
		RecentSuccessCount:       0,
		RecentFailureCount:       0,
		RecentWindowUpdatedAtMs:  n,
		BreakerLevel:             0,
		BreakerUntilMs:           nil,
		LastUpdatedAtMs:          n,
		LastFailureAtMs:          nil,
		LastSuccessAtMs:          nil,
	}
	states[siteID] = s
	return s
}

func getOrCreateSiteRuntimeHealthState(siteID int64) *SiteRuntimeHealthState {
	return getOrCreateRuntimeHealthState(siteRuntimeHealthStates, siteID)
}

func getSiteModelRuntimeHealthState(siteID int64, modelName string) *SiteRuntimeHealthState {
	modelKey := NormalizeModelAlias(modelName)
	if modelKey == "" {
		return nil
	}
	if modelStates, ok := siteModelRuntimeHealthStates[siteID]; ok {
		if state, ok := modelStates[modelKey]; ok {
			return state
		}
	}
	return nil
}

func getOrCreateSiteModelRuntimeHealthState(siteID int64, modelName string) *SiteRuntimeHealthState {
	modelKey := NormalizeModelAlias(modelName)
	if modelKey == "" {
		return nil
	}
	modelStates, ok := siteModelRuntimeHealthStates[siteID]
	if !ok {
		modelStates = make(map[string]*SiteRuntimeHealthState)
		siteModelRuntimeHealthStates[siteID] = modelStates
	}
	state, ok := modelStates[modelKey]
	if ok {
		nextPenalty := getDecayedSiteRuntimePenalty(state)
		now := nowMs()
		if nextPenalty != state.PenaltyScore || state.LastUpdatedAtMs != now {
			state.PenaltyScore = nextPenalty
			state.LastUpdatedAtMs = now
		}
		return state
	}
	n := nowMs()
	s := &SiteRuntimeHealthState{
		PenaltyScore:             0,
		LatencyEMAMs:             nil,
		TransientFailureStreak:   0,
		LastTransientFailureAtMs: nil,
		RecentSuccessCount:       0,
		RecentFailureCount:       0,
		RecentWindowUpdatedAtMs:  n,
		BreakerLevel:             0,
		BreakerUntilMs:           nil,
		LastUpdatedAtMs:          n,
		LastFailureAtMs:          nil,
		LastSuccessAtMs:          nil,
	}
	modelStates[modelKey] = s
	return s
}

func isRuntimeHealthBreakerOpen(state *SiteRuntimeHealthState) bool {
	if state == nil {
		return false
	}
	return state.BreakerUntilMs != nil && *state.BreakerUntilMs > nowMs()
}

func resolveSiteRuntimeBreakerMs(level int64) int64 {
	if level < 0 {
		level = 0
	}
	if level >= int64(len(SiteRuntimeBreakerLevelsMs)) {
		level = int64(len(SiteRuntimeBreakerLevelsMs) - 1)
	}
	return SiteRuntimeBreakerLevelsMs[level]
}

// GetRuntimeHealthMultiplier returns the health multiplier for a state.
func GetRuntimeHealthMultiplier(state *SiteRuntimeHealthState) float64 {
	if state == nil {
		return 1
	}
	if isRuntimeHealthBreakerOpen(state) {
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
		ModelKey:           modelKey,
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

// ---- Failure / Success recording ----

func applyRuntimeHealthFailure(state *SiteRuntimeHealthState, ctx SiteRuntimeFailureContext) {
	n := nowMs()
	refreshRecentOutcomeWindow(state)
	state.RecentFailureCount += 1
	state.PenaltyScore += ResolveSiteRuntimeFailurePenalty(ctx)

	if IsTransientSiteRuntimeFailure(ctx) {
		if state.LastTransientFailureAtMs != nil && (n-*state.LastTransientFailureAtMs) <= SiteTransientStreakWindowMs {
			state.TransientFailureStreak += 1
		} else {
			state.TransientFailureStreak = 1
		}
		state.LastTransientFailureAtMs = &n
		if state.TransientFailureStreak >= SiteRuntimeBreakerStreakThreshold {
			state.BreakerLevel = min(state.BreakerLevel+1, int64(len(SiteRuntimeBreakerLevelsMs)-1))
			breakerMs := resolveSiteRuntimeBreakerMs(state.BreakerLevel)
			if breakerMs > 0 {
				until := n + breakerMs
				state.BreakerUntilMs = &until
			} else {
				state.BreakerUntilMs = nil
			}
			state.TransientFailureStreak = 0
		}
	} else {
		state.TransientFailureStreak = 0
		state.LastTransientFailureAtMs = nil
	}
	state.LastFailureAtMs = &n
}

func applyRuntimeHealthSuccess(state *SiteRuntimeHealthState, latencyMs float64, firstByteMs ...float64) {
	n := nowMs()
	refreshRecentOutcomeWindow(state)
	state.RecentSuccessCount += 1
	state.PenaltyScore = math.Max(0, state.PenaltyScore*0.2-0.3)
	state.TransientFailureStreak = 0
	state.LastTransientFailureAtMs = nil
	state.BreakerLevel = 0
	state.BreakerUntilMs = nil
	state.LastSuccessAtMs = &n

	if state.LatencyEMAMs == nil {
		state.LatencyEMAMs = &latencyMs
	} else {
		ema := (*state.LatencyEMAMs)*(1-SiteRuntimeLatencyEMAAlpha) + latencyMs*SiteRuntimeLatencyEMAAlpha
		state.LatencyEMAMs = &ema
	}
	if len(firstByteMs) > 0 && firstByteMs[0] > 0 {
		fb := firstByteMs[0]
		if state.FirstByteEMAMs == nil {
			state.FirstByteEMAMs = &fb
		} else {
			ema := (*state.FirstByteEMAMs)*(1-SiteRuntimeLatencyEMAAlpha) + fb*SiteRuntimeLatencyEMAAlpha
			state.FirstByteEMAMs = &ema
		}
	}
}

// RecordSiteRuntimeFailure records a failure against a site and model.
func RecordSiteRuntimeFailure(siteID int64, ctx SiteRuntimeFailureContext) {
	healthStateMu.Lock()
	defer healthStateMu.Unlock()

	globalState := getOrCreateSiteRuntimeHealthState(siteID)
	applyRuntimeHealthFailure(globalState, ctx)

	if ctx.ModelName != nil && *ctx.ModelName != "" {
		modelState := getOrCreateSiteModelRuntimeHealthState(siteID, *ctx.ModelName)
		if modelState != nil {
			applyRuntimeHealthFailure(modelState, ctx)
		}
	}
	scheduleSiteRuntimeHealthPersistence()
}

// RecordSiteRuntimeSuccess records a success against a site and model.
func RecordSiteRuntimeSuccess(siteID int64, latencyMs float64, modelName *string, firstByteMs ...float64) {
	healthStateMu.Lock()
	defer healthStateMu.Unlock()

	globalState := getOrCreateSiteRuntimeHealthState(siteID)
	applyRuntimeHealthSuccess(globalState, latencyMs, firstByteMs...)

	if modelName != nil && *modelName != "" {
		modelState := getOrCreateSiteModelRuntimeHealthState(siteID, *modelName)
		if modelState != nil {
			applyRuntimeHealthSuccess(modelState, latencyMs, firstByteMs...)
		}
	}
	scheduleSiteRuntimeHealthPersistence()
}

// ProbeStatus is the operator-visible background probe outcome for a site.
// ---- Historical health ----

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
		sc := c.Channel.SuccessCount
		fc := c.Channel.FailCount
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
			st.totalLatencyMs += max(0, c.Channel.TotalLatencyMs)
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

