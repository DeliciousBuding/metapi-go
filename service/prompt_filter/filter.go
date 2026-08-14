// Package promptfilter implements a lightweight, pattern-based prompt safety
// filter that blocks high-risk prompts before they reach shared OAuth upstream
// accounts (Codex, Claude, Gemini CLI, Grok). This is a first line of defense
// only — pure substring/regex matching, no AI classification.
//
// The filter is gated by config.PromptFilterEnabled (env PROMPT_FILTER_ENABLED,
// default false). When enabled, the proxy chat/responses handlers call Check
// on the parsed request body before forwarding upstream; a blocked request
// returns 403 with type "safety_filter".
//
// Privacy: Check never logs the prompt content — callers only log the matched
// pattern name. Performance: regexes are compiled once at construction; Check
// stays well under 1ms for the seed list (~20 patterns) on typical prompts.
package promptfilter

import (
	"fmt"
	"regexp"
	"strings"
)

// Filter is an immutable, goroutine-safe prompt safety filter. Construct once
// (via NewFilter) and reuse across requests.
type Filter struct {
	patterns        []DenyPattern
	repeatedRunLimit int
}

// NewFilter builds a Filter from the seed deny list plus optional extra
// substring patterns (typically sourced from PROMPT_FILTER_DENY_PATTERNS at
// runtime). Extra patterns are treated as case-insensitive substring rules.
// Returns an error if any regex pattern fails to compile.
func NewFilter(extraSubstrings []string) (*Filter, error) {
	patterns := make([]DenyPattern, 0, len(seedPatterns)+len(extraSubstrings))

	// Compile the seed list once.
	for _, seed := range seedPatterns {
		dp, err := compileDenyPattern(seed)
		if err != nil {
			return nil, fmt.Errorf("prompt_filter: seed pattern %q: %w", seed.Name, err)
		}
		patterns = append(patterns, dp)
	}

	// Append runtime extras as substring rules. Skip empty/duplicates of the
	// seed list to keep the hot path short.
	for _, raw := range extraSubstrings {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		name := "runtime_" + sanitizePatternName(trimmed)
		dp, err := compileDenyPattern(DenyPattern{
			Name: name,
			Kind: PatternSubstring,
			Raw:  trimmed,
		})
		if err != nil {
			return nil, fmt.Errorf("prompt_filter: runtime pattern %q: %w", trimmed, err)
		}
		patterns = append(patterns, dp)
	}

	return &Filter{
		patterns:        patterns,
		repeatedRunLimit: maxRepeatedRunThreshold,
	}, nil
}

// compileDenyPattern precomputes the lowercased substring or compiles the
// case-insensitive regex so the hot path is allocation-free.
func compileDenyPattern(p DenyPattern) (DenyPattern, error) {
	switch p.Kind {
	case PatternSubstring:
		p.lower = strings.ToLower(p.Raw)
		return p, nil
	case PatternRegex:
		// (?i) makes the match case-insensitive without per-call flags.
		compiled, err := regexp.Compile("(?i)" + p.Raw)
		if err != nil {
			return p, err
		}
		p.compiled = &safeRegexp{matchString: compiled.MatchString}
		return p, nil
	default:
		return p, fmt.Errorf("unknown pattern kind %d", p.Kind)
	}
}

// sanitizePatternName turns a substring into a log-safe name fragment.
func sanitizePatternName(s string) string {
	lower := strings.ToLower(s)
	lower = strings.TrimSpace(lower)
	if len(lower) > 32 {
		lower = lower[:32]
	}
	var b strings.Builder
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// Check evaluates the parsed chat/responses request body and reports whether
// the request should be blocked. When blocked is true, reason is the matched
// pattern name (safe to log — never contains prompt content). An empty/nil
// body or one without prompt text returns (false, "").
func (f *Filter) Check(body map[string]any) (blocked bool, reason string) {
	if f == nil || len(f.patterns) == 0 {
		return false, ""
	}
	text := ExtractPromptText(body)
	if text == "" {
		return false, ""
	}

	// Lowercase once for all substring rules; regex rules use the original
	// case text (they carry their own (?i) flag).
	lower := strings.ToLower(text)

	for _, p := range f.patterns {
		switch p.Kind {
		case PatternSubstring:
			if strings.Contains(lower, p.lower) {
				return true, p.Name
			}
		case PatternRegex:
			if p.compiled != nil && p.compiled.matchString(text) {
				return true, p.Name
			}
		}
	}

	if f.repeatedRunLimit > 0 {
		if runLen := longestRepeatedRun(text); runLen >= f.repeatedRunLimit {
			return true, fmt.Sprintf("repeated_token_run_%d", runLen)
		}
	}

	return false, ""
}

// PatternCount returns the number of deny patterns (seed + runtime extras).
// Useful for diagnostics and tests.
func (f *Filter) PatternCount() int {
	if f == nil {
		return 0
	}
	return len(f.patterns)
}

// ExtractPromptText flattens the prompt-bearing fields of a chat/responses
// request body into a single string for pattern matching. It walks the same
// fields used by routing.EstimateRequestContextTokens:
//   - messages[].content (OpenAI chat / Claude messages)
//   - input (OpenAI responses — string or message array)
//   - system (Claude system prompt — string or block array)
//   - contents[].parts[].text (Gemini generateContent)
//
// Non-string leaves (role, type, model, tool calls) are ignored so the filter
// only inspects human/model-authored text.
func ExtractPromptText(body map[string]any) string {
	if body == nil {
		return ""
	}
	var b strings.Builder
	writeContentText(&b, body["messages"])
	writeContentText(&b, body["input"])
	writeContentText(&b, body["system"])
	writeContentText(&b, body["contents"])
	return b.String()
}

// writeContentText recursively appends string leaves from a parsed JSON value.
// Handles the shapes a message/content field can take across OpenAI, Claude,
// and Gemini dialects:
//   - string
//   - []any of the above
//   - map[string]any with text/content/parts children
func writeContentText(b *strings.Builder, v any) {
	switch val := v.(type) {
	case string:
		if val != "" {
			b.WriteString(val)
			b.WriteByte('\n')
		}
	case []any:
		for _, el := range val {
			writeContentText(b, el)
		}
	case map[string]any:
		// OpenAI content part: {"type":"text","text":"..."}
		if text, ok := val["text"].(string); ok && text != "" {
			b.WriteString(text)
			b.WriteByte('\n')
		}
		// Nested content (Claude message blocks, responses input array).
		if content, ok := val["content"]; ok {
			writeContentText(b, content)
		}
		// Gemini parts array.
		if parts, ok := val["parts"].([]any); ok {
			for _, part := range parts {
				writeContentText(b, part)
			}
		}
	}
}

// longestRepeatedRun returns the length of the longest contiguous run of the
// same rune in s. It is allocation-free (iterates runes directly). Used to
// detect token-repetition / padding-bomb payloads.
func longestRepeatedRun(s string) int {
	var maxRun, curRun int
	var prev rune
	started := false
	for _, r := range s {
		if !started {
			started = true
			prev = r
			curRun = 1
			maxRun = 1
			continue
		}
		if r == prev {
			curRun++
			if curRun > maxRun {
				maxRun = curRun
			}
		} else {
			curRun = 1
			prev = r
		}
	}
	return maxRun
}
