package sharedcount

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
)

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
// The script is sent as EVALSHA once the server holds it (see
// rollbackScriptSHA), with a single EVAL fallback on NOSCRIPT.
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

// rollbackScriptSHA is the digest Redis identifies a cached script by, computed
// once per process. Rollbacks sit on the rate-limit-reject and the failure
// compensation paths, so sending EVALSHA (40 hex bytes) instead of the whole
// script body keeps both the upload and the server-side parse off the hot path;
// the body only travels again when a server reports it does not hold the script.
var rollbackScriptSHA = func() string {
	sum := sha1.Sum([]byte(rollbackScript))
	return hex.EncodeToString(sum[:])
}()

// rollback runs rollbackScript for a negative delta and returns the window total
// afterwards, or 0 when the key was dropped.
//
// The script is negotiated per attempt instead of being uploaded every time:
// EVALSHA first, and one EVAL fallback when the server answers NOSCRIPT (which
// both runs the script and re-caches it). No "the server has it" flag is kept:
// such a flag would have to be invalidated on exactly the events that produce
// NOSCRIPT — a restart, SCRIPT FLUSH, a failover to a replica — and the fallback
// already costs one extra round trip only when it is genuinely needed.
func (r *RedisCounter) rollback(ctx context.Context, key string, delta int64) (int64, error) {
	if delta >= 0 {
		return 0, fmt.Errorf("sharedcount: rollback delta must be negative, got %d", delta)
	}
	deltaArg := strconv.FormatInt(delta, 10)
	var count int64
	err := r.withConn(ctx, func(conn net.Conn) error {
		n, err := redisDoInt(conn, "EVALSHA", rollbackScriptSHA, "1", key, deltaArg)
		kind := classifyScriptRefusal(err)
		if kind == scriptRefusalNotCached {
			// The server does not hold the digest: send the body once, which runs
			// it and puts it back in the server-side cache.
			n, err = redisDoInt(conn, "EVAL", rollbackScript, "1", key, deltaArg)
			kind = classifyScriptRefusal(err)
		}
		if err == nil {
			count = n
			return nil
		}
		if kind != scriptRefusalPermission && kind != scriptRefusalUnsupported {
			// Anything else — a transport failure, or a script the server ran and
			// that then errored — is a real error. Degrading on it would hide a
			// broken script or a broken connection behind a weaker rollback.
			return err
		}
		// This connection may not run scripts. Apply the decrement on its own and
		// skip the self-heal: an immortal non-positive key undercounts one window,
		// whereas a non-atomic DEL discards live reservations.
		r.warnScriptRefused(kind, err)
		n, incrErr := redisDoInt(conn, "INCRBY", key, deltaArg)
		if incrErr != nil {
			return incrErr
		}
		count = n
		return nil
	})
	return count, err
}

// scriptRefusal says why a scripting command (EVALSHA / EVAL) did not run. Only
// these shapes are recognised; anything else is a transport failure or a script
// that ran and errored, and has to reach the caller as an error.
type scriptRefusal int

const (
	// scriptRefusalNone means the failure says nothing about script support.
	scriptRefusalNone scriptRefusal = iota
	// scriptRefusalNotCached: "-NOSCRIPT No matching script" — the server does
	// not hold this digest (restart, SCRIPT FLUSH, a replica that never saw it).
	// Recoverable by sending the body with EVAL.
	scriptRefusalNotCached
	// scriptRefusalPermission: "-NOPERM this user has no permissions to run the
	// 'evalsha' command" — the ACL user may not run scripts. Scripting exists,
	// this connection may not use it: degrade and tell the operator what to fix.
	scriptRefusalPermission
	// scriptRefusalUnsupported: "-ERR unknown command 'EVALSHA'", or a
	// compatible server's "unsupported command" — the build has no scripting at
	// all: degrade.
	scriptRefusalUnsupported
)

// classifyScriptRefusal maps a Redis error reply to a scriptRefusal. NOSCRIPT and
// NOPERM are matched on the leading error code, which the protocol fixes, rather
// than on the prose that follows it. Only the "no scripting in this build" case
// has to match wording, because Redis reports an unknown command as a plain
// "-ERR ..." with no code of its own.
func classifyScriptRefusal(err error) scriptRefusal {
	var re *replyError
	if !errors.As(err, &re) {
		return scriptRefusalNone
	}
	msg := strings.ToLower(strings.TrimSpace(re.msg))
	code, _, _ := strings.Cut(msg, " ")
	switch {
	case code == "noscript":
		return scriptRefusalNotCached
	case code == "noperm":
		return scriptRefusalPermission
	case strings.Contains(msg, "unknown command"), strings.Contains(msg, "unsupported command"):
		return scriptRefusalUnsupported
	}
	return scriptRefusalNone
}

// warnScriptRefused logs the degradation once per counter, naming the two
// refusals differently because only one of them is fixable from this side of the
// connection: a permission refusal means an ACL to correct, while a build
// without scripting is simply what the server is.
func (r *RedisCounter) warnScriptRefused(kind scriptRefusal, err error) {
	r.scriptRefusalWarned.Do(func() {
		switch kind {
		case scriptRefusalPermission:
			slog.Warn("sharedcount: Redis refused scripting for this connection (ACL); compensating rollbacks degrade to INCRBY and skip the atomic self-heal — grant eval/evalsha to this user, or accept that a rollback landing after its window leaves a non-positive key behind for one window",
				"error", err)
		case scriptRefusalUnsupported:
			slog.Warn("sharedcount: Redis server runs no scripting at all; compensating rollbacks degrade to INCRBY and skip the atomic self-heal",
				"error", err)
		}
	})
}
