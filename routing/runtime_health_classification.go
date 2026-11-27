package routing

import "regexp"

var siteTransientFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)bad\s+gateway`),
	regexp.MustCompile(`(?i)gateway\s+time-?out`),
	regexp.MustCompile(`(?i)service\s+unavailable`),
	regexp.MustCompile(`(?i)temporar(?:y|ily)\s+unavailable`),
	regexp.MustCompile(`(?i)cpu\s+overloaded`),
	regexp.MustCompile(`(?i)overloaded`),
	regexp.MustCompile(`(?i)connection\s+reset`),
	regexp.MustCompile(`(?i)connection\s+refused`),
	regexp.MustCompile(`(?i)econnreset`),
	regexp.MustCompile(`(?i)econnrefused`),
	regexp.MustCompile(`(?i)timeout`),
	regexp.MustCompile(`(?i)timed\s*out`),
}

var siteModelFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)unsupported\s+model`),
	regexp.MustCompile(`(?i)model\s+not\s+supported`),
	regexp.MustCompile(`(?i)does\s+not\s+support(?:\s+the)?\s+model`),
	regexp.MustCompile(`(?i)no\s+such\s+model`),
	regexp.MustCompile(`(?i)unknown\s+model`),
	regexp.MustCompile(`(?i)unknown\s+provider\s+for\s+model`),
	regexp.MustCompile(`(?i)invalid\s+model`),
	regexp.MustCompile(`(?i)model.*does\s+not\s+exist`),
}

var siteProtocolFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)unsupported\s+legacy\s+protocol`),
	regexp.MustCompile(`(?i)please\s+use\s+/v1/responses`),
	regexp.MustCompile(`(?i)please\s+use\s+/v1/messages`),
	regexp.MustCompile(`(?i)please\s+use\s+/v1/chat/completions`),
	regexp.MustCompile(`(?i)does\s+not\s+allow\s+/v1/`),
	regexp.MustCompile(`(?i)unsupported\s+endpoint`),
	regexp.MustCompile(`(?i)unsupported\s+path`),
	regexp.MustCompile(`(?i)unknown\s+endpoint`),
	regexp.MustCompile(`(?i)unrecognized\s+request\s+url`),
	regexp.MustCompile(`(?i)no\s+route\s+matched`),
}

var siteValidationFailurePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)invalid\s+request\s+body`),
	regexp.MustCompile(`(?i)validation`),
	regexp.MustCompile(`(?i)missing\s+required`),
	regexp.MustCompile(`(?i)required\s+parameter`),
	regexp.MustCompile(`(?i)unknown\s+parameter`),
	regexp.MustCompile(`(?i)unrecognized\s+(?:field|key|parameter)`),
	regexp.MustCompile(`(?i)malformed`),
	regexp.MustCompile(`(?i)invalid\s+json`),
	regexp.MustCompile(`(?i)cannot\s+parse`),
	regexp.MustCompile(`(?i)unsupported\s+media\s+type`),
}

var usageLimitRateLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)usage_limit_reached`),
	regexp.MustCompile(`(?i)usage\s+limit\s+has\s+been\s+reached`),
	regexp.MustCompile(`(?i)quota\s+exceeded`),
	regexp.MustCompile(`(?i)rate\s+limit`),
	regexp.MustCompile(`(?i)\blimit\b`),
}

func matchesAnyPattern(patterns []*regexp.Regexp, text string) bool {
	if text == "" {
		return false
	}
	for _, p := range patterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

// IsUsageLimitRateLimitFailure checks if a 429 is from a usage/rate limit.
func IsUsageLimitRateLimitFailure(ctx SiteRuntimeFailureContext) bool {
	status := 0
	if ctx.Status != nil {
		status = *ctx.Status
	}
	if status != 429 {
		return false
	}
	errorText := ""
	if ctx.ErrorText != nil {
		errorText = *ctx.ErrorText
	}
	return matchesAnyPattern(usageLimitRateLimitPatterns, errorText)
}

func isModelScopedRuntimeFailure(ctx SiteRuntimeFailureContext) bool {
	text := ""
	if ctx.ErrorText != nil {
		text = *ctx.ErrorText
	}
	return matchesAnyPattern(siteModelFailurePatterns, text)
}

func isProtocolRuntimeFailure(ctx SiteRuntimeFailureContext) bool {
	text := ""
	if ctx.ErrorText != nil {
		text = *ctx.ErrorText
	}
	return matchesAnyPattern(siteProtocolFailurePatterns, text)
}

func isValidationRuntimeFailure(ctx SiteRuntimeFailureContext) bool {
	text := ""
	if ctx.ErrorText != nil {
		text = *ctx.ErrorText
	}
	return matchesAnyPattern(siteValidationFailurePatterns, text)
}

// ResolveSiteRuntimeFailurePenalty assigns a penalty score based on failure context.
func ResolveSiteRuntimeFailurePenalty(ctx SiteRuntimeFailureContext) float64 {
	status := 0
	if ctx.Status != nil {
		status = *ctx.Status
	}
	errorText := ""
	if ctx.ErrorText != nil {
		errorText = *ctx.ErrorText
	}

	if IsUsageLimitRateLimitFailure(ctx) {
		return 0.4
	}
	if isModelScopedRuntimeFailure(ctx) {
		return 0.9
	}
	if isProtocolRuntimeFailure(ctx) {
		return 0.6
	}
	if isValidationRuntimeFailure(ctx) {
		return 0.25
	}
	if status >= 500 || matchesAnyPattern(siteTransientFailurePatterns, errorText) {
		return 2.5
	}
	if status == 429 {
		return 2.2
	}
	if status == 401 || status == 403 {
		return 1.8
	}
	if status >= 400 && status < 500 {
		return 0.9
	}
	return 1.2
}

// IsTransientSiteRuntimeFailure checks if a failure is transient.
func IsTransientSiteRuntimeFailure(ctx SiteRuntimeFailureContext) bool {
	status := 0
	if ctx.Status != nil {
		status = *ctx.Status
	}
	errorText := ""
	if ctx.ErrorText != nil {
		errorText = *ctx.ErrorText
	}

	if IsUsageLimitRateLimitFailure(ctx) {
		return false
	}
	if isModelScopedRuntimeFailure(ctx) {
		return false
	}
	if isProtocolRuntimeFailure(ctx) {
		return false
	}
	if isValidationRuntimeFailure(ctx) {
		return false
	}
	return status >= 500 || status == 429 || matchesAnyPattern(siteTransientFailurePatterns, errorText)
}
