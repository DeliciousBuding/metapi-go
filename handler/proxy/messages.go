package proxyhandler

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/handler/shared"
)

// HandleClaudeMessages handles POST /v1/messages.
// Surface format: "claude".
func HandleClaudeMessages(w http.ResponseWriter, r *http.Request) {
	ctx, errResp := PrepareCtx(r, SurfConfig{
		Endpoint:       "messages",
		DownstreamPath: "/v1/messages",
		RequireModel:   true,
		SurfaceFormat:  "claude",
	})
	if errResp != nil {
		writeJSONError(w, errResp.Status, errResp.Error, errResp.ErrorType)
		return
	}

	dispatchUpstream(w, r, ctx)
}

// HandleClaudeCountTokens handles POST /v1/messages/count_tokens.
func HandleClaudeCountTokens(w http.ResponseWriter, r *http.Request) {
	ctx, errResp := PrepareCtx(r, SurfConfig{
		Endpoint:       "messages",
		DownstreamPath: "/v1/messages/count_tokens",
		RequireModel:   false,
	})
	if errResp != nil {
		writeJSONError(w, errResp.Status, errResp.Error, errResp.ErrorType)
		return
	}

	dispatchUpstream(w, r, ctx)
}

// Helper functions

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// writeJSON writes a JSON response via the shared encoder.
func writeJSON(w http.ResponseWriter, status int, body any) {
	shared.WriteJSON(w, status, body)
}
