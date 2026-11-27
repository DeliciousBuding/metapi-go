package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/service"
)

const (
	codexWSDefaultBeta = "responses_websockets=2026-02-06"
	codexWSDialTimeout = 20 * time.Second
)

// SelectedChannelSupportsCodexWebsocketTransport reports whether an upstream
// Codex wss dial should be attempted (TS selectedChannelSupportsCodexWebsocketTransport).
// Platform must be codex; global flag CodexUpstreamWebsocketEnabled must be on;
// optional account extraConfig websockets false disables.
func SelectedChannelSupportsCodexWebsocketTransport(platform string, accountExtraConfig *string, requestModel string) bool {
	_ = requestModel // model match is applied by channel selection; probe stays platform-level.
	if strings.ToLower(strings.TrimSpace(platform)) != "codex" {
		return false
	}
	if rt := config.RuntimeSafe(); rt == nil || !rt.CodexUpstreamWebsocketEnabled {
		return false
	}
	if accountExtraConfig == nil || strings.TrimSpace(*accountExtraConfig) == "" {
		return true
	}
	var extra map[string]any
	if err := json.Unmarshal([]byte(*accountExtraConfig), &extra); err != nil {
		return true
	}
	// Look for websockets flags in extraConfig / attributes / metadata / oauth.providerData.
	candidates := []any{
		extra["websockets"],
	}
	if attrs, ok := extra["attributes"].(map[string]any); ok {
		candidates = append(candidates, attrs["websockets"])
	}
	if meta, ok := extra["metadata"].(map[string]any); ok {
		candidates = append(candidates, meta["websockets"])
	}
	if oauth, ok := extra["oauth"].(map[string]any); ok {
		if pd, ok := oauth["providerData"].(map[string]any); ok {
			candidates = append(candidates, pd["websockets"])
			if attrs, ok := pd["attributes"].(map[string]any); ok {
				candidates = append(candidates, attrs["websockets"])
			}
			if meta, ok := pd["metadata"].(map[string]any); ok {
				candidates = append(candidates, meta["websockets"])
			}
		}
	}
	for _, c := range candidates {
		if b, ok := toBoolLike(c); ok {
			return b
		}
	}
	return true
}

func toBoolLike(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		switch s {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		}
	case float64:
		return t != 0, true
	case int:
		return t != 0, true
	}
	return false, false
}

// ToCodexWebsocketURL converts an https/http responses URL to wss/ws.
func ToCodexWebsocketURL(requestURL string) string {
	u := strings.TrimSpace(requestURL)
	if u == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(strings.ToLower(u), "https:"):
		return "wss:" + u[6:]
	case strings.HasPrefix(strings.ToLower(u), "http:"):
		return "ws:" + u[5:]
	default:
		return u
	}
}

// BuildCodexWebsocketHandshakeHeaders ensures OpenAI-Beta includes responses_websockets.
func BuildCodexWebsocketHandshakeHeaders(headers map[string]string) map[string]string {
	next := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		next[k] = v
	}
	beta := codexWSDefaultBeta
	if cfg := safeConfigGet(); cfg != nil && strings.TrimSpace(cfg.CodexResponsesWebsocketBeta) != "" {
		beta = strings.TrimSpace(cfg.CodexResponsesWebsocketBeta)
	}
	existing := ""
	for k, v := range next {
		if strings.EqualFold(strings.TrimSpace(k), "openai-beta") {
			existing = strings.TrimSpace(v)
			break
		}
	}
	if existing == "" {
		next["OpenAI-Beta"] = beta
		return next
	}
	if !strings.Contains(existing, "responses_websockets=") {
		// Replace the openAI-beta key with merged value (preserve original casing if present).
		for k := range next {
			if strings.EqualFold(strings.TrimSpace(k), "openai-beta") {
				next[k] = existing + "," + beta
				return next
			}
		}
		next["OpenAI-Beta"] = existing + "," + beta
	}
	return next
}

func (session *codexWSSocketSession) connect(ctx context.Context, input CodexWebsocketSendInput) (*websocket.Conn, bool, error) {
	wsURL := ToCodexWebsocketURL(input.RequestURL)
	if wsURL == "" {
		return nil, false, &CodexWebsocketRuntimeError{Message: "missing upstream websocket url", Status: 502}
	}

	// Resin reverse-proxy rewrite for native wss dials (the only path that
	// bypasses BuildPlatformProxyConfig / the HTTP forward proxy). Resin
	// requires the client→resin leg to be ws; the protocol segment encodes
	// the original target scheme (https for wss, http for ws).
	resinAccount := strings.TrimSpace(input.ResinAccount)
	resinPlatform := strings.TrimSpace(input.ResinPlatform)
	runtimeCfg := safeConfigGet()
	// The caller already resolved the site override. Dialing checks the
	// global flag and the supplied identity.
	if runtimeCfg != nil && service.ResinEnabled(runtimeCfg, nil) && resinAccount != "" && resinPlatform != "" {
		if rewritten := service.RewriteWSURL(wsURL, runtimeCfg, resinPlatform, resinAccount); rewritten != "" {
			wsURL = rewritten
		}
	}

	conn := session.conn
	if conn != nil && !session.lastUsed.IsZero() && time.Since(session.lastUsed) >= codexSessionTTL() {
		clearSessionSocket(session, conn)
		_ = conn.Close(websocket.StatusNormalClosure, "idle session expired")
		conn = nil
		clearCodexSessionResponseID(input.SessionID)
	}
	if conn != nil && session.socketURL == wsURL {
		return conn, true, nil
	}
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "replace session")
		session.conn = nil
		session.socketURL = ""
	}
	headers := BuildCodexWebsocketHandshakeHeaders(input.Headers)
	// Resin reverse-proxy identity header (only meaningful when the URL
	// was rewritten above; harmless otherwise since Resin ignores it on
	// non-rewritten paths, but we gate it to the rewrite case for hygiene).
	if runtimeCfg != nil && service.ResinEnabled(runtimeCfg, nil) && resinAccount != "" && resinPlatform != "" {
		if headers == nil {
			headers = map[string]string{}
		}
		headers["X-Resin-Account"] = resinAccount
	}
	httpHeader := http.Header{}
	for k, v := range headers {
		if strings.TrimSpace(k) == "" || strings.TrimSpace(v) == "" {
			continue
		}
		httpHeader.Set(k, v)
	}
	dialCtx, cancel := context.WithTimeout(ctx, codexWSDialTimeout)
	client := input.HTTPClient
	if client == nil {
		client = httpclient.NewClient(httpclient.SharedTransport(), 0, nil)
	}
	next, _, err := websocket.Dial(dialCtx, wsURL, &websocket.DialOptions{
		HTTPHeader: httpHeader,
		// Bound the upgrade's dial/TLS phases on the pooled baseline
		// transport; the dial context remains the deadline owner.
		HTTPClient: client,
	})
	cancel()
	if err != nil {
		return nil, false, &CodexWebsocketRuntimeError{
			Message: fmt.Sprintf("upstream websocket upgrade failed: %v", err),
			Status:  502,
		}
	}
	next.SetReadLimit(codexWSMaxMsgBytes)
	session.conn = next
	session.socketURL = wsURL
	return next, false, nil
}

// LogCodexWebsocketDialFallback records honest dial→HTTP fallback.
func LogCodexWebsocketDialFallback(sessionID string, err error) {
	slog.Info("codex upstream wss dial failed; falling back to HTTP bridge",
		"session", normalizeCodexSessionKey(sessionID),
		"err", err,
	)
}
