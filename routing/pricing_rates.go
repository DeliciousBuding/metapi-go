package routing

import (
	"math"
	"strings"
	"sync/atomic"
)

// Claude / Anthropic official prompt-cache multipliers used when an upstream
// pricing payload omits cache_ratio / cache_creation_ratio for Claude models.

// Source of intent: cita-777/metapi (AnyRouter /api/pricing often lacks
// these fields and the historical normalizeRatio(undefined, 1) fallback made
// cache_read == input and cache_creation == input). Anthropic public pricing
// ratios relative to input are:

//	cache read = 0.10 × input
//	cache creation = 1.25 × input

// Non-Claude models keep the original Metapi fallback of 1.0 so we do not
// invent vendor-specific multipliers without evidence.
const (
	cacheWrite1hWeight  = 1.6
	cacheWrite12hWeight = 2
	// DefaultCacheRatio is the historical Metapi fallback when cache_ratio is
	// missing for non-Claude models (input × 1.0).
	DefaultCacheRatio = 1.0
	// DefaultCacheCreationRatio is the historical Metapi fallback when
	// cache_creation_ratio is missing for non-Claude models.
	DefaultCacheCreationRatio = 1.0

	// ClaudeCacheRatio is Anthropic's cache-read multiplier (input × 0.1).
	ClaudeCacheRatio = 0.1
	// ClaudeCacheCreationRatio is Anthropic's cache-write / creation
	// multiplier (input × 1.25).
	ClaudeCacheCreationRatio = 1.25

	// oneHubPerCallRatio mirrors TS ONE_HUB_PER_CALL_RATIO for times pricing.
	oneHubPerCallRatio = 0.002
)

// Admin-overridable cache-ratio fallbacks. The consts above remain the
// code defaults; an operator can tune these via settings (keys
// cache_ratio_default / cache_ratio_claude) without a code change. NaN/Inf
// and non-positive overrides are ignored (fall back to the const).
var (
	cacheRatioOverride               atomic.Pointer[float64]
	cacheCreationRatioOverride       atomic.Pointer[float64]
	claudeCacheRatioOverride         atomic.Pointer[float64]
	claudeCacheCreationRatioOverride atomic.Pointer[float64]
)

// SetCacheRatioDefaults overrides the cache-ratio fallbacks at runtime.
// Any value <= 0, NaN, or Inf resets that ratio to its code default.
// Called by the admin settings handler when cache_ratio_* settings change.
func SetCacheRatioDefaults(defaultRatio, defaultCreation, claudeRatio, claudeCreation float64) {
	set := func(p *atomic.Pointer[float64], v, fallback float64) {
		if !(v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)) {
			p.Store(nil)
			return
		}
		vv := v
		p.Store(&vv)
	}
	set(&cacheRatioOverride, defaultRatio, DefaultCacheRatio)
	set(&cacheCreationRatioOverride, defaultCreation, DefaultCacheCreationRatio)
	set(&claudeCacheRatioOverride, claudeRatio, ClaudeCacheRatio)
	set(&claudeCacheCreationRatioOverride, claudeCreation, ClaudeCacheCreationRatio)
}

func overriddenOr(p *atomic.Pointer[float64], fallback float64) float64 {
	if v := p.Load(); v != nil {
		return *v
	}
	return fallback
}

// IsClaudeModel reports whether modelName belongs to the Claude / Anthropic
// family. Matching is case-insensitive substring on "claude".
func IsClaudeModel(modelName string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(modelName)), "claude")
}

// DefaultCacheRatioForModel returns the intentional cache_ratio fallback when
// an upstream pricing row omits the field. Claude models use Anthropic public
// ratios; everything else keeps the historical Metapi 1.0 fallback. Both are
// admin-overridable via SetCacheRatioDefaults.
func DefaultCacheRatioForModel(modelName string) float64 {
	if IsClaudeModel(modelName) {
		return overriddenOr(&claudeCacheRatioOverride, ClaudeCacheRatio)
	}
	return overriddenOr(&cacheRatioOverride, DefaultCacheRatio)
}

// DefaultCacheCreationRatioForModel returns the intentional
// cache_creation_ratio fallback when the field is missing.
func DefaultCacheCreationRatioForModel(modelName string) float64 {
	if IsClaudeModel(modelName) {
		return overriddenOr(&claudeCacheCreationRatioOverride, ClaudeCacheCreationRatio)
	}
	return overriddenOr(&cacheCreationRatioOverride, DefaultCacheCreationRatio)
}

// ResolveCacheRatio picks an explicit ratio when present and finite (>= 0),
// otherwise the model-family default. Explicit 0 is preserved (free cache read).
func ResolveCacheRatio(modelName string, cacheRatio *float64) float64 {
	if cacheRatio != nil && isFiniteNonNeg(*cacheRatio) {
		return *cacheRatio
	}
	return DefaultCacheRatioForModel(modelName)
}

// ResolveCacheCreationRatio picks an explicit creation ratio when present,
// otherwise the model-family default. Explicit 0 is preserved.
func ResolveCacheCreationRatio(modelName string, cacheCreationRatio *float64) float64 {
	if cacheCreationRatio != nil && isFiniteNonNeg(*cacheCreationRatio) {
		return *cacheCreationRatio
	}
	return DefaultCacheCreationRatioForModel(modelName)
}

// CacheAwarePerMillionRates derives input/output/cache $/M rates for catalog
// display and routing reference helpers. Uses Claude-aware cache defaults.
func CacheAwarePerMillionRates(model PricingModel, groupMultiplier float64) (input, output, cacheRead, cacheCreation float64) {
	if groupMultiplier <= 0 || !isFiniteFloat(groupMultiplier) {
		groupMultiplier = 1
	}
	modelRatio := model.ModelRatio
	if modelRatio <= 0 || !isFiniteFloat(modelRatio) {
		modelRatio = 1
	}
	completionRatio := model.CompletionRatio
	if completionRatio <= 0 || !isFiniteFloat(completionRatio) {
		completionRatio = 1
	}
	cacheRatio := ResolveCacheRatio(model.ModelName, model.CacheRatio)
	cacheCreationRatio := ResolveCacheCreationRatio(model.ModelName, model.CacheCreationRatio)

	input = roundCost(modelRatio * 2 * groupMultiplier)
	output = roundCost(modelRatio * completionRatio * 2 * groupMultiplier)
	cacheRead = roundCost(modelRatio * cacheRatio * 2 * groupMultiplier)
	cacheCreation = roundCost(modelRatio * cacheCreationRatio * 2 * groupMultiplier)
	return
}

func isFiniteNonNeg(v float64) bool {
	return isFiniteFloat(v) && v >= 0
}
