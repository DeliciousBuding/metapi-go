package proxy

import "strings"

// UpstreamEndpoint represents an upstream API endpoint type.
type UpstreamEndpoint string

const (
	EndpointChat      UpstreamEndpoint = "chat"      // /v1/chat/completions
	EndpointMessages  UpstreamEndpoint = "messages"  // /v1/messages (Anthropic)
	EndpointResponses UpstreamEndpoint = "responses" // /v1/responses (Codex)
)

// PathForEndpoint returns the canonical upstream path for a known endpoint type.
// Unknown endpoints return "".
func PathForEndpoint(endpoint UpstreamEndpoint) string {
	switch endpoint {
	case EndpointChat:
		return "/v1/chat/completions"
	case EndpointMessages:
		return "/v1/messages"
	case EndpointResponses:
		return "/v1/responses"
	default:
		return ""
	}
}

// EndpointFromPath maps a downstream path to a known chat-family endpoint.
// Non chat-family paths return ("", false).
func EndpointFromPath(path string) (UpstreamEndpoint, bool) {
	path = strings.TrimSpace(path)
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimRight(path, "/")
	switch {
	case strings.HasSuffix(path, "/v1/chat/completions") || path == "/chat/completions" || path == "/v1/chat/completions":
		return EndpointChat, true
	case strings.HasSuffix(path, "/v1/messages") || path == "/messages" || path == "/v1/messages" ||
		strings.HasSuffix(path, "/anthropic/v1/messages"):
		return EndpointMessages, true
	case strings.HasSuffix(path, "/v1/responses") || path == "/responses" || path == "/v1/responses":
		return EndpointResponses, true
	default:
		return "", false
	}
}

// ResolveEndpointCandidates lives in site_preference.go (with responses-only options).
// See ResolveEndpointCandidates / ResolveEndpointCandidatesWithOptions.

// ShouldDowngradeToNextEndpoint reports whether an upstream error indicates the
// current protocol/path is wrong and a different endpoint candidate should be tried.
func ShouldDowngradeToNextEndpoint(status int, rawErrText string) bool {
	if status <= 0 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(rawErrText))
	if text == "" {
		return false
	}
	// Protocol redirect hints from upstream (also in retry_policy patterns).
	if strings.Contains(text, "please use /v1/chat/completions") ||
		strings.Contains(text, "please use /v1/messages") ||
		strings.Contains(text, "please use /v1/responses") ||
		strings.Contains(text, "unsupported endpoint") ||
		strings.Contains(text, "unsupported path") ||
		strings.Contains(text, "unknown endpoint") ||
		strings.Contains(text, "unrecognized request url") ||
		strings.Contains(text, "unsupported legacy protocol") {
		return true
	}
	// 404 on a protocol path is a common wrong-endpoint signal.
	if status == 404 {
		return true
	}
	return false
}

// BuildUpstreamURL constructs an upstream URL from a site URL and path.
func BuildUpstreamURL(siteURL, path string) string {
	siteURL = strings.TrimRight(siteURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if hasVersionedBasePath(siteURL) {
		path = stripLeadingVersionSegment(path)
	}
	return siteURL + path
}

// ContainsPathTraversal reports whether a request path contains a ".."
// segment. Callers pass percent-decoded paths (net/http decodes the request
// target into r.URL.Path before handlers run, so %2e%2e arrives as ".."),
// which makes a plain segment scan sufficient for both literal and encoded
// traversal. Single-dot segments are not traversal on their own and are left
// alone so legitimate paths are never altered.
func ContainsPathTraversal(path string) bool {
	for _, segment := range strings.Split(path, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func hasVersionedBasePath(siteURL string) bool {
	base := siteURL
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		return false
	}
	lastSlash := strings.LastIndex(base, "/")
	if lastSlash < 0 || lastSlash == len(base)-1 {
		return false
	}
	return isVersionSegment(base[lastSlash+1:])
}

func stripLeadingVersionSegment(path string) string {
	trimmed := strings.TrimPrefix(path, "/")
	segment, rest, found := strings.Cut(trimmed, "/")
	if !found || !isVersionSegment(segment) {
		return path
	}
	if rest == "" {
		return "/"
	}
	return "/" + rest
}

func isVersionSegment(segment string) bool {
	if len(segment) < 2 || (segment[0] != 'v' && segment[0] != 'V') || !isASCIIDigit(segment[1]) {
		return false
	}
	for i := 2; i < len(segment); i++ {
		c := segment[i]
		if !isASCIIDigit(c) && !isASCIIAlpha(c) {
			return false
		}
	}
	return true
}

func isASCIIDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isASCIIAlpha(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
