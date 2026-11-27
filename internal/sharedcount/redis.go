package sharedcount

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

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

	// scriptRefusalWarned keeps the "scripting refused" WARN to one line per
	// counter. Rollbacks run on the reject and compensation paths, so a
	// misconfigured ACL would otherwise log on every refused request.
	scriptRefusalWarned sync.Once

	// pool is lazily built, so a RedisCounter created as a struct literal (tests)
	// needs no constructor.
	pool redisPool
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
