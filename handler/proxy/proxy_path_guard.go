package proxyhandler

import (
	"net/url"
	"path"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
)

// maxProxiedResourceIDLen bounds an upstream resource id that is concatenated
// into a proxied path. OpenAI-compatible file ids look like "file-<token>";
// public video ids are "video_<hex>" / "video_direct_<hex>". 200 leaves room
// for longer provider ids while keeping the forwarded path bounded.
const maxProxiedResourceIDLen = 200

// upstreamPathDecodeRounds bounds how many percent-decoding layers the
// dispatch guard inspects. One layer is what net/http applies to the request
// target; the extra rounds cover callers that build DownstreamPath from
// already-escaped values and the documented double-encoding bypass. Anything
// still carrying an encoded separator after the budget is rejected below.
const upstreamPathDecodeRounds = 2

// validProxiedResourceID reports whether id can be concatenated into an
// upstream path without escaping. The whitelist deliberately excludes '%',
// '/', '\', '.', whitespace and control characters, so encoded traversal
// (%2e%2e%2f), double encoding, separator smuggling, NUL and CRLF cannot
// survive any decoding layer the upstream may apply.
func validProxiedResourceID(id string) bool {
	if len(id) == 0 || len(id) > maxProxiedResourceIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// validUpstreamDispatchPath is the last line of defense before a downstream
// path is concatenated onto an upstream base URL (proxy.BuildUpstreamURL).
// chi's URLParam returns the raw (still percent-encoded) segment whenever
// RawPath is set, so a segment-only ".." check is not enough: decoding must be
// repeated, and the resource surfaces must still sit under their expected
// prefix after path.Clean.
//
// It never rewrites the path; callers forward the original bytes on success.
// Query strings are split off before validation so they are not touched.
func validUpstreamDispatchPath(upstreamPath string) bool {
	rawPath, _, _ := strings.Cut(upstreamPath, "?")
	if rawPath == "" || hasControlBytes(rawPath) || strings.Contains(rawPath, `\`) {
		return false
	}

	decoded := rawPath
	for round := 0; round < upstreamPathDecodeRounds; round++ {
		next, err := url.PathUnescape(decoded)
		if err != nil {
			// A literal '%' that is not an escape is only reachable after an
			// earlier round produced it (e.g. an escaped upstream task id);
			// keep the last successfully decoded value.
			break
		}
		if next == decoded {
			break
		}
		decoded = next
	}
	if hasControlBytes(decoded) || strings.Contains(decoded, `\`) {
		return false
	}
	lower := strings.ToLower(decoded)
	for _, token := range []string{"%2e", "%2f", "%5c", "%00", "%0d", "%0a"} {
		if strings.Contains(lower, token) {
			return false
		}
	}
	if proxy.ContainsPathTraversal(decoded) {
		return false
	}

	switch {
	case decoded == "/v1/files" || strings.HasPrefix(decoded, "/v1/files/"):
		return validFilesUpstreamPath(decoded)
	case decoded == "/v1/videos" || strings.HasPrefix(decoded, "/v1/videos/"):
		clean := path.Clean(decoded)
		return clean == "/v1/videos" || strings.HasPrefix(clean, "/v1/videos/")
	}
	return true
}

// validFilesUpstreamPath enforces the only shapes the files handlers emit:
// /v1/files/{id} and /v1/files/{id}/content. File ids are not escaped on the
// wire, so the strict id whitelist applies after decoding.
func validFilesUpstreamPath(decoded string) bool {
	if decoded == "/v1/files" {
		return true
	}
	parts := strings.Split(strings.TrimPrefix(decoded, "/v1/files/"), "/")
	if len(parts) == 1 {
		return validProxiedResourceID(parts[0])
	}
	return len(parts) == 2 && parts[1] == "content" && validProxiedResourceID(parts[0])
}

// hasControlBytes reports whether s contains a byte that must never reach an
// upstream request line (NUL, CR, LF and the rest of C0/C1 controls).
func hasControlBytes(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}
