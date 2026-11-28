package routing

import (
	"regexp"
	"strings"
	"sync"
)

// isRegexPrefix reports whether s starts with the "re:" prefix in any letter
// casing. Allocation-free: checks the three ASCII bytes directly. Acceptance
// and stripping must agree on casing (see ParseRegexModelPattern), otherwise a
// mixed-case prefix like "Re:" would be accepted but never stripped.
func isRegexPrefix(s string) bool {
	return len(s) >= 3 &&
		(s[0] == 'r' || s[0] == 'R') &&
		(s[1] == 'e' || s[1] == 'E') &&
		s[2] == ':'
}

// IsRegexModelPattern returns true if the pattern is a regex pattern (starts with "re:").
func IsRegexModelPattern(pattern string) bool {
	return isRegexPrefix(strings.TrimSpace(pattern))
}

// ParseRegexModelPattern parses a "re:..." pattern into a compiled regexp.
// Compiled bodies are cached process-wide (regexCompileCache), so repeated
// calls with the same pattern reuse one compiled, goroutine-safe instance.
func ParseRegexModelPattern(pattern string) *regexp.Regexp {
	raw := strings.TrimSpace(pattern)
	if !isRegexPrefix(raw) {
		return nil
	}
	// Strip the 3-byte prefix regardless of its letter casing; the body is
	// preserved verbatim (regex bodies stay case-sensitive).
	reBody := strings.TrimSpace(raw[len("re:"):])
	if reBody == "" {
		return nil
	}
	return compileRegexCached(reBody)
}

// regexCompileCache maps regex bodies (the pattern with its "re:" prefix
// trimmed) to compiled *regexp.Regexp instances so hot routing paths stop
// recompiling identical patterns on every request/retry. regexp.Regexp values
// are immutable and safe for concurrent use, so sharing one compiled instance
// across goroutines is safe. Invalid bodies are cached as nil so a bad
// operator-authored pattern pays the compile-error cost once, not per request.
//
// Capacity: keys are operator-authored pattern bodies (route model patterns,
// downstream-policy supported models, model-mapping keys); the entry count is
// bounded by configuration size (roughly the route count), not by traffic, so
// no eviction is needed.
var regexCompileCache sync.Map

// compileRegexCached returns the shared compiled regexp for body, compiling
// and caching it on first use. It returns nil (and caches the failure) for
// invalid bodies.
func compileRegexCached(body string) *regexp.Regexp {
	if v, ok := regexCompileCache.Load(body); ok {
		re, _ := v.(*regexp.Regexp)
		return re
	}
	re, err := regexp.Compile(body)
	if err != nil {
		re = nil
	}
	regexCompileCache.Store(body, re)
	return re
}

// IsExactRouteModelPattern returns true if the pattern is an exact model name (no wildcards, no "re:" prefix).
func IsExactRouteModelPattern(pattern string) bool {
	normalized := strings.TrimSpace(pattern)
	if normalized == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(normalized), "re:") {
		return false
	}
	return !strings.ContainsAny(normalized, "*?")
}

// MatchesModelPattern checks whether a model string matches a pattern (exact, glob, or regex).
func MatchesModelPattern(model, pattern string) bool {
	normalizedPattern := strings.TrimSpace(pattern)
	if normalizedPattern == "" {
		return false
	}
	normalizedModel := strings.TrimSpace(model)
	if normalizedModel == "" {
		return false
	}

	// Exact match
	if normalizedPattern == normalizedModel {
		return true
	}

	// Regex patterns
	if IsRegexModelPattern(normalizedPattern) {
		re := ParseRegexModelPattern(normalizedPattern)
		if re != nil {
			return re.MatchString(normalizedModel)
		}
		return false
	}

	// Glob pattern matching (supports * and ? wildcards)
	return globMatch(normalizedPattern, normalizedModel)
}

// globMatch implements simple glob matching with * and ?. Matching operates on
// runes, not bytes, so `?` consumes exactly one rune and multi-byte UTF-8 model
// names (legal in operator-authored patterns) match correctly. For pure-ASCII
// inputs rune and byte semantics are identical.
func globMatch(pattern, value string) bool {
	pRunes := []rune(pattern)
	vRunes := []rune(value)
	p, v := 0, 0
	pLen, vLen := len(pRunes), len(vRunes)
	pStar, vStar := -1, 0

	for v < vLen {
		if p < pLen && (pRunes[p] == '?' || pRunes[p] == vRunes[v]) {
			p++
			v++
		} else if p < pLen && pRunes[p] == '*' {
			pStar = p
			vStar = v
			p++
		} else if pStar != -1 {
			p = pStar + 1
			vStar++
			v = vStar
		} else {
			return false
		}
	}

	for p < pLen && pRunes[p] == '*' {
		p++
	}
	return p == pLen
}
