package proxyhandler

import "testing"

func TestParseReasoningSuffix(t *testing.T) {
	cases := []struct {
		in       string
		wantBase string
		wantEff  string
	}{
		{"", "", ""},
		{"gpt-4o", "gpt-4o", ""},
		{"gpt-4o-mini", "gpt-4o-mini", ""}, // -mini is not a reasoning suffix
		{"gpt-5-thinking", "gpt-5", "high"},
		{"claude-4-high", "claude-4", "high"},
		{"claude-4-medium", "claude-4", "medium"},
		{"gemini-low", "gemini", "low"},
		{"GPT-5-THINKING", "GPT-5", "high"}, // case-insensitive suffix
		{"o3-reasoning", "o3", "high"},
		{"-thinking", "-thinking", ""}, // empty base → no match (returns original, no effort)
		{"base-high-low", "base-high", "low"}, // last suffix wins
		{"  gpt-5-thinking  ", "gpt-5", "high"}, // trimmed
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			base, eff := ParseReasoningSuffix(c.in)
			if base != c.wantBase || eff != c.wantEff {
				t.Fatalf("ParseReasoningSuffix(%q) = (%q, %q), want (%q, %q)", c.in, base, eff, c.wantBase, c.wantEff)
			}
		})
	}
}
