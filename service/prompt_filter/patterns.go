package promptfilter

// PatternKind distinguishes substring vs regex deny patterns.
type PatternKind int

const (
	// PatternSubstring matches a case-insensitive literal substring.
	PatternSubstring PatternKind = iota
	// PatternRegex matches a case-insensitive regular expression (RE2).
	PatternRegex
)

// DenyPattern is a single prompt-filter deny rule. Substring patterns are
// compared case-insensitively against the flattened prompt text; regex
// patterns are compiled once with the (?i) flag at construction time.
type DenyPattern struct {
	Name string
	Kind PatternKind
	// Raw is the original pattern source (substring text or regex source).
	Raw string
	// lower is the pre-lowercased substring (PatternSubstring only).
	lower string
	// compiled is the pre-compiled regex (PatternRegex only).
	compiled *safeRegexp
}

// safeRegexp wraps regexp.Regexp so the patterns list can be assembled without
// a direct import of regexp in patterns.go (keeps the seed table declarative).
type safeRegexp struct {
	matchString func(string) bool
}

// seedPatterns is the hardcoded first-line-of-defense deny list. These are
// NOT comprehensive — operators extend at runtime via PROMPT_FILTER_DENY_PATTERNS.
//
// Grouping:
//   - Jailbreak attempts (override system instructions / role manipulation)
//   - Credential / system-prompt exfiltration
//   - Known abuse payloads
var seedPatterns = []DenyPattern{
	// ---- Jailbreak attempts ----
	{Name: "jailbreak_ignore_instructions", Kind: PatternSubstring, Raw: "ignore your instructions"},
	{Name: "jailbreak_ignore_all_previous", Kind: PatternSubstring, Raw: "ignore all previous"},
	{Name: "jailbreak_ignore_above", Kind: PatternSubstring, Raw: "ignore the above"},
	{Name: "jailbreak_disregard_previous", Kind: PatternSubstring, Raw: "disregard previous instructions"},
	{Name: "jailbreak_you_are_not", Kind: PatternSubstring, Raw: "you are not"},
	{Name: "jailbreak_act_as_no_restrictions", Kind: PatternRegex, Raw: `act as[^\n]{0,120}no restrictions`},
	{Name: "jailbreak_developer_mode", Kind: PatternSubstring, Raw: "developer mode"},
	{Name: "jailbreak_dan", Kind: PatternRegex, Raw: `\bDAN\b`},
	{Name: "jailbreak_stan", Kind: PatternRegex, Raw: `\bSTAN\b`},
	{Name: "jailbreak_aim", Kind: PatternRegex, Raw: `\bAIM\b`},
	{Name: "jailbreak_keyword", Kind: PatternSubstring, Raw: "jailbreak"},

	// ---- Credential / system-prompt exfiltration ----
	{Name: "exfil_api_key", Kind: PatternSubstring, Raw: "api key"},
	{Name: "exfil_secret_key", Kind: PatternSubstring, Raw: "secret key"},
	{Name: "exfil_show_prompt", Kind: PatternRegex, Raw: `show me your[^\n]{0,80}prompt`},
	{Name: "exfil_system_prompt", Kind: PatternSubstring, Raw: "system prompt"},
	{Name: "exfil_reveal_instructions", Kind: PatternSubstring, Raw: "reveal your instructions"},
	{Name: "exfil_print_rules", Kind: PatternRegex, Raw: `print[^\n]{0,40}(your|the)[^\n]{0,20}(rules|instructions)`},

	// ---- Known abuse payloads ----
	{Name: "abuse_ignore_restrictions", Kind: PatternSubstring, Raw: "ignore restrictions"},
	{Name: "abuse_override_instructions", Kind: PatternSubstring, Raw: "override your instructions"},
}

// maxRepeatedRunThreshold is the longest run of the same rune that is still
// considered legitimate. Longer runs (>500 identical chars) are treated as
// token-repetition / padding-bomb abuse.
const maxRepeatedRunThreshold = 500
