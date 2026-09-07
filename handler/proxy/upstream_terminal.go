package proxyhandler

import (
	"bytes"
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
	normalizer  transformshared.NativeTerminalStream
	pending     []byte
	output      []byte
	end         error
	passthrough bool
	readBuf     [4096]byte
}

func withNativeTerminalBody(body io.ReadCloser, path string) io.ReadCloser {
	protocol := nativeTerminalProtocol(path)
	if protocol == 0 {
		return body
	}
	return &nativeTerminalBody{ReadCloser: body, normalizer: transformshared.NativeTerminalStream{Protocol: protocol}}
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
				// Stop normalizing after an oversized frame: client-visible bytes and
				// the existing analyzer's oversized/error verdict remain authoritative.
				b.output = append(b.output, b.pending...)
				b.pending = nil
				b.passthrough = true
			}
		}
		if err != nil {
			b.output = append(b.output, b.pending...)
			b.pending = nil
			b.end = err
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
	if event == nil || event.Data == "" {
		return block
	}
	raw := []byte(event.Data)
	normalized := b.normalizer.AddData(raw)
	if bytes.Equal(normalized, raw) {
		return block
	}
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
