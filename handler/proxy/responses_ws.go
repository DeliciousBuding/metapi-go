package proxyhandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
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

// insecureAPIKeySubprotocolPrefix mirrors auth's browser WebSocket key
// convention. The value is offered by the client and echoed on accept.
const insecureAPIKeySubprotocolPrefix = "openai-insecure-api-key."

// insecureAPIKeySubprotocols returns the client-offered insecure-key
// subprotocols that the server may select. Only this prefix is negotiated;
// unrelated protocols stay unselected.
func insecureAPIKeySubprotocols(r *http.Request) []string {
	if r == nil {
		return nil
	}
	var protocols []string
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, part := range strings.Split(header, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, insecureAPIKeySubprotocolPrefix) && len(part) > len(insecureAPIKeySubprotocolPrefix) {
				protocols = append(protocols, part)
			}
		}
	}
	return protocols
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
		// Echo the OpenAI insecure-key subprotocol so browser WebSocket clients
		// can authenticate without putting the key in the URL (audit L2).
		Subprotocols: insecureAPIKeySubprotocols(r),
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
		if session.directRuntimeKey != "" {
			CloseCodexWebsocketSession(session.directRuntimeKey)
			clearCodexSessionResponseID(session.directRuntimeKey)
		}
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
	codexRuntimeKeys     []string
	directRuntimeKey     string
	directAuthentication *channelplugins.AuthenticationLease
	// message serialisation: one in-flight turn at a time (Codex client queue).
	queueMu sync.Mutex
}

func (s *responsesWSSession) loop(ctx context.Context) error {
	ctx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	// Canceling a pending Read closes its socket without a close handshake.
	// Finish the peer handshake first, then stop the session reader.
	defer func() { _ = s.conn.Close(websocket.StatusNormalClosure, "session end") }()
	type incoming struct {
		data []byte
		err  error
	}
	// Keep reading while a turn is upstream so downstream disconnect cancels
	// its queue wait/socket read immediately. Only one pending frame is held;
	// all authorization and turn execution remains serial below.
	messages := make(chan incoming, 1)
	go func() {
		for {
			readCtx, cancel := context.WithTimeout(ctx, wsReadIdleTimeout)
			kind, data, err := s.conn.Read(readCtx)
			cancel()
			if err != nil {
				cancelSession()
				select {
				case messages <- incoming{err: err}:
				default:
				}
				return
			}
			if kind != websocket.MessageText && kind != websocket.MessageBinary {
				continue
			}
			select {
			case messages <- incoming{data: data}:
			case <-ctx.Done():
				return
			default:
				// A caller may queue one following turn. Do not retain an
				// unbounded client burst while an upstream turn is running.
				cancelSession()
				_ = s.conn.CloseNow()
				return
			}
		}
	}()
	for {
		var next incoming
		select {
		case next = <-messages:
		case <-ctx.Done():
			return ctx.Err()
		}
		if next.err != nil {
			return next.err
		}
		s.queueMu.Lock()
		err := s.handleMessage(ctx, next.data)
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

// extractWSTurnState extracts the x-codex-turn-state header value.
func extractWSTurnState(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.Header.Get(wsTurnStateHeader))
}

func newResponsesWSClientSessionID() string {
	return fmt.Sprintf("ws-%d-%d", time.Now().UnixNano(), time.Now().Unix()%1000)
}
