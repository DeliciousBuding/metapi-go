package proxyhandler

import (
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// streamIdleTimeout returns the configured SSE chunk-gap timeout from the
// startup-loaded config singleton, falling back to
// DefaultProxyStreamIdleTimeoutSec when config is not yet loaded (tests that
// bypass TestMain) or the operator value is non-positive. Mirrors the
// per-request config.GetSafe() pattern of streamResponseByteLimit; like the
// other PROXY_* timeouts the value itself is frozen at startup.
func streamIdleTimeout() time.Duration {
	sec := config.DefaultProxyStreamIdleTimeoutSec
	if cfg := config.GetSafe(); cfg != nil && cfg.ProxyStreamIdleTimeoutSec > 0 {
		sec = cfg.ProxyStreamIdleTimeoutSec
	}
	return time.Duration(sec) * time.Second
}

// streamIdleBody wraps the upstream SSE body with the chunk-gap guard. Read
// pushes the idle deadline forward for every non-empty chunk; Close stops
// the watchdog and closes the underlying body exactly once. When the idle
// deadline passes with no chunk, the watchdog closes the underlying body to
// unblock a pending Read with an error, which the relay loop classifies via
// the guard's fired flag.
type streamIdleBody struct {
	io.ReadCloser
	timing    *attemptStreamTiming
	guard     *streamIdleGuard
	closeOnce sync.Once
	closeErr  error
}

func (b *streamIdleBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		if b.timing != nil {
			b.timing.arrival = time.Now()
		}
		b.guard.reset()
	}
	return n, err
}

// closeUnderlying closes the wrapped body exactly once. Invoked both by the
// idle watchdog (to interrupt a blocked Read) and by Close.
func (b *streamIdleBody) closeUnderlying() {
	b.closeOnce.Do(func() { b.closeErr = b.ReadCloser.Close() })
}

func (b *streamIdleBody) Close() error {
	if b.guard != nil {
		b.guard.stop()
	}
	b.closeUnderlying()
	return b.closeErr
}

// streamIdleGuard bounds the gap between upstream SSE chunks. A single
// watchdog goroutine sleeps until the current deadline; every relayed chunk
// stores a later deadline (atomic, lock-free on the hot path). When the
// deadline passes with no new chunk the guard fires closeFn exactly once.
type streamIdleGuard struct {
	idle     time.Duration
	closeFn  func()
	deadline atomic.Int64 // unixnano of the current idle cutoff
	fired    atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
}

func newStreamIdleGuard(idle time.Duration, closeFn func()) *streamIdleGuard {
	g := &streamIdleGuard{
		idle:    idle,
		closeFn: closeFn,
		stopCh:  make(chan struct{}),
	}
	g.deadline.Store(time.Now().Add(idle).UnixNano())
	go g.watch()
	return g
}

// reset extends the idle window; called for every non-empty relayed chunk.
func (g *streamIdleGuard) reset() {
	g.deadline.Store(time.Now().Add(g.idle).UnixNano())
}

func (g *streamIdleGuard) stop() {
	g.stopOnce.Do(func() { close(g.stopCh) })
}

func (g *streamIdleGuard) watch() {
	for {
		wait := time.Until(time.Unix(0, g.deadline.Load()))
		if wait <= 0 {
			if g.tryFire() {
				return
			}
			continue // a chunk moved the deadline; re-arm
		}
		timer := time.NewTimer(wait)
		select {
		case <-g.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			if g.tryFire() {
				return
			}
			// A chunk moved the deadline while the timer was running; loop
			// and re-arm with the remaining window.
		}
	}
}

// tryFire fires the guard unless a chunk arrived between timer expiry and
// now. Returns true when the guard fired (or had already fired) so the
// watchdog can exit.
func (g *streamIdleGuard) tryFire() bool {
	if time.Now().UnixNano() < g.deadline.Load() {
		return false
	}
	if g.fired.CompareAndSwap(false, true) {
		g.closeFn()
	}
	return true
}
