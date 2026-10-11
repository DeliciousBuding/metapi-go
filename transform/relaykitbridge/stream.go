package relaykitbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/relayconvert"
)

const (
	maxFrameBytes  = 16 << 20
	maxStreamBytes = 64 << 20
)

// Stream consumes complete SSE frames. The transport owns reading/framing and
// cancellation; the request context is also checked before every library call.
type Stream struct {
	session                 *Session
	state                   *relayconvert.ResponseStreamState
	source                  terminalState
	inputBytes, outputBytes int
	pending                 []byte
	finished                bool
	err                     error
}

func (s *Session) NewResponseStream() (*Stream, error) {
	if s == nil || s.streamCreated {
		return nil, fmt.Errorf("response stream already created or missing session: %w", ErrConversion)
	}
	from, _ := kitFormat(s.to)
	to, _ := kitFormat(s.from)
	state, err := relayconvert.NewResponseStreamState(from, to, relayconvert.ResponseStreamOptions{Model: s.model, IncludeUsage: true, EmitSequenceNumber: true})
	if err != nil {
		return nil, ErrConversion
	}
	s.streamCreated = true
	return &Stream{session: s, state: state}, nil
}

func (s *Stream) fail(err error) ([]byte, error) { s.err = err; s.pending = nil; return nil, err }

func (s *Stream) TransformEvent(frame []byte) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.finished {
		return s.fail(fmt.Errorf("event after stream Finish: %w", ErrConversion))
	}
	if s.session.from == s.session.to {
		return frame, nil
	}
	if err := s.session.ctx.Err(); err != nil {
		return s.fail(err)
	}
	s.inputBytes += len(frame)
	if len(frame) > maxFrameBytes || s.inputBytes > maxStreamBytes {
		return s.fail(fmt.Errorf("protocol stream byte limit exceeded: %w", ErrConversion))
	}
	event, data, err := parseFrame(frame)
	if err != nil {
		return s.fail(err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	if event == "error" {
		return s.fail(fmt.Errorf("upstream error event: %w", ErrConversion))
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		if s.session.to != Chat || !s.source.logicalEnd || s.source.done {
			return s.fail(fmt.Errorf("unexpected stream terminator: %w", ErrConversion))
		}
		s.source.done = true
		return nil, nil
	}
	if err := safeResponse(s.session.to, data, true); err != nil {
		return s.fail(err)
	}
	obj, err := readObject(data)
	if err != nil {
		return s.fail(err)
	}
	if event != "" && (s.session.to == Messages || s.session.to == Responses) {
		if typ := text(obj["type"]); typ != "" && typ != event {
			return s.fail(fmt.Errorf("SSE event type mismatch: %w", ErrConversion))
		}
		if text(obj["type"]) == "" {
			obj["type"], _ = json.Marshal(event)
			data, _ = json.Marshal(obj)
		}
	}
	if err := s.source.observe(s.session.to, obj); err != nil {
		return s.fail(err)
	}
	value, err := decode(s.session.to, "stream", data)
	if err != nil {
		return s.fail(err)
	}
	results, err := relayconvert.ConvertStreamResponseChunk(s.session.ctx, s.session.meta, s.state, value)
	if err != nil {
		return s.fail(fmt.Errorf("stream conversion: %w", ErrConversion))
	}
	if err := checkDiagnostics(s.state.Diagnostics()); err != nil {
		return s.fail(err)
	}
	out, err := s.encode(results)
	if err != nil {
		return s.fail(err)
	}
	// Libraries may synthesize success as soon as they see a finish reason.
	// Withhold that chunk and everything after it until source EOF is checked.
	if s.source.logicalEnd {
		s.pending = append(s.pending, out...)
		return nil, nil
	}
	return out, nil
}

func (s *Stream) Finish() ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.finished {
		return nil, nil
	}
	s.finished = true
	if s.session.from == s.session.to {
		return nil, nil
	}
	if err := s.session.ctx.Err(); err != nil {
		return s.fail(err)
	}
	if !s.source.done {
		return s.fail(fmt.Errorf("upstream ended without its terminal event: %w", ErrConversion))
	}
	if err := checkDiagnostics(s.state.Diagnostics()); err != nil {
		return s.fail(err)
	}
	results, err := relayconvert.FinalizeStreamResponse(s.session.ctx, s.session.meta, s.state)
	if err != nil {
		return s.fail(fmt.Errorf("stream finalization: %w", ErrConversion))
	}
	if err := checkDiagnostics(s.state.Diagnostics()); err != nil {
		return s.fail(err)
	}
	tail, err := s.encode(results)
	if err != nil {
		return s.fail(err)
	}
	out := append(s.pending, tail...)
	s.pending = nil
	if s.session.from == Chat {
		out = append(out, []byte("data: [DONE]\n\n")...)
	}
	return out, nil
}

func (s *Stream) encode(results []relayconvert.ResponseResult) ([]byte, error) {
	var out []byte
	for _, result := range results {
		if err := checkDiagnostics(result.Diagnostics); err != nil {
			return nil, err
		}
		if result.Value == nil {
			continue
		}
		raw, err := encodeResponse(result.Value)
		if err != nil {
			return nil, err
		}
		if s.session.from == Messages || s.session.from == Responses {
			obj, err := readObject(raw)
			if err != nil {
				return nil, err
			}
			typ := text(obj["type"])
			if typ == "" || strings.ContainsAny(typ, "\r\n") {
				return nil, ErrConversion
			}
			out = append(out, []byte("event: "+typ+"\n")...)
		}
		out = append(out, []byte("data: ")...)
		out = append(out, raw...)
		out = append(out, '\n', '\n')
	}
	s.outputBytes += len(out)
	if s.outputBytes > maxStreamBytes {
		return nil, fmt.Errorf("converted stream byte limit exceeded: %w", ErrConversion)
	}
	return out, nil
}

func parseFrame(frame []byte) (string, []byte, error) {
	var event string
	var data []byte
	boundary := false
	for _, line := range bytes.Split(bytes.ReplaceAll(frame, []byte("\r\n"), []byte("\n")), []byte("\n")) {
		if len(line) == 0 {
			if len(data) > 0 {
				boundary = true
			}
			continue
		}
		if line[0] == ':' {
			continue
		}
		if boundary {
			return "", nil, fmt.Errorf("multiple SSE frames in one event: %w", ErrConversion)
		}
		field, value, _ := bytes.Cut(line, []byte(":"))
		value = bytes.TrimPrefix(value, []byte(" "))
		switch string(field) {
		case "event":
			event = string(value)
		case "data":
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, value...)
		case "id", "retry":
		default:
			return "", nil, fmt.Errorf("invalid SSE field: %w", ErrConversion)
		}
	}
	return event, data, nil
}
