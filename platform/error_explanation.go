package platform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// upstreamStatusRE extracts the first HTTP status an upstream error message
// carries. It is deliberately stricter than containsHTTPStatus, which accepts any
// word-boundary hit because it only ever asks "does this message mention 401?".
// An explanation prints the number it found, so it must not print an address or a
// duration instead: the digits have to stand alone (start/space/paren before,
// end/space/comma/colon/paren after). That rejects "127.0.0.1:1" (an IP octet is
// followed by a dot), "500ms" and "quota 500000" (a digit run continues), while
// still matching "HTTP 401:", "429 Too Many Requests" and "returned 503".
var upstreamStatusRE = regexp.MustCompile(`(?:^|[\s(])(?:http\s*)?([1-5][0-9]{2})(?:$|[\s:,)])`)

// upstreamHTTPStatus returns the HTTP status embedded in an upstream error
// message, or 0 when it carries none (a transport failure never got a response).
func upstreamHTTPStatus(message string) int {
	match := upstreamStatusRE.FindStringSubmatch(strings.ToLower(message))
	if match == nil {
		return 0
	}
	status, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	return status
}

// ExplainUpstreamFailure renders one compact, credential-safe sentence naming why
// an upstream call failed, for operator-facing surfaces such as a 502 body or an
// account health reason. It never echoes the raw upstream text: that text can
// carry a URL, a token fragment or a request id, and it already reaches the
// server log in full.
//
// The class comes from ClassifyUpstreamError, but ClassUnknown is not read as
// "nothing to say". That classifier is deliberately conservative because only
// ClassExpired may write accounts.status='expired'; an operator still needs to
// hear that a 401 from the upstream auth endpoint is a credential problem even
// when the wording was too weak to auto-mark the account. So an unclassified
// failure falls back to what its status alone proves (#1210: a bare
// "balance refresh failed" hid `HTTP 401: Token has expired`, which the server
// had already logged).
//
// It returns "" only when nothing safe can be named, and the caller then keeps
// its own stable message prefix instead of inventing a cause.
func ExplainUpstreamFailure(httpStatus int, message string) string {
	status := httpStatus
	if status == 0 {
		status = upstreamHTTPStatus(message)
	}

	switch ClassifyUpstreamError(status, message) {
	case ClassExpired:
		return withUpstreamStatus("upstream credential expired", status)
	case ClassAuth:
		return withUpstreamStatus("upstream rejected the credential", status)
	case ClassBilling:
		return withUpstreamStatus("upstream reported a billing or quota problem", status)
	case ClassModel:
		return withUpstreamStatus("upstream does not serve the requested model", status)
	case ClassValidation:
		return withUpstreamStatus("upstream rejected the request", status)
	case ClassTransient:
		return withUpstreamStatus(explainTransientUpstream(status, message), status)
	}

	// Unclassified: name only what the status itself proves, without claiming a
	// cause the classifier refused to confirm.
	switch {
	case status == 401 || status == 403:
		return withUpstreamStatus("upstream rejected the credential", status)
	case status == 404:
		return withUpstreamStatus("upstream has no such endpoint or account", status)
	case status == 429:
		return withUpstreamStatus("upstream rate-limited the request", status)
	case status >= 500:
		return withUpstreamStatus("upstream errored", status)
	case status >= 400:
		return withUpstreamStatus("upstream rejected the request", status)
	}
	if reason := explainTransportFailure(message); reason != "" {
		return reason
	}
	return ""
}

// explainTransientUpstream splits the transient class into the causes an operator
// acts on differently: too many requests, an upstream that answered too late, an
// upstream that could not be reached at all, and a generic 5xx.
func explainTransientUpstream(status int, message string) string {
	text := strings.ToLower(message)
	switch {
	case status == 429,
		strings.Contains(text, "rate limit"),
		strings.Contains(text, "rate_limit"),
		strings.Contains(text, "too many requests"):
		return "upstream rate-limited the request"
	case status >= 500:
		return "upstream errored"
	}
	if reason := explainTransportFailure(message); reason != "" {
		return reason
	}
	return "upstream was temporarily unavailable"
}

// explainTransportFailure names the failures where nothing came back, so there is
// no status to show and the fix is network-side rather than credential-side.
func explainTransportFailure(message string) string {
	text := strings.ToLower(message)
	switch {
	case strings.Contains(text, "connection refused"), strings.Contains(text, "econnrefused"):
		return "upstream refused the connection"
	case strings.Contains(text, "connection reset"), strings.Contains(text, "econnreset"):
		return "upstream reset the connection"
	case strings.Contains(text, "no such host"), strings.Contains(text, "name resolution"), strings.Contains(text, "server misbehaving"):
		return "upstream host could not be resolved"
	case strings.Contains(text, "unreachable"), strings.Contains(text, "no route to host"):
		return "upstream is unreachable"
	case strings.Contains(text, "timeout"), strings.Contains(text, "timed out"), strings.Contains(text, "deadline exceeded"):
		return "upstream timed out"
	case strings.Contains(text, "x509"), strings.Contains(text, "certificate"), strings.Contains(text, "tls"):
		return "upstream TLS handshake failed"
	case strings.Contains(text, "eof"):
		return "upstream closed the connection"
	}
	return ""
}

// withUpstreamStatus suffixes the status the explanation was derived from, so an
// operator can tell a 401 from a 403 without being shown the upstream body.
func withUpstreamStatus(reason string, status int) string {
	if status == 0 {
		return reason
	}
	return fmt.Sprintf("%s (HTTP %d)", reason, status)
}
