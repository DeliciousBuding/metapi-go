package routing

import (
	"math"
	"testing"
)

func TestSetCacheRatioDefaults_OverrideAndReset(t *testing.T) {
	// Reset to defaults first (no overrides).
	SetCacheRatioDefaults(0, 0, 0, 0)
	if got := DefaultCacheRatioForModel("gpt-4o"); got != DefaultCacheRatio {
		t.Fatalf("non-claude default after reset = %v, want %v", got, DefaultCacheRatio)
	}
	if got := DefaultCacheRatioForModel("claude-3-5-sonnet"); got != ClaudeCacheRatio {
		t.Fatalf("claude default after reset = %v, want %v", got, ClaudeCacheRatio)
	}

	// Apply positive overrides.
	SetCacheRatioDefaults(0.5, 0, 0.2, 0)
	if got := DefaultCacheRatioForModel("gpt-4o"); got != 0.5 {
		t.Fatalf("non-claude override = %v, want 0.5", got)
	}
	if got := DefaultCacheRatioForModel("claude-3-5-sonnet"); got != 0.2 {
		t.Fatalf("claude override = %v, want 0.2", got)
	}

	// Non-positive / NaN / Inf values reset to code defaults.
	SetCacheRatioDefaults(-1, 0, math.NaN(), 0)
	if got := DefaultCacheRatioForModel("gpt-4o"); got != DefaultCacheRatio {
		t.Fatalf("non-claude after bad override = %v, want default %v", got, DefaultCacheRatio)
	}
	if got := DefaultCacheRatioForModel("claude-3-5-sonnet"); got != ClaudeCacheRatio {
		t.Fatalf("claude after bad override = %v, want default %v", got, ClaudeCacheRatio)
	}

	// Explicit per-row ratio still wins over the override (ResolveCacheRatio).
	r := 0.3
	if got := ResolveCacheRatio("gpt-4o", &r); got != 0.3 {
		t.Fatalf("explicit cache ratio should win, got %v want 0.3", got)
	}
}

func TestSetCacheRatioDefaults_ZeroIsFallback(t *testing.T) {
	SetCacheRatioDefaults(0, 0, 0, 0)
	// 0 means "no override" → code default, NOT "free cache read" at the
	// fallback level (explicit 0 in a pricing row is preserved by ResolveCacheRatio).
	if got := DefaultCacheRatioForModel("gpt-4o"); got != DefaultCacheRatio {
		t.Fatalf("0 override should fall back to code default, got %v", got)
	}
}
