package proxyhandler

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
)

// clientProtocolHeaders are the downstream client headers that carry protocol
// semantics for the upstream. The data plane builds a fresh upstream request
// (Content-Type + site custom_headers + the selected account token), so without
// this whitelist every client-side protocol switch was silently dropped —
// Claude Code's anthropic-beta feature flags and OpenAI's openai-beta among
// them, which made those clients degrade to baseline behavior.
//
// Applied fill-only (see applyClientProtocolHeaders): a site's custom_headers
// and its anti-bot identity always win, and the selected token is set after
// this call so a client can never override it. Auth-bearing client headers
// (authorization, x-api-key, cookie) are deliberately absent — a downstream key
// must never reach the upstream.
var clientProtocolHeaders = []string{
	"anthropic-version",
	"anthropic-beta",
	"openai-beta",
	"user-agent",
}

// clientProtocolHeaderPrefixes forwards whole SDK telemetry namespaces that are
// safe to pass verbatim and let upstreams attribute traffic (the stainless
// headers every official OpenAI/Anthropic SDK sends: lang, package-version, os,
// arch, runtime, retry-count, ...).
var clientProtocolHeaderPrefixes = []string{"x-stainless-"}

// applyClientProtocolHeaders copies the whitelisted client headers onto the
// upstream request without overriding anything the site configuration already
// put there. upstreamPath drives the Anthropic-native default below.
func applyClientProtocolHeaders(req *http.Request, client http.Header, upstreamPath string) {
	if req == nil || len(client) == 0 {
		return
	}
	for _, name := range clientProtocolHeaders {
		copyHeaderIfAbsent(req.Header, client, name)
	}
	// Prefix namespaces arrive with arbitrary suffixes, so they are matched on
	// the canonical keys the client actually sent.
	for canonical, values := range client {
		if len(values) == 0 || !isClientProtocolPrefix(canonical) {
			continue
		}
		copyHeaderIfAbsent(req.Header, client, canonical)
	}
	// Anthropic-native upstreams reject /v1/messages without anthropic-version.
	// Clients that never send it (an OpenAI-surface request transformed into the
	// Anthropic shape, curl, homegrown SDKs) get the same default the platform
	// adapters use.
	if endpoint, ok := proxy.EndpointFromPath(upstreamPath); ok && endpoint == proxy.EndpointMessages {
		if req.Header.Get("anthropic-version") == "" {
			req.Header.Set("anthropic-version", platform.ClaudeDefaultAnthropicVersion)
		}
	}
}

func isClientProtocolPrefix(canonical string) bool {
	lower := strings.ToLower(canonical)
	for _, prefix := range clientProtocolHeaderPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// copyHeaderIfAbsent copies every value of name from src to dst, keeping dst's
// existing values untouched (fill-only precedence).
func copyHeaderIfAbsent(dst http.Header, src http.Header, name string) {
	canonical := http.CanonicalHeaderKey(name)
	values := src.Values(name)
	if len(values) == 0 || len(dst.Values(canonical)) > 0 {
		return
	}
	dst[canonical] = append([]string(nil), values...)
}

// upstreamResponseHeaders is the whitelist of upstream response headers relayed
// to the downstream client on buffered (non-SSE) responses.
//
// The relay used to copy every upstream header, which leaked the upstream's
// identity and state into our response: vendor fingerprint headers
// (X-New-Api-Version and friends, observable through metapi), Set-Cookie from
// the upstream's own session, and an upstream X-Request-Id that clobbered ours
// and broke cross-layer log correlation.
//
// Rate-limit headers are deliberately excluded so the only X-Ratelimit-* a
// client sees is Metapi's own, and framing/hop-by-hop headers stay out because
// the buffered body is re-framed by net/http. The SSE relay keeps its own
// policy (writeSSEHeaders): a stream is always re-framed by us.
//
// Content-Encoding stays on the list, but it can only ever describe the bytes we
// actually relay: net/http removes it when it transparently decoded a negotiated
// gzip answer, and normalizeBufferedUpstreamBody removes it when we decoded the
// body ourselves. The single case where it survives is a body we could not
// decode, which is relayed verbatim — see upstream_encoding.go.
var upstreamResponseHeaders = []string{
	"Accept-Ranges",
	"Cache-Control",
	"Content-Disposition",
	"Content-Encoding",
	"Content-Language",
	"Content-Range",
	"Content-Type",
	"ETag",
	"Last-Modified",
	"Location",
	"Retry-After",
}

// relayUpstreamResponseHeaders copies the whitelisted upstream headers onto the
// downstream response. Headers Metapi already set (X-Request-Id, security
// headers, CORS) are never touched.
func relayUpstreamResponseHeaders(w http.ResponseWriter, upstream http.Header) {
	if w == nil || len(upstream) == 0 {
		return
	}
	dst := w.Header()
	for _, name := range upstreamResponseHeaders {
		if values := upstream.Values(name); len(values) > 0 {
			dst[name] = append([]string(nil), values...)
		}
	}
}
