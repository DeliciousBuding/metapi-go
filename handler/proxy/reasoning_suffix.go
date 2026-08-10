package proxyhandler

import "strings"

// Reasoning suffix → OpenAI reasoning_effort mapping.

// A downstream client may request a reasoning variant by appending a suffix
// to the model name, e.g. "gpt-5-thinking" / "claude-4-high" / "gemini-low".
// ParseReasoningSuffix splits the suffix off so routing matches the base
// model, and returns the OpenAI reasoning_effort level to inject.

// Returns effort == "" when no known suffix is present (the model name is
// returned unchanged). The base is always non-empty when a suffix matched.

// Injection is OpenAI-surface only (reasoning_effort is an OpenAI field);
// Anthropic (thinking) / Gemini (thinkingConfig) dialects strip for routing
// but do not inject here — cross-dialect injection is a documented follow-up.
func ParseReasoningSuffix(model string) (base string, effort string) {
	m := strings.TrimSpace(model)
	if m == "" {
		return m, ""
	}
	// Order matters: check longer/more-specific suffixes first.
	suffixes := []struct {
		suf    string
		effort string
	}{
		{"-thinking", "high"},
		{"-reasoning", "high"},
		{"-high", "high"},
		{"-medium", "medium"},
		{"-low", "low"},
	}
	lower := strings.ToLower(m)
	for _, s := range suffixes {
		if strings.HasSuffix(lower, s.suf) {
			base = strings.TrimSpace(m[:len(m)-len(s.suf)])
			if base == "" {
				return m, ""
			}
			return base, s.effort
		}
	}
	return m, ""
}
