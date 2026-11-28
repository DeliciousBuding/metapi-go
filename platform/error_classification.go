package platform

import "strings"

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
