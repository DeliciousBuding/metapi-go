package proxyhandler

import (
	"fmt"
	"io"
	"strings"
)

// Install as the first filter of the existing bounded SSE bridge, after its
// codec/idle guard. That reader owns raw-byte limits, cancellation and actual
// upstream usage, even when a malformed provider event fails normalization.
func newDirectRouterChatStream() protocolEventStream {
	return &directRouterChatStream{choices: make(map[int]bool)}
}

type directRouterChatStream struct {
	choices map[int]bool
	done    bool
}

func (s *directRouterChatStream) TransformEvent(frame []byte) ([]byte, error) {
	event := parseSseBlock(string(frame))
	if event == nil {
		for _, line := range strings.Split(string(frame), "\n") {
			line = strings.TrimSuffix(line, "\r")
			if line != "" && !strings.HasPrefix(line, ":") && !strings.HasPrefix(line, "id:") && !strings.HasPrefix(line, "retry:") {
				return nil, fmt.Errorf("router Chat stream has invalid SSE framing")
			}
		}
		return frame, nil
	}
	if event.Event == "error" {
		return nil, fmt.Errorf("router Chat stream reported an error")
	}
	if s.done {
		return nil, fmt.Errorf("router Chat stream event received after [DONE]")
	}
	data := strings.TrimSpace(event.Data)
	if data == "[DONE]" {
		if len(s.choices) == 0 {
			return nil, fmt.Errorf("router Chat stream ended without choices")
		}
		for _, finished := range s.choices {
			if !finished {
				return nil, fmt.Errorf("router Chat stream ended without finish_reason")
			}
		}
		s.done = true
		// The source transformer appends DONE on any EOF. Require its actual
		// marker and clean transport EOF so truncation/errors cannot succeed.
		return nil, nil
	}
	out, choices, err := routerChatJSON([]byte(data), true, normalizeRouterChatMessage)
	if err != nil {
		return nil, err
	}
	for _, choice := range choices {
		if s.choices[choice.index] {
			return nil, fmt.Errorf("router Chat choice continued after finish_reason")
		}
		s.choices[choice.index] = choice.finished
	}
	return append(append([]byte("data: "), out...), '\n', '\n'), nil
}

func (s *directRouterChatStream) Finish() ([]byte, error) {
	if !s.done {
		return nil, fmt.Errorf("router Chat stream ended without [DONE]: %w", io.ErrUnexpectedEOF)
	}
	return []byte("data: [DONE]\n\n"), nil
}
