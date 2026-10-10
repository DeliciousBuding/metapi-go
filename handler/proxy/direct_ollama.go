package proxyhandler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/transform/ollama"
)

// Normalize native NDJSON before the existing Chat response/usage bridge. The
// codec and native reader are lazy; both idle guards use the configured budget.
func normalizeDirectOllamaStream(ctx context.Context, resp *http.Response, model string, stream bool) error {
	if !stream || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	decoded := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
	if !decoded.Readable {
		return fmt.Errorf("unsupported Ollama stream encoding")
	}
	if decoded.Reader != nil {
		resp.Body = decoded.Reader
	}
	resp.Body = ollama.NewNDJSONReader(ctx, resp.Body, model, ollama.StreamOptions{MaxBytes: streamResponseByteLimit(), IdleTimeout: streamIdleTimeout()})
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.Header.Del("ETag")
	resp.Header.Set("Content-Type", "text/event-stream")
	resp.ContentLength = -1
	return nil
}

func normalizeDirectOllamaJSON(body []byte, readable bool) ([]byte, error) {
	if !readable {
		return nil, fmt.Errorf("unsupported Ollama response encoding")
	}
	return ollama.ToChatResponse(body)
}
