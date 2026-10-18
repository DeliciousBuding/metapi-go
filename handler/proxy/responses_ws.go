package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
)

// Responses WebSocket uses native Codex wss when enabled and supported by the
// selected channel, otherwise bridging HTTP SSE. Sessions and response IDs are
// process-local. Each message enforces model policy and managed-key request quota.
const (
	ResponsesWebsocketResidualStatus = "c3_codex_upstream_wss"
	ResponsesWebsocketResidualDoc    = "docs/internal/responses-websocket-residual.md"

	wsTurnStateHeader                 = "x-codex-turn-state"
	responsesWebsocketTransportHeader = "x-metapi-responses-websocket-transport"
	responsesWebsocketModeHeader      = "x-metapi-responses-websocket-mode"
	wsMaxMessageBytes                 = 8 << 20 // 8 MiB per client frame
	wsReadIdleTimeout                 = 10 * time.Minute
	wsWriteTimeout                    = 60 * time.Second
)

// responsesWebsocketTransportRegistered is set by EnsureResponsesWebsocketTransport.
var responsesWebsocketTransportRegistered atomic.Bool

// errResponsesWSSessionEnded is returned by handleMessage when the per-message
// authorization check finds a terminal key state (disabled/expired/revoked,
// exhausted budget, blocked policy). The session loop stops reading and the
// connection closes; transient rate windows only reject the current turn.
var errResponsesWSSessionEnded = errors.New("responses websocket session ended after authorization failure")

// ResponsesWebsocketTransportRegistered reports whether the transport registration
// entrypoint has been called in this process.
func ResponsesWebsocketTransportRegistered() bool {
	return responsesWebsocketTransportRegistered.Load()
}

// ResetResponsesWebsocketTransportForTest clears registration state (tests only).
func ResetResponsesWebsocketTransportForTest() {
	responsesWebsocketTransportRegistered.Store(false)
}

// EnsureResponsesWebsocketTransport marks the Responses WS transport as registered.
// C1 does not install a separate ConnState hook — upgrade is handled on the
// GET /v1/responses (and alias) route handlers after ProxyAuth middleware.
func EnsureResponsesWebsocketTransport(srv *http.Server, cfg WebSocketConfig) {
	responsesWebsocketTransportRegistered.Store(true)
	_ = srv
	_ = cfg
	slog.Info("responses WebSocket transport registered",
		"status", ResponsesWebsocketResidualStatus,
		"http_get", "426 Upgrade Required (non-upgrade GET)",
		"upgrade", "coder/websocket + multi-turn HTTP bridge + Codex upstream wss (C3)",
		"doc", ResponsesWebsocketResidualDoc,
	)
}

// WebSocketConfig holds optional hooks for Responses WebSocket transport.
// Registration accepts the config so boot wiring stays stable.
type WebSocketConfig struct {
	// AuthTokenValidation is reserved for non-middleware upgrade paths.
	// Unused while upgrade runs under ProxyAuth middleware (preferred).
	AuthTokenValidation func(token string) (*auth.ProxyAuthContext, error)
}

// IsWebsocketUpgradeRequest reports whether r is a WebSocket upgrade attempt.
func IsWebsocketUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	for _, part := range strings.Split(r.Header.Get("Connection"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "upgrade") {
			return true
		}
	}
	return false
}

// HandleResponsesWebsocket serves a Responses WebSocket connection (C1).
// Caller must only invoke this when IsWebsocketUpgradeRequest(r) is true.
// Auth is expected from ProxyAuth middleware (GetProxyAuth); if missing, upgrade
// is refused with 401 before Accept.
func HandleResponsesWebsocket(w http.ResponseWriter, r *http.Request) {
	path := "/v1/responses"
	if r != nil && r.URL != nil && strings.TrimSpace(r.URL.Path) != "" {
		path = r.URL.Path
	}

	authCtx := GetProxyAuth(r)
	if authCtx == nil {
		writeJSONError(w, http.StatusUnauthorized,
			"Missing or invalid proxy authentication for WebSocket upgrade",
			"invalid_request_error")
		return
	}

	acceptOpts := &websocket.AcceptOptions{
		// Browser clients are not the primary Codex surface; keep origin check open
		// for local tools. Operators can put this behind a reverse proxy ACL.
		InsecureSkipVerify: true,
		CompressionMode:    websocket.CompressionDisabled,
	}
	conn, err := websocket.Accept(w, r, acceptOpts)
	if err != nil {
		slog.Warn("responses websocket accept failed",
			"err", err,
			"path", path,
		)
		return
	}
	conn.SetReadLimit(wsMaxMessageBytes)

	turnState := extractWSTurnState(r)
	if turnState != "" {
		slog.Debug("responses websocket turn-state",
			"path", path,
			"turn_state_len", len(turnState),
		)
	}

	ctx := r.Context()
	session := &responsesWSSession{
		conn:            conn,
		req:             r,
		auth:            authCtx,
		path:            path,
		turnState:       turnState,
		clientIP:        auth.ClientIP(r),
		clientSessionID: newResponsesWSClientSessionID(),
	}

	defer func() {
		for _, key := range session.codexRuntimeKeys {
			CloseCodexWebsocketSession(key)
		}
		_ = conn.Close(websocket.StatusNormalClosure, "session end")
	}()

	if err := session.loop(ctx); err != nil {
		if websocket.CloseStatus(err) == websocket.StatusNormalClosure ||
			websocket.CloseStatus(err) == websocket.StatusGoingAway ||
			ctx.Err() != nil {
			return
		}
		slog.Debug("responses websocket session ended",
			"err", err,
			"path", path,
		)
	}
}

// HandleResponsesWebsocketUpgradeResidual is the legacy entrypoint.
// C1: with auth context → real transport; without auth → clear HTTP 401
// (unit tests without middleware; never Hijack-silent-close).
func HandleResponsesWebsocketUpgradeResidual(w http.ResponseWriter, r *http.Request) {
	if GetProxyAuth(r) != nil {
		HandleResponsesWebsocket(w, r)
		return
	}
	writeJSONError(w, http.StatusUnauthorized,
		"Responses WebSocket requires proxy authentication (see "+ResponsesWebsocketResidualDoc+")",
		"invalid_request_error")
}

// ---- session ----

type responsesWSSession struct {
	conn      *websocket.Conn
	req       *http.Request
	auth      *auth.ProxyAuthContext
	path      string
	turnState string
	// clientIP is captured at handshake time so each message can re-run the
	// key's IP rules without a per-message HTTP request.
	clientIP string

	mu          sync.Mutex
	lastRequest map[string]any
	lastOutput  []any
	// Client-facing WS session id for Codex upstream runtime keying (process-local).
	clientSessionID string
	// Upstream Codex wss session keys opened this connection (closed on defer).
	codexRuntimeKeys []string
	// message serialisation: one in-flight turn at a time (Codex client queue).
	queueMu sync.Mutex
}

func (s *responsesWSSession) loop(ctx context.Context) error {
	for {
		readCtx, cancel := context.WithTimeout(ctx, wsReadIdleTimeout)
		msgType, data, err := s.conn.Read(readCtx)
		cancel()
		if err != nil {
			return err
		}
		if msgType != websocket.MessageText && msgType != websocket.MessageBinary {
			continue
		}
		s.queueMu.Lock()
		err = s.handleMessage(ctx, data)
		s.queueMu.Unlock()
		if err != nil {
			if errors.Is(err, errResponsesWSSessionEnded) {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Debug("responses websocket turn error", "err", err)
		}
	}
}

func (s *responsesWSSession) handleMessage(ctx context.Context, raw []byte) (turnErr error) {
	writer := gatewayRequestWriter(defaultGatewayRequestWriter)
	parent := requestLifecycleFromContext(ctx)
	if parent != nil {
		writer = parent.writer
	}
	state, turnRequest := newRequestLifecycle(s.req.WithContext(ctx), "websocket", writer)
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
		refreshed := auth.ReauthorizeManagedKeyForTurn(ctx, s.auth.Token, s.clientIP, responsesWSTurnTokenEstimate(msg, raw))
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
		fresh.KeyName = refreshed.Key.Name
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
	if model, _ := normalized.request["model"].(string); strings.TrimSpace(model) != "" {
		requestModel = strings.TrimSpace(model)
	}

	// C3: try Codex upstream wss when channel/platform allows; else HTTP bridge.
	if output, handled, cerr := s.tryCodexUpstreamWSS(ctx, normalized.request, requestModel, supportsIncremental); handled {
		if cerr != nil {
			return cerr
		}
		s.mu.Lock()
		s.lastRequest = cloneMap(normalized.nextSnapshot)
		s.lastOutput = output
		s.mu.Unlock()
		return nil
	}

	output, err := s.bridgeHTTP(ctx, normalized.request, supportsIncremental)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.lastRequest = cloneMap(normalized.nextSnapshot)
	s.lastOutput = output
	s.mu.Unlock()
	return nil
}

type normalizeResult struct {
	request      map[string]any
	nextSnapshot map[string]any
}

type normalizeError struct {
	status  int
	message string
}

func normalizeResponsesWSRequest(
	msg *ResponsesWSMessage,
	lastRequest map[string]any,
	lastOutput []any,
	supportsIncremental bool,
) (*normalizeResult, *normalizeError) {
	reqType := strings.TrimSpace(msg.Type)
	raw := msg.Raw
	if raw == nil {
		raw = map[string]any{}
	}

	if lastRequest == nil {
		if reqType != "response.create" {
			return nil, &normalizeError{status: 400, message: "websocket request received before response.create"}
		}
		next := cloneMap(raw)
		delete(next, "type")
		// Prewarm-only: strip generate=false when not on incremental path (TS parity).
		if !supportsIncremental && msg.Generate != nil && !*msg.Generate {
			delete(next, "generate")
		}
		next["stream"] = true
		if _, ok := next["input"]; !ok {
			next["input"] = []any{}
		}
		model, _ := next["model"].(string)
		model = strings.TrimSpace(model)
		if model == "" {
			return nil, &normalizeError{status: 400, message: "missing model in response.create request"}
		}
		return &normalizeResult{request: next, nextSnapshot: cloneMap(next)}, nil
	}

	input, ok := raw["input"].([]any)
	if !ok && msg.Input != nil {
		input = msg.Input
		ok = true
	}
	if !ok {
		return nil, &normalizeError{status: 400, message: "websocket request requires array field: input"}
	}

	next := cloneMap(raw)
	delete(next, "type")
	next["stream"] = true
	if _, hasModel := next["model"]; !hasModel {
		if m, ok := lastRequest["model"]; ok {
			next["model"] = m
		}
	}
	if _, hasInst := next["instructions"]; !hasInst {
		if inst, ok := lastRequest["instructions"]; ok {
			next["instructions"] = inst
		}
	}

	// An incremental request carries its previous_response_id and new input.
	if supportsIncremental && reqType == "response.create" && responsesWSPreviousResponseID(msg) != "" {
		return &normalizeResult{request: next, nextSnapshot: cloneMap(next)}, nil
	}

	// Without an incremental ID, supply the accumulated turn history.
	merged := make([]any, 0, 16)
	if prevIn, ok := lastRequest["input"].([]any); ok {
		merged = append(merged, prevIn...)
	}
	merged = append(merged, lastOutput...)
	merged = append(merged, input...)
	delete(next, "previous_response_id")
	next["input"] = merged

	return &normalizeResult{request: next, nextSnapshot: cloneMap(next)}, nil
}

// clientRequestsResponsesWSIncremental preserves an explicit previous_response_id
// so the upstream can resolve history for an incremental turn.
func clientRequestsResponsesWSIncremental(msg *ResponsesWSMessage) bool {
	return responsesWSPreviousResponseID(msg) != ""
}

func responsesWSPreviousResponseID(msg *ResponsesWSMessage) string {
	if msg == nil {
		return ""
	}
	if id := strings.TrimSpace(msg.PreviousResponseID); id != "" {
		return id
	}
	if msg.Raw != nil {
		if s, ok := msg.Raw["previous_response_id"].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func shouldHandleLocalPrewarm(msg *ResponsesWSMessage, lastRequest map[string]any, supportsIncremental bool) bool {
	if supportsIncremental || lastRequest != nil {
		return false
	}
	if msg == nil || strings.TrimSpace(msg.Type) != "response.create" {
		return false
	}
	return msg.Generate != nil && !*msg.Generate
}

// bridgeHTTP performs an in-process POST /v1/responses and forwards SSE/JSON to the WS.
func (s *responsesWSSession) bridgeHTTP(ctx context.Context, payload map[string]any, preserveIncremental bool) ([]any, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		_ = s.writeError(ctx, 400, "failed to encode request payload")
		return nil, err
	}

	bridgeReq := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/responses", bytes.NewReader(bodyBytes))
	bridgeReq.Header.Set("Content-Type", "application/json")
	bridgeReq.Header.Set("Accept", "text/event-stream")
	bridgeReq.Header.Set(responsesWebsocketTransportHeader, "1")
	if preserveIncremental {
		bridgeReq.Header.Set(responsesWebsocketModeHeader, "incremental")
	}

	// Inject auth so PrepareCtx / GetProxyAuth succeed.
	if authz := strings.TrimSpace(s.req.Header.Get("Authorization")); authz != "" {
		bridgeReq.Header.Set("Authorization", authz)
	} else if s.auth != nil && strings.TrimSpace(s.auth.Token) != "" {
		bridgeReq.Header.Set("Authorization", "Bearer "+s.auth.Token)
	}
	if k := strings.TrimSpace(s.req.Header.Get("x-api-key")); k != "" {
		bridgeReq.Header.Set("x-api-key", k)
	}
	if k := strings.TrimSpace(s.req.Header.Get("x-goog-api-key")); k != "" {
		bridgeReq.Header.Set("x-goog-api-key", k)
	}
	// Passthrough safe client headers for Codex detection / originator.
	for _, h := range []string{
		"user-agent", "originator", "session-id", "session_id",
		"conversation-id", "conversation_id", "openai-beta",
		"x-codex-beta-features", "x-codex-turn-metadata", wsTurnStateHeader,
		"accept-language",
	} {
		if v := strings.TrimSpace(s.req.Header.Get(h)); v != "" {
			bridgeReq.Header.Set(h, v)
		}
	}

	// Seed proxy auth into context so HandleResponses works without re-running
	// middleware (avoids double used_requests consume on managed keys).
	if s.auth != nil {
		bridgeReq = bridgeReq.WithContext(auth.WithProxyAuth(bridgeReq.Context(), s.auth))
	}

	// C1 accumulates full SSE body via recorder. Live flush-to-WS can upgrade later.
	rec := httptest.NewRecorder()
	HandleResponses(rec, bridgeReq, "/v1/responses")

	status := rec.Code
	respBody := rec.Body.Bytes()
	ct := strings.ToLower(rec.Header().Get("Content-Type"))

	if status < 200 || status >= 300 {
		var errPayload any
		_ = json.Unmarshal(respBody, &errPayload)
		msg := strings.TrimSpace(http.StatusText(status))
		if msg == "" {
			msg = "Upstream error"
		}
		if m, ok := errPayload.(map[string]any); ok {
			if e, ok := m["error"].(map[string]any); ok {
				if em, ok := e["message"].(string); ok && strings.TrimSpace(em) != "" {
					msg = em
				}
			}
		}
		_ = s.writeErrorWithPayload(ctx, status, msg, errPayload)
		return nil, fmt.Errorf("bridge http status %d", status)
	}

	if !strings.Contains(ct, "text/event-stream") {
		var payload any
		if err := json.Unmarshal(respBody, &payload); err != nil {
			_ = s.writeError(ctx, 502, "Unexpected non-JSON websocket proxy response")
			return nil, err
		}
		if err := s.writeJSON(ctx, payload); err != nil {
			return nil, err
		}
		return collectResponsesOutput([]any{payload}), nil
	}

	events, _ := ParseSseStream(string(respBody))
	forwarded := make([]any, 0, len(events))
	sawTerminal := false
	for _, ev := range events {
		if ev.Data == "" || ev.Data == "[DONE]" {
			continue
		}
		var frame any
		if err := json.Unmarshal([]byte(ev.Data), &frame); err != nil {
			continue
		}
		forwarded = append(forwarded, frame)
		if m, ok := frame.(map[string]any); ok {
			t, _ := m["type"].(string)
			switch strings.TrimSpace(t) {
			case "response.completed", "response.failed", "response.incomplete":
				sawTerminal = true
			}
		}
		if err := s.writeJSON(ctx, frame); err != nil {
			return collectResponsesOutput(forwarded), err
		}
	}
	if !sawTerminal {
		_ = s.writeError(ctx, 408, "stream closed before response.completed")
	}
	return collectResponsesOutput(forwarded), nil
}

func (s *responsesWSSession) writeJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
	defer cancel()
	state := requestLifecycleFromContext(ctx)
	if frame, ok := v.(map[string]any); ok && state != nil {
		typ, _ := frame["type"].(string)
		if typ == "error" || typ == "response.failed" || typ == "response.incomplete" {
			state.mu.Lock()
			state.protocolFailed = state.protocolFailed || state.attempted
			// An error envelope relays the HTTP bridge's existing disposition;
			// it must not turn a zero-dispatch refusal into an upstream attempt.
			if state.outcome == "" || state.outcome == shared.OutcomeSuccess {
				state.outcome = shared.OutcomeUpstreamError
			}
			if state.attempted && state.entry.OutputStarted && state.outcome != shared.OutcomeClientError {
				state.failureAfterOutput = true
			}
			state.mu.Unlock()
		}
	}
	err = s.conn.Write(writeCtx, websocket.MessageText, data)
	if state != nil {
		state.mu.Lock()
		state.entry.OutputStarted = state.entry.OutputStarted || err == nil
		state.writeFailed = state.writeFailed || err != nil
		if err == nil {
			if frame, ok := v.(map[string]any); ok && frame["type"] == "response.completed" {
				terminal := nativeTerminalBody{completionProtocol: proxy.EndpointResponses}
				event := SseEvent{Data: string(data)}
				terminal.observeTerminalEvent(&event)
				state.protocolCompleted = terminal.terminalSeen && !terminal.terminalFailed
				state.protocolFailed = state.protocolFailed || terminal.terminalFailed
			}
			if state.entry.FirstOutputLatencyMs == nil && hasGeneratedSseOutput(SseEvent{Data: string(data)}) {
				elapsed := time.Since(state.started).Milliseconds()
				state.entry.FirstOutputLatencyMs = &elapsed
			}
		}
		state.mu.Unlock()
	}
	return err
}

func (s *responsesWSSession) writeError(ctx context.Context, status int, message string) error {
	return s.writeErrorWithPayload(ctx, status, message, nil)
}

func (s *responsesWSSession) writeErrorWithPayload(ctx context.Context, status int, message string, errPayload any) error {
	if state := requestLifecycleFromContext(ctx); state != nil && status >= 400 && status < 500 {
		state.mu.Lock()
		if !state.attempted {
			state.rejection = "invalid_request"
		}
		state.mu.Unlock()
	}
	frame := ResponsesWSError(status, message)
	if errPayload != nil {
		if m, ok := errPayload.(map[string]any); ok {
			if e, ok := m["error"]; ok {
				frame["error"] = e
			}
		}
	}
	return s.writeJSON(ctx, frame)
}

// ---- helpers ----

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

// extractWSTurnState extracts the x-codex-turn-state header value.
func extractWSTurnState(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(wsTurnStateHeader))
}

// ResponsesWSMessage is a WebSocket message from the downstream client.
type ResponsesWSMessage struct {
	Type               string         `json:"type"`
	Model              string         `json:"model,omitempty"`
	Generate           *bool          `json:"generate,omitempty"`
	Input              []any          `json:"input,omitempty"`
	Instructions       any            `json:"instructions,omitempty"`
	PreviousResponseID string         `json:"previous_response_id,omitempty"`
	Raw                map[string]any `json:"-"`
}

// ParseResponsesWSMessage parses a WebSocket JSON message.
func ParseResponsesWSMessage(raw []byte) (*ResponsesWSMessage, error) {
	var msg ResponsesWSMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	msg.Raw = make(map[string]any)
	_ = json.Unmarshal(raw, &msg.Raw)
	return &msg, nil
}

// ResponsesWSError builds a WebSocket-shaped error object.
func ResponsesWSError(status int, message string) map[string]any {
	typ := "invalid_request_error"
	if status >= 500 {
		typ = "server_error"
	}
	return map[string]any{
		"type":   "error",
		"status": status,
		"error": map[string]any{
			"type":    typ,
			"message": message,
		},
	}
}

// SynthesizePrewarmResponsePayloads generates pre-warm response.created + response.completed
// for Codex file-search pre-warm mode (generate=false without incremental input).
func SynthesizePrewarmResponsePayloads(model string, responseID string) []map[string]any {
	if responseID == "" {
		responseID = "resp_prewarm_metapi"
	}
	modelName := model
	if modelName == "" {
		modelName = "unknown"
	}
	createdAt := time.Now().Unix()
	return []map[string]any{
		{
			"type": "response.created",
			"response": map[string]any{
				"id":         responseID,
				"object":     "response",
				"created_at": createdAt,
				"status":     "in_progress",
				"model":      modelName,
				"output":     []any{},
			},
		},
		{
			"type": "response.completed",
			"response": map[string]any{
				"id":         responseID,
				"object":     "response",
				"created_at": createdAt,
				"status":     "completed",
				"model":      modelName,
				"output":     []any{},
				"usage": map[string]any{
					"input_tokens":  0,
					"output_tokens": 0,
					"total_tokens":  0,
				},
			},
		},
	}
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	b, err := json.Marshal(in)
	if err != nil {
		out := make(map[string]any, len(in))
		for k, v := range in {
			out[k] = v
		}
		return out
	}
	out := make(map[string]any)
	if err := json.Unmarshal(b, &out); err != nil {
		out2 := make(map[string]any, len(in))
		for k, v := range in {
			out2[k] = v
		}
		return out2
	}
	return out
}

// collectResponsesOutput extracts assistant output items from forwarded events
// (TS collectResponsesOutput parity, simplified).
func collectResponsesOutput(payloads []any) []any {
	outputByIndex := map[int]any{}
	var completed []any
	for _, payload := range payloads {
		m, ok := payload.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		switch typ {
		case "response.output_item.added", "response.output_item.done":
			if idx, ok := asInt(m["output_index"]); ok {
				if item, ok := m["item"]; ok {
					outputByIndex[idx] = item
				}
			}
		case "response.completed", "response.incomplete", "response.failed":
			if resp, ok := m["response"].(map[string]any); ok {
				if out, ok := resp["output"].([]any); ok {
					completed = out
				}
			}
		}
		if out, ok := m["output"].([]any); ok && completed == nil {
			completed = out
		}
	}
	if completed != nil {
		return completed
	}
	if len(outputByIndex) == 0 {
		return nil
	}
	maxIdx := -1
	for i := range outputByIndex {
		if i > maxIdx {
			maxIdx = i
		}
	}
	out := make([]any, 0, len(outputByIndex))
	for i := 0; i <= maxIdx; i++ {
		if v, ok := outputByIndex[i]; ok {
			out = append(out, v)
		}
	}
	return out
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func newResponsesWSClientSessionID() string {
	return fmt.Sprintf("ws-%d-%d", time.Now().UnixNano(), time.Now().Unix()%1000)
}

// tryCodexUpstreamWSS attempts Codex platform upstream wss. Returns handled=false
// to fall through to HTTP bridge (including dial failures with zero events).
func (s *responsesWSSession) tryCodexUpstreamWSS(ctx context.Context, body map[string]any, requestModel string, preserveIncremental bool) (output []any, handled bool, err error) {
	cfg := getUpstreamConfig()
	if cfg == nil || cfg.Router == nil {
		return nil, false, nil
	}
	runtimeCfg := config.RuntimeSafe()
	if runtimeCfg == nil || !runtimeCfg.CodexUpstreamWebsocketEnabled {
		return nil, false, nil
	}

	policy := RoutingPolicyFromAuth(auth.EmptyDownstreamRoutingPolicy)
	if s.auth != nil {
		policy = RoutingPolicyFromAuth(s.auth.Policy)
	}
	policy.RequestedContextTokens = routing.EstimateRequestContextTokens(body)
	selected, selErr := cfg.Router.SelectChannel(ctx, requestModel, policy)
	if selErr != nil || selected == nil {
		return nil, false, nil
	}
	if !SelectedChannelSupportsCodexWebsocketTransport(selected.Site.Platform, selected.Account.ExtraConfig, requestModel) {
		return nil, false, nil
	}

	upstreamModel := selected.ActualModel
	if strings.TrimSpace(upstreamModel) == "" {
		upstreamModel = requestModel
	}
	upBody := cloneMap(body)
	upBody["model"] = upstreamModel
	upBody["stream"] = true

	upstreamURL := proxyBuildUpstreamURL(selected.Site.URL, "/v1/responses")
	if upstreamURL == "" {
		return nil, false, nil
	}

	headers := map[string]string{}
	if selected.TokenValue != "" {
		headers["Authorization"] = "Bearer " + selected.TokenValue
	}
	if s.req != nil {
		for _, h := range []string{"user-agent", "originator", "session-id", "session_id", "openai-beta", wsTurnStateHeader} {
			if v := strings.TrimSpace(s.req.Header.Get(h)); v != "" {
				headers[h] = v
			}
		}
	}
	headers[responsesWebsocketTransportHeader] = "1"
	if preserveIncremental {
		headers[responsesWebsocketModeHeader] = "incremental"
	}

	sessionKey := BuildCodexSessionResponseStoreKey(s.clientSessionID, selected.Site.ID, selected.Account.ID, selected.Channel.ID)
	if sessionKey == "" {
		sessionKey = s.clientSessionID
	}
	found := false
	for _, k := range s.codexRuntimeKeys {
		if k == sessionKey {
			found = true
			break
		}
	}
	if !found {
		s.codexRuntimeKeys = append(s.codexRuntimeKeys, sessionKey)
	}

	// Resin sticky identity for native wss reverse-proxy rewrite. The wss
	// dial bypasses BuildPlatformProxyConfig, so we pass the identity through
	// the input; sendCodexWSSessionAttempt rewrites the URL + sets
	// X-Resin-Account when Resin is enabled. Platform = cfg.ResinPlatformName
	// (fallback site.Platform); Account = acc-{selected.Account.ID}.
	resinAccount := ""
	resinPlatform := ""
	if runtimeCfg := safeConfigGet(); runtimeCfg != nil && service.ResinEnabled(runtimeCfg, &selected.Site) {
		resinAccount = service.AccountFor(&selected.Account, &selected.Site)
		resinPlatform = service.ResolveResinPlatform(runtimeCfg, &selected.Site)
		if resinAccount != "" {
			service.TouchResinLease(resinAccount)
		}
	}

	result, sendErr := SendCodexWebsocketRequest(ctx, CodexWebsocketSendInput{
		SessionID:     sessionKey,
		RequestURL:    upstreamURL,
		Headers:       headers,
		Body:          upBody,
		ResinAccount:  resinAccount,
		ResinPlatform: resinPlatform,
		ObserveAttempt: func(events []map[string]any, attemptErr error, duration time.Duration) {
			attemptCtx := beginGatewayAttempt(ctx)
			proxyCtx := &Ctx{Auth: s.auth, RequestedModel: requestModel, DownstreamPath: "/v1/responses", IsStream: true, lifecycle: requestLifecycleFromContext(ctx)}
			usage := ParsedUsage{Source: usageSourceUnknown}
			for _, event := range events {
				data, _ := json.Marshal(event)
				if parsed := ParseUsageFromBody(data); parsed.Found {
					usage = parsed
				}
			}
			requestID := proxy.RequestIDFromContext(ctx)
			if attemptErr != nil {
				writeFailureProxyLog(attemptCtx, cfg, selected, proxyCtx, upstreamModel, "/v1/responses", duration.Milliseconds(), nil, 0, true, usage, 0, requestID, "upstream websocket attempt failed")
				proxyCtx.lifecycle.observe(shared.OutcomeUpstreamError, true)
			} else {
				writeSuccessProxyLog(attemptCtx, cfg, selected, proxyCtx, upstreamModel, "/v1/responses", duration.Milliseconds(), nil, 0, true, usage, 0, requestID)
				proxyCtx.lifecycle.observe(shared.OutcomeSuccess, true)
			}
		},
	})
	if sendErr != nil {
		runtimeErr, ok := sendErr.(*CodexWebsocketRuntimeError)
		if ok && runtimeErr.Status != 0 && len(runtimeErr.Events) == 0 {
			LogCodexWebsocketDialFallback(sessionKey, sendErr)
			return nil, false, nil
		}
		if ok {
			for _, ev := range runtimeErr.Events {
				if werr := s.writeJSON(ctx, ev); werr != nil {
					return nil, true, werr
				}
			}
			sawTerminal := false
			for _, ev := range runtimeErr.Events {
				t, _ := ev["type"].(string)
				switch strings.TrimSpace(t) {
				case "response.completed", "response.failed", "response.incomplete":
					sawTerminal = true
				}
			}
			if !sawTerminal {
				status := runtimeErr.Status
				if status == 0 {
					status = 408
				}
				_ = s.writeError(ctx, status, runtimeErr.Error())
			}
			return collectResponsesOutputFromMaps(runtimeErr.Events), true, nil
		}
		LogCodexWebsocketDialFallback(sessionKey, sendErr)
		return nil, false, nil
	}

	for _, ev := range result.Events {
		if werr := s.writeJSON(ctx, ev); werr != nil {
			return nil, true, werr
		}
	}
	return collectResponsesOutputFromMaps(result.Events), true, nil
}

func proxyBuildUpstreamURL(siteURL, path string) string {
	base := strings.TrimRight(strings.TrimSpace(siteURL), "/")
	if base == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func collectResponsesOutputFromMaps(events []map[string]any) []any {
	out := make([]any, 0)
	for _, ev := range events {
		if resp, ok := ev["response"].(map[string]any); ok {
			if arr, ok := resp["output"].([]any); ok && len(arr) > 0 {
				out = append(out, arr...)
			}
		}
	}
	return out
}
