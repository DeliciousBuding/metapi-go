package proxyhandler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
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
