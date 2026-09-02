package sharedcount

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WindowCounter increments a named key inside a sliding/fixed window and returns
// the post-increment count. Used for multi-instance RPM/TPM admission.
type WindowCounter interface {
	// Incr increments key by 1 within window and returns the new count.
	// Implementations may approximate sliding windows with fixed TTL buckets.
	Incr(ctx context.Context, key string, window time.Duration) (count int64, err error)
	// Decr decrements key by 1 (compensating rollback for a prior Incr,).
	// Does not extend/refresh TTL. Count is floored at 0 for memory; Redis DECR
	// may go negative, and a rollback that lands after the window expired drops
	// the re-created non-positive key instead of leaving it behind without a TTL.
	Decr(ctx context.Context, key string, window time.Duration) (count int64, err error)
	// IncrBy increments key by delta within window and returns the new total.
	// Used for TPM token reservations. delta==0 returns the current total.
	// Negative delta is a compensating rollback and does not refresh TTL; like
	// Decr it removes the key when the rollback takes the total to <= 0.
	IncrBy(ctx context.Context, key string, delta int64, window time.Duration) (count int64, err error)
	// Get returns the current count without incrementing.
	Get(ctx context.Context, key string) (count int64, err error)
}

// MemoryCounter is the default single-process implementation.
type MemoryCounter struct {
	mu   sync.Mutex
	keys map[string]*memWindow
	now  func() time.Time
}

type memWindow struct {
	times  []int64    // unix ms — unit Incr events (RPM)
	points []memPoint // weighted IncrBy events (TPM)
}

type memPoint struct {
	atMs  int64
	delta int64
}

func NewMemoryCounter() *MemoryCounter {
	return &MemoryCounter{keys: make(map[string]*memWindow), now: time.Now}
}

func (m *MemoryCounter) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = ctx
	if window <= 0 {
		window = time.Minute
	}
	nowMs := m.now().UnixMilli()
	start := nowMs - window.Milliseconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.keys[key]
	if w == nil {
		w = &memWindow{}
		m.keys[key] = w
	}
	// prune
	i := 0
	for i < len(w.times) && w.times[i] < start {
		i++
	}
	if i > 0 {
		w.times = append([]int64(nil), w.times[i:]...)
	}
	w.times = append(w.times, nowMs)
	return int64(len(w.times)), nil
}

func (m *MemoryCounter) Decr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = ctx
	if window <= 0 {
		window = time.Minute
	}
	nowMs := m.now().UnixMilli()
	start := nowMs - window.Milliseconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.keys[key]
	if w == nil {
		return 0, nil
	}
	// prune
	i := 0
	for i < len(w.times) && w.times[i] < start {
		i++
	}
	if i > 0 {
		w.times = append([]int64(nil), w.times[i:]...)
	}
	// Compensating rollback: drop the most recent unit event when present.
	if len(w.times) > 0 {
		w.times = w.times[:len(w.times)-1]
	}
	return int64(len(w.times)), nil
}

func (m *MemoryCounter) IncrBy(ctx context.Context, key string, delta int64, window time.Duration) (int64, error) {
	_ = ctx
	if window <= 0 {
		window = time.Minute
	}
	nowMs := m.now().UnixMilli()
	start := nowMs - window.Milliseconds()
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.keys[key]
	if w == nil {
		w = &memWindow{}
		m.keys[key] = w
	}
	// prune
	i := 0
	for i < len(w.points) && w.points[i].atMs < start {
		i++
	}
	if i > 0 {
		w.points = append([]memPoint(nil), w.points[i:]...)
	}
	// delta==0 is a read-only peek; positive reserves; negative rolls back.
	if delta != 0 {
		w.points = append(w.points, memPoint{atMs: nowMs, delta: delta})
	}
	var sum int64
	for _, p := range w.points {
		sum += p.delta
	}
	if sum < 0 {
		// Keep memory totals non-negative for admission math.
		sum = 0
		w.points = nil
	}
	return sum, nil
}

func (m *MemoryCounter) Get(ctx context.Context, key string) (int64, error) {
	_ = ctx
	nowMs := m.now().UnixMilli()
	start := nowMs - 60_000
	m.mu.Lock()
	defer m.mu.Unlock()
	w := m.keys[key]
	if w == nil {
		return 0, nil
	}
	n := 0
	for _, t := range w.times {
		if t >= start {
			n++
		}
	}
	return int64(n), nil
}

// maxRedisConns bounds one RedisCounter's connection pool: at most this many
// connections are open at once and callers block (bounded by ctx) while all of
// them are in flight. Redis itself is single-threaded, so a handful of
// connections is already enough to keep the round trips pipelined behind the
// admission path while the FD footprint stays small.
const maxRedisConns = 8

// RedisCounter is a minimal RESP client (INCR + PEXPIRE / GET) over TCP.
// No third-party dependency. Failures return errors for callers to fail-open.
//
// Connections are pooled and reused: AUTH/SELECT run once per connection instead
// of once per command, a connection the server closed while it sat idle is
// replaced transparently, and every operation honors ctx cancellation/deadline on
// top of the per-operation timeout. Safe for concurrent use.
type RedisCounter struct {
	addr     string // host:port
	password string
	db       int
	timeout  time.Duration
	// dial is injectable for tests.
	dial func(network, address string, timeout time.Duration) (net.Conn, error)

	// pool is lazily built, so a RedisCounter created as a struct literal (tests)
	// needs no constructor.
	pool redisPool
}

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

// ParseRedisURL parses redis://[:password@]host:port[/db] into RedisCounter fields.
// Empty password/db are ok. redis://localhost:6379/0 is the common form.
func ParseRedisURL(raw string) (addr, password string, db int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", 0, fmt.Errorf("empty redis url")
	}
	// Accept host:port without scheme.
	if !strings.Contains(raw, "://") {
		if !strings.Contains(raw, ":") {
			raw = raw + ":6379"
		}
		return raw, "", 0, nil
	}
	// Very small parser for redis://[:pass@]host:port[/db]
	u := raw
	u = strings.TrimPrefix(u, "redis://")
	u = strings.TrimPrefix(u, "rediss://")
	// password
	if at := strings.LastIndex(u, "@"); at >= 0 {
		userinfo := u[:at]
		u = u[at+1:]
		if strings.HasPrefix(userinfo, ":") {
			password = userinfo[1:]
		} else if i := strings.Index(userinfo, ":"); i >= 0 {
			password = userinfo[i+1:]
		} else {
			password = userinfo
		}
	}
	// db
	if slash := strings.Index(u, "/"); slash >= 0 {
		dbPart := u[slash+1:]
		u = u[:slash]
		if dbPart != "" {
			n, e := strconv.Atoi(strings.Split(dbPart, "?")[0])
			if e != nil {
				return "", "", 0, fmt.Errorf("invalid redis db: %w", e)
			}
			db = n
		}
	}
	if !strings.Contains(u, ":") {
		u = u + ":6379"
	}
	return u, password, db, nil
}

func NewRedisCounter(redisURL string) (*RedisCounter, error) {
	addr, pass, db, err := ParseRedisURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisCounter{
		addr:     addr,
		password: pass,
		db:       db,
		timeout:  800 * time.Millisecond,
		dial:     net.DialTimeout,
	}, nil
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

// replyError is a Redis protocol error reply ("-ERR ..."). It is never retried on
// a fresh connection: the server answered, so the failure is not transport-level.
type replyError struct{ msg string }

func (e *replyError) Error() string { return "redis error: " + e.msg }

func isReplyError(err error) bool {
	var re *replyError
	return errors.As(err, &re)
}

// isCtxError reports a cancellation/deadline outcome, including the case where
// the connection deadline fired just before the ctx timer goroutine did.
func isCtxError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (r *RedisCounter) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	if window <= 0 {
		window = time.Minute
	}
	var count int64
	err := r.withConn(ctx, func(conn net.Conn) error {
		// Fixed-window approximation: INCR + PEXPIRE only when count==1.
		n, err := redisDoInt(conn, "INCR", key)
		if err != nil {
			return err
		}
		count = n
		if n == 1 {
			ms := strconv.FormatInt(window.Milliseconds(), 10)
			if err := redisDo(conn, "PEXPIRE", key, ms); err != nil {
				return err
			}
		}
		return nil
	})
	return count, err
}

func (r *RedisCounter) Decr(ctx context.Context, key string, window time.Duration) (int64, error) {
	_ = window // compensating rollback must not refresh TTL
	return r.rollback(ctx, key, -1)
}

// rollbackScript applies a compensating decrement and drops the key when the
// window total is no longer positive, as one atomic server-side operation.
//
// The rollback used to be a DECR round trip followed by a conditional DEL. The
// server executed them separately, so an INCR that another instance landed in
// between was deleted together with the stale key: the shared window silently
// undercounted and the key was admitted more traffic than its configured limit.
// A script runs as a single command, which is the only way to express
// "decrement, then remove if the total is not positive" without a race.
//
// Dropping a non-positive key is still the right self-heal: a rollback that
// lands after the window expired re-creates the key with no TTL and a negative
// value, and INCR only arms PEXPIRE when its result is exactly 1, so such a key
// would otherwise linger and undercount the next window. A non-positive total
// means no instance holds a live reservation, so nothing is lost.
const rollbackScript = `local total = redis.call('INCRBY', KEYS[1], ARGV[1])
if total <= 0 then
  redis.call('DEL', KEYS[1])
  return 0
end
return total`

// rollback runs rollbackScript for a negative delta and returns the window total
// afterwards, or 0 when the key was dropped.
func (r *RedisCounter) rollback(ctx context.Context, key string, delta int64) (int64, error) {
	if delta >= 0 {
		return 0, fmt.Errorf("sharedcount: rollback delta must be negative, got %d", delta)
	}
	deltaArg := strconv.FormatInt(delta, 10)
	var count int64
	err := r.withConn(ctx, func(conn net.Conn) error {
		n, err := redisDoInt(conn, "EVAL", rollbackScript, "1", key, deltaArg)
		if err != nil {
			if !isScriptUnsupportedError(err) {
				return err
			}
			// The server has no EVAL. Apply the decrement on its own and skip
			// the self-heal: an immortal non-positive key undercounts one
			// window, whereas a non-atomic DEL discards live reservations.
			n, err = redisDoInt(conn, "INCRBY", key, deltaArg)
			if err != nil {
				return err
			}
		}
		count = n
		return nil
	})
	return count, err
}

// isScriptUnsupportedError reports a server that does not implement EVAL, as
// opposed to a script that ran and failed.
func isScriptUnsupportedError(err error) bool {
	var re *replyError
	if !errors.As(err, &re) {
		return false
	}
	msg := strings.ToLower(re.msg)
	return strings.Contains(msg, "unknown command") || strings.Contains(msg, "unsupported command")
}

func (r *RedisCounter) IncrBy(ctx context.Context, key string, delta int64, window time.Duration) (int64, error) {
	if window <= 0 {
		window = time.Minute
	}
	switch {
	case delta == 0:
		// No reservation — read current total without changing TTL.
		return r.Get(ctx, key)
	case delta < 0:
		// Compensating rollback: same atomic self-heal as Decr. It never arms
		// PEXPIRE, so a rollback cannot extend the window it is compensating.
		return r.rollback(ctx, key, delta)
	}
	var count int64
	err := r.withConn(ctx, func(conn net.Conn) error {
		n, err := redisDoInt(conn, "INCRBY", key, strconv.FormatInt(delta, 10))
		if err != nil {
			return err
		}
		count = n
		if n == delta {
			// First positive write in the window (post-increment equals delta
			// ⇒ the key was absent/0): arm the expiry.
			ms := strconv.FormatInt(window.Milliseconds(), 10)
			if err := redisDo(conn, "PEXPIRE", key, ms); err != nil {
				return err
			}
		}
		return nil
	})
	return count, err
}

func (r *RedisCounter) Get(ctx context.Context, key string) (int64, error) {
	var count int64
	err := r.withConn(ctx, func(conn net.Conn) error {
		// GET may return null bulk → 0
		n, err := redisDoIntNullable(conn, "GET", key)
		if err != nil {
			return err
		}
		count = n
		return nil
	})
	return count, err
}

// ---- minimal RESP ----

func redisDo(conn net.Conn, parts ...string) error {
	_, err := redisDoRaw(conn, parts...)
	return err
}

func redisDoInt(conn net.Conn, parts ...string) (int64, error) {
	raw, err := redisDoRaw(conn, parts...)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(raw, 10, 64)
}

func redisDoIntNullable(conn net.Conn, parts ...string) (int64, error) {
	raw, err := redisDoRaw(conn, parts...)
	if err != nil {
		return 0, err
	}
	if raw == "" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

func redisDoRaw(conn net.Conn, parts ...string) (string, error) {
	var b strings.Builder
	b.WriteString("*")
	b.WriteString(strconv.Itoa(len(parts)))
	b.WriteString("\r\n")
	for _, p := range parts {
		b.WriteString("$")
		b.WriteString(strconv.Itoa(len(p)))
		b.WriteString("\r\n")
		b.WriteString(p)
		b.WriteString("\r\n")
	}
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return "", err
	}
	return redisRead(conn)
}

func redisRead(conn net.Conn) (string, error) {
	buf := make([]byte, 0, 256)
	tmp := make([]byte, 256)
	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil && len(buf) == 0 {
			return "", err
		}
		if len(buf) == 0 {
			continue
		}
		switch buf[0] {
		case '+':
			if i := indexCRLF(buf); i >= 0 {
				return string(buf[1:i]), nil
			}
		case '-':
			if i := indexCRLF(buf); i >= 0 {
				return "", &replyError{msg: string(buf[1:i])}
			}
		case ':':
			if i := indexCRLF(buf); i >= 0 {
				return string(buf[1:i]), nil
			}
		case '$':
			// bulk: $<len>\r\n<data>\r\n or hmtBc1\r\n
			if i := indexCRLF(buf); i >= 0 {
				lenStr := string(buf[1:i])
				if lenStr == "-1" {
					return "", nil
				}
				ln, err := strconv.Atoi(lenStr)
				if err != nil {
					return "", err
				}
				start := i + 2
				if len(buf) >= start+ln+2 {
					return string(buf[start : start+ln]), nil
				}
			}
		default:
			return "", fmt.Errorf("unexpected redis reply prefix %q", buf[0])
		}
		if err != nil {
			return "", err
		}
	}
}

func indexCRLF(b []byte) int {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == 13 && b[i+1] == 10 {
			return i
		}
	}
	return -1
}
