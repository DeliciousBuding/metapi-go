package proxy

import (
	"testing"
	"time"
)

func TestRequestCeiling_DefaultsToNinetySeconds(t *testing.T) {
	for _, sec := range []int{0, -5} {
		if got := RequestCeiling(sec); got != DefaultRequestCeiling {
			t.Errorf("RequestCeiling(%d) = %v, want %v", sec, got, DefaultRequestCeiling)
		}
	}
}

func TestRequestCeiling_DoublesFirstByteWindowOnlyAboveDefault(t *testing.T) {
	cases := []struct {
		sec  int
		want time.Duration
	}{
		{15, DefaultRequestCeiling}, // 30s < 90s: default wins
		{45, DefaultRequestCeiling}, // 90s == 90s: default wins
		{60, 120 * time.Second},     // 120s > 90s: doubled window wins
		{300, 600 * time.Second},    // multi-endpoint fallback keeps headroom
	}
	for _, tc := range cases {
		if got := RequestCeiling(tc.sec); got != tc.want {
			t.Errorf("RequestCeiling(%d) = %v, want %v", tc.sec, got, tc.want)
		}
	}
}

// TestWriteBudget_NeverInvertsRequestCeiling locks the audit T1 invariant: the
// server-side write budget must always be at least the whole-request ceiling,
// otherwise a buffered response arriving late is killed while being written.
func TestWriteBudget_NeverInvertsRequestCeiling(t *testing.T) {
	for _, sec := range []int{-1, 0, 15, 45, 60, 120, 600, 3600} {
		budget, ceiling := WriteBudget(sec), RequestCeiling(sec)
		if budget <= ceiling {
			t.Fatalf("WriteBudget(%d) = %v must exceed RequestCeiling = %v", sec, budget, ceiling)
		}
		if budget > ceiling+time.Hour {
			t.Fatalf("WriteBudget(%d) = %v is unreasonably far above ceiling %v", sec, budget, ceiling)
		}
	}
}
