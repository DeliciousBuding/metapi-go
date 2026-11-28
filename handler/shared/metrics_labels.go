package shared

import (
	"net/http"
	"strings"
)

// Bounded outcome status labels (never raw HTTP codes or error strings).
const (
	OutcomeSuccess       = "success"
	OutcomeError         = "error"
	OutcomeTimeout       = "timeout"
	OutcomeUnavailable   = "unavailable"
	OutcomeUpstreamError = "upstream_error"
	OutcomeClientError   = "client_error"
)

// Bounded endpoint labels derived from path suffixes (never full URLs/models).
const (
	EndpointChat        = "chat"
	EndpointMessages    = "messages"
	EndpointResponses   = "responses"
	EndpointEmbeddings  = "embeddings"
	EndpointCompletions = "completions"
	EndpointImages      = "images"
	EndpointRerank      = "rerank"
	EndpointSearch      = "search"
	EndpointGemini      = "gemini"
	EndpointModels      = "models"
	EndpointOther       = "other"
)

// EndpointLabelFromPath maps a request path to a low-cardinality endpoint label.
// Never returns raw paths; unknown paths become "other".
func EndpointLabelFromPath(path string) string {
	path = strings.TrimSpace(path)
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	path = strings.ToLower(strings.TrimRight(path, "/"))
	switch {
	case strings.HasSuffix(path, "/v1/chat/completions") || path == "/chat/completions" || path == "/v1/chat/completions":
		return EndpointChat
	case strings.HasSuffix(path, "/v1/messages") || path == "/messages" || path == "/v1/messages" ||
		strings.HasSuffix(path, "/anthropic/v1/messages") || strings.HasSuffix(path, "/messages/count_tokens"):
		return EndpointMessages
	case strings.Contains(path, "/v1/responses") || path == "/responses":
		return EndpointResponses
	case strings.HasSuffix(path, "/v1/embeddings") || path == "/embeddings":
		return EndpointEmbeddings
	case strings.HasSuffix(path, "/v1/completions") || path == "/completions":
		return EndpointCompletions
	case strings.Contains(path, "/v1/images"):
		return EndpointImages
	case strings.HasSuffix(path, "/v1/rerank") || path == "/rerank":
		return EndpointRerank
	case strings.HasSuffix(path, "/v1/search") || path == "/search":
		return EndpointSearch
	case strings.Contains(path, "generatecontent") || strings.Contains(path, "counttokens") ||
		strings.Contains(path, "/v1beta/") || strings.Contains(path, "/v1internal"):
		return EndpointGemini
	case strings.HasSuffix(path, "/v1/models") || path == "/models" || strings.HasSuffix(path, "/models"):
		return EndpointModels
	default:
		return EndpointOther
	}
}

// StatusFromHTTP maps an HTTP status to a bounded outcome label.
func StatusFromHTTP(code int) string {
	switch {
	case code >= 200 && code < 300:
		return OutcomeSuccess
	case code == http.StatusRequestTimeout || code == 408:
		return OutcomeTimeout
	case code == http.StatusServiceUnavailable || code == http.StatusTooManyRequests:
		return OutcomeUnavailable
	case code >= 400 && code < 500:
		return OutcomeClientError
	case code >= 500:
		return OutcomeUpstreamError
	default:
		return OutcomeError
	}
}

func sanitizeEndpoint(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case EndpointChat, EndpointMessages, EndpointResponses, EndpointEmbeddings,
		EndpointCompletions, EndpointImages, EndpointRerank, EndpointSearch,
		EndpointGemini, EndpointModels, EndpointOther:
		return strings.TrimSpace(strings.ToLower(v))
	case "":
		return EndpointOther
	default:
		return EndpointOther
	}
}

func sanitizeStatus(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case OutcomeSuccess, OutcomeError, OutcomeTimeout, OutcomeUnavailable,
		OutcomeUpstreamError, OutcomeClientError:
		return strings.TrimSpace(strings.ToLower(v))
	case "":
		return OutcomeError
	default:
		return OutcomeError
	}
}
