package routing

// Deep-testing probe: edge cases for the model matcher / mapping / usage
// normalization. These probes target boundary conditions that the existing
// matcher_test.go does not cover: mixed-case regex prefixes, multi-byte
// glob wildcards, JSON-escape fidelity in the hand-rolled mapping parser,
// and int64 overflow in usage normalization.

import (
	"math"
	"testing"
)

// IsRegexModelPattern treats the prefix case-insensitively (it lowercases
// before checking), so ParseRegexModelPattern must strip the prefix for every
// accepted casing. Otherwise a pattern like "Re:^gpt" compiles as the literal
// regex "Re:^gpt" and can never match any real model name.
func TestProbe_ParseRegexModelPattern_MixedCasePrefix(t *testing.T) {
	cases := []string{"re:^gpt", "RE:^gpt", "Re:^gpt", "rE:^gpt"}
	for _, pattern := range cases {
		if !IsRegexModelPattern(pattern) {
			t.Fatalf("IsRegexModelPattern(%q) = false, want true", pattern)
		}
		if !MatchesModelPattern("gpt-4o", pattern) {
			t.Errorf("MatchesModelPattern(%q, %q) = false, want true (prefix casing should not change semantics)", "gpt-4o", pattern)
		}
	}
}

// globMatch is implemented over bytes, so a single "?" wildcard only consumes
// one byte of a multi-byte UTF-8 rune. Model names with non-ASCII characters
// (legal in user-supplied patterns) then fail to match even though the
// rune-level reading of the pattern clearly should match.
func TestProbe_GlobMatch_QuestionMarkShouldMatchWholeRune(t *testing.T) {
	cases := []struct {
		pattern string
		value   string
	}{
		{"模型?", "模型甲"},
		{"modèle-?", "modèle-x"},
		{"?-model", "甲-model"},
	}
	for _, tc := range cases {
		if !MatchesModelPattern(tc.value, tc.pattern) {
			t.Errorf("MatchesModelPattern(%q, %q) = false, want true ('?' should match one rune, not one byte)", tc.value, tc.pattern)
		}
	}
}

// ParseModelMappingRecord hand-rolls a JSON parser. Standard JSON escape
// sequences beyond the handful of ReplaceAll cases must still round-trip:
// \uXXXX is valid JSON and encoding/json decodes it, so mappings authored
// with unicode escapes should resolve to the same keys/values.
func TestProbe_ParseModelMappingRecord_UnicodeEscape(t *testing.T) {
	// {"gpt-4":"\u0067pt-4o"} — value decodes to "gpt-4o" under encoding/json.
	raw := `{"gpt-4":"\u0067pt-4o"}`
	parsed := ParseModelMappingRecord(&raw)
	if parsed == nil {
		t.Fatalf("ParseModelMappingRecord returned nil for valid JSON %q", raw)
	}
	if got := parsed["gpt-4"]; got != "gpt-4o" {
		t.Errorf(`ParseModelMappingRecord(%q)["gpt-4"] = %q, want "gpt-4o" (\uXXXX escape not decoded)`, raw, got)
	}
}

// Escaped quotes inside keys/values must survive the hand-rolled unquoting.
func TestProbe_ParseModelMappingRecord_EscapedQuote(t *testing.T) {
	raw := `{"a\"b":"x\"y"}`
	parsed := ParseModelMappingRecord(&raw)
	if parsed == nil {
		t.Fatalf("ParseModelMappingRecord returned nil for valid JSON %q", raw)
	}
	if got, ok := parsed[`a"b`]; !ok || got != `x"y` {
		t.Errorf(`ParseModelMappingRecord(%q) = %#v, want key a"b -> x"y`, raw, parsed)
	}
}

// normalizeUsageBreakdownInput repairs totalTokens when it is smaller than
// prompt+completion. With adversarial upstream usage values near MaxInt64 the
// prompt+completion addition overflows int64, silently disabling the repair.
// The normalized total must never be smaller than either component.
func TestProbe_UsageBreakdown_TokenCountOverflow(t *testing.T) {
	model := PricingModel{ModelName: "probe-model", QuotaType: 0, ModelRatio: 1, CompletionRatio: 1}
	detail := CalculateModelUsageBreakdown(model, UsageForCost{
		PromptTokens:     math.MaxInt64,
		CompletionTokens: 1,
		TotalTokens:      0,
	}, nil)
	if detail == nil {
		t.Fatalf("CalculateModelUsageBreakdown returned nil for token-quota model")
	}
	total := detail.Usage.TotalTokens
	if total < detail.Usage.PromptTokens || total < detail.Usage.CompletionTokens {
		t.Errorf("normalized TotalTokens = %d, smaller than prompt (%d) or completion (%d): int64 overflow disabled the total repair",
			total, detail.Usage.PromptTokens, detail.Usage.CompletionTokens)
	}
}
