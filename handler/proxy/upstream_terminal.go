package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
	transformshared "github.com/deliciousbuding/metapi-go/transform/shared"
)

func nativeTerminalProtocol(path string) transformshared.NativeTerminalProtocol {
	endpoint, ok := proxy.EndpointFromPath(path)
	if !ok {
		return 0
	}
	switch endpoint {
	case proxy.EndpointChat:
		return transformshared.NativeChatCompletions
	case proxy.EndpointMessages:
		return transformshared.NativeMessages
	default:
		return 0
	}
}

// This reader sits after decoding and the existing upstream idle guard. It holds
// at most one bounded SSE frame, preserves untouched framing/metadata verbatim,
// and never appends a terminal event at EOF or hides an underlying read error.
type nativeTerminalBody struct {
	io.ReadCloser
	normalizer    transformshared.NativeTerminalStream
	responseModel string
	original      *incrementalSseAnalyzer
	pending       []byte
	output        []byte
	end           error
	passthrough   bool
	// Completion is independent of tool-stop repair and public model mapping.
	// Only protocols with an explicit stream terminator participate.
	completionProtocol proxy.UpstreamEndpoint
	terminalSeen       bool
	terminalFailed     bool
	readBuf            [4096]byte
}

func withNativeTerminalBody(body io.ReadCloser, path string, responseModels ...string) io.ReadCloser {
	model := ""
	if len(responseModels) > 0 {
		model = responseModels[0]
	}
	protocol := nativeTerminalProtocol(path)
	completion, _ := proxy.EndpointFromPath(path)
	if completion != proxy.EndpointChat && completion != proxy.EndpointMessages && completion != proxy.EndpointResponses {
		completion = ""
	}
	if protocol == 0 && model == "" && completion == "" {
		return body
	}
	result := &nativeTerminalBody{ReadCloser: body, normalizer: transformshared.NativeTerminalStream{Protocol: protocol}, responseModel: model, completionProtocol: completion}
	if model != "" {
		result.original = newIncrementalSseAnalyzer()
	}
	return result
}

func (b *nativeTerminalBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(b.output) == 0 {
		if b.end != nil {
			return 0, b.end
		}
		n, err := b.ReadCloser.Read(b.readBuf[:])
		if b.original != nil && n > 0 {
			b.original.Push(b.readBuf[:n])
		}
		if b.passthrough {
			b.output = append(b.output, b.readBuf[:n]...)
		} else {
			b.pending = append(b.pending, b.readBuf[:n]...)
			for {
				boundary, sepLen := nextSseBoundary(string(b.pending))
				if boundary < 0 {
					break
				}
				if boundary > maxIncrementalSsePendingBytes {
					if b.responseModel != "" {
						b.end = fmt.Errorf("SSE frame exceeds model mapping buffer limit")
						b.pending = nil
						break
					}
					b.output = append(b.output, b.pending...)
					b.pending = nil
					b.passthrough = true
					break
				}
				block := b.pending[:boundary]
				b.output = append(b.output, b.normalizeBlock(block)...)
				b.output = append(b.output, b.pending[boundary:boundary+sepLen]...)
				b.pending = b.pending[boundary+sepLen:]
			}
			if len(b.pending) > maxIncrementalSsePendingBytes {
				if b.responseModel != "" {
					b.end = fmt.Errorf("SSE frame exceeds model mapping buffer limit")
					b.pending = nil
					continue
				}
				// Stop normalizing after an oversized frame: client-visible bytes and
				// the existing analyzer's oversized/error verdict remain authoritative.
				b.output = append(b.output, b.pending...)
				b.pending = nil
				b.passthrough = true
			}
		}
		if err != nil {
			if err == io.EOF {
				b.observeTerminalEvent(parseSseBlock(string(b.pending)))
			}
			if b.responseModel != "" && err == io.EOF {
				b.output = append(b.output, b.normalizeBlock(b.pending)...)
			} else {
				b.output = append(b.output, b.pending...)
			}
			b.pending = nil
			b.end = err
			if err == io.EOF && b.completionProtocol != "" && (!b.terminalSeen || b.terminalFailed) {
				b.end = fmt.Errorf("native %s stream ended without a successful terminal event", b.completionProtocol)
			}
		}
		if n == 0 && err == nil {
			return 0, nil
		}
	}
	n := copy(p, b.output)
	b.output = b.output[n:]
	return n, nil
}

func (b *nativeTerminalBody) normalizeBlock(block []byte) []byte {
	event := parseSseBlock(string(block))
	b.observeTerminalEvent(event)
	if event == nil || event.Data == "" {
		return block
	}
	raw := []byte(event.Data)
	normalized := b.normalizer.AddData(raw)
	normalized = restoreDownstreamResponseModel(normalized, b.responseModel)
	if bytes.Equal(normalized, raw) {
		return block
	}
	return replaceSSEBlockData(block, normalized)
}

func (b *nativeTerminalBody) observeTerminalEvent(event *SseEvent) {
	if b.completionProtocol == "" || event == nil {
		return
	}
	if b.completionProtocol == proxy.EndpointChat && strings.TrimSpace(event.Data) == "[DONE]" {
		b.terminalSeen = true
		return
	}
	// The existing SSE error analyzer owns provider errors. Treat the event as
	// terminal so EOF does not add an unrelated missing-terminator error.
	if IsSseErrorEvent(*event) {
		b.terminalSeen = true
		return
	}
	if b.completionProtocol == proxy.EndpointChat {
		return
	}
	var payload struct {
		Type     string `json:"type"`
		Response struct {
			Status string `json:"status"`
		} `json:"response"`
	}
	if json.Unmarshal([]byte(event.Data), &payload) != nil {
		return
	}
	kind := payload.Type
	if kind == "" {
		kind = event.Event
	}
	switch b.completionProtocol {
	case proxy.EndpointMessages:
		if kind == "message_stop" {
			b.terminalSeen = true
		}
	case proxy.EndpointResponses:
		if kind == "response.completed" {
			b.terminalSeen = true
			if payload.Response.Status != "" && payload.Response.Status != "completed" {
				b.terminalFailed = true
			}
		} else if kind == "response.incomplete" {
			b.terminalSeen = true
			b.terminalFailed = true
		}
	}
}

func replaceSSEBlockData(block, normalized []byte) []byte {
	// Only the data field changes. Comments, event/id/retry and line endings
	// retain their original order. A multiline JSON payload may become one line.
	lines := bytes.Split(block, []byte("\n"))
	out := make([][]byte, 0, len(lines))
	replaced := false
	for _, line := range lines {
		if !bytes.HasPrefix(line, []byte("data:")) {
			out = append(out, line)
			continue
		}
		if replaced {
			continue
		}
		replaced = true
		next := append([]byte("data: "), normalized...)
		if bytes.HasSuffix(line, []byte("\r")) {
			next = append(next, '\r')
		}
		out = append(out, next)
	}
	if !replaced {
		return block
	}
	return bytes.Join(out, []byte("\n"))
}

func normalizeNativeTerminalResponse(resp *http.Response, body []byte, path string) []byte {
	protocol := nativeTerminalProtocol(path)
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if protocol == 0 || err != nil || (mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json")) || strings.HasPrefix(strings.ToLower(resp.Header.Get("Content-Disposition")), "attachment") {
		return body
	}
	normalized := transformshared.NormalizeNativeTerminalJSON(body, protocol)
	if !bytes.Equal(normalized, body) {
		// Original validators and lengths describe different response bytes.
		resp.Header.Del("Content-Length")
		resp.Header.Del("ETag")
		resp.Header.Del("Content-MD5")
		resp.Header.Del("Digest")
	}
	return normalized
}
