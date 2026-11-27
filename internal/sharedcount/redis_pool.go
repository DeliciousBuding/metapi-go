package sharedcount

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"
	"time"
)

// maxRedisConns bounds one RedisCounter's connection pool: at most this many
// connections are open at once and callers block (bounded by ctx) while all of
// them are in flight. Redis itself is single-threaded, so a handful of
// connections is already enough to keep the round trips pipelined behind the
// admission path while the FD footprint stays small.
const maxRedisConns = 8

// redisPool keeps a RedisCounter's reusable connections.
//
// sem bounds concurrent operations to maxRedisConns, which keeps the FD footprint
// flat when admission is bursty; idle is a LIFO free list, so the warmest
// connection is reused first and one the server dropped while idle is picked up by
// the retry in withConn. The mutex is held only for the slice push/pop, never
// across I/O.
type redisPool struct {
	once sync.Once
	sem  chan struct{}
	mu   sync.Mutex
	idle []net.Conn
}

// enter claims a connection slot, blocking until one is free or ctx ends.
func (p *redisPool) enter(ctx context.Context) error {
	p.once.Do(func() { p.sem = make(chan struct{}, maxRedisConns) })
	select {
	case p.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// leave releases a connection slot.
func (p *redisPool) leave() { <-p.sem }

// take pops the most recently used idle connection, if any.
func (p *redisPool) take() net.Conn {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := len(p.idle)
	if n == 0 {
		return nil
	}
	conn := p.idle[n-1]
	p.idle[n-1] = nil // drop the reference before shrinking
	p.idle = p.idle[:n-1]
	return conn
}

// put parks a connection for reuse.
func (p *redisPool) put(conn net.Conn) {
	p.mu.Lock()
	p.idle = append(p.idle, conn)
	p.mu.Unlock()
}

// acquire claims a pool slot and returns a ready-to-use connection. pooled
// reports whether it came from the free list and may therefore have been closed by
// the server while idle. Blocks until a slot is free, bounded by ctx.
func (r *RedisCounter) acquire(ctx context.Context) (conn net.Conn, pooled bool, err error) {
	if err := r.pool.enter(ctx); err != nil {
		return nil, false, err
	}
	if conn := r.pool.take(); conn != nil {
		return conn, true, nil
	}
	conn, err = r.dialConn(ctx)
	if err != nil {
		r.pool.leave()
		return nil, false, err
	}
	return conn, false, nil
}

// release returns a connection to the pool, or drops it when it is no longer
// usable, and gives the pool slot back either way.
func (r *RedisCounter) release(conn net.Conn, reuse bool) {
	if reuse {
		r.pool.put(conn)
	} else {
		_ = conn.Close()
	}
	r.pool.leave()
}

// dialConn opens a connection and applies AUTH/SELECT once for its lifetime.
func (r *RedisCounter) dialConn(ctx context.Context) (net.Conn, error) {
	dial := r.dial
	if dial == nil {
		dial = net.DialTimeout
	}
	conn, err := dial("tcp", r.addr, r.timeout)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(r.deadline(ctx))
	if r.password != "" {
		if err := redisDo(conn, "AUTH", r.password); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	if r.db != 0 {
		if err := redisDo(conn, "SELECT", strconv.Itoa(r.db)); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// deadline bounds one operation: the counter timeout, tightened by ctx when ctx
// expires sooner.
func (r *RedisCounter) deadline(ctx context.Context) time.Time {
	d := time.Now().Add(r.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(d) {
		return dl
	}
	return d
}

// attempt runs fn on conn with the operation deadline applied and ctx
// cancellation wired to the connection, and reports whether conn may be reused.
func (r *RedisCounter) attempt(ctx context.Context, conn net.Conn, fn func(net.Conn) error) (reuse bool, err error) {
	deadline := r.deadline(ctx)
	// deadlineFromCtx records that the bound we are about to enforce is the ctx
	// deadline, which lets a connection timeout be reported as the ctx error even
	// when the ctx timer goroutine has not marked it done yet.
	_, deadlineFromCtx := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	var g connGuard
	g.conn = conn
	if ctx.Done() != nil {
		// A blocked Read/Write cannot observe cancellation by itself, so close the
		// connection out from under it. connGuard guarantees that never closes a
		// connection the operation already finished with.
		g.stop = make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				g.abort()
			case <-g.stop:
			}
		}()
	}
	err = fn(conn)
	g.finish()
	if err != nil {
		// Report cancellation/deadline rather than a transport detail such as
		// "use of closed connection".
		switch {
		case ctx.Err() != nil:
			err = ctx.Err()
		case deadlineFromCtx && isTimeoutError(err):
			// The connection deadline came from ctx and fired a hair before the
			// ctx timer goroutine ran; the cause is the same, so report it as such
			// (otherwise callers see a bare net timeout instead of the ctx error).
			err = context.DeadlineExceeded
		}
		return false, err
	}
	return !g.aborted(), nil
}

// isTimeoutError reports a connection deadline hit.
func isTimeoutError(err error) bool {
	var nerr net.Error
	return errors.As(err, &nerr) && nerr.Timeout()
}

// withConn runs fn on a pooled connection.
//
// A connection taken from the free list may have been closed by the server while
// it sat idle (idle timeout, restart, failover). Such a transport failure is
// retried, and because every failure discards its connection the free list drains
// after at most maxRedisConns attempts and a fresh one is dialed. Only transport
// failures are retried: a Redis error reply (rejected AUTH, unknown command, ...)
// comes back as-is because a new connection would fail identically, and a failure
// on a freshly dialed connection comes back as-is because there is nothing stale
// to replace.
//
// A retry can double-apply a non-idempotent INCR in the rare case the server ran
// the command but the reply was lost. For admission counters that is the safe
// direction (over-count costs one extra 429), whereas surfacing the error would
// fail open and under-count the window.
func (r *RedisCounter) withConn(ctx context.Context, fn func(net.Conn) error) error {
	if err := ctx.Err(); err != nil {
		return ctx.Err()
	}
	var lastErr error
	for attempt := 0; attempt <= maxRedisConns; attempt++ {
		conn, pooled, err := r.acquire(ctx)
		if err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}
		reuse, err := r.attempt(ctx, conn, fn)
		r.release(conn, reuse)
		if err == nil {
			return nil
		}
		lastErr = err
		// Retry only a transport failure on a connection that came from the free
		// list: an error reply would fail identically on a fresh connection, a
		// freshly dialed connection has nothing stale to replace, and a finished
		// ctx must not be turned into another round trip.
		if !pooled || isReplyError(err) || isCtxError(err) || ctx.Err() != nil {
			return err
		}
	}
	return lastErr
}

// connGuard lets ctx cancellation interrupt in-flight I/O without ever closing a
// connection that has already been handed back to the pool.
type connGuard struct {
	mu     sync.Mutex
	conn   net.Conn
	done   bool // operation finished — the watcher must not close conn
	closed bool // watcher closed conn — the caller must not reuse it
	stop   chan struct{}
}

// abort closes the connection to unblock a pending Read/Write. No-op once the
// operation has finished.
func (g *connGuard) abort() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.done {
		return
	}
	g.closed = true
	_ = g.conn.Close()
}

// finish disarms the watcher.
func (g *connGuard) finish() {
	g.mu.Lock()
	g.done = true
	g.mu.Unlock()
	if g.stop != nil {
		close(g.stop)
	}
}

// aborted reports whether the watcher closed the connection.
func (g *connGuard) aborted() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}

// isCtxError reports a cancellation/deadline outcome, including the case where
// the connection deadline fired just before the ctx timer goroutine did.
func isCtxError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
