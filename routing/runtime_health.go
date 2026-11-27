package routing

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sync"
	"time"
)

// ---- Site runtime health constants ----

const (
	SiteRuntimeHealthDecayHalfLifeMs  = 10 * 60 * 1000
	SiteRuntimeMinMultiplier          = 0.08
	SiteRuntimeLatencyBaselineMs      = 2500
	SiteRuntimeLatencyWindowMs        = 30000
	SiteRuntimeMaxLatencyPenalty      = 0.35
	SiteRuntimeLatencyEMAAlpha        = 0.3
	SiteRuntimeBreakerStreakThreshold = 3
	SiteTransientStreakWindowMs       = 5 * 60 * 1000

	SiteRuntimeHealthSettingKey        = "token_router_site_runtime_health_v1"
	SiteRuntimeHealthPersistDebounceMs = 500
	SiteRuntimeHealthPersistStaleTTLMs = 7 * 24 * 60 * 60 * 1000
	SiteRuntimeHealthPersistIdleTTLMs  = 12 * 60 * 60 * 1000
	SiteRuntimeHealthPersistMinPenalty = 0.02
)

// SiteRuntimeBreakerLevelsMs defines breaker durations: [0ms, 60s, 5min, 30min].
var SiteRuntimeBreakerLevelsMs = []int64{0, 60_000, 5 * 60_000, 30 * 60 * 1000}

// SiteRuntimeHalfOpenProbeTimeoutMs bounds the half-open probe window. If the
// admitted recovery probe never records an outcome (process restart, dropped
// request), the breaker releases back to the expired state so the next request
// retries the probe instead of leaving the site isolated forever. Upstream HTTP
// timeouts are far below this bound, so live probes resolve long before it.
const SiteRuntimeHalfOpenProbeTimeoutMs = 10 * 60 * 1000

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
	// HalfOpenSinceMs marks the half-open probe window: non-nil while the single
	// post-cooldown recovery probe is in flight. Cleared when the probe outcome
	// is recorded or the probe window times out.
	HalfOpenSinceMs *int64 `json:"halfOpenSinceMs,omitempty"`
	LastUpdatedAtMs int64  `json:"lastUpdatedAtMs"`
	LastFailureAtMs *int64 `json:"lastFailureAtMs,omitempty"`
	LastSuccessAtMs *int64 `json:"lastSuccessAtMs,omitempty"`
	// Background probe status (). Soft operator signal; never marks keys expired.
	LastProbeAtMs      *int64   `json:"lastProbeAtMs,omitempty"`
	LastProbeStatus    string   `json:"lastProbeStatus,omitempty"` // success | failure | inconclusive
	LastProbeLatencyMs *float64 `json:"lastProbeLatencyMs,omitempty"`
	LastProbeError     *string  `json:"lastProbeError,omitempty"`
	LastProbeModel     string   `json:"lastProbeModel,omitempty"`
	LastProbeChannelID *int64   `json:"lastProbeChannelId,omitempty"`
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

// ---- State management ----

func nowMs() int64 {
	return time.Now().UnixMilli()
}

func refreshRuntimeHealthPenalty(state *SiteRuntimeHealthState) {
	nextPenalty := getDecayedSiteRuntimePenalty(state)
	now := nowMs()
	if nextPenalty != state.PenaltyScore || state.LastUpdatedAtMs != now {
		state.PenaltyScore = nextPenalty
		state.LastUpdatedAtMs = now
	}
}

func newRuntimeHealthState() *SiteRuntimeHealthState {
	n := nowMs()
	return &SiteRuntimeHealthState{LastUpdatedAtMs: n, RecentWindowUpdatedAtMs: n}
}

func getOrCreateSiteRuntimeHealthState(siteID int64) *SiteRuntimeHealthState {
	if state, ok := siteRuntimeHealthStates[siteID]; ok {
		refreshRuntimeHealthPenalty(state)
		return state
	}
	state := newRuntimeHealthState()
	siteRuntimeHealthStates[siteID] = state
	return state
}

func getSiteModelRuntimeHealthState(siteID int64, modelName string) *SiteRuntimeHealthState {
	modelKey := runtimeHealthModelKey(modelName)
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

// maxSiteModelHealthStates bounds the per-site model map. A wildcard route
// with no mapping records the client-supplied model name, so an attacker
// could otherwise create an unbounded number of permanently resident states.
const maxSiteModelHealthStates = 1024

// runtimeHealthModelKey normalizes a model name for the health map and hashes
// pathologically long names so each entry has bounded size.
func runtimeHealthModelKey(modelName string) string {
	key := NormalizeModelAlias(modelName)
	if len(key) > 256 {
		sum := sha256.Sum256([]byte(key))
		return "sha256:" + hex.EncodeToString(sum[:])
	}
	return key
}

// evictOneModelHealthLocked drops an arbitrary entry when the per-site map is
// full. The evicted model simply starts from a neutral health state.
func evictOneModelHealthLocked(modelStates map[string]*SiteRuntimeHealthState) {
	for key := range modelStates {
		delete(modelStates, key)
		return
	}
}

func getOrCreateSiteModelRuntimeHealthState(siteID int64, modelName string) *SiteRuntimeHealthState {
	modelKey := runtimeHealthModelKey(modelName)
	if modelKey == "" {
		return nil
	}
	modelStates, ok := siteModelRuntimeHealthStates[siteID]
	if !ok {
		modelStates = make(map[string]*SiteRuntimeHealthState)
		siteModelRuntimeHealthStates[siteID] = modelStates
	}
	if state, ok := modelStates[modelKey]; ok {
		refreshRuntimeHealthPenalty(state)
		return state
	}
	if len(modelStates) >= maxSiteModelHealthStates {
		evictOneModelHealthLocked(modelStates)
	}
	state := newRuntimeHealthState()
	modelStates[modelKey] = state
	return state
}

func isRuntimeHealthBreakerOpen(state *SiteRuntimeHealthState) bool {
	if state == nil {
		return false
	}
	return state.BreakerUntilMs != nil && *state.BreakerUntilMs > nowMs()
}

// isRuntimeHealthHalfOpenActive reports whether the half-open probe window is
// active: the cooldown elapsed, the single recovery probe was admitted, and its
// outcome has neither been recorded nor timed out yet. Read-only — the stale
// probe window is lazily released by the admission gate (write path).
func isRuntimeHealthHalfOpenActive(state *SiteRuntimeHealthState) bool {
	if state == nil || state.HalfOpenSinceMs == nil {
		return false
	}
	return nowMs()-*state.HalfOpenSinceMs <= SiteRuntimeHalfOpenProbeTimeoutMs
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

// ---- Failure / Success recording ----

func applyRuntimeHealthFailure(state *SiteRuntimeHealthState, ctx SiteRuntimeFailureContext) {
	n := nowMs()
	refreshRecentOutcomeWindow(state)
	state.RecentFailureCount += 1
	state.PenaltyScore += ResolveSiteRuntimeFailurePenalty(ctx)

	if state.HalfOpenSinceMs != nil {
		// The only requests dispatched while half-open are the recovery probe
		// itself, so this outcome is the probe's. A transient failure re-opens
		// the breaker at the next cooldown level (extended backoff); a
		// non-transient failure (client-caused) proves nothing about upstream
		// health, so the breaker stays expired and the next request retries
		// the probe.
		state.HalfOpenSinceMs = nil
		if IsTransientSiteRuntimeFailure(ctx) {
			state.BreakerLevel = min(state.BreakerLevel+1, int64(len(SiteRuntimeBreakerLevelsMs)-1))
			until := n + resolveSiteRuntimeBreakerMs(state.BreakerLevel)
			state.BreakerUntilMs = &until
		}
		state.TransientFailureStreak = 0
		state.LastTransientFailureAtMs = nil
		state.LastFailureAtMs = &n
		return
	}

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
	state.HalfOpenSinceMs = nil
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
