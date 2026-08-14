// Package proxyhandler implements proxy route handlers.
// Thin HTTP handlers that delegate to endpoint flow + transform.
package proxyhandler

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/go-chi/chi/v5"
)

// RegisterProxyRoutes registers all /v1/* proxy surfaces on the given chi router.
// The router is expected to be mounted under /v1 with proxy auth middleware applied.
func RegisterProxyRoutes(r chi.Router) {
	// OpenAI Chat surface
	r.Post("/chat/completions", HandleChatCompletions)

	// Claude Messages surface
	r.Post("/messages", HandleClaudeMessages)
	r.Post("/messages/count_tokens", HandleClaudeCountTokens)

	// Completions surface
	r.Post("/completions", HandleCompletions)

	// Responses surface (HTTP)
	r.Post("/responses", func(w http.ResponseWriter, r *http.Request) {
		HandleResponses(w, r, "/v1/responses")
	})
	r.Get("/responses", HandleResponsesGet426)
	r.Post("/responses/compact", func(w http.ResponseWriter, r *http.Request) {
		HandleResponses(w, r, "/v1/responses/compact")
	})

	// Models surface
	r.Get("/models", HandleModels)

	// Embeddings surface
	r.Post("/embeddings", HandleEmbeddings)

	// Rerank surface (OpenAI-compatible / Cohere-style)
	r.Post("/rerank", HandleRerank)

	// Images surface
	r.Post("/images/generations", HandleImagesGenerations)
	r.Post("/images/edits", HandleImagesEdits)
	r.Post("/images/variations", HandleImagesVariations)

	// Videos surface
	r.Post("/videos", HandleVideosCreate)
	r.Get("/videos/{id}", HandleVideosGet)
	r.Delete("/videos/{id}", HandleVideosDelete)

	// Search surface
	r.Post("/search", HandleSearch)

	// Files surface
	RegisterFilesRoutes(r)
}

// RegisterNonV1ProxyRoutes registers proxy routes that are NOT under /v1.
// This includes: /chat/completions, /responses aliases, and Gemini routes.
// The router must have proxy auth middleware applied.
func RegisterNonV1ProxyRoutes(r chi.Router) {
	// /chat/completions alias
	r.Post("/chat/completions", HandleChatCompletions)

	// /responses aliases (Codex native paths)
	r.Post("/responses", func(w http.ResponseWriter, r *http.Request) {
		HandleResponses(w, r, "/v1/responses")
	})
	r.Post("/responses/*", HandleResponsesAliasPost)
	r.Get("/responses", HandleResponsesGet426)
	r.Get("/responses/*", HandleResponsesAliasGet426)

	// Gemini surface — all 7 routes are top-level (not under /v1)
	RegisterGeminiRoutes(r)
}

// writeJSONError writes a JSON error response without a request id.
// Prefer writeJSONErrorWithRequest when the ingress request/trace id is known.
func writeJSONError(w http.ResponseWriter, status int, message, typ string) {
	writeJSONErrorWithRequest(w, status, message, typ, "")
}

// writeJSONErrorWithRequest writes an OpenAI-shaped JSON error and, when
// requestID is non-empty, attaches it both in the body and as X-Request-Id
// when the response header is still unset.
func writeJSONErrorWithRequest(w http.ResponseWriter, status int, message, typ, requestID string) {
	if requestID != "" && w.Header().Get("X-Request-Id") == "" {
		w.Header().Set("X-Request-Id", requestID)
	}
	errBody := map[string]any{
		"message": message,
		"type":    typ,
	}
	if requestID != "" {
		errBody["request_id"] = requestID
	}
	shared.WriteJSON(w, status, map[string]any{"error": errBody})
}

// GetProxyAuth extracts the proxy auth context from the request.
func GetProxyAuth(r *http.Request) *auth.ProxyAuthContext {
	return auth.GetProxyAuth(r.Context())
}
