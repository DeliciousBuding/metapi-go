package sharedcount

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---- fake RESP Redis server ----

// fakeRedis is a minimal in-memory RESP server for exercising RedisCounter
// over a real TCP loopback connection (so NewRedisCounter + net.DialTimeout
// are covered end-to-end). It supports AUTH, SELECT, INCR, DECR, INCRBY, GET
// and PEXPIRE with real TTL expiry.
type fakeRedis struct {
	mu       sync.Mutex
	data     map[string]int64
	ttl      map[string]time.Time
	authPass string
	ln       net.Listener
	wg       sync.WaitGroup

	// conns tracks live connections: a pooling client parks idle connections, so
	// the handler goroutines must be unblocked explicitly on shutdown.
	connsMu sync.Mutex
	conns   map[net.Conn]struct{}
	peak    int

	// auths/selects count handshake commands (asserted once per connection).
	auths   atomic.Int64
	selects atomic.Int64

	// delayMs is an artificial per-command latency (0 = none).
	delayMs atomic.Int64
}

func newFakeRedis(tb testing.TB) *fakeRedis {
	tb.Helper()
	return newFakeRedisWithAuth(tb, "")
}

func newFakeRedisWithAuth(tb testing.TB, password string) *fakeRedis {
	tb.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("fake redis listen: %v", err)
	}
	f := &fakeRedis{
		data:     make(map[string]int64),
		ttl:      make(map[string]time.Time),
		authPass: password,
		ln:       ln,
	}
	f.wg.Add(1)
	go f.serve()
	return f
}

func (f *fakeRedis) addr() string { return f.ln.Addr().String() }

func (f *fakeRedis) close() {
	_ = f.ln.Close()
	f.closeConns()
	f.wg.Wait()
}

// setDelay makes every command sleep d before it is handled.
func (f *fakeRedis) setDelay(d time.Duration) {
	f.delayMs.Store(d.Milliseconds())
}

// closeConns closes every live connection but keeps the listener accepting, i.e.
// it simulates a server (or an idle-timeout/firewall) dropping connections that
// a pooling client still believes are usable.
func (f *fakeRedis) closeConns() {
	f.connsMu.Lock()
	conns := make([]net.Conn, 0, len(f.conns))
	for c := range f.conns {
		conns = append(conns, c)
	}
	f.connsMu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

// peakConns returns the highest number of simultaneously live connections.
func (f *fakeRedis) peakConns() int {
	f.connsMu.Lock()
	defer f.connsMu.Unlock()
	return f.peak
}

func (f *fakeRedis) track(c net.Conn) {
	f.connsMu.Lock()
	defer f.connsMu.Unlock()
	if f.conns == nil {
		f.conns = make(map[net.Conn]struct{})
	}
	f.conns[c] = struct{}{}
	if len(f.conns) > f.peak {
		f.peak = len(f.conns)
	}
}

func (f *fakeRedis) untrack(c net.Conn) {
	f.connsMu.Lock()
	defer f.connsMu.Unlock()
	delete(f.conns, c)
}

func (f *fakeRedis) serve() {
	defer f.wg.Done()
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			// Listener gone: unblock handlers parked on idle pooled connections.
			f.closeConns()
			return
		}
		f.track(conn)
		f.wg.Add(1)
		go func(c net.Conn) {
			defer f.wg.Done()
			defer f.untrack(c)
			f.handle(c)
		}(conn)
	}
}

// expireLocked evicts a key whose TTL has elapsed. Caller must hold f.mu.
func (f *fakeRedis) expireLocked(key string) {
	if t, ok := f.ttl[key]; ok && time.Now().After(t) {
		delete(f.data, key)
		delete(f.ttl, key)
	}
}

func (f *fakeRedis) handle(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	authed := f.authPass == ""
	for {
		parts, err := readRESPArray(reader)
		if err != nil {
			return
		}
		if len(parts) == 0 {
			continue
		}
		if d := f.delayMs.Load(); d > 0 {
			time.Sleep(time.Duration(d) * time.Millisecond)
		}
		cmd := strings.ToUpper(parts[0])
		switch cmd {
		case "AUTH":
			f.auths.Add(1)
			if f.authPass != "" && (len(parts) < 2 || parts[1] != f.authPass) {
				writeError(conn, "ERR invalid password")
				return
			}
			authed = true
			writeOK(conn)
		case "SELECT":
			f.selects.Add(1)
			writeOK(conn)
		case "PING":
			writeOK(conn)
		default:
			if !authed {
				writeError(conn, "NOAUTH Authentication required")
				return
			}
			if !f.handleCommand(conn, cmd, parts) {
				return
			}
		}
	}
}

// handleCommand returns false to signal the connection should close.
func (f *fakeRedis) handleCommand(conn net.Conn, cmd string, parts []string) bool {
	switch cmd {
	case "INCR":
		if len(parts) < 2 {
			writeError(conn, "ERR wrong number of arguments")
			return true
		}
		key := parts[1]
		f.mu.Lock()
		f.expireLocked(key)
		v := f.data[key] + 1
		f.data[key] = v
		f.mu.Unlock()
		writeInt(conn, v)
	case "DECR":
		if len(parts) < 2 {
			writeError(conn, "ERR wrong number of arguments")
			return true
		}
		key := parts[1]
		f.mu.Lock()
		f.expireLocked(key)
		v := f.data[key] - 1
		f.data[key] = v
		f.mu.Unlock()
		writeInt(conn, v)
	case "INCRBY":
		if len(parts) < 3 {
			writeError(conn, "ERR wrong number of arguments")
			return true
		}
		key := parts[1]
		delta, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			writeError(conn, "ERR value is not an integer")
			return true
		}
		f.mu.Lock()
		f.expireLocked(key)
		v := f.data[key] + delta
		f.data[key] = v
		f.mu.Unlock()
		writeInt(conn, v)
	case "GET":
		if len(parts) < 2 {
			writeError(conn, "ERR wrong number of arguments")
			return true
		}
		key := parts[1]
		f.mu.Lock()
		f.expireLocked(key)
		v, ok := f.data[key]
		f.mu.Unlock()
		if !ok {
			writeNullBulk(conn)
		} else {
			writeBulk(conn, strconv.FormatInt(v, 10))
		}
	case "PEXPIRE":
		if len(parts) < 3 {
			writeError(conn, "ERR wrong number of arguments")
			return true
		}
		key := parts[1]
		ms, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			writeError(conn, "ERR value is not an integer")
			return true
		}
		f.mu.Lock()
		f.ttl[key] = time.Now().Add(time.Duration(ms) * time.Millisecond)
		f.mu.Unlock()
		writeInt(conn, 1)
	case "QUIT":
		writeOK(conn)
		return false
	default:
		writeError(conn, "ERR unknown command '"+cmd+"'")
	}
	return true
}

// ---- RESP read/write helpers ----

// readRESPArray reads one RESP array of bulk strings from a buffered reader.
func readRESPArray(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		if err == io.EOF && line == "" {
			return nil, err
		}
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return nil, nil
	}
	if line[0] != '*' {
		// Inline command (rare from this client) — split on whitespace.
		return strings.Fields(line), nil
	}
	n, err := strconv.Atoi(line[1:])
	if err != nil {
		return nil, fmt.Errorf("bad array header: %w", err)
	}
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hdr, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		hdr = strings.TrimRight(hdr, "\r\n")
		if len(hdr) == 0 || hdr[0] != '$' {
			return nil, fmt.Errorf("expected bulk header, got %q", hdr)
		}
		ln, err := strconv.Atoi(hdr[1:])
		if err != nil {
			return nil, fmt.Errorf("bad bulk length: %w", err)
		}
		if ln < 0 {
			parts = append(parts, "")
			continue
		}
		body := make([]byte, ln+2)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, err
		}
		parts = append(parts, string(body[:ln]))
	}
	return parts, nil
}

func writeOK(conn net.Conn) {
	_, _ = conn.Write([]byte("+OK\r\n"))
}

func writeError(conn net.Conn, msg string) {
	_, _ = fmt.Fprintf(conn, "-%s\r\n", msg)
}

func writeInt(conn net.Conn, n int64) {
	_, _ = fmt.Fprintf(conn, ":%d\r\n", n)
}

func writeBulk(conn net.Conn, s string) {
	_, _ = fmt.Fprintf(conn, "$%d\r\n%s\r\n", len(s), s)
}

func writeNullBulk(conn net.Conn) {
	_, _ = conn.Write([]byte("$-1\r\n"))
}

// ---- test helpers ----

// newCounterAt constructs a RedisCounter pointing at the fake redis address.
func newCounterAt(t *testing.T, addr string) *RedisCounter {
	t.Helper()
	c, err := NewRedisCounter("redis://" + addr + "/0")
	if err != nil {
		t.Fatalf("NewRedisCounter: %v", err)
	}
	c.timeout = 2 * time.Second
	return c
}

func newCounterWithAuth(t *testing.T, addr, password string) *RedisCounter {
	t.Helper()
	url := "redis://:" + password + "@" + addr + "/0"
	c, err := NewRedisCounter(url)
	if err != nil {
		t.Fatalf("NewRedisCounter: %v", err)
	}
	c.timeout = 2 * time.Second
	return c
}

// ---- RedisCounter round-trip tests ----

func TestRedisCounter_IncrAndDecr(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())

	ctx := context.Background()
	n, err := c.Incr(ctx, "rpm", time.Minute)
	if err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if n != 1 {
		t.Fatalf("first Incr = %d, want 1", n)
	}
	n, err = c.Incr(ctx, "rpm", time.Minute)
	if err != nil || n != 2 {
		t.Fatalf("second Incr = %d, %v, want 2", n, err)
	}
	n, err = c.Decr(ctx, "rpm", time.Minute)
	if err != nil || n != 1 {
		t.Fatalf("Decr = %d, %v, want 1", n, err)
	}
	// Decr must not refresh TTL — but functionally the count still drops.
	n, err = c.Decr(ctx, "rpm", time.Minute)
	if err != nil || n != 0 {
		t.Fatalf("Decr2 = %d, %v, want 0", n, err)
	}
}

func TestRedisCounter_IncrByAndPeek(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())

	ctx := context.Background()
	n, err := c.IncrBy(ctx, "tpm", 500, time.Minute)
	if err != nil || n != 500 {
		t.Fatalf("first IncrBy = %d, %v, want 500", n, err)
	}
	n, err = c.IncrBy(ctx, "tpm", 250, time.Minute)
	if err != nil || n != 750 {
		t.Fatalf("second IncrBy = %d, %v, want 750", n, err)
	}
	// Zero delta is a peek (no reservation, no TTL refresh).
	n, err = c.IncrBy(ctx, "tpm", 0, time.Minute)
	if err != nil || n != 750 {
		t.Fatalf("peek = %d, %v, want 750", n, err)
	}
	// Negative delta rolls back without refreshing TTL.
	n, err = c.IncrBy(ctx, "tpm", -300, time.Minute)
	if err != nil || n != 450 {
		t.Fatalf("rollback = %d, %v, want 450", n, err)
	}
}

func TestRedisCounter_GetMissingReturnsZero(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())

	n, err := c.Get(context.Background(), "never-set")
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if n != 0 {
		t.Fatalf("Get missing = %d, want 0", n)
	}
}

func TestRedisCounter_IncrSetsTTL(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	ctx := context.Background()

	// First Incr on a fresh key sets the TTL (PEXPIRE fires when count==1).
	if _, err := c.Incr(ctx, "ttl-key", 50*time.Millisecond); err != nil {
		t.Fatalf("Incr: %v", err)
	}
	// Key exists immediately after.
	n, err := c.Get(ctx, "ttl-key")
	if err != nil || n != 1 {
		t.Fatalf("Get before expiry = %d, %v, want 1", n, err)
	}
	// After TTL elapses the fake redis evicts it.
	time.Sleep(90 * time.Millisecond)
	n, err = c.Get(ctx, "ttl-key")
	if err != nil || n != 0 {
		t.Fatalf("Get after TTL = %d, %v, want 0", n, err)
	}
}

// ---- Redis failure paths ----

func TestRedisCounter_DialFailure(t *testing.T) {
	t.Parallel()
	// Inject a dial that always errors — deterministic, no network.
	c := &RedisCounter{
		addr:    "unreachable:1",
		timeout: 200 * time.Millisecond,
		dial: func(network, address string, timeout time.Duration) (net.Conn, error) {
			return nil, errors.New("dial boom")
		},
	}
	_, err := c.Incr(context.Background(), "k", time.Minute)
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
	_, err = c.Decr(context.Background(), "k", time.Minute)
	if err == nil {
		t.Fatal("expected dial error on Decr, got nil")
	}
	_, err = c.IncrBy(context.Background(), "k", 5, time.Minute)
	if err == nil {
		t.Fatal("expected dial error on IncrBy, got nil")
	}
	_, err = c.Get(context.Background(), "k")
	if err == nil {
		t.Fatal("expected dial error on Get, got nil")
	}
}

func TestRedisCounter_AuthFailure(t *testing.T) {
	t.Parallel()
	f := newFakeRedisWithAuth(t, "real-secret")
	defer f.close()
	// Dial with the wrong password — fake redis rejects AUTH and closes.
	c := newCounterWithAuth(t, f.addr(), "wrong-pass")
	c.timeout = 500 * time.Millisecond
	_, err := c.Incr(context.Background(), "k", time.Minute)
	if err == nil {
		t.Fatal("expected AUTH error, got nil")
	}
}

func TestRedisCounter_ServerErrorReply(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	// Peek on a missing key succeeds (returns 0) — GET returns null bulk.
	_, err := c.IncrBy(context.Background(), "k", 0, time.Minute)
	if err != nil {
		t.Fatalf("peek missing should succeed: %v", err)
	}
}

// ---- default window & edge cases ----

func TestRedisCounter_ZeroOrNegativeWindowDefaultsToMinute(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	ctx := context.Background()
	// window <= 0 must not crash; the implementation falls back to a minute.
	n, err := c.Incr(ctx, "win", 0)
	if err != nil || n != 1 {
		t.Fatalf("Incr window=0 = %d, %v", n, err)
	}
	n, err = c.IncrBy(ctx, "win-by", 10, -1)
	if err != nil || n != 10 {
		t.Fatalf("IncrBy window=-1 = %d, %v", n, err)
	}
}

func TestRedisCounter_LargeDeltasDoNotOverflow(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	ctx := context.Background()
	big := int64(2_000_000_000)
	n, err := c.IncrBy(ctx, "big", big, time.Minute)
	if err != nil || n != big {
		t.Fatalf("big IncrBy = %d, %v, want %d", n, err, big)
	}
	n, err = c.IncrBy(ctx, "big", big, time.Minute)
	if err != nil || n != big*2 {
		t.Fatalf("second big IncrBy = %d, %v, want %d", n, err, big*2)
	}
}

// ---- MemoryCounter edge cases not covered by memory_test.go ----

func TestMemoryCounter_GetMissingReturnsZero(t *testing.T) {
	t.Parallel()
	m := NewMemoryCounter()
	n, err := m.Get(context.Background(), "nope")
	if err != nil || n != 0 {
		t.Fatalf("Get missing = %d, %v, want 0", n, err)
	}
}

func TestMemoryCounter_ZeroOrNegativeWindowDefaultsToMinute(t *testing.T) {
	t.Parallel()
	m := NewMemoryCounter()
	base := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	now := base
	m.now = func() time.Time { return now }
	n, err := m.Incr(context.Background(), "k", 0)
	if err != nil || n != 1 {
		t.Fatalf("Incr window=0 = %d, %v", n, err)
	}
	n, err = m.Incr(context.Background(), "k", -5)
	if err != nil || n != 2 {
		t.Fatalf("Incr window=-5 = %d, %v", n, err)
	}
}

func TestMemoryCounter_IncrByOverrollbackFloorsAtZero(t *testing.T) {
	t.Parallel()
	m := NewMemoryCounter()
	// Over-rollback beyond zero resets the window and floors at 0.
	n, err := m.IncrBy(context.Background(), "tpm", -10_000, time.Minute)
	if err != nil || n != 0 {
		t.Fatalf("over-rollback = %d, %v, want 0", n, err)
	}
}

// ---- concurrency safety ----

func TestMemoryCounter_ConcurrentIncrNoRace(t *testing.T) {
	t.Parallel()
	m := NewMemoryCounter()
	const goroutines = 64
	const perGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	ctx := context.Background()
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < perGoroutine; j++ {
				_, _ = m.Incr(ctx, "race-key", time.Minute)
			}
		}()
	}
	close(start)
	wg.Wait()
	n, err := m.Get(ctx, "race-key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := int64(goroutines * perGoroutine)
	if n != want {
		t.Fatalf("concurrent Incr total = %d, want %d", n, want)
	}
}

func TestMemoryCounter_ConcurrentIncrByNetZero(t *testing.T) {
	t.Parallel()
	m := NewMemoryCounter()
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	ctx := context.Background()
	start := make(chan struct{})
	// Half increment, half decrement — net should be ~0 (floored at 0).
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 100; j++ {
				_, _ = m.IncrBy(ctx, "tpm", 10, time.Minute)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 100; j++ {
				_, _ = m.IncrBy(ctx, "tpm", -10, time.Minute)
			}
		}()
	}
	close(start)
	wg.Wait()
	n, err := m.IncrBy(ctx, "tpm", 0, time.Minute)
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if n < 0 {
		t.Fatalf("net IncrBy = %d, must not be negative", n)
	}
}

func TestRedisCounter_ConcurrentIncrAccurate(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())
	const goroutines = 32
	const perGoroutine = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var total atomic.Int64
	ctx := context.Background()
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < perGoroutine; j++ {
				n, err := c.Incr(ctx, "concurrent", time.Minute)
				if err != nil {
					t.Errorf("concurrent Incr: %v", err)
					return
				}
				total.Add(n - n) // no-op to use n; correctness checked below
			}
		}()
	}
	close(start)
	wg.Wait()
	n, err := c.Get(ctx, "concurrent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	want := int64(goroutines * perGoroutine)
	if n != want {
		t.Fatalf("concurrent Incr total = %d, want %d", n, want)
	}
}

// ---- ParseRedisURL extra coverage ----

func TestParseRedisURL_Empty(t *testing.T) {
	t.Parallel()
	if _, _, _, err := ParseRedisURL(""); err == nil {
		t.Fatal("expected error for empty url")
	}
	if _, _, _, err := ParseRedisURL("   "); err == nil {
		t.Fatal("expected error for whitespace-only url")
	}
}

func TestParseRedisURL_HostOnlyAppendsPort(t *testing.T) {
	t.Parallel()
	addr, pass, db, err := ParseRedisURL("redis.example.com")
	if err != nil {
		t.Fatalf("ParseRedisURL host-only: %v", err)
	}
	if addr != "redis.example.com:6379" {
		t.Fatalf("addr = %q, want redis.example.com:6379", addr)
	}
	if pass != "" || db != 0 {
		t.Fatalf("pass=%q db=%d, want empty/0", pass, db)
	}
}

func TestParseRedisURL_HostPortNoScheme(t *testing.T) {
	t.Parallel()
	addr, pass, db, err := ParseRedisURL("10.0.0.5:6380")
	if err != nil {
		t.Fatalf("ParseRedisURL host:port: %v", err)
	}
	if addr != "10.0.0.5:6380" || pass != "" || db != 0 {
		t.Fatalf("addr=%q pass=%q db=%d", addr, pass, db)
	}
}

func TestParseRedisURL_UserWithoutPassword(t *testing.T) {
	t.Parallel()
	// "redis://user@host:6379" — user with no colon means userinfo is the password.
	addr, _, _, err := ParseRedisURL("redis://user@localhost:6379")
	if err != nil {
		t.Fatalf("ParseRedisURL user@host: %v", err)
	}
	if addr != "localhost:6379" {
		t.Fatalf("addr = %q, want localhost:6379", addr)
	}
}

func TestParseRedisURL_InvalidDB(t *testing.T) {
	t.Parallel()
	if _, _, _, err := ParseRedisURL("redis://localhost:6379/abc"); err == nil {
		t.Fatal("expected error for non-numeric db")
	}
}

func TestParseRedisURL_RedissScheme(t *testing.T) {
	t.Parallel()
	addr, pass, db, err := ParseRedisURL("rediss://:secret@db.example:6390/3")
	if err != nil {
		t.Fatalf("ParseRedisURL rediss://: %v", err)
	}
	if addr != "db.example:6390" || pass != "secret" || db != 3 {
		t.Fatalf("addr=%q pass=%q db=%d", addr, pass, db)
	}
}

func TestNewRedisCounter_BuildsFields(t *testing.T) {
	t.Parallel()
	c, err := NewRedisCounter("redis://:pw@127.0.0.1:6390/4")
	if err != nil {
		t.Fatalf("NewRedisCounter: %v", err)
	}
	if c.addr != "127.0.0.1:6390" {
		t.Fatalf("addr = %q", c.addr)
	}
	if c.password != "pw" {
		t.Fatalf("password = %q", c.password)
	}
	if c.db != 4 {
		t.Fatalf("db = %d", c.db)
	}
	if c.timeout != 800*time.Millisecond {
		t.Fatalf("timeout = %v, want 800ms", c.timeout)
	}
	if c.dial == nil {
		t.Fatal("dial should default to net.DialTimeout")
	}
}
