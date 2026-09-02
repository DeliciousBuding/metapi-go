package proxyhandler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
)

// This file owns the data plane's compression-encoding honesty: what we put on
// the wire decides whether we can read the answer, and what we can read decides
// whether we may account usage and judge content at all.
//
// Two invariants, both enforced here and nowhere else:
//
//  1. Outbound: a data-plane request never carries a site/adapter-configured
//     Accept-Encoding (stripUpstreamAcceptEncoding). That header is not
//     identity or protocol semantics an operator may choose — its value decides
//     whether net/http transparently decodes the answer, i.e. whether Metapi can
//     read the body it is about to bill for and health-check.
//  2. Inbound: bytes we cannot decode are never parsed, never judged and never
//     turned into tokens (normalizeBufferedUpstreamBody /
//     prepareStreamUpstreamBody). They are relayed verbatim with their
//     Content-Encoding, and the accounting gap is made visible through
//     proxy.UpstreamEncodingSkippedMessage.

// upstreamAcceptEncodingDroppedMessage is the STABLE warn text for invariant 1.
const upstreamAcceptEncodingDroppedMessage = "site custom_headers set Accept-Encoding on a data-plane upstream request; the header was dropped so net/http can keep transparently decoding upstream bodies"

// upstreamAcceptEncodingWarnDedupeCap bounds the dedupe set below. The warning
// describes a static misconfiguration, so it is logged once per
// (site, channel, value) instead of once per request; the cap only exists so a
// pathological fleet of misconfigured sites cannot grow the set without bound.
// Beyond it new keys are silently suppressed — the first N are already on the
// record.
const upstreamAcceptEncodingWarnDedupeCap = 512

var (
	upstreamAcceptEncodingWarnMu     sync.Mutex
	upstreamAcceptEncodingWarnedKeys = map[string]struct{}{}
)

// stripUpstreamAcceptEncoding removes Accept-Encoding from an outbound
// data-plane request.
//
// Why: the outbound request is built from a whitelist (client protocol headers)
// plus site custom_headers plus the selected account token, and the client's own
// Accept-Encoding is deliberately not on that whitelist. When the request
// reaches the transport with no Accept-Encoding, net/http adds "gzip" itself,
// transparently decompresses the answer and removes Content-Encoding /
// Content-Length from resp.Header — so the body the usage parser and the content
// judge see is the real content. A site custom_headers entry silently switched
// that off: the body then arrived as compressed bytes, usage extraction found no
// tokens (billing under-count), the keyword scan read noise, and with
// PROXY_EMPTY_CONTENT_FAIL enabled a perfectly good answer could be recorded as
// an empty-content 502 — a false failure that poisons channel health.
//
// proxyConfig is cleared at the source as well, not just req.Header: the
// site-proxy path (platform.DoWithProxy / SiteProxy.Do) re-applies
// proxyConfig.CustomHeaders to the request after we built it, so a value left
// in that map would come straight back onto the wire. The config is rebuilt per
// attempt by service.BuildPlatformProxyConfig, so removing the key is scoped to
// this one dispatch.
func stripUpstreamAcceptEncoding(req *http.Request, proxyConfig *platform.ProxyConfig, siteID, channelID int64) {
	dropped := deleteAcceptEncodingCustomHeader(proxyConfig)
	if req != nil && req.Header != nil {
		if values := req.Header.Values("Accept-Encoding"); len(values) > 0 {
			req.Header.Del("Accept-Encoding")
			dropped = append(dropped, values...)
		}
	}
	if len(dropped) == 0 {
		return
	}
	warnUpstreamAcceptEncodingDropped(siteID, channelID, strings.Join(dropped, ","))
}

// deleteAcceptEncodingCustomHeader removes every Accept-Encoding key (any
// casing) from a site's custom_headers map and returns the values it carried.
func deleteAcceptEncodingCustomHeader(proxyConfig *platform.ProxyConfig) []string {
	if proxyConfig == nil || len(proxyConfig.CustomHeaders) == 0 {
		return nil
	}
	var dropped []string
	for name, value := range proxyConfig.CustomHeaders {
		if strings.EqualFold(strings.TrimSpace(name), "accept-encoding") {
			dropped = append(dropped, value)
			delete(proxyConfig.CustomHeaders, name)
		}
	}
	return dropped
}

// warnUpstreamAcceptEncodingDropped logs invariant 1 at most once per
// (site, channel, dropped value).
func warnUpstreamAcceptEncodingDropped(siteID, channelID int64, dropped string) {
	key := fmt.Sprintf("%d|%d|%s", siteID, channelID, dropped)
	upstreamAcceptEncodingWarnMu.Lock()
	if _, seen := upstreamAcceptEncodingWarnedKeys[key]; seen {
		upstreamAcceptEncodingWarnMu.Unlock()
		return
	}
	if len(upstreamAcceptEncodingWarnedKeys) >= upstreamAcceptEncodingWarnDedupeCap {
		upstreamAcceptEncodingWarnMu.Unlock()
		return
	}
	upstreamAcceptEncodingWarnedKeys[key] = struct{}{}
	upstreamAcceptEncodingWarnMu.Unlock()

	slog.Warn(upstreamAcceptEncodingDroppedMessage,
		"site_id", siteID,
		"channel_id", channelID,
		"dropped_accept_encoding", dropped,
	)
}

// resetUpstreamAcceptEncodingWarnings clears the dedupe set. Test-only: the
// warning is once-per-configuration on purpose, so tests that assert on it need
// a clean slate.
func resetUpstreamAcceptEncodingWarnings() {
	upstreamAcceptEncodingWarnMu.Lock()
	upstreamAcceptEncodingWarnedKeys = map[string]struct{}{}
	upstreamAcceptEncodingWarnMu.Unlock()
}

// upstreamBodyIdent names the attempt a body-level warning belongs to, so an
// operator can trace an accounting gap back to a site, a channel and a request.
type upstreamBodyIdent struct {
	siteID    int64
	channelID int64
	requestID string
	stream    bool
}

// bufferedUpstreamBody is the data plane's view of a buffered upstream body
// after the single encoding decision (proxy.NormalizeUpstreamBufferedBody) has
// been applied. readable=false means the bytes are still encoded and were
// relayed verbatim: no parsing, no judging, no tokens.
type bufferedUpstreamBody struct {
	bytes    []byte
	readable bool
}

// parseUsage extracts usage only from bytes we can actually read. An
// undecodable body yields the explicit unknown source — never invented tokens.
func (b bufferedUpstreamBody) parseUsage() ParsedUsage {
	if !b.readable {
		return ParsedUsage{Source: usageSourceUnknown}
	}
	return ParseUsageFromBody(b.bytes)
}

// judgeFacts builds the content-judge facts for this body. Unreadable bytes are
// handed over as an explicit "no evidence" fact rather than as noise the judge
// would have to guess about, so the single judge stays the single owner of the
// verdict.
func (b bufferedUpstreamBody) judgeFacts(statusCode int, usage ParsedUsage) proxy.UpstreamContentFacts {
	facts := proxy.UpstreamContentFacts{
		StatusCode: statusCode,
		Usage:      usage.ToUsageSummary(),
		Unreadable: !b.readable,
	}
	if b.readable {
		facts.RawText = string(b.bytes)
	}
	return facts
}

// normalizeBufferedUpstreamBody applies the encoding decision to a buffered
// upstream body and returns the bytes the caller may parse and relay. It also
// owns the honest-abandonment warning: resp.Header has already been corrected
// by proxy.NormalizeUpstreamBufferedBody when the body was decoded, so the
// existing relay whitelist can only forward a Content-Encoding that still
// describes the bytes we are about to write.
func normalizeBufferedUpstreamBody(resp *http.Response, body []byte, ident upstreamBodyIdent) bufferedUpstreamBody {
	if resp == nil {
		return bufferedUpstreamBody{bytes: body, readable: true}
	}
	normalized := proxy.NormalizeUpstreamBufferedBody(resp.Header, body)
	if !normalized.Readable {
		warnUpstreamEncodingSkipped(resp, normalized.Encoding, normalized.DecodeErr, ident)
	}
	return bufferedUpstreamBody{bytes: normalized.Bytes, readable: normalized.Readable}
}

// prepareStreamUpstreamBody applies the same encoding decision to a streaming
// upstream body. The streaming path must not grow its own semantics: a codec we
// implement is decoded inline (we re-frame every chunk, so no Content-Encoding
// goes downstream), and a body we cannot read is relayed verbatim with the
// upstream's own Content-Encoding — the one case where the SSE response does
// carry it, because in that case we rewrote nothing and the header is true.
func prepareStreamUpstreamBody(resp *http.Response, w http.ResponseWriter, ident upstreamBodyIdent) (readable bool) {
	if resp == nil {
		return true
	}
	prepared := proxy.WrapUpstreamStreamBody(resp.Header, resp.Body)
	if prepared.Reader != nil {
		resp.Body = prepared.Reader
	}
	if prepared.Readable {
		return true
	}
	if prepared.Encoding.RelayHeader() {
		// Verbatim passthrough: the bytes we relay are exactly the encoded bytes
		// the upstream sent, so keeping its Content-Encoding is the truthful
		// framing (and it lets the client decode what we could not).
		w.Header().Set("Content-Encoding", prepared.Encoding.Value)
	}
	warnUpstreamEncodingSkipped(resp, prepared.Encoding, nil, ident)
	return false
}

// warnUpstreamEncodingSkipped emits the stable operator-facing warning for a
// body we could not read. One line per affected response: the accounting gap is
// real and per-request, so it must not be deduped into silence.
func warnUpstreamEncodingSkipped(resp *http.Response, enc proxy.UpstreamEncoding, decodeErr error, ident upstreamBodyIdent) {
	upstreamHost := ""
	if resp != nil && resp.Request != nil && resp.Request.URL != nil {
		upstreamHost = resp.Request.URL.Host
	}
	errText := ""
	if decodeErr != nil {
		errText = decodeErr.Error()
	}
	attrs := []any{
		"content_encoding", enc.Value,
		"encoding_class", enc.String(),
		"decode_err", errText,
		"upstream_host", upstreamHost,
		"stream", ident.stream,
		"usage_source", usageSourceUnknown,
		"request_id", ident.requestID,
	}
	// The streaming relay does not carry the routing selection, so a zero id
	// means "unknown here" and is left out rather than logged as a fake 0.
	if ident.siteID != 0 {
		attrs = append(attrs, "site_id", ident.siteID)
	}
	if ident.channelID != 0 {
		attrs = append(attrs, "channel_id", ident.channelID)
	}
	slog.Warn(proxy.UpstreamEncodingSkippedMessage, attrs...)
}
