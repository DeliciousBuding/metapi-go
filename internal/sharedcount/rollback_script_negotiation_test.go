package sharedcount

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// A compensating rollback runs on the reject and failure-compensation paths, so
// the script negotiation has to be paid once and not per rollback: EVALSHA
// carries a 40-byte digest instead of the script body, and only a NOSCRIPT
// answer (a server that restarted, flushed its script cache, or a replica that
// never saw the script) justifies uploading the body again.
//
// Capability detection matters for the same reason: an ACL that forbids scripts
// answers NOPERM, which is neither "the digest is unknown" nor "the connection
// broke". Treating it as a transport error fails every rollback and leaves the
// shared window counting high; treating it as "no scripting" degrades to a plain
// INCRBY and says so in the log.

// wantRollbackScriptSHA derives the digest the RESP scripting protocol defines
// for rollbackScript — SHA1 of the exact bytes an EVAL would upload — without
// reading it back from the client.
func wantRollbackScriptSHA() string {
	sum := sha1.Sum([]byte(rollbackScript))
	return hex.EncodeToString(sum[:])
}

// seedReservations puts n live reservations on key so a rollback has something
// to compensate.
func seedReservations(t *testing.T, c *RedisCounter, key string, n int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		if _, err := c.Incr(ctx, key, time.Minute); err != nil {
			t.Fatalf("seed Incr %d: %v", i, err)
		}
	}
}

// TestRedisCounter_RollbackNegotiatesScriptByDigest pins the hot path: one
// command, sent as EVALSHA with the digest of rollbackScript, and the script
// body never travels.
func TestRedisCounter_RollbackNegotiatesScriptByDigest(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	f.preloadScript(rollbackScript) // a server that already holds the script
	c := newCounterAt(t, f.addr())
	ctx := context.Background()

	seedReservations(t, c, "rpm:negotiate", 2)
	f.resetCommandLog()

	n, err := c.Decr(ctx, "rpm:negotiate", time.Minute)
	if err != nil {
		t.Fatalf("Decr: %v", err)
	}
	if n != 1 {
		t.Fatalf("Decr = %d, want 1", n)
	}

	cmds := f.recordedCommands()
	if len(cmds) != 1 {
		t.Fatalf("rollback sent %d commands %v, want exactly one", len(cmds), cmds)
	}
	if !strings.EqualFold(cmds[0][0], "EVALSHA") {
		t.Fatalf("rollback sent %v, want EVALSHA <sha> 1 <key> <delta>", cmds[0])
	}
	if got, want := cmds[0][1], wantRollbackScriptSHA(); got != want {
		t.Fatalf("EVALSHA digest = %q, want %q (the digest of the script the fallback would upload)", got, want)
	}
	for _, arg := range cmds[0] {
		if strings.Contains(arg, "redis.call") {
			t.Fatalf("the script body travelled on the hot path: %v", cmds[0])
		}
	}
}

// TestRedisCounter_RollbackFallsBackToEvalOnceOnNoScript pins the cold path: a
// NOSCRIPT answer reloads the script with exactly one EVAL, the rollback still
// succeeds atomically, and the next rollback runs on the digest alone.
func TestRedisCounter_RollbackFallsBackToEvalOnceOnNoScript(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close() // the double starts with an empty script cache
	c := newCounterAt(t, f.addr())
	ctx := context.Background()

	seedReservations(t, c, "rpm:cold", 3)
	f.resetCommandLog()

	n, err := c.Decr(ctx, "rpm:cold", time.Minute)
	if err != nil {
		t.Fatalf("Decr on a server without the script cached: %v", err)
	}
	if n != 2 {
		t.Fatalf("Decr = %d, want 2", n)
	}
	if got := f.countCommands("EVALSHA"); got != 1 {
		t.Fatalf("EVALSHA calls = %d, want 1 (the digest is tried first). wire=%v", got, f.recordedCommands())
	}
	if got := f.countCommands("EVAL"); got != 1 {
		t.Fatalf("EVAL calls = %d, want exactly one reload. wire=%v", got, f.recordedCommands())
	}
	if got := f.countCommands("INCRBY"); got != 0 {
		t.Fatalf("INCRBY calls = %d, want 0: NOSCRIPT is not a reason to drop the atomic self-heal. wire=%v", got, f.recordedCommands())
	}

	// The reload stuck: the next rollback is a single EVALSHA again.
	f.resetCommandLog()
	if n, err := c.Decr(ctx, "rpm:cold", time.Minute); err != nil || n != 1 {
		t.Fatalf("second Decr = %d, %v, want 1", n, err)
	}
	if got := f.countCommands("EVAL"); got != 0 {
		t.Fatalf("EVAL calls on the second rollback = %d, want 0 (the body was uploaded again). wire=%v", got, f.recordedCommands())
	}
	if got := f.countCommands("EVALSHA"); got != 1 {
		t.Fatalf("EVALSHA calls on the second rollback = %d, want 1. wire=%v", got, f.recordedCommands())
	}
}

// TestRedisCounter_RollbackDegradesWhenACLRefusesScripting covers the refusal
// that is a permission, not a missing capability: the rollback must still apply
// its decrement (fail-closed on the count, never a surfaced error), skip the
// self-heal it can no longer do atomically, and say in the log that an ACL is
// what needs fixing.
//
// Not parallel: it captures the process-wide slog default to read the WARN.
func TestRedisCounter_RollbackDegradesWhenACLRefusesScripting(t *testing.T) {
	f := newFakeRedis(t)
	defer f.close()
	f.denyScripts()
	c := newCounterAt(t, f.addr())
	ctx := context.Background()

	var logs bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })

	seedReservations(t, c, "rpm:acl", 2)
	f.resetCommandLog()

	n, err := c.Decr(ctx, "rpm:acl", time.Minute)
	if err != nil {
		t.Fatalf("Decr = %v, want nil: an ACL refusal must degrade, not fail the rollback", err)
	}
	if n != 1 {
		t.Fatalf("Decr = %d, want 1 (the decrement must still be applied)", n)
	}
	if got := f.countCommands("INCRBY"); got != 1 {
		t.Fatalf("INCRBY calls = %d, want 1 (the degraded path). wire=%v", got, f.recordedCommands())
	}
	if got, err := c.Get(ctx, "rpm:acl"); err != nil || got != 1 {
		t.Fatalf("window total = %d, %v, want 1", got, err)
	}

	logged := logs.String()
	if !strings.Contains(logged, "WARN") {
		t.Fatalf("no WARN logged for a permission refusal; log = %q", logged)
	}
	if !strings.Contains(logged, "ACL") && !strings.Contains(strings.ToLower(logged), "permission") {
		t.Fatalf("WARN does not say the refusal is a permission/ACL problem an operator can fix; log = %q", logged)
	}
}

// TestRedisCounter_RollbackWithoutScriptSupportStillDegrades is the sibling
// refusal: a build with no scripting at all takes the same degraded path.
func TestRedisCounter_RollbackWithoutScriptSupportStillDegrades(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	f.disableEval()
	c := newCounterAt(t, f.addr())
	ctx := context.Background()

	seedReservations(t, c, "rpm:noscripting", 2)
	f.resetCommandLog()

	n, err := c.Decr(ctx, "rpm:noscripting", time.Minute)
	if err != nil {
		t.Fatalf("Decr without scripting support: %v", err)
	}
	if n != 1 {
		t.Fatalf("Decr = %d, want 1", n)
	}
	if got := f.countCommands("INCRBY"); got != 1 {
		t.Fatalf("INCRBY calls = %d, want 1 (the degraded path). wire=%v", got, f.recordedCommands())
	}
}

// TestRedisCounter_RollbackSurfacesScriptRuntimeError guards the other
// direction: a script the server accepted and then failed to run is a real
// error. Degrading on it would hide a broken script behind a silently weaker
// rollback.
func TestRedisCounter_RollbackSurfacesScriptRuntimeError(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	f.failScripts("ERR Error running script (call to f_9d1c0): @user_script:2: user_script:2: attempt to perform arithmetic on a nil value")
	c := newCounterAt(t, f.addr())
	ctx := context.Background()

	if _, err := c.Decr(ctx, "rpm:crash", time.Minute); err == nil {
		t.Fatal("Decr = nil error, want the script failure surfaced")
	}
	if got := f.countCommands("INCRBY"); got != 0 {
		t.Fatalf("INCRBY calls = %d, want 0: a script that ran and failed must not degrade silently", got)
	}
}

// TestRedisCounter_RollbackSurfacesTransportFailure is the same guard for a
// connection that cannot be established: no capability may be inferred from it.
func TestRedisCounter_RollbackSurfacesTransportFailure(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	addr := f.addr()
	f.close() // nothing listens any more

	c := newCounterAt(t, addr)
	if _, err := c.Decr(context.Background(), "rpm:down", time.Minute); err == nil {
		t.Fatal("Decr against a dead server = nil error, want the transport failure surfaced")
	}
}
