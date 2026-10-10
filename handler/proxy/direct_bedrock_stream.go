package proxyhandler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"sync"
	"unicode/utf8"
)

// AWS EventStream limits apply before allocating or decoding an untrusted
// frame. The shared SSE reader separately enforces the configured stream limit.
const (
	directBedrockMaxHeaders = 128 << 10
	directBedrockMaxPayload = 24 << 20
)

type directBedrockStream struct {
	ctx              context.Context
	source           io.ReadCloser
	pending          []byte
	err              error
	started, stopped bool
	closeOnce        sync.Once
	closeErr         error
	stopCancel       func() bool
}

func newDirectBedrockStream(ctx context.Context, source io.ReadCloser) *directBedrockStream {
	s := &directBedrockStream{ctx: ctx, source: source}
	s.stopCancel = context.AfterFunc(ctx, func() { _ = s.closeSource() })
	return s
}

func (s *directBedrockStream) closeSource() error {
	s.closeOnce.Do(func() { s.closeErr = s.source.Close() })
	return s.closeErr
}

func (s *directBedrockStream) Close() error {
	s.stopCancel()
	return s.closeSource()
}

// Read emits exactly one complete Anthropic SSE event at a time. It never
// fabricates message_stop on EOF, exception, CRC failure, or cancellation.
func (s *directBedrockStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	if len(s.pending) == 0 && s.err == nil {
		s.pending, s.err = s.next()
		if s.err != nil {
			_ = s.Close()
		}
	}
	if len(s.pending) != 0 {
		n := copy(p, s.pending)
		s.pending = s.pending[n:]
		return n, nil
	}
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	return 0, s.err
}

func (s *directBedrockStream) next() ([]byte, error) {
	headers, payload, err := readDirectBedrockFrame(s.source)
	if err != nil {
		if errors.Is(err, io.EOF) && !s.stopped {
			return nil, fmt.Errorf("Bedrock event stream ended without message_stop: %w", io.ErrUnexpectedEOF)
		}
		return nil, err
	}
	switch headers[":message-type"] {
	case "exception":
		kind := headers[":exception-type"]
		if kind == "" {
			return nil, fmt.Errorf("Bedrock exception is missing :exception-type")
		}
		var detail struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(payload, &detail); err != nil {
			return nil, fmt.Errorf("Bedrock %s has invalid exception JSON: %w", kind, err)
		}
		return nil, fmt.Errorf("Bedrock %s: %s", kind, detail.Message)
	case "error":
		return nil, fmt.Errorf("Bedrock %s: %s", headers[":error-code"], headers[":error-message"])
	case "event":
		if headers[":event-type"] != "chunk" {
			return nil, fmt.Errorf("Bedrock unsupported event type %q", headers[":event-type"])
		}
	default:
		return nil, fmt.Errorf("Bedrock missing or invalid :message-type")
	}
	if ct := headers[":content-type"]; ct != "" && ct != "application/json" {
		return nil, fmt.Errorf("Bedrock chunk has unsupported content type %q", ct)
	}
	var chunk struct {
		Bytes string `json:"bytes"`
	}
	if err := json.Unmarshal(payload, &chunk); err != nil || chunk.Bytes == "" {
		return nil, fmt.Errorf("Bedrock chunk requires base64 bytes")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(chunk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("Bedrock chunk has invalid base64: %w", err)
	}
	var event struct {
		Type string `json:"type"`
	}
	if !utf8.Valid(data) || json.Unmarshal(data, &event) != nil || !directBedrockEventName(event.Type) {
		return nil, fmt.Errorf("Bedrock chunk requires a JSON event with a valid type")
	}
	if s.stopped {
		return nil, fmt.Errorf("Bedrock event received after message_stop")
	}
	switch event.Type {
	case "message_start":
		if s.started {
			return nil, fmt.Errorf("Bedrock duplicated message_start")
		}
		s.started = true
	case "ping", "error":
	default:
		if !s.started {
			return nil, fmt.Errorf("Bedrock event received before message_start")
		}
	}
	if event.Type == "message_stop" {
		s.stopped = true
	}
	// Compact JSON so pretty-printed provider events remain a single SSE data
	// line. Content, signatures, tools, unknown fields, and number lexemes survive.
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return nil, err
	}
	out := []byte("event: " + event.Type + "\ndata: ")
	out = append(out, compact.Bytes()...)
	return append(out, '\n', '\n'), nil
}

func directBedrockEventName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

func readDirectBedrockFrame(r io.Reader) (map[string]string, []byte, error) {
	var prelude [12]byte
	if _, err := io.ReadFull(r, prelude[:]); err != nil {
		return nil, nil, err
	}
	if crc32.ChecksumIEEE(prelude[:8]) != binary.BigEndian.Uint32(prelude[8:]) {
		return nil, nil, fmt.Errorf("Bedrock eventstream prelude CRC mismatch")
	}
	total := uint64(binary.BigEndian.Uint32(prelude[:4]))
	headerLen := uint64(binary.BigEndian.Uint32(prelude[4:8]))
	if total < 16 || headerLen > total-16 || headerLen > directBedrockMaxHeaders || total-16-headerLen > directBedrockMaxPayload {
		return nil, nil, fmt.Errorf("Bedrock eventstream frame exceeds limits or has invalid lengths")
	}
	rest := make([]byte, int(total)-12)
	if _, err := io.ReadFull(r, rest); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, nil, err
	}
	checksum := crc32.Update(crc32.ChecksumIEEE(prelude[:]), crc32.IEEETable, rest[:len(rest)-4])
	if checksum != binary.BigEndian.Uint32(rest[len(rest)-4:]) {
		return nil, nil, fmt.Errorf("Bedrock eventstream message CRC mismatch")
	}
	headers, err := readDirectBedrockHeaders(rest[:headerLen])
	if err != nil {
		return nil, nil, err
	}
	return headers, rest[headerLen : len(rest)-4], nil
}

// AWS headers have ten typed encodings. Only string-valued routing headers
// are consumed, but all types must be parsed to find the next header safely.
func readDirectBedrockHeaders(raw []byte) (map[string]string, error) {
	headers := make(map[string]string)
	seen := make(map[string]bool)
	for len(raw) > 0 {
		nameLen := int(raw[0])
		raw = raw[1:]
		if nameLen == 0 || len(raw) < nameLen+1 || !utf8.Valid(raw[:nameLen]) {
			return nil, fmt.Errorf("Bedrock invalid eventstream header name")
		}
		name, typ := string(raw[:nameLen]), raw[nameLen]
		raw = raw[nameLen+1:]
		if seen[name] {
			return nil, fmt.Errorf("Bedrock duplicate eventstream header %q", name)
		}
		seen[name] = true
		length := 0
		switch typ {
		case 0, 1: // true, false
		case 2:
			length = 1
		case 3:
			length = 2
		case 4:
			length = 4
		case 5, 8: // int64, timestamp
			length = 8
		case 6, 7: // byte array, UTF-8 string
			if len(raw) < 2 {
				return nil, fmt.Errorf("Bedrock truncated eventstream header length")
			}
			length = int(binary.BigEndian.Uint16(raw[:2]))
			raw = raw[2:]
		case 9:
			length = 16
		default:
			return nil, fmt.Errorf("Bedrock invalid eventstream header type %d", typ)
		}
		if len(raw) < length {
			return nil, fmt.Errorf("Bedrock truncated eventstream header value")
		}
		if typ == 7 {
			if !utf8.Valid(raw[:length]) {
				return nil, fmt.Errorf("Bedrock invalid UTF-8 eventstream header")
			}
			headers[name] = string(raw[:length])
		} else if name[0] == ':' {
			return nil, fmt.Errorf("Bedrock routing header %q must be a string", name)
		}
		raw = raw[length:]
	}
	return headers, nil
}
