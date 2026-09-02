package auth

import (
	"encoding/json"
	"net/http"
)

// proxyAuthErrorClass maps downstream auth Reason values onto the OpenAI SDK
// error taxonomy (type) plus a stable snake_case machine code. The /v1 error
// envelope is {"error":{"message","type","code"?}} — the same shape the
// handler layer already emits (handler/proxy/router.go
// writeJSONErrorWithRequest), so one /v1 surface has exactly one error format
// whether the failure happened in middleware or in the handler.
//
// Status codes live on DownstreamTokenAuthResult (auth/downstream.go):
//   - missing/invalid            → 401 (OpenAI convention: an unknown key is
//     an authentication failure, not a permission one — SDKs treat 403 as
//     non-retryable permission errors and skip the key-rotation path)
//   - disabled/expired/IP policy → 403
//   - over_cost/over_requests    → 429 (OpenAI reports exhausted quota as
//     insufficient_quota on 429)
func proxyAuthErrorClass(reason string) (errType, code string) {
	switch reason {
	case "missing":
		return "authentication_error", "missing_api_key"
	case "invalid":
		return "authentication_error", "invalid_api_key"
	case "disabled":
		return "permission_error", "key_disabled"
	case "expired":
		return "permission_error", "key_expired"
	case "over_cost", "over_requests":
		return "insufficient_quota", "insufficient_quota"
	case "ip_blocked":
		return "permission_error", "ip_blocked"
	case "ip_not_allowed":
		return "permission_error", "ip_not_allowed"
	case "over_rpm":
		return "rate_limit_error", "over_rpm"
	case "over_tpm":
		return "rate_limit_error", "over_tpm"
	default:
		return "authentication_error", reason
	}
}

// writeProxyError writes the OpenAI-shaped /v1 error envelope. The ingress
// request id (when the middleware already set X-Request-Id) is mirrored into
// the body exactly like the handler layer does.
func writeProxyError(w http.ResponseWriter, status int, errType, code, message string) {
	body := map[string]any{
		"message": message,
		"type":    errType,
	}
	if code != "" {
		body["code"] = code
	}
	if rid := w.Header().Get("X-Request-Id"); rid != "" {
		body["request_id"] = rid
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": body})
}
