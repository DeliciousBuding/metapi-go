package routing

import (
	"regexp"
	"strings"
	"unicode"
)

// ---- Structured cooldown reasons (P0-3) ----

// Cooldown reason trigger codes. The code is persisted into
// route_channels.cooldown_reason_code / oauth_route_unit_members and surfaced
// verbatim in the admin API; the UI localizes a label per code and shows the
// raw code as fallback. Codes are an append-only vocabulary: never rename or
// reuse a retired code, persisted rows must keep their meaning forever.
const (
	CooldownReasonCodeUsageLimit    = "usage_limit"
	CooldownReasonCodeRateLimited   = "rate_limited"
	CooldownReasonCodeAuthError     = "auth_error"
	CooldownReasonCodeUpstreamError = "upstream_error"
	CooldownReasonCodeClientError   = "client_error"
	CooldownReasonCodeTimeout       = "timeout"
	CooldownReasonCodeNetworkError  = "network_error"
	CooldownReasonCodeProbeFailure  = "probe_failure"
	CooldownReasonCodeUnknown       = "unknown"
)

// CooldownTriggerSource identifies which path recorded the failure that set
// the cooldown.
type CooldownTriggerSource string

const (
	// CooldownTriggerTraffic is a real downstream request failure.
	CooldownTriggerTraffic CooldownTriggerSource = "traffic"
	// CooldownTriggerProbe is a synthetic background health-probe failure.
	CooldownTriggerProbe CooldownTriggerSource = "probe"
)

// CooldownReasonSummaryMaxRunes caps the persisted error summary so upstream
// error bodies cannot bloat the row or smuggle unbounded text into the admin
// UI.
const CooldownReasonSummaryMaxRunes = 200

// cooldownTimeoutPatterns narrows the transient set to deadline-style failures
// so they classify as timeout before the broader network bucket.
var cooldownTimeoutPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)timeout`),
	regexp.MustCompile(`(?i)timed\s*out`),
	regexp.MustCompile(`(?i)deadline\s+exceeded`),
	regexp.MustCompile(`(?i)context\s+deadline`),
}

// ClassifyCooldownReason maps a failure context to a stable trigger code plus
// a sanitized, truncated error summary. Classification priority mirrors the
// runtime-health penalty ladder (usage-limit first, then HTTP status classes,
// then text patterns), so the reason code stays consistent with the penalty
// the router actually applied. The summary is "" when the failure carried no
// error text — callers persist NULL for empty summaries.
func ClassifyCooldownReason(ctx SiteRuntimeFailureContext, source CooldownTriggerSource) (code string, summary string) {
	status := 0
	if ctx.Status != nil {
		status = *ctx.Status
	}
	errorText := ""
	if ctx.ErrorText != nil {
		errorText = *ctx.ErrorText
	}
	summary = SanitizeCooldownReasonSummary(errorText)

	switch {
	case IsUsageLimitRateLimitFailure(ctx):
		return CooldownReasonCodeUsageLimit, summary
	case status == 429:
		return CooldownReasonCodeRateLimited, summary
	case status == 401 || status == 403:
		return CooldownReasonCodeAuthError, summary
	case status >= 500:
		return CooldownReasonCodeUpstreamError, summary
	case status >= 400:
		return CooldownReasonCodeClientError, summary
	case matchesAnyPattern(cooldownTimeoutPatterns, errorText):
		return CooldownReasonCodeTimeout, summary
	case matchesAnyPattern(siteTransientFailurePatterns, errorText):
		return CooldownReasonCodeNetworkError, summary
	case source == CooldownTriggerProbe:
		// Probe failed without an HTTP status or recognizable text: name the
		// trigger honestly instead of pretending we know the upstream cause.
		return CooldownReasonCodeProbeFailure, summary
	default:
		return CooldownReasonCodeUnknown, summary
	}
}

// CooldownReasonSummaryArg converts a sanitized summary into a nullable SQL
// argument: empty summaries persist as NULL (never ""), so "no summary
// recorded" stays distinguishable from a recorded-but-empty one.
func CooldownReasonSummaryArg(summary string) interface{} {
	if summary == "" {
		return nil
	}
	return summary
}

// reasonSummaryPtr is the pointer variant for in-memory cache patches.
func reasonSummaryPtr(summary string) *string {
	if summary == "" {
		return nil
	}
	return &summary
}

// SanitizeCooldownReasonSummary flattens control characters (newlines, tabs,
// invisible codepoints) into spaces and truncates to
// CooldownReasonSummaryMaxRunes so a persisted summary cannot inject fake
// structure into logs/UI or grow unbounded. Returns "" for blank input.
func SanitizeCooldownReasonSummary(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(trimmed))
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	runes := []rune(out)
	if len(runes) > CooldownReasonSummaryMaxRunes {
		out = string(runes[:CooldownReasonSummaryMaxRunes])
	}
	return out
}
