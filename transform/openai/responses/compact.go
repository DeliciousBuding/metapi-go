// Package responses provides OpenAI Responses API transformer and compact mode.
package responses

import (
	"strings"
)

// ShouldStripCompactResponsesStore returns true for codex/sub2api platforms.
func ShouldStripCompactResponsesStore(sitePlatform string) bool {
	n := strings.ToLower(strings.TrimSpace(sitePlatform))
	return n == "codex" || n == "sub2api"
}

// ShouldForceResponsesUpstreamStream returns true when stream must be forced for upstream.
func ShouldForceResponsesUpstreamStream(sitePlatform string, isCompactRequest bool) bool {
	if isCompactRequest {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(sitePlatform))
	return n == "codex" || n == "sub2api"
}

// SanitizeCompactResponsesRequestBody removes stream/stream_options and
// conditionally store. Compact never continues a prior stored response, so
// previous_response_id is always stripped here (see previous_response_id.go).

// Input items (including multi-turn reasoning with encrypted_content / summary /
// content) are preserved verbatim — compact must not drop required reasoning
// content needed for Hermes/Codex second-turn replay.
func SanitizeCompactResponsesRequestBody(body map[string]any, sitePlatform string) map[string]any {
	next := map[string]any{}
	for k, v := range body {
		next[k] = v
	}
	delete(next, "stream")
	delete(next, "stream_options")
	delete(next, PreviousResponseIDField)
	if ShouldStripCompactResponsesStore(sitePlatform) {
		delete(next, "store")
	}
	// Explicit: never touch "input" / reasoning item fields here.
	return next
}

// EnsureCompactResponsesJSONAcceptHeader forces accept: application/json for codex/sub2api.
func EnsureCompactResponsesJSONAcceptHeader(headers map[string]string, sitePlatform string) map[string]string {
	if !ShouldStripCompactResponsesStore(sitePlatform) {
		return headers
	}
	next := map[string]string{}
	for k, v := range headers {
		next[k] = v
	}
	delete(next, "Accept")
	delete(next, "accept")
	next["accept"] = "application/json"
	return next
}

// Inbound parses a responses request body.
func Inbound(body any) (map[string]any, error) {
	m, ok := body.(map[string]any)
	if !ok {
		return map[string]any{}, nil
	}
	return m, nil
}
