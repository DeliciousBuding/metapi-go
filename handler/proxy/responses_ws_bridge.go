package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
)

// responsesBridgeRecorder captures an in-process HTTP handler response without
// pulling net/http/httptest (and the testing package) into production binaries.
// It matches http.ResponseWriter / httptest.ResponseRecorder commit semantics:
// first Write/Flush commits 200, WriteHeader validates status codes and honors
// only the first call, 1xx/204/304 reject body writes with http.ErrBodyNotAllowed,
// and an unset Content-Type is sniffed on the first non-empty write.
type responsesBridgeRecorder struct {
	header      http.Header
	body        bytes.Buffer
	statusCode  int
	wroteHeader bool
}

func newResponsesBridgeRecorder() *responsesBridgeRecorder {
	return &responsesBridgeRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *responsesBridgeRecorder) Header() http.Header { return r.header }

func (r *responsesBridgeRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	if code < 100 || code > 999 {
		panic(fmt.Sprintf("invalid WriteHeader code %v", code))
	}
	r.statusCode = code
	r.wroteHeader = true
}

func (r *responsesBridgeRecorder) bodyAllowed() bool {
	if !r.wroteHeader {
		return true
	}
	switch {
	case r.statusCode >= 100 && r.statusCode <= 199:
		return false
	case r.statusCode == http.StatusNoContent:
		return false
	case r.statusCode == http.StatusNotModified:
		return false
	}
	return true
}

func (r *responsesBridgeRecorder) writeHeader(b []byte) {
	if r.wroteHeader {
		return
	}
	_, hasType := r.header["Content-Type"]
	hasTE := r.header.Get("Transfer-Encoding") != ""
	if !hasType && !hasTE {
		r.header.Set("Content-Type", http.DetectContentType(b))
	}
	r.WriteHeader(http.StatusOK)
}

func (r *responsesBridgeRecorder) Write(p []byte) (int, error) {
	r.writeHeader(p)
	r.body.Write(p)
	if !r.bodyAllowed() {
		return 0, http.ErrBodyNotAllowed
	}
	return len(p), nil
}

func (r *responsesBridgeRecorder) Flush() {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
}

// bridgeHTTP performs an in-process POST /v1/responses and forwards SSE/JSON to the WS.
func (s *responsesWSSession) bridgeHTTP(ctx context.Context, payload map[string]any, preserveIncremental bool) ([]any, error) {
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		_ = s.writeError(ctx, 400, "failed to encode request payload")
		return nil, err
	}

	bridgeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://metapi.local/v1/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		_ = s.writeError(ctx, 500, "failed to construct bridge request")
		return nil, err
	}
	bridgeReq.RequestURI = "/v1/responses"
	bridgeReq.RemoteAddr = s.req.RemoteAddr
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

	// Accumulate full SSE/JSON body in-memory before projecting to WS frames.
	rec := newResponsesBridgeRecorder()
	HandleResponses(rec, bridgeReq, "/v1/responses")

	status := rec.statusCode
	respBody := rec.body.Bytes()
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
			sawTerminal = sawTerminal || isResponsesTerminalType(asJSONString(m["type"]))
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
