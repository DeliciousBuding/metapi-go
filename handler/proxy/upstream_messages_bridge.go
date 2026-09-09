package proxyhandler

import (
	"fmt"
	"io"
	"net/http"

	"github.com/deliciousbuding/metapi-go/handler/shared"

	"github.com/deliciousbuding/metapi-go/proxy"
	messages "github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func isMessagesChatBridge(downstreamPath, upstreamPath string) bool {
	downstream, _ := proxy.EndpointFromPath(downstreamPath)
	upstream, _ := proxy.EndpointFromPath(upstreamPath)
	return downstream == proxy.EndpointMessages && upstream == proxy.EndpointChat
}

// messagesChatBody translates after decompression and the existing idle guard.
// It buffers one bounded SSE frame, never falls back to leaking Chat bytes,
// and retains upstream usage independently of the client-facing conversion.
// HTTP cancellation, byte limits, errors and retries remain owned by the relay.
type messagesChatBody struct {
	io.ReadCloser
	stream               *messages.ChatStream
	original             *incrementalSseAnalyzer
	pending, output      []byte
	end                  error
	readBuf              [4096]byte
	readBytes, byteLimit int64
}

var errMessagesChatStreamLimit = fmt.Errorf("upstream stream exceeded configured byte limit")

func newMessagesChatBody(body io.ReadCloser, model string, byteLimit int64, options ...messages.Options) *messagesChatBody {
	return &messagesChatBody{ReadCloser: body, stream: messages.NewChatStream(model, options...), original: newIncrementalSseAnalyzer(), byteLimit: byteLimit}
}

func (b *messagesChatBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(b.output) == 0 {
		if b.end != nil {
			return 0, b.end
		}
		n, readErr := b.ReadCloser.Read(b.readBuf[:])
		// Suppressed comments/reasoning still consume the upstream byte budget.
		// Counting only converted output would leave an unbounded input stream.
		if remaining := b.byteLimit - b.readBytes; int64(n) > remaining {
			n = int(max(remaining, 0))
			readErr = errMessagesChatStreamLimit
		}
		b.readBytes += int64(n)
		if n > 0 {
			b.original.Push(b.readBuf[:n])
			b.pending = append(b.pending, b.readBuf[:n]...)
			for {
				boundary, sepLen := nextSseBoundary(string(b.pending))
				if boundary < 0 {
					break
				}
				if boundary > maxIncrementalSsePendingBytes {
					b.end = fmt.Errorf("Chat SSE frame exceeds the Messages bridge buffer limit")
					break
				}
				converted, err := b.stream.TransformEvent(b.pending[:boundary+sepLen])
				if err != nil {
					b.end = err
					break
				}
				b.output = append(b.output, converted...)
				b.pending = b.pending[boundary+sepLen:]
			}
			if len(b.pending) > maxIncrementalSsePendingBytes && b.end == nil {
				b.end = fmt.Errorf("Chat SSE frame exceeds the Messages bridge buffer limit")
			}
		}
		if b.end != nil {
			b.pending = nil
			continue
		}
		if readErr != nil {
			b.end = readErr
			if readErr == io.EOF {
				if len(b.pending) > 0 {
					converted, err := b.stream.TransformEvent(b.pending)
					if err != nil {
						b.end = err
					} else {
						b.output = append(b.output, converted...)
					}
					// Account for a final SSE event terminated by EOF rather than a blank line.
					b.original.Push([]byte("\n\n"))
				}
				if b.end == io.EOF {
					converted, err := b.stream.Finish()
					if err != nil {
						b.end = err
					} else {
						b.output = append(b.output, converted...)
					}
				}
			}
			b.pending = nil
		}
		if n == 0 && readErr == nil {
			return 0, nil
		}
	}
	n := copy(p, b.output)
	b.output = b.output[n:]
	return n, nil
}

func writeMessagesReplayFailure(w http.ResponseWriter, ctx *Ctx, requestID string) {
	writeJSONErrorWithRequest(w, http.StatusBadRequest, "Cannot safely resume this Messages tool conversation; its bridge state or authorized upstream is unavailable. Start a new conversation", "invalid_request_error", requestID)
	observeProxyTerminal(ctx, shared.OutcomeClientError, ctx.IsStream, 0)
}
