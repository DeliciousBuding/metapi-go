package proxyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/routing"
)

// errResponsesWSSessionEnded is returned by handleMessage when the per-message
// authorization check finds a terminal key state (disabled/expired/revoked,
// exhausted budget, blocked policy). The session loop stops reading and the
// connection closes; transient rate windows only reject the current turn.
var errResponsesWSSessionEnded = errors.New("responses websocket session ended after authorization failure")

func (s *responsesWSSession) handleMessage(ctx context.Context, raw []byte) (turnErr error) {
	scope := proxyScopeFromContext(ctx)
	parent := scope.lifecycle
	writer := defaultGatewayRequestWriter(scope.db)
	if parent != nil {
		writer = parent.writer
	}
	state, turnRequest := newRequestLifecycle(scope.db, s.req.WithContext(ctx), "websocket", writer)
	if parent != nil {
		// Authentication has already removed credential-shaped correlation IDs.
		// A turn inherits that result, not the original handshake header.
		parent.mu.Lock()
		state.entry.ExternalRequestID = parent.entry.ExternalRequestID
		parent.mu.Unlock()
	}
	ctx = turnRequest.Context()
	state.bind(s.auth, "/v1/responses", "")
	defer func() {
		if panicValue := recover(); panicValue != nil {
			state.mu.Lock()
			state.gatewayFailure = "handler_panic"
			state.failureAfterOutput = state.entry.OutputStarted
			state.mu.Unlock()
			state.finish(ctx)
			panic(panicValue)
		}
		if turnErr != nil {
			state.mu.Lock()
			if state.outcome == "" && state.rejection == "" {
				state.outcome = shared.OutcomeUpstreamError
			}
			state.mu.Unlock()
		}
		state.finish(ctx)
	}()
	msg, err := ParseResponsesWSMessage(raw)
	if err != nil {
		return s.writeError(ctx, 400, "Invalid websocket JSON payload")
	}
	reqType := strings.TrimSpace(msg.Type)
	if reqType == "" {
		return s.writeError(ctx, 400, "unsupported websocket request type: unknown")
	}

	// C1/C2: response.create (+ response.append merge when a prior turn exists).
	if reqType != "response.create" && reqType != "response.append" {
		return s.writeError(ctx, 400, "unsupported websocket request type: "+reqType)
	}

	// Re-authorize every turn against the current key state. The handshake was
	// authorized once (and deliberately not billed at Accept), so this loop is
	// the single request-count point, and a disabled/expired key or tightened
	// policy must not survive an open socket. The chain is the same one
	// ProxyAuth runs for HTTP requests (enabled/expiry/cost/quota/IP/RPM/TPM
	// then max_requests consume).
	if s.auth != nil && s.auth.Source == "managed" && s.auth.KeyID != nil {
		refreshed := auth.ReauthorizeManagedKeyForTurn(ctx, scope.db, s.auth.Token, s.clientIP, responsesWSTurnTokenEstimate(msg, raw))
		// This turn's quota hold settles through its proxy-log sink. Release it
		// if the turn exits without billable usage (policy rejection, prewarm,
		// or a failure before any attempt is logged).
		if refreshed.QuotaReservationID != "" {
			defer auth.ReleaseManagedKeyQuotaReservation(scope.db, refreshed.QuotaReservationID)
		}
		if !refreshed.OK {
			status, message, endSession := responsesWSTurnAuthError(refreshed)
			_ = s.writeError(ctx, status, message)
			if endSession {
				return errResponsesWSSessionEnded
			}
			return nil
		}
		// Carry the freshly loaded policy into this turn: the model gate below
		// and routing both read s.auth.Policy.
		fresh := *s.auth
		fresh.Policy = refreshed.Policy
		fresh.OwnerUserID = refreshed.Key.OwnerUserID
		fresh.UserGroupID = refreshed.Key.UserGroupID
		fresh.KeyName = refreshed.Key.Name
		fresh.QuotaReservationID = refreshed.QuotaReservationID
		fresh.ProxyURL = nil
		if refreshed.Key.ProxyURL != nil {
			if trimmed := strings.TrimSpace(*refreshed.Key.ProxyURL); trimmed != "" {
				fresh.ProxyURL = &trimmed
			}
		}
		s.auth = &fresh
	}

	s.mu.Lock()
	lastReq := s.lastRequest
	lastOut := s.lastOutput
	s.mu.Unlock()

	// Model policy gate before normalize/quota (TS isModelAllowedByPolicyOrAllowedRoutes).
	requestModel := strings.TrimSpace(msg.Model)
	if requestModel == "" && lastReq != nil {
		if m, ok := lastReq["model"].(string); ok {
			requestModel = strings.TrimSpace(m)
		}
	}
	state.bind(s.auth, "/v1/responses", requestModel)
	if requestModel != "" && s.auth != nil && !IsModelAllowedByPolicy(requestModel, s.auth.Policy) {
		return s.writeError(ctx, 403, "model is not allowed for this downstream key")
	}

	supportsIncremental := clientRequestsResponsesWSIncremental(msg)

	normalized, nerr := normalizeResponsesWSRequest(msg, lastReq, lastOut, supportsIncremental)
	if nerr != nil {
		return s.writeError(ctx, nerr.status, nerr.message)
	}

	// Prompt safety filter (#681, audit L1): the native Codex WSS branch below
	// runs before bridgeHTTP, so it must apply the same pre-forward check as the
	// HTTP Responses handler. A blocked prompt never reaches either transport.
	if blocked := checkPromptFilter(&Ctx{Body: normalized.request, RequestedModel: requestModel}); blocked != nil {
		return s.writeError(ctx, blocked.Status, blocked.Error)
	}

	// Local prewarm: generate=false on first create — emit synthetic created+completed.
	if shouldHandleLocalPrewarm(msg, lastReq, supportsIncremental) {
		model, _ := normalized.request["model"].(string)
		for _, payload := range SynthesizePrewarmResponsePayloads(model, "") {
			if err := s.writeJSON(ctx, payload); err != nil {
				return err
			}
		}
		s.mu.Lock()
		s.lastRequest = cloneMap(normalized.nextSnapshot)
		s.lastOutput = nil
		s.mu.Unlock()
		state.observe(shared.OutcomeSuccess, true)
		return nil
	}

	normalized.request["stream"] = true
	if status, message := state.admitUserBudget(ctx, s.auth, "/v1/responses", normalized.request); status != 0 {
		return s.writeError(ctx, status, message)
	}
	if model, _ := normalized.request["model"].(string); strings.TrimSpace(model) != "" {
		requestModel = strings.TrimSpace(model)
	}

	// Both transports must evaluate the same turn, including headers omitted
	// from the HTTP bridge's upstream forwarding allow-list. Keep only request
	// facts here; each selection still uses the freshly authorized Key policy.
	associationBody, err := json.Marshal(normalized.request)
	if err != nil {
		return s.writeError(ctx, 400, "failed to encode request payload")
	}
	ctx = context.WithValue(ctx, associationRequestContextKey{},
		lazyAssociationRequest("/v1/responses", associationBody, s.req.Header, true, time.Now()))
	ctx = context.WithValue(ctx, traceIdentityContextKey{}, requestTraceIdentity(ctx, s.auth, s.req.Header, requestModel))

	// C3: try Codex upstream wss when channel/platform allows; else HTTP bridge.
	output, handled, err := s.tryCodexUpstreamWSS(ctx, normalized.request, requestModel, supportsIncremental)
	if !handled {
		output, err = s.bridgeHTTP(ctx, normalized.request, supportsIncremental)
	}
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.lastRequest = cloneMap(normalized.nextSnapshot)
	s.lastOutput = output
	s.mu.Unlock()
	return nil
}

// responsesWSTurnTokenEstimate returns the TPM reservation for one WS message.
// Reuses the routing request estimator; when it finds no prompt strings, the
// raw frame size is a conservative floor. Zero is allowed here because the
// auth chain reserves a 1-token minimum whenever maxTPM is set.
func responsesWSTurnTokenEstimate(msg *ResponsesWSMessage, raw []byte) int64 {
	if msg != nil && msg.Raw != nil {
		if tokens := routing.EstimateRequestContextTokens(msg.Raw); tokens > 0 {
			return tokens
		}
	}
	return int64(len(raw)) / 4
}

// responsesWSTurnAuthError maps a failed per-message reauthorization onto the
// Responses WS error frame. Terminal key states end the session; transient
// rate windows reject only this turn so the client can retry.
func responsesWSTurnAuthError(result auth.DownstreamTokenAuthResult) (status int, message string, endSession bool) {
	status = result.StatusCode
	if status == 0 {
		status = http.StatusForbidden
	}
	message = strings.TrimSpace(result.Error)
	if message == "" {
		message = "API key authorization failed"
	}
	switch result.Reason {
	case "over_rpm", "over_tpm", "quota_unavailable", "unavailable":
		return status, message, false
	default:
		return status, message, true
	}
}
