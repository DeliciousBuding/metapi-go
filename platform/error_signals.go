package platform

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	endpointDispatchDeniedRe = regexp.MustCompile(`does\s+not\s+allow\s+/v1/[a-z0-9/_:-]+\s+dispatch`)
	invalidAccessTokenRe     = regexp.MustCompile(`invalid\s+access\s+token`)
	accessTokenIsInvalidRe   = regexp.MustCompile(`access\s+token\s+is\s+invalid`)
	modelNotSupportedRe      = regexp.MustCompile(`model\s+.+\s+is\s+not\s+supported`)
	// Auth-oriented token references. Intentionally avoids bare "token"
	// so phrases like "input token limit" do not look like credential failures.
	authTokenRefRe = regexp.MustCompile(
		`access\s+token|api[_\s-]?key|jwt|访问令牌|令牌|\binvalid\s+token\b|\btoken\s+(?:is\s+)?(?:invalid|expired)\b|\btoken\s+expired\b`,
	)
	authFailureSignalRe = regexp.MustCompile(
		`unauthorized|unauthenticated|authentication\s+(?:failed|required|error)|not\s+login|not\s+logged|未登录|未授权|无权|forbidden`,
	)
)

func isEndpointDispatchDeniedMessage(message string) bool {
	text := strings.ToLower(message)
	if text == "" {
		return false
	}
	return endpointDispatchDeniedRe.MatchString(text) || strings.Contains(text, "dispatch denied")
}

func isRequestValidationFailure(text string) bool {
	if text == "" {
		return false
	}
	return strings.Contains(text, "invalid_argument") ||
		strings.Contains(text, "invalid_request_error") ||
		strings.Contains(text, "input token limit") ||
		strings.Contains(text, "context length") ||
		strings.Contains(text, "maximum context") ||
		strings.Contains(text, "max_tokens") ||
		strings.Contains(text, "max tokens") ||
		strings.Contains(text, "string_above_max_length") ||
		strings.Contains(text, "invalid request body") ||
		strings.Contains(text, "validation error") ||
		strings.Contains(text, "missing required")
}

func isCapabilityFailure(text string) bool {
	if text == "" {
		return false
	}
	return modelNotSupportedRe.MatchString(text) ||
		strings.Contains(text, "not supported for format") ||
		strings.Contains(text, "model not supported") ||
		strings.Contains(text, "unsupported model") ||
		strings.Contains(text, "does not support the model") ||
		strings.Contains(text, "does not support model") ||
		strings.Contains(text, "no such model") ||
		strings.Contains(text, "unknown model") ||
		strings.Contains(text, "model_not_found") ||
		strings.Contains(text, "不支持所选模型") ||
		(strings.Contains(text, "不支持") && strings.Contains(text, "模型"))
}

func isBillingFailure(text string) bool {
	if text == "" {
		return false
	}
	return strings.Contains(text, "no payment method") ||
		strings.Contains(text, "payment method") ||
		strings.Contains(text, "billing") ||
		strings.Contains(text, "insufficient_quota") ||
		strings.Contains(text, "insufficient quota") ||
		strings.Contains(text, "quota exceeded") ||
		strings.Contains(text, "exceeded your current quota") ||
		strings.Contains(text, "credit balance") ||
		strings.Contains(text, "out of credits") ||
		strings.Contains(text, "余额不足") ||
		strings.Contains(text, "额度不足") ||
		strings.Contains(text, "配额不足") ||
		strings.Contains(text, "充值")
}

func isTransientFailure(httpStatus int, text string) bool {
	if httpStatus == 408 || httpStatus == 409 || httpStatus == 425 || httpStatus == 429 || httpStatus >= 500 {
		return true
	}
	if text == "" {
		return false
	}
	return strings.Contains(text, "rate limit") ||
		strings.Contains(text, "rate_limit") ||
		strings.Contains(text, "too many requests") ||
		strings.Contains(text, "timeout") ||
		strings.Contains(text, "timed out") ||
		strings.Contains(text, "temporar") ||
		strings.Contains(text, "service unavailable") ||
		strings.Contains(text, "bad gateway") ||
		strings.Contains(text, "gateway time") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "connection refused") ||
		strings.Contains(text, "econnreset") ||
		strings.Contains(text, "econnrefused")
}

// IsCloudflareChallengeMessage reports whether the upstream is serving a
// Cloudflare challenge instead of the answer. One owner: service/alert and the
// check-in failure classifier both used to keep a byte-identical copy, and an
// ablation (putting the copy back) kept the whole suite green — neither had an
// owner. It normalizes its own input so a caller cannot lose matches by passing
// the message un-lowercased.
func IsCloudflareChallengeMessage(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return false
	}
	return strings.Contains(text, "cloudflare") ||
		strings.Contains(text, "cf challenge") ||
		strings.Contains(text, "challenge required")
}

func isStrongTokenExpiredSignal(text string) bool {
	if text == "" {
		return false
	}
	if strings.Contains(text, "jwt expired") ||
		strings.Contains(text, "token expired") ||
		strings.Contains(text, "access token expired") ||
		invalidAccessTokenRe.MatchString(text) ||
		accessTokenIsInvalidRe.MatchString(text) {
		return true
	}
	return hasExplicitExpiryOrInvalidCredential(text)
}

func hasExplicitExpiryOrInvalidCredential(text string) bool {
	if text == "" {
		return false
	}
	if strings.Contains(text, "jwt expired") ||
		strings.Contains(text, "token expired") ||
		strings.Contains(text, "access token expired") ||
		invalidAccessTokenRe.MatchString(text) ||
		accessTokenIsInvalidRe.MatchString(text) {
		return true
	}
	// Chinese credential expiry / invalidity.
	if (strings.Contains(text, "令牌") || strings.Contains(text, "访问令牌")) &&
		(strings.Contains(text, "过期") || strings.Contains(text, "无效")) {
		return true
	}
	// Auth-oriented token ref + invalid/expired (not bare "token").
	if hasAuthTokenReference(text) &&
		(strings.Contains(text, "invalid") || strings.Contains(text, "expired") ||
			strings.Contains(text, "无效") || strings.Contains(text, "过期")) {
		return true
	}
	return false
}

func hasAuthTokenReference(text string) bool {
	return authTokenRefRe.MatchString(text)
}

func hasAuthFailureSignal(text string) bool {
	return authFailureSignalRe.MatchString(text)
}

func containsHTTPStatus(message string, status int) bool {
	pattern := fmt.Sprintf(`(?:^|\b)(?:http\s*)?%d(?:\b|:)`, status)
	re := regexp.MustCompile(pattern)
	return re.MatchString(strings.ToLower(message))
}
