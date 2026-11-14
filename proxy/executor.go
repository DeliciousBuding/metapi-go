package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
)

// rejectCrossOriginRedirect is a package-local alias of the shared SSOT in
// platform.RejectCrossOriginRedirect so RuntimeExecutor and existing tests keep
// a short name without diverging policy copies.
func rejectCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	return platform.RejectCrossOriginRedirect(req, via)
}

// NewStreamTransport builds a reusable stream transport whose header budget is
// owned by httpclient.DoWithHeaderBudget, not a competing fixed transport timer.
// Callers must dispatch through that helper (as RuntimeExecutor does), including
// when first-byte observation is disabled. Dial/TLS/idle limits remain intact.
func NewStreamTransport() *http.Transport {
	return httpclient.NewTransport(httpclient.Options{
		// Keep explicitly forbidden site URLs out of proxies; direct dials
		// also validate and pin DNS answers through the shared site guard.
		// Private ranges stay allowed for self-hosted upstreams.
		SiteDialGuard: true,
	})
}

// RuntimeExecutor dispatches upstream HTTP requests.
type RuntimeExecutor struct {
	client *http.Client
	// streamClient relays SSE responses. Unlike client it carries no
	// whole-request timeout — a healthy stream may keep running while chunks
	// flow; liveness is enforced per chunk by the relay's idle guard
	// (PROXY_STREAM_IDLE_TIMEOUT_SEC). DoWithHeaderBudget bounds the header
	// phase with the explicit first-byte budget or DefaultRequestCeiling.
	streamClient *http.Client
}

// NewRuntimeExecutor creates a new RuntimeExecutor with the given timeout.
// The client refuses cross-origin redirects (and https→http) to block SSRF
// via 302 to a different host / private / metadata endpoint.
func NewRuntimeExecutor(requestTimeout time.Duration) *RuntimeExecutor {
	return &RuntimeExecutor{
		client: &http.Client{
			Timeout: requestTimeout,
			// The header phase tracks the whole-request timeout
			// (max(90s, first-byte*2) in app wiring, operator-influenced via
			// PROXY_FIRST_BYTE_TIMEOUT_SEC) so it never pre-empts a request
			// that would finish within its own deadline.
			Transport: httpclient.NewTransport(httpclient.Options{
				ResponseHeaderTimeout: requestTimeout,
				SiteDialGuard:         true, // see NewStreamTransport
			}),
			CheckRedirect: rejectCrossOriginRedirect,
		},
		streamClient: &http.Client{
			Transport:     NewStreamTransport(),
			CheckRedirect: rejectCrossOriginRedirect,
		},
	}
}

// ErrObservedFirstByteTimeout is returned by DoWithObservedFirstByte when the
// first-byte / response-header deadline fires before headers arrive.
//
// Unit note: firstByteTimeoutMs is milliseconds. Config field
// PROXY_FIRST_BYTE_TIMEOUT_SEC is seconds; convert with FirstByteTimeoutMs.
var ErrObservedFirstByteTimeout = httpclient.ErrResponseHeaderTimeout

// FirstByteTimeoutMs converts ProxyFirstByteTimeoutSec (seconds) to the
// internal first-byte observation unit (milliseconds). Values <= 0 disable
// first-byte observation (returns 0).
//
// Matches original TS: firstByteTimeoutMs = proxyFirstByteTimeoutSec * 1000.
func FirstByteTimeoutMs(proxyFirstByteTimeoutSec int) int64 {
	if proxyFirstByteTimeoutSec <= 0 {
		return 0
	}
	return int64(proxyFirstByteTimeoutSec) * 1000
}

// DoWithObservedFirstByte sends req and observes first-byte (response header)
// latency. Callers own body buffering or streaming and must close resp.Body.
//
// When firstByteTimeoutMs > 0 and the deadline fires before headers arrive,
// it returns (nil, ErrObservedFirstByteTimeout).
// firstByteTimeoutMs is milliseconds (see FirstByteTimeoutMs).
//
// After headers arrive the first-byte timer is stopped so the body stream is not
// cancelled by the observation deadline.
func (e *RuntimeExecutor) DoWithObservedFirstByte(
	ctx context.Context,
	req *http.Request,
	firstByteTimeoutMs int64,
) (*http.Response, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("dispatch: executor is not configured")
	}
	if firstByteTimeoutMs <= 0 {
		return e.client.Do(req)
	}
	return httpclient.DoWithHeaderBudget(req.WithContext(ctx), firstByteTimeoutMs, e.client.Do)
}

// DoStreamWithObservedFirstByte dispatches through the stream client without a
// whole-request timeout. The shared header-budget owner uses the explicit
// first-byte timeout, or DefaultRequestCeiling when observation is disabled.
// Once headers arrive, only the caller and relay body-idle guard may cancel a
// healthy stream; callers must close resp.Body to release the request context.
func (e *RuntimeExecutor) DoStreamWithObservedFirstByte(
	ctx context.Context,
	req *http.Request,
	firstByteTimeoutMs int64,
) (*http.Response, error) {
	if e == nil || e.streamClient == nil {
		return nil, fmt.Errorf("dispatch: executor is not configured")
	}
	return httpclient.DoWithHeaderBudget(req.WithContext(ctx), firstByteTimeoutMs, e.streamClient.Do)
}

// IsObservedFirstByteTimeoutError reports whether err is a first-byte timeout.
func IsObservedFirstByteTimeoutError(err error) bool {
	return errors.Is(err, ErrObservedFirstByteTimeout)
}
