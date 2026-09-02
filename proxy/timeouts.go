package proxy

import "time"

// DefaultRequestCeiling is the whole-request timeout ceiling for buffered
// (non-stream) upstream dispatch. It is a safety net for a hung upstream, not
// the primary liveness control: the header phase is observed separately via
// PROXY_FIRST_BYTE_TIMEOUT_SEC, and streams are governed per chunk by the
// relay's idle guard (PROXY_STREAM_IDLE_TIMEOUT_SEC) instead of total elapsed
// time.
const DefaultRequestCeiling = 90 * time.Second

// bufferedWriteSlack is the extra time a proxy response write gets on top of
// the request ceiling. When the write starts the buffered body is already in
// memory, so the slack only has to cover transmitting it to a slow client.
const bufferedWriteSlack = 2 * time.Minute

// RequestCeiling returns the whole-request timeout for buffered upstream
// dispatch: max(DefaultRequestCeiling, firstByteTimeout*2). The doubling keeps
// multi-endpoint fallback able to complete when an operator raises the
// first-byte window above the default ceiling.
//
// This is the single source of truth shared by the executor wiring
// (app.WireProxyUpstream) and the server-side write budget below, so the two
// can never drift apart again.
func RequestCeiling(firstByteTimeoutSec int) time.Duration {
	ceiling := DefaultRequestCeiling
	if ms := FirstByteTimeoutMs(firstByteTimeoutSec); ms > 0 {
		if doubled := time.Duration(ms) * time.Millisecond * 2; doubled > ceiling {
			ceiling = doubled
		}
	}
	return ceiling
}

// WriteBudget returns the write-deadline budget for a proxy surface response.
//
// net/http arms the connection write deadline with Server.WriteTimeout (60s in
// app.newHTTPServer) right after the request header is read, so that deadline
// also covers the time the handler spends waiting for the upstream. With the
// executor ceiling at 90s the two inverted: a buffered response that arrived
// between 61s and 90s was killed while writing it back to the client. The
// proxy surface therefore re-arms its own deadline (router.ProxyWriteDeadline)
// from this budget, which is always >= RequestCeiling.
func WriteBudget(firstByteTimeoutSec int) time.Duration {
	return RequestCeiling(firstByteTimeoutSec) + bufferedWriteSlack
}
