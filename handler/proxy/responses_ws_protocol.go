package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
)

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
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&msg.Raw); err != nil {
		return nil, err
	}
	return &msg, nil
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
		return cloneJSONMapShallow(in)
	}
	out := make(map[string]any)
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return cloneJSONMapShallow(in)
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
		switch {
		case typ == "response.output_item.added" || typ == "response.output_item.done":
			if idx, ok := asInt(m["output_index"]); ok {
				if item, ok := m["item"]; ok {
					outputByIndex[idx] = item
				}
			}
		case isResponsesTerminalType(typ):
			if out, ok := responsesEventOutput(m); ok {
				completed = out
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

func isResponsesTerminalType(typ string) bool {
	return typ == "response.completed" || typ == "response.incomplete" || typ == "response.failed"
}

func responsesEventOutput(event map[string]any) ([]any, bool) {
	response, _ := event["response"].(map[string]any)
	output, ok := response["output"].([]any)
	return output, ok
}

// Native WS retains nested output from every frame for the next turn.
func collectResponsesOutputFromMaps(events []map[string]any) []any {
	out := make([]any, 0)
	for _, ev := range events {
		items, _ := responsesEventOutput(ev)
		out = append(out, items...)
	}
	return out
}

func (s *responsesWSSession) writeJSON(ctx context.Context, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
	defer cancel()
	state := requestLifecycleFromContext(ctx)
	acceptedIncomplete, _ := ctx.Value(acceptedNativeResponsesIncompleteKey{}).(bool)
	if frame, ok := v.(map[string]any); ok && state != nil {
		typ, _ := frame["type"].(string)
		if typ == "error" || typ == "response.failed" || typ == "response.incomplete" && !acceptedIncomplete {
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
			if acceptedIncomplete {
				state.protocolCompleted = true
			}
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
