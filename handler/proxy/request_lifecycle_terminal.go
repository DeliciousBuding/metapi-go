package proxyhandler

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
)

func (s *requestLifecycle) observe(status string, stream bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcome = status
	if status == shared.OutcomeSuccess && !stream && s.requestContext.Err() == nil && !s.writeFailed {
		s.protocolCompleted = true
	}
	s.knownFailure = status != shared.OutcomeSuccess && s.requestContext.Err() == nil
	if status != shared.OutcomeSuccess && status != shared.OutcomeClientError {
		s.failureAfterOutput = s.entry.OutputStarted && (s.entry.Transport == "websocket" || s.entry.HTTPStatus != nil && *s.entry.HTTPStatus < 400)
	}
}

func (s *requestLifecycle) firstOutput() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entry.Transport != "websocket" && s.entry.FirstOutputLatencyMs == nil {
		value := time.Since(s.started).Milliseconds()
		s.entry.FirstOutputLatencyMs = &value
	}
}

func (s *requestLifecycle) finish(ctx context.Context) {
	s.once.Do(func() {
		s.mu.Lock()
		if s.hijacked {
			s.mu.Unlock()
			return
		}
		s.entry.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.entry.LatencyMs = time.Since(s.started).Milliseconds()
		s.entry.AttemptCount = s.attempts
		status, reason := s.terminal(ctx)
		s.entry.TerminalStatus, s.entry.ReasonCode = status, &reason
		entry := s.entry
		s.mu.Unlock()
		// A disconnected caller must not cancel its own completion record. The
		// bounded write cannot rewrite the response or start a new retry.
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		s.settleUserBudget(writeCtx)
		if err := s.writer(writeCtx, entry); err != nil {
			slog.Warn("gateway request record failed", "gateway_request_id", entry.ID, "err", err)
		}
	})
}

// Called with mu held. A protocol failure already observed wins over a later
// socket close; a mere write failure is not evidence of user-initiated cancel.
func (s *requestLifecycle) terminal(ctx context.Context) (string, string) {
	if s.gatewayFailure != "" {
		if s.failureAfterOutput {
			return "output_interrupted", s.gatewayFailure
		}
		return "gateway_failed", s.gatewayFailure
	}
	if s.failureAfterOutput || s.protocolFailed && s.entry.Transport != "websocket" && s.entry.HTTPStatus != nil && *s.entry.HTTPStatus < 400 {
		return "output_interrupted", "protocol_interrupted"
	}
	if s.protocolCompleted && !s.protocolFailed && !s.writeFailed && s.outcome == shared.OutcomeSuccess {
		if s.firstFailure != "" {
			return "completed", s.firstFailure
		}
		return "completed", "normal_completion"
	}
	if errors.Is(ctx.Err(), context.Canceled) && !s.knownFailure && s.rejection == "" {
		return "client_cancelled", "downstream_cancelled"
	}
	if s.writeFailed {
		return "output_interrupted", "downstream_write_failed"
	}
	if s.rejection != "" {
		if s.entry.HTTPStatus != nil && *s.entry.HTTPStatus >= 500 {
			return "gateway_failed", s.rejection
		}
		return "rejected_policy", s.rejection
	}
	if s.protocolFailed && s.entry.Transport == "websocket" {
		return "upstream_failed", "protocol_interrupted"
	}
	if s.outcome == shared.OutcomeSuccess {
		if s.firstFailure != "" {
			return "completed", s.firstFailure
		}
		return "completed", "normal_completion"
	}
	if s.outcome == shared.OutcomeUnavailable && !s.attempted {
		if s.noRouteReason != "" {
			return "rejected_no_route", s.noRouteReason
		}
		return "rejected_no_route", "no_available_candidate"
	}
	if s.outcome == shared.OutcomeClientError {
		if s.attempted {
			if s.firstFailure != "" {
				return "upstream_failed", s.firstFailure
			}
			return "upstream_failed", "upstream_rejected"
		}
		return "rejected_policy", "invalid_request"
	}
	if s.outcome != "" {
		if s.firstFailure != "" {
			return "upstream_failed", s.firstFailure
		}
		return "upstream_failed", s.outcome
	}
	if s.entry.HTTPStatus != nil {
		status := *s.entry.HTTPStatus
		if status >= 400 && status < 500 {
			return "rejected_policy", "invalid_request"
		}
		if status >= 500 {
			return "gateway_failed", "internal_error"
		}
		if status >= 200 && status < 300 && !s.dispatched {
			return "completed", "local_response"
		}
	}
	return "gateway_failed", "completion_unobserved"
}

func (s *requestLifecycle) reject(code string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	// The shared JSON writer must not replace a specific admission reason.
	if code != "invalid_request" || s.rejection == "" {
		s.rejection = code
	}
	s.mu.Unlock()
}
