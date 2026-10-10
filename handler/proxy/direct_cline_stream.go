package proxyhandler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
)

var errDirectClineIdleTimeout = errors.New("Cline stream idle timeout")

// Normalize before shared Chat accounting and any downstream protocol bridge.
// No body read occurs until the caller reads resp.Body. Buffered responses are
// handled by normalizeDirectClineJSON after the shared bounded decode step.
func normalizeDirectClineResponse(ctx context.Context, resp *http.Response, stream bool) error {
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("Cline response has no body")
	}
	if !stream || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "text/event-stream" {
		return fmt.Errorf("Cline stream requires text/event-stream")
	}
	prepared := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
	if !prepared.Readable {
		return fmt.Errorf("unsupported Cline stream encoding")
	}
	source := resp.Body
	if prepared.Reader != nil {
		// Cancellation closes only the transport. The lazy codec belongs to the
		// read goroutine, avoiding races while it initializes or decompresses.
		source = struct {
			io.Reader
			io.Closer
		}{prepared.Reader, resp.Body}
	}
	resp.Body = newDirectClineBody(ctx, source, streamResponseByteLimit(), streamIdleTimeout())
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("ETag")
	resp.Header.Set("Content-Type", "text/event-stream")
	return nil
}

type directClineBody struct {
	ctx        context.Context
	bridge     *messagesChatBody
	idle       *streamIdleBody
	stopCancel func() bool
}

func newDirectClineBody(ctx context.Context, source io.ReadCloser, limit int64, idleTimeout time.Duration) *directClineBody {
	idle := &streamIdleBody{ReadCloser: source}
	idle.guard = newStreamIdleGuard(idleTimeout, idle.closeUnderlying)
	bridge := newProtocolBridgeBody(idle, &directClineStream{choices: make(map[int]bool)}, limit)
	// Wrapped usage is not OpenAI usage yet. The caller's shared analyzer sees
	// only normalized Chat chunks and owns accounting.
	bridge.skipOriginalAnalysis = true
	body := &directClineBody{ctx: ctx, bridge: bridge, idle: idle}
	body.stopCancel = context.AfterFunc(ctx, idle.closeUnderlying)
	return body
}

func (b *directClineBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := b.ctx.Err(); err != nil {
		_ = b.Close()
		return 0, err
	}
	n, err := b.bridge.Read(p)
	if contextErr := b.ctx.Err(); contextErr != nil {
		_ = b.Close()
		return 0, contextErr
	}
	if b.idle.guard.fired.Load() {
		_ = b.Close()
		return 0, errDirectClineIdleTimeout
	}
	if err != nil {
		_ = b.Close()
	}
	return n, err
}

func (b *directClineBody) Close() error {
	b.stopCancel()
	return b.idle.Close()
}

type directClineChoice struct {
	index    int
	finished bool
}

type directClineStream struct {
	choices map[int]bool
	done    bool
}

func (s *directClineStream) TransformEvent(frame []byte) ([]byte, error) {
	event := parseSseBlock(string(frame))
	if event == nil {
		for _, line := range strings.Split(string(frame), "\n") {
			line = strings.TrimSuffix(line, "\r")
			if line != "" && !strings.HasPrefix(line, ":") && !strings.HasPrefix(line, "id:") && !strings.HasPrefix(line, "retry:") {
				return nil, fmt.Errorf("Cline stream has invalid SSE framing")
			}
		}
		// Preserve heartbeat comments so the outer idle guard sees activity.
		return frame, nil
	}
	if event.Event == "error" {
		if obj, err := directClineObject([]byte(event.Data)); err == nil {
			if err := directClinePayloadError(obj); err != nil {
				return nil, err
			}
		}
		return nil, fmt.Errorf("Cline stream error")
	}
	data := strings.TrimSpace(event.Data)
	if s.done {
		return nil, fmt.Errorf("Cline stream event received after [DONE]")
	}
	if data == "[DONE]" {
		if len(s.choices) == 0 {
			return nil, fmt.Errorf("Cline stream ended without Chat choices")
		}
		for _, finished := range s.choices {
			if !finished {
				return nil, fmt.Errorf("Cline stream ended before finish_reason")
			}
		}
		s.done = true
		// Hold the terminal marker until clean EOF, so a late error or body
		// limit cannot be hidden behind an already-emitted success marker.
		return nil, nil
	}
	converted, choices, err := directClineChatJSON([]byte(data), true)
	if err != nil {
		return nil, err
	}
	for _, choice := range choices {
		if s.choices[choice.index] {
			return nil, fmt.Errorf("Cline stream choice continued after finish_reason")
		}
		s.choices[choice.index] = choice.finished
	}
	return append(append([]byte("data: "), converted...), '\n', '\n'), nil
}

func (s *directClineStream) Finish() ([]byte, error) {
	if !s.done {
		return nil, fmt.Errorf("Cline stream ended without [DONE]: %w", io.ErrUnexpectedEOF)
	}
	return []byte("data: [DONE]\n\n"), nil
}
