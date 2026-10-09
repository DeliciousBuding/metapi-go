package httpclient

import (
	"net/http"
	"strings"
)

// ExpandClientHeaderTemplate expands only explicitly allowed request metadata.
// Backup validation and live forwarding use the same parser and allowlist.
// Invalid templates return no partial value.
func ExpandClientHeaderTemplate(value string, client http.Header) (string, bool) {
	const marker = "{client_header:"
	var out strings.Builder
	for {
		relative := strings.Index(strings.ToLower(value), marker)
		if relative < 0 {
			out.WriteString(value)
			return out.String(), true
		}
		out.WriteString(value[:relative])
		nameStart := relative + len(marker)
		endRel := strings.IndexByte(value[nameStart:], '}')
		if endRel < 0 {
			return "", false
		}
		name := strings.TrimSpace(value[nameStart : nameStart+endRel])
		if !safeClientHeaderName(name) {
			return "", false
		}
		out.WriteString(client.Get(name))
		value = value[nameStart+endRel+1:]
	}
}

func safeClientHeaderName(name string) bool {
	switch strings.ToLower(name) {
	case "idempotency-key", "openai-beta", "x-request-id", "x-correlation-id", "traceparent", "tracestate":
		return true
	default:
		return false
	}
}
