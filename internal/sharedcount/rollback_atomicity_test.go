package sharedcount

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// A compensating rollback is not a private operation: while one instance rolls a
// reservation back, other instances keep reserving against the same window key.
// These tests pin that a rollback can never discard somebody else's reservation,
// and that the self-heal (dropping a key whose total is no longer positive)
// survives on servers with and without scripting support.

// rollbackDuringReservation drives one rollback while a second counter — a
// separate connection pool, i.e. a second instance — reserves the same key at
// the moment the rollback is about to decide whether the window key is dropped:
// the test double runs the callback after parsing that command and before
// handling it, so the concurrent write is already on the server when the
// rollback executes. It returns the window total observed afterwards.
func rollbackDuringReservation(t *testing.T, rollback func(ctx context.Context, c *RedisCounter) error) (total int64, interleaved bool) {
	t.Helper()

	f := newFakeRedis(t)
	defer f.close()
	// Warm the server-side script cache: the interleave below has to land on the
	// command that actually executes the rollback, not on a NOSCRIPT rejection of
	// a cold-start EVALSHA (that negotiation is covered by
	// TestRedisCounter_RollbackFallsBackToEvalOnceOnNoScript).
	f.preloadScript(rollbackScript)
	c := newCounterAt(t, f.addr())
	other := newCounterAt(t, f.addr())
	ctx := context.Background()

	const key = "rpm:shared"
	// One live reservation, which the rollback below compensates.
	if _, err := c.Incr(ctx, key, time.Minute); err != nil {
		t.Fatalf("seed Incr: %v", err)
	}

	var fired, reserved atomic.Int64
	f.setBeforeCommand(func(cmd, gotKey string) {
		// The interesting command is the one that may drop the key: the atomic
		// rollback script (by body or by digest), or — for a two-command
		// rollback — its separate DEL.
		if gotKey != key || (cmd != "EVAL" && cmd != "EVALSHA" && cmd != "DEL") || fired.Swap(1) == 1 {
			return
		}
		// Another instance reserves the same window at that exact moment. A
		// rollback that already decremented and now deletes in a second round
		// trip removes this reservation together with the stale key; a single
		// atomic command counts it instead of discarding it.
		if _, err := other.Incr(ctx, key, time.Minute); err != nil {
			t.Errorf("concurrent Incr: %v", err)
			return
		}
		reserved.Store(1)
	})

	if err := rollback(ctx, c); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if fired.Load() == 0 {
		t.Fatal("the rollback never reached the test double, so nothing was interleaved")
	}
	if reserved.Load() == 0 {
		t.Fatal("the concurrent reservation did not complete before the rollback answered")
	}

	total, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get after rollback: %v", err)
	}
	return total, true
}

func TestRedisCounter_DecrDoesNotDiscardConcurrentReservation(t *testing.T) {
	t.Parallel()
	total, _ := rollbackDuringReservation(t, func(ctx context.Context, c *RedisCounter) error {
		_, err := c.Decr(ctx, "rpm:shared", time.Minute)
		return err
	})
	if total != 1 {
		t.Fatalf("window total = %d, want 1: the reservation made during the rollback was discarded", total)
	}
}

func TestRedisCounter_NegativeIncrByDoesNotDiscardConcurrentReservation(t *testing.T) {
	t.Parallel()
	total, _ := rollbackDuringReservation(t, func(ctx context.Context, c *RedisCounter) error {
		_, err := c.IncrBy(ctx, "rpm:shared", -1, time.Minute)
		return err
	})
	if total != 1 {
		t.Fatalf("window total = %d, want 1: the reservation made during the rollback was discarded", total)
	}
}

// TestRedisCounter_RollbackIsASingleServerOperation pins the mechanism, not just
// the outcome: the decrement and the conditional delete must reach the server as
// one command, otherwise the guarantees above depend on timing again.
func TestRedisCounter_RollbackIsASingleServerOperation(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	// A server that already holds the script: this pins the hot path. A cold
	// server legitimately costs one extra NOSCRIPT round trip, which
	// TestRedisCounter_RollbackFallsBackToEvalOnceOnNoScript covers.
	f.preloadScript(rollbackScript)
	c := newCounterAt(t, f.addr())
	ctx := context.Background()

	var commands atomic.Int64
	f.setBeforeCommand(func(cmd, key string) {
		if key == "rpm:single" {
			commands.Add(1)
		}
	})

	if _, err := c.Decr(ctx, "rpm:single", time.Minute); err != nil {
		t.Fatalf("Decr on missing key: %v", err)
	}
	if got := commands.Load(); got != 1 {
		t.Fatalf("rollback issued %d server commands, want 1 (a multi-command rollback reopens the race)", got)
	}
}

// TestRedisCounter_RollbackWithoutScriptSupport covers a server that answers
// EVAL the way a build without scripting does. The decrement must still be
// applied and must not surface as an error; the self-heal is skipped rather than
// deleting a key non-atomically.
func TestRedisCounter_RollbackWithoutScriptSupport(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	f.disableEval()
	c := newCounterAt(t, f.addr())
	ctx := context.Background()

	if _, err := c.Incr(ctx, "rpm:live", time.Minute); err != nil {
		t.Fatalf("Incr 1: %v", err)
	}
	if _, err := c.Incr(ctx, "rpm:live", time.Minute); err != nil {
		t.Fatalf("Incr 2: %v", err)
	}

	n, err := c.Decr(ctx, "rpm:live", time.Minute)
	if err != nil {
		t.Fatalf("Decr without EVAL support: %v", err)
	}
	if n != 1 {
		t.Fatalf("Decr = %d, want 1", n)
	}
	if got, err := c.Get(ctx, "rpm:live"); err != nil || got != 1 {
		t.Fatalf("Get = %d, %v, want 1", got, err)
	}

	// The TPM path takes the same branch.
	if _, err := c.IncrBy(ctx, "tpm:live", 500, time.Minute); err != nil {
		t.Fatalf("IncrBy(+500): %v", err)
	}
	if n, err := c.IncrBy(ctx, "tpm:live", -200, time.Minute); err != nil || n != 300 {
		t.Fatalf("IncrBy(-200) = %d, %v, want 300", n, err)
	}
}

// TestRedisCounter_RollbackRejectsPositiveDelta guards the internal contract: a
// positive delta must go through Incr/IncrBy so the window TTL is armed.
func TestRedisCounter_RollbackRejectsPositiveDelta(t *testing.T) {
	t.Parallel()
	f := newFakeRedis(t)
	defer f.close()
	c := newCounterAt(t, f.addr())

	if _, err := c.rollback(context.Background(), "rpm:bad", 1); err == nil {
		t.Fatal("rollback with a positive delta = nil error, want a contract error")
	}
}
