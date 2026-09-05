package platform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// UpstreamErrorClass is the high-level class for upstream failure signals.
// Used for account-status decisions, retry UX, and residual-risk documentation.
type UpstreamErrorClass string

const (
	// ClassExpired means the upstream credential/session looks expired or invalid.
	// Only this class may mark accounts.status = 'expired'.
	ClassExpired UpstreamErrorClass = "expired"
	// ClassAuth means an auth/session problem that is not clearly token expiry.
	ClassAuth UpstreamErrorClass = "auth"
	// ClassBilling means payment / quota / credit failures.
	ClassBilling UpstreamErrorClass = "billing"
	// ClassModel means model capability / unsupported-model failures.
	ClassModel UpstreamErrorClass = "model"
	// ClassValidation means request validation / argument errors.
	ClassValidation UpstreamErrorClass = "validation"
	// ClassTransient means rate-limit / timeout / 5xx / network-like failures.
	ClassTransient UpstreamErrorClass = "transient"
	// ClassUnknown is the residual bucket.
	ClassUnknown UpstreamErrorClass = "unknown"
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

// ClassifyUpstreamError maps an upstream HTTP status + message to a class.
// Classification is intentionally conservative for ClassExpired: non-auth
// 401/403 bodies (billing, model, validation, rate-limit) must not mark keys expired.
func ClassifyUpstreamError(httpStatus int, message string) UpstreamErrorClass {
	raw := message
	text := strings.ToLower(strings.TrimSpace(message))

	// Explicit non-auth exclusions first (even when status is 401).
	if isEndpointDispatchDeniedMessage(raw) {
		return ClassValidation
	}
	if text != "" && strings.Contains(text, "未登录且未提供 access token") {
		// NewAPI probe noise: missing credential header, not a stored-token expiry.
		return ClassAuth
	}
	if isRequestValidationFailure(text) {
		return ClassValidation
	}
	if isCapabilityFailure(text) {
		return ClassModel
	}
	if isBillingFailure(text) {
		return ClassBilling
	}

	// Strong credential-expiry phrases beat mixed residual text such as
	// "jwt expired, connection timeout" (checkin failure-reason priority 6 > 8).
	if isStrongTokenExpiredSignal(text) {
		return ClassExpired
	}

	if isTransientFailure(httpStatus, text) {
		return ClassTransient
	}
	if IsCloudflareChallengeMessage(text) {
		return ClassTransient
	}

	// HTTP 401 / "HTTP 401 …" without an explicit credential-expiry phrase is auth
	// residual only. Marking accounts.status='expired' requires confirmed
	// invalid/expired credential wording (isStrongTokenExpiredSignal above).
	// Bare/generic 401 used to mark expired and caused over-expiry flaps.
	if httpStatus == 401 || containsHTTPStatus(raw, 401) {
		if text == "" {
			return ClassAuth
		}
		if hasExplicitExpiryOrInvalidCredential(text) {
			return ClassExpired
		}
		if hasAuthFailureSignal(text) || hasAuthTokenReference(text) {
			return ClassAuth
		}
		// 401 with an opaque/non-auth residual body must not mark expired.
		return ClassUnknown
	}

	if httpStatus == 403 || containsHTTPStatus(raw, 403) {
		if hasAuthFailureSignal(text) || hasAuthTokenReference(text) {
			return ClassAuth
		}
		return ClassUnknown
	}

	if hasAuthFailureSignal(text) && hasAuthTokenReference(text) {
		return ClassAuth
	}

	return ClassUnknown
}

// IsTokenExpiredError reports whether the signal is a confirmed expired/invalid
// stored credential. This matches mark policy (ClassExpired only). Auto-relogin
// callers that need broader 401/unauthorized heuristics must use local
// shouldAttemptAutoRelogin* patterns, not this gate.
// Mirrors historical TS isTokenExpiredError() with non-auth + generic-401 guards.
func IsTokenExpiredError(httpStatus int, message string) bool {
	return ShouldMarkAccountExpired(httpStatus, message)
}

// ShouldMarkAccountExpired is the guard used before writing accounts.status='expired'.
// Only ClassExpired (confirmed credential invalid/expiry wording) may mark.
// Generic 401/unauthorized, network, 429, and 5xx must return false.
func ShouldMarkAccountExpired(httpStatus int, message string) bool {
	return ClassifyUpstreamError(httpStatus, message) == ClassExpired
}

// IsAuthRelatedUpstreamError is true for expired or other auth classes.
func IsAuthRelatedUpstreamError(httpStatus int, message string) bool {
	c := ClassifyUpstreamError(httpStatus, message)
	return c == ClassExpired || c == ClassAuth
}

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

// upstreamStatusRE extracts the first HTTP status an upstream error message
// carries. It is deliberately stricter than containsHTTPStatus, which accepts any
// word-boundary hit because it only ever asks "does this message mention 401?".
// An explanation prints the number it found, so it must not print an address or a
// duration instead: the digits have to stand alone (start/space/paren before,
// end/space/comma/colon/paren after). That rejects "127.0.0.1:1" (an IP octet is
// followed by a dot), "500ms" and "quota 500000" (a digit run continues), while
// still matching "HTTP 401:", "429 Too Many Requests" and "returned 503".
var upstreamStatusRE = regexp.MustCompile(`(?:^|[\s(])(?:http\s*)?([1-5][0-9]{2})(?:$|[\s:,)])`)

// upstreamHTTPStatus returns the HTTP status embedded in an upstream error
// message, or 0 when it carries none (a transport failure never got a response).
func upstreamHTTPStatus(message string) int {
	match := upstreamStatusRE.FindStringSubmatch(strings.ToLower(message))
	if match == nil {
		return 0
	}
	status, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return status
}

// ExplainUpstreamFailure renders one compact, credential-safe sentence naming why
// an upstream call failed, for operator-facing surfaces such as a 502 body or an
// account health reason. It never echoes the raw upstream text: that text can
// carry a URL, a token fragment or a request id, and it already reaches the
// server log in full.
//
// The class comes from ClassifyUpstreamError, but ClassUnknown is not read as
// "nothing to say". That classifier is deliberately conservative because only
// ClassExpired may write accounts.status='expired'; an operator still needs to
// hear that a 401 from the upstream auth endpoint is a credential problem even
// when the wording was too weak to auto-mark the account. So an unclassified
// failure falls back to what its status alone proves (#1210: a bare
// "balance refresh failed" hid `HTTP 401: Token has expired`, which the server
// had already logged).
//
// It returns "" only when nothing safe can be named, and the caller then keeps
// its own stable message prefix instead of inventing a cause.
func ExplainUpstreamFailure(httpStatus int, message string) string {
	status := httpStatus
	if status == 0 {
		status = upstreamHTTPStatus(message)
	}

	switch ClassifyUpstreamError(status, message) {
	case ClassExpired:
		return withUpstreamStatus("upstream credential expired", status)
	case ClassAuth:
		return withUpstreamStatus("upstream rejected the credential", status)
	case ClassBilling:
		return withUpstreamStatus("upstream reported a billing or quota problem", status)
	case ClassModel:
		return withUpstreamStatus("upstream does not serve the requested model", status)
	case ClassValidation:
		return withUpstreamStatus("upstream rejected the request", status)
	case ClassTransient:
		return withUpstreamStatus(explainTransientUpstream(status, message), status)
	}

	// Unclassified: name only what the status itself proves, without claiming a
	// cause the classifier refused to confirm.
	switch {
	case status == 401 || status == 403:
		return withUpstreamStatus("upstream rejected the credential", status)
	case status == 404:
		return withUpstreamStatus("upstream has no such endpoint or account", status)
	case status == 429:
		return withUpstreamStatus("upstream rate-limited the request", status)
	case status >= 500:
		return withUpstreamStatus("upstream errored", status)
	case status >= 400:
		return withUpstreamStatus("upstream rejected the request", status)
	}
	if reason := explainTransportFailure(message); reason != "" {
		return reason
	}
	return ""
}

// explainTransientUpstream splits the transient class into the causes an operator
// acts on differently: too many requests, an upstream that answered too late, an
// upstream that could not be reached at all, and a generic 5xx.
func explainTransientUpstream(status int, message string) string {
	text := strings.ToLower(message)
	switch {
	case status == 429,
		strings.Contains(text, "rate limit"),
		strings.Contains(text, "rate_limit"),
		strings.Contains(text, "too many requests"):
		return "upstream rate-limited the request"
	case status >= 500:
		return "upstream errored"
	}
	if reason := explainTransportFailure(message); reason != "" {
		return reason
	}
	return "upstream was temporarily unavailable"
}

// explainTransportFailure names the failures where nothing came back, so there is
// no status to show and the fix is network-side rather than credential-side.
func explainTransportFailure(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "connection refused"), strings.Contains(text, "econnrefused"):
		return "upstream refused the connection"
	case strings.Contains(text, "connection reset"), strings.Contains(text, "econnreset"):
		return "upstream reset the connection"
	case strings.Contains(text, "no such host"), strings.Contains(text, "name resolution"), strings.Contains(text, "server misbehaving"):
		return "upstream host could not be resolved"
	case strings.Contains(text, "unreachable"), strings.Contains(text, "no route to host"):
		return "upstream is unreachable"
	case strings.Contains(text, "timeout"), strings.Contains(text, "timed out"), strings.Contains(text, "deadline exceeded"):
		return "upstream timed out"
	case strings.Contains(text, "x509"), strings.Contains(text, "certificate"), strings.Contains(text, "tls"):
		return "upstream TLS handshake failed"
	case strings.Contains(text, "eof"):
		return "upstream closed the connection"
	}
	return ""
}

// withUpstreamStatus suffixes the status the explanation was derived from, so an
// operator can tell a 401 from a 403 without being shown the upstream body.
func withUpstreamStatus(reason string, status int) string {
	if status == 0 {
		return reason
	}
	return fmt.Sprintf("%s (HTTP %d)", reason, status)
}
