package proxyhandler

import (
	"context"
	"math"
	"net/http"

	"github.com/deliciousbuding/metapi-go/proxy"
)

type gatewayAttemptKey struct{}

func beginGatewayAttempt(ctx context.Context) context.Context {
	s := requestLifecycleFromContext(ctx)
	if s == nil {
		return ctx
	}
	s.mu.Lock()
	ordinal := s.attempts
	s.attempts++
	s.attempted = true
	s.mu.Unlock()
	return context.WithValue(ctx, gatewayAttemptKey{}, ordinal)
}

// isZeroUsageUpstreamRejection reports whether a failed attempt without a
// usage payload is a definitive pre-generation upstream HTTP rejection (auth,
// forbidden, missing endpoint/model, rate limit, or bad request) that cannot
// have billed generated tokens.
func isZeroUsageUpstreamRejection(entry *proxy.ProxyLogEntry) bool {
	if entry == nil || entry.Status != "failed" {
		return false
	}
	switch entry.HTTPStatus {
	case http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusUnprocessableEntity,
		http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}

func upstreamFailureReason(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "upstream_auth_rejected"
	case http.StatusRequestTimeout:
		return "upstream_first_byte_timeout"
	case http.StatusTooManyRequests:
		return "upstream_rate_limited"
	case http.StatusBadRequest, http.StatusNotFound, http.StatusUnprocessableEntity:
		return "upstream_request_rejected"
	default:
		if status >= 500 {
			return "upstream_server_error"
		}
		return "upstream_transport_failed"
	}
}

func (s *requestLifecycle) recordAttempt(ctx context.Context, entry *proxy.ProxyLogEntry) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.GatewayRequestID = s.entry.ID
	if entry.Status == "failed" && s.firstFailure == "" {
		// Retain the first actionable upstream outcome without copying raw error
		// bodies or transport URLs into the request-level reason field.
		s.firstFailure = upstreamFailureReason(entry.HTTPStatus)
	}
	ordinal, hasOrdinal := ctx.Value(gatewayAttemptKey{}).(int)
	if !hasOrdinal {
		ordinal = s.attempts
	}
	if budget := s.userBudget; budget != nil && !budget.seenAttempts[ordinal] {
		if budget.seenAttempts == nil {
			budget.seenAttempts = map[int]bool{}
		}
		budget.seenAttempts[ordinal] = true
		if entry.TotalTokens != nil && *entry.TotalTokens >= 0 && *entry.TotalTokens <= math.MaxInt64-budget.tokens {
			budget.tokens += *entry.TotalTokens
			budget.usageFound = true
		} else if isZeroUsageUpstreamRejection(entry) {
			// A definitive pre-generation upstream rejection (e.g. 401/403/404/429
			// with no usage payload) consumed zero tokens; treat it as 0 rather
			// than poisoning a subsequent failover success into a permanent
			// unknown-usage budget freeze.
			budget.usageFound = true
		} else {
			budget.usageUnknown = true
		}
	}
	if ordinal, ok := ctx.Value(gatewayAttemptKey{}).(int); ok {
		entry.RetryCount = ordinal
		return
	}
	// Non-dispatch surfaces retain their existing log-driven observation.
	if s.entry.Transport == "websocket" {
		entry.RetryCount = s.attempts
	}
	s.attempts++
	s.attempted = true
}
