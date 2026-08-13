package pricing

import (
	"math"
	"testing"
)

func TestMultiplierToUSDPerMillion(t *testing.T) {
	tests := []struct {
		name       string
		multiplier float64
		want       float64
	}{
		{"base ratio", 1, 2},
		{"half ratio", 0.5, 1},
		{"fractional", 2.5, 5},
		{"zero", 0, 0},
		{"negative", -1, 0},
		{"NaN", math.NaN(), 0},
		{"positive inf", math.Inf(1), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MultiplierToUSDPerMillion(tt.multiplier)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("MultiplierToUSDPerMillion(%v) = %v, want %v", tt.multiplier, got, tt.want)
			}
		})
	}
}

func TestNormalizeToProductCanonical_ExplicitPerMillionWins(t *testing.T) {
	in := 3.0
	out := 15.0
	model := NormalizeToProductCanonical(NormalizeInput{
		ModelName:        "gpt-4o",
		InputPerMillion:  &in,
		OutputPerMillion: &out,
		ModelRatio:       toPtr(99.0),
		CompletionRatio:  toPtr(99.0),
	})
	if model.ModelName != "gpt-4o" {
		t.Fatalf("model name = %q", model.ModelName)
	}
	if model.InputPerMillion != 3 || model.OutputPerMillion != 15 {
		t.Fatalf("rates = %v/%v, want 3/15", model.InputPerMillion, model.OutputPerMillion)
	}
}

func TestNormalizeToProductCanonical_Ratios(t *testing.T) {
	mr := 2.5
	cr := 4.0
	model := NormalizeToProductCanonical(NormalizeInput{
		ModelName:       "gpt-x",
		ModelRatio:      &mr,
		CompletionRatio: &cr,
	})
	// input = 2.5 * 2 = 5; output = 2.5 * 4 * 2 = 20
	if model.InputPerMillion != 5 || model.OutputPerMillion != 20 {
		t.Fatalf("rates = %v/%v, want 5/20", model.InputPerMillion, model.OutputPerMillion)
	}
}

func TestNormalizeToProductCanonical_GroupMultiplier(t *testing.T) {
	mr := 2.5
	cr := 4.0
	gr := 0.5
	model := NormalizeToProductCanonical(NormalizeInput{
		ModelName:       "gpt-x",
		ModelRatio:      &mr,
		CompletionRatio: &cr,
		GroupMultiplier: &gr,
	})
	// input = 2.5 * 0.5 * 2 = 2.5; output = 2.5 * 4 * 0.5 * 2 = 10
	if model.InputPerMillion != 2.5 || model.OutputPerMillion != 10 {
		t.Fatalf("rates = %v/%v, want 2.5/10", model.InputPerMillion, model.OutputPerMillion)
	}
}

func TestNormalizeToProductCanonical_DefaultsAndClamps(t *testing.T) {
	model := NormalizeToProductCanonical(NormalizeInput{ModelName: "  bare  "})
	if model.ModelName != "bare" {
		t.Fatalf("model name trimmed = %q", model.ModelName)
	}
	// Defaults: modelRatio=1, completionRatio=1, group=1 → 2/2.
	if model.InputPerMillion != 2 || model.OutputPerMillion != 2 {
		t.Fatalf("default rates = %v/%v, want 2/2", model.InputPerMillion, model.OutputPerMillion)
	}

	neg := -1.0
	clamped := NormalizeToProductCanonical(NormalizeInput{
		ModelName:       "clamped",
		ModelRatio:      &neg,
		CompletionRatio: &neg,
		GroupMultiplier: &neg,
	})
	if clamped.InputPerMillion != 2 || clamped.OutputPerMillion != 2 {
		t.Fatalf("clamped rates = %v/%v, want 2/2 (negative falls back to 1)", clamped.InputPerMillion, clamped.OutputPerMillion)
	}
}

func toPtr(v float64) *float64 {
	return &v
}
