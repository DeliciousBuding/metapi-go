package routing

import (
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestParseStatusRanges_ValidSpecs(t *testing.T) {
	tests := []struct {
		spec string
		want []StatusRange
	}{
		{"", nil},
		{"   ", nil},
		{"401", []StatusRange{{Lo: 401, Hi: 401}}},
		{"500-599", []StatusRange{{Lo: 500, Hi: 599}}},
		{"429,500-599", []StatusRange{{Lo: 429, Hi: 429}, {Lo: 500, Hi: 599}}},
		{" 500 - 599 ", []StatusRange{{Lo: 500, Hi: 599}}},
		// Adjacent ranges merge into one interval.
		{"499,500-599", []StatusRange{{Lo: 499, Hi: 599}}},
		// Overlapping ranges merge.
		{"500-550,525-599", []StatusRange{{Lo: 500, Hi: 599}}},
		// Out-of-order entries are sorted.
		{"500-599,401", []StatusRange{{Lo: 401, Hi: 401}, {Lo: 500, Hi: 599}}},
	}
	for _, tt := range tests {
		got, err := ParseStatusRanges(tt.spec)
		if err != nil {
			t.Fatalf("ParseStatusRanges(%q) unexpected error: %v", tt.spec, err)
		}
		if len(got) != len(tt.want) {
			t.Fatalf("ParseStatusRanges(%q) = %v, want %v", tt.spec, got, tt.want)
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("ParseStatusRanges(%q) = %v, want %v", tt.spec, got, tt.want)
			}
		}
	}
}

func TestParseStatusRanges_RejectsInvalidSpecs(t *testing.T) {
	invalid := []string{
		"abc",
		"4xx",
		"99",
		"600",
		"500-400",
		"-500",
		"500-",
		"500--599",
		"401,,429",
		"500,abc,600",
		"99-199",
		"500-600",
		"1-599",
	}
	for _, spec := range invalid {
		if _, err := ParseStatusRanges(spec); err == nil {
			t.Errorf("ParseStatusRanges(%q) expected error, got nil", spec)
		}
	}
}

func TestStatusInRanges(t *testing.T) {
	ranges := []StatusRange{{Lo: 401, Hi: 401}, {Lo: 500, Hi: 599}}
	for _, status := range []int{401, 500, 550, 599} {
		if !StatusInRanges(status, ranges) {
			t.Errorf("StatusInRanges(%d) = false, want true", status)
		}
	}
	for _, status := range []int{400, 402, 499, 600, 0} {
		if StatusInRanges(status, ranges) {
			t.Errorf("StatusInRanges(%d) = true, want false", status)
		}
	}
	if StatusInRanges(500, nil) {
		t.Error("StatusInRanges with nil ranges = true, want false")
	}
}

func TestActiveRetryStatusRanges_FallsBackToDefaults(t *testing.T) {
	prev := config.RuntimeSafe()
	defer config.SetRuntime(prev)

	config.SetRuntime(nil)
	got := ActiveRetryStatusRanges()
	want, err := ParseStatusRanges(DefaultRetryStatusRangesSpec)
	if err != nil || len(got) != len(want) {
		t.Fatalf("nil config: got %v (err %v), want default %v", got, err, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("nil config: got %v, want %v", got, want)
		}
	}

	// Blank config field also falls back to the historical default.
	config.SetRuntime(&config.RuntimeSettings{})
	got = ActiveRetryStatusRanges()
	if len(got) == 0 || got[0] != (StatusRange{Lo: 401, Hi: 401}) {
		t.Fatalf("blank config: got %v, want default ranges", got)
	}

	// Invalid persisted value must not poison the hot path: fall back.
	config.SetRuntime(&config.RuntimeSettings{ProxyRetryStatusRanges: "not-a-range"})
	got = ActiveRetryStatusRanges()
	if len(got) == 0 {
		t.Fatal("invalid spec: got empty ranges, want default fallback")
	}
}

func TestActiveRetryStatusRanges_OperatorOverride(t *testing.T) {
	prev := config.RuntimeSafe()
	defer config.SetRuntime(prev)

	config.SetRuntime(&config.RuntimeSettings{ProxyRetryStatusRanges: "502,504"})
	got := ActiveRetryStatusRanges()
	if len(got) != 2 {
		t.Fatalf("override: got %v, want two single-code ranges", got)
	}
	if !StatusInRanges(502, got) || !StatusInRanges(504, got) {
		t.Fatalf("override: ranges %v must match 502/504", got)
	}
	if StatusInRanges(500, got) {
		t.Fatalf("override: ranges %v must not match 500", got)
	}
}

func TestActiveDisableStatusRanges_DefaultNone(t *testing.T) {
	prev := config.RuntimeSafe()
	defer config.SetRuntime(prev)

	config.SetRuntime(nil)
	if got := ActiveDisableStatusRanges(); len(got) != 0 {
		t.Fatalf("default disable ranges = %v, want none", got)
	}
	config.SetRuntime(&config.RuntimeSettings{ProxyDisableStatusRanges: "401"})
	got := ActiveDisableStatusRanges()
	if len(got) != 1 || got[0] != (StatusRange{Lo: 401, Hi: 401}) {
		t.Fatalf("disable override = %v, want [401]", got)
	}
}

func TestDefaultSpecsAreWellFormed(t *testing.T) {
	if _, err := ParseStatusRanges(DefaultRetryStatusRangesSpec); err != nil {
		t.Fatalf("default retry spec unparsable: %v", err)
	}
	if _, err := ParseStatusRanges(DefaultDisableStatusRangesSpec); err != nil {
		t.Fatalf("default disable spec unparsable: %v", err)
	}
	if !strings.Contains(DefaultRetryStatusRangesSpec, "500-599") {
		t.Fatal("default retry spec must keep the 5xx sweep")
	}
}
