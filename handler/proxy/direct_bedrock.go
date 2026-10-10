package proxyhandler

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func prepareDirectBedrockRequest(body []byte, downstream http.Header) ([]byte, error) {
	return messages.PrepareBedrockRequest(body, downstream.Values("Anthropic-Beta"))
}

func buildDirectBedrockHeaders(headers http.Header) {
	headers.Set("Anthropic-Version", "bedrock-2023-05-31")
	headers.Set("Accept", "application/json")
	// Flags have already moved into anthropic_beta in the request body.
	headers["Anthropic-Beta"] = nil
}

// prefix is the configured URL through /model. A model ID is one path
// segment, including when the ID is an ARN containing slashes.
func directBedrockRequestURL(prefix, model string, stream bool) string {
	action := "/invoke"
	if stream {
		action = "/invoke-with-response-stream"
	}
	target, err := url.Parse(prefix)
	if err != nil {
		return ""
	}
	escaped := strings.TrimRight(target.EscapedPath(), "/") + "/" + url.PathEscape(model) + action
	target.Path, err = url.PathUnescape(escaped)
	if err != nil {
		return ""
	}
	target.RawPath = escaped
	return target.String()
}

// normalizeDirectBedrockResponse runs on a successful HTTP response before
// shared stream parsing/accounting. It reads nothing eagerly: the same body
// read deadlines and cancellation still govern each upstream event.
// Non-streaming invoke responses are already native Anthropic JSON.
func normalizeDirectBedrockResponse(ctx context.Context, resp *http.Response, stream bool) error {
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("Bedrock response has no body")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !stream {
		return nil
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || contentType != "application/vnd.amazon.eventstream" {
		return fmt.Errorf("Bedrock stream requires application/vnd.amazon.eventstream")
	}
	prepared := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
	if !prepared.Readable {
		return fmt.Errorf("Bedrock stream has unsupported content encoding")
	}
	source := resp.Body
	if prepared.Reader != nil {
		// Codec state is owned by the read goroutine. Cancellation closes the
		// original transport, without racing a lazy decoder's initialization.
		// The supported gzip/deflate readers own no external resources.
		source = struct {
			io.Reader
			io.Closer
		}{prepared.Reader, resp.Body}
	}
	resp.Body = newDirectBedrockStream(ctx, source)
	resp.ContentLength = -1
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.Header.Del("Content-Length")
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("ETag")
	return nil
}
