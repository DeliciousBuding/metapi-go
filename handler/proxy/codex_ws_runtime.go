package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

const (
	codexWSReadTimeout = 10 * time.Minute
	codexWSMaxMsgBytes = 8 << 20
)

// CodexWebsocketRuntimeError is returned when an upstream Codex wss turn fails.
// Dispatched marks a generation write, including ambiguous write failures.
type CodexWebsocketRuntimeError struct {
	Message    string
	Status     int
	Events     []map[string]any
	Payload    any
	Dispatched bool
}

func (e *CodexWebsocketRuntimeError) Error() string {
	if e == nil {
		return "codex websocket runtime error"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return "codex websocket runtime error"
}

// CodexWebsocketRuntimeResult is a successful terminal turn.
type CodexWebsocketRuntimeResult struct {
	Events        []map[string]any
	ReusedSession bool
}

// CodexWebsocketSendInput is one upstream wss turn.
type CodexWebsocketSendInput struct {
	// ObserveAttempt accounts for each attempted generation write, including
	// continuation recovery. A dial failure has no generation attempt.
	ObserveAttempt func([]map[string]any, error, time.Duration)
	// DecodeEvent observes/adapts a bounded raw frame before the native parser.
	// The host callback owns observations; the transport never retries its error.
	DecodeEvent             func([]byte) ([]byte, error)
	BeforeAttempt           func() error
	HTTPClient              *http.Client
	DisablePreviousRecovery bool
	AllowNativeIncomplete   bool
	SessionID               string
	RequestURL              string
	Headers                 map[string]string
	Body                    map[string]any
	// ResinAccount carries the Resin business identity for the native wss
	// reverse-proxy rewrite. When non-empty (and ResinPlatform is set) and
	// Resin is enabled, the dial URL is rewritten to
	// ws://{host}/{token}/{Platform}/{protocol}/{target} and X-Resin-Account
	// is set on the dial headers.
	ResinAccount string
	// ResinPlatform is the Platform segment for the resin reverse-proxy path.
	// The caller (which has the site in scope) resolves it from
	// cfg.ResinPlatformName (falling back to site.Platform).
	ResinPlatform string
}

// SendCodexWebsocketRequest dials (or reuses) an upstream Codex wss session and
// waits for a terminal event. previous_response_id not-found recovers once by strip.
func SendCodexWebsocketRequest(ctx context.Context, input CodexWebsocketSendInput) (*CodexWebsocketRuntimeResult, error) {
	sessionID := normalizeCodexSessionKey(input.SessionID)
	if sessionID == "" {
		return nil, &CodexWebsocketRuntimeError{Message: "missing websocket session id", Status: 400}
	}
	session := getOrCreateCodexWSSocketSession(sessionID)
	if session == nil {
		return nil, &CodexWebsocketRuntimeError{Message: "websocket session capacity exhausted", Status: 503}
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.lastUsed.IsZero() && time.Since(session.lastUsed) >= codexSessionTTL() {
		clearCodexSessionResponseID(sessionID)
	}

	currentBody := buildContinuationAwareRuntimeBody(sessionID, input.Body)
	previousRecoveryTried := false

	for {
		result, err := sendCodexWSSessionAttempt(ctx, session, CodexWebsocketSendInput{
			ObserveAttempt:        input.ObserveAttempt,
			DecodeEvent:           input.DecodeEvent,
			BeforeAttempt:         input.BeforeAttempt,
			HTTPClient:            input.HTTPClient,
			AllowNativeIncomplete: input.AllowNativeIncomplete,
			SessionID:             sessionID,
			RequestURL:            input.RequestURL,
			Headers:               input.Headers,
			Body:                  currentBody,
			ResinAccount:          input.ResinAccount,
			ResinPlatform:         input.ResinPlatform,
		})
		if err == nil {
			return result, nil
		}
		if input.DisablePreviousRecovery {
			return nil, err
		}
		runtimeErr, ok := err.(*CodexWebsocketRuntimeError)
		if !ok {
			return nil, err
		}
		if previousRecoveryTried {
			return nil, runtimeErr
		}
		payload := runtimeErr.Payload
		if payload == nil && len(runtimeErr.Events) > 0 {
			payload = runtimeErr.Events[len(runtimeErr.Events)-1]
		}
		if !isResponsesPreviousResponseNotFoundError(runtimeErr.Message, payload) {
			return nil, runtimeErr
		}
		stripped, removed := stripResponsesPreviousResponseID(currentBody)
		if !removed {
			return nil, runtimeErr
		}
		previousRecoveryTried = true
		clearCodexSessionResponseID(sessionID)
		currentBody = stripped
	}
}

func sendCodexWSSessionAttempt(ctx context.Context, session *codexWSSocketSession, input CodexWebsocketSendInput) (result *CodexWebsocketRuntimeResult, attemptErr error) {
	started := time.Now()
	dispatched := false
	defer func() {
		session.lastUsed = time.Now()
		if runtimeErr, ok := attemptErr.(*CodexWebsocketRuntimeError); ok {
			runtimeErr.Dispatched = dispatched
		}
		if !dispatched || input.ObserveAttempt == nil {
			return
		}
		var events []map[string]any
		if result != nil {
			events = result.Events
		}
		if runtimeErr, ok := attemptErr.(*CodexWebsocketRuntimeError); ok {
			events = runtimeErr.Events
		}
		input.ObserveAttempt(events, attemptErr, time.Since(started))
	}()
	conn, reused, err := session.connect(ctx, input)
	if err != nil {
		return nil, err
	}

	body := BuildCodexWebsocketRequestBody(input.Body)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &CodexWebsocketRuntimeError{Message: "failed to encode upstream websocket body", Status: 400}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if input.BeforeAttempt != nil {
		if err := input.BeforeAttempt(); err != nil {
			return nil, err
		}
	}
	dispatched = true

	writeCtx, writeCancel := context.WithTimeout(ctx, 30*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, payload)
	writeCancel()
	if err != nil {
		clearSessionSocket(session, conn)
		_ = conn.Close(websocket.StatusGoingAway, "write failed")
		return nil, &CodexWebsocketRuntimeError{
			Message: fmt.Sprintf("failed to send upstream websocket request: %v", err),
			Status:  502,
		}
	}

	events := make([]map[string]any, 0, 16)
	totalBytes := int64(0)
	decodedBytes := int64(0)
	collector := &responses.CodexResponseCollector{AllowIncomplete: input.AllowNativeIncomplete}
	for {
		readCtx, readCancel := context.WithTimeout(ctx, codexWSReadTimeout)
		_, data, readErr := conn.Read(readCtx)
		readCancel()
		if readErr != nil {
			clearSessionSocket(session, conn)
			return nil, &CodexWebsocketRuntimeError{
				Message: "stream closed before response.completed",
				Status:  408,
				Events:  events,
			}
		}
		var parsed map[string]any
		totalBytes += int64(len(data))
		if totalBytes > codexWSMaxMsgBytes {
			clearSessionSocket(session, conn)
			_ = conn.Close(websocket.StatusGoingAway, "invalid or oversized response")
			return nil, &CodexWebsocketRuntimeError{Message: "invalid or oversized upstream websocket response", Status: 502, Events: events}
		}
		if input.DecodeEvent != nil {
			var err error
			data, err = input.DecodeEvent(data)
			if err != nil {
				clearSessionSocket(session, conn)
				_ = conn.Close(websocket.StatusGoingAway, "provider event decoding failed")
				return nil, &CodexWebsocketRuntimeError{Message: "provider event decoding failed", Status: 502, Events: events}
			}
		}
		decodedBytes += int64(len(data))
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&parsed); err != nil || parsed == nil || decoder.Decode(new(any)) != io.EOF || decodedBytes > codexWSMaxMsgBytes {
			clearSessionSocket(session, conn)
			_ = conn.Close(websocket.StatusGoingAway, "invalid or oversized response")
			return nil, &CodexWebsocketRuntimeError{Message: "invalid or oversized upstream websocket response", Status: 502, Events: events}
		}
		events = append(events, parsed)
		if collectErr := collector.AddData(data); collectErr != nil {
			clearSessionSocket(session, conn)
			_ = conn.Close(websocket.StatusGoingAway, "invalid terminal response")
			return nil, &CodexWebsocketRuntimeError{Message: extractTerminalErrorMessage(parsed), Status: extractFailureTerminalStatus(parsed), Events: events, Payload: parsed}
		}
		if !isTerminalResponsesEvent(parsed) {
			continue
		}
		if isRuntimeErrorEvent(parsed) || isResponsesPreviousResponseNotFoundError(extractTerminalErrorMessage(parsed), parsed) {
			clearSessionSocket(session, conn)
			_ = conn.Close(websocket.StatusGoingAway, "upstream error")
			return nil, &CodexWebsocketRuntimeError{
				Message: extractTerminalErrorMessage(parsed),
				Status:  extractFailureTerminalStatus(parsed),
				Events:  events,
				Payload: parsed,
			}
		}
		if rid := extractResponsesTerminalResponseID(parsed); rid != "" {
			setCodexSessionResponseID(session.sessionID, rid)
		}
		return &CodexWebsocketRuntimeResult{Events: events, ReusedSession: reused}, nil
	}
}
