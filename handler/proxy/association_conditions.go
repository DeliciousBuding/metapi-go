package proxyhandler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/routecondition"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

type associationRequestContextKey struct{}

func associationRequestFromContext(ctx context.Context) func() *routecondition.Request {
	resolve, _ := ctx.Value(associationRequestContextKey{}).(func() *routecondition.Request)
	return resolve
}

func lazyAssociationRequest(path string, raw []byte, headers http.Header, stream bool, now time.Time) func() *routecondition.Request {
	// RawBody is the immutable downstream input, not an attempt's rewritten
	// payload. Reuse it; schema/token parsing is only needed by conditional rows.
	headers = headers.Clone()
	return sync.OnceValue(func() *routecondition.Request {
		return associationRequest(path, raw, headers, stream, now)
	})
}

func associationRequest(path string, raw []byte, headers http.Header, stream bool, now time.Time) *routecondition.Request {
	endpoint, _ := proxy.EndpointFromPath(path)
	var format string
	switch endpoint {
	case proxy.EndpointChat:
		format = "openai/chat_completions"
	case proxy.EndpointResponses:
		format = "openai/responses"
	case proxy.EndpointMessages:
		format = "anthropic/messages"
	case proxy.EndpointGemini:
		format = "gemini/contents"
	default:
		switch proxy.DirectProtocolForPath(path) {
		case store.DirectProtocolCompletions:
			format = "openai/completions"
		case store.DirectProtocolEmbeddings:
			format = "openai/embeddings"
		case store.DirectProtocolRerank:
			format = "jina/rerank"
		case store.DirectProtocolModerations:
			format = "openai/moderations"
		case store.DirectProtocolGeminiEmbeddings:
			format = "gemini/embeddings"
		case store.DirectProtocolJinaEmbeddings:
			format = "jina/embeddings"
		case store.DirectProtocolResponsesCompact:
			format = "openai/responses_compact"
		default:
			return nil
		}
	}
	input, err := routecondition.BuildRequest(raw, format, headers, stream, now)
	if err != nil {
		// The normal protocol handler owns invalid-body errors. A request
		// that cannot be described must never gain conditional candidates.
		return nil
	}
	return input
}
