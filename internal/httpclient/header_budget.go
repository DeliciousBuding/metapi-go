package httpclient

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"sync/atomic"
	"time"
)

// DefaultRequestCeiling bounds buffered upstream requests and the header wait
// for streams whose first-byte observation is disabled. It never limits a
// stream's body lifetime.
const DefaultRequestCeiling = 90 * time.Second

// ErrResponseHeaderTimeout preserves the proxy's first-byte timeout identity.
var ErrResponseHeaderTimeout = errors.New("first byte timeout")

// DoWithHeaderBudget owns the header phase of one dispatch, including redirects.
// firstByteTimeoutMs is milliseconds; non-positive values use DefaultRequestCeiling.
// Streaming dispatch must reuse a client with no whole-request or transport
// header timeout. Buffered callers retain their whole-request ceiling and skip this
// helper when first-byte observation is disabled.
//
// The timer stops when do returns headers. The derived context remains alive for
// body reads and is cancelled on error or Body.Close; body-idle limits belong to
// the relay. Neither the client nor its pooled transport is changed per request.
func DoWithHeaderBudget(req *http.Request, firstByteTimeoutMs int64, do func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	parent := req.Context()
	ctx, cancel := context.WithCancel(parent)
	const (
		waiting uint32 = iota
		headers
		expired
	)
	var phase atomic.Uint32
	timer := time.AfterFunc(responseHeaderBudget(firstByteTimeoutMs), func() {
		if phase.CompareAndSwap(waiting, expired) {
			cancel()
		}
	})

	resp, err := do(req.WithContext(ctx))
	// Stop alone cannot prevent an already-scheduled callback from cancelling
	// the body. Only the winner of this transition may finish the header phase.
	phase.CompareAndSwap(waiting, headers)
	timer.Stop()
	if phase.Load() == expired {
		cancel()
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		return nil, ErrResponseHeaderTimeout
	}
	if err != nil {
		cancel()
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return nil, err
	}
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

func responseHeaderBudget(firstByteTimeoutMs int64) time.Duration {
	if firstByteTimeoutMs <= 0 {
		return DefaultRequestCeiling
	}
	if firstByteTimeoutMs > math.MaxInt64/int64(time.Millisecond) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(firstByteTimeoutMs) * time.Millisecond
}

// cancelOnCloseBody releases the request context even if the underlying Close
// blocks waiting for cancellation or returns an error.
type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelOnCloseBody) Close() error {
	b.cancel()
	return b.ReadCloser.Close()
}
