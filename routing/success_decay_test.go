package routing

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// =============================================================================
// FailCount decay on success — recovered channels are not over-penalized

// RecordSuccess must halve accumulated failCount instead of leaving it
// untouched, so that a recovered channel's next failure cools with Fibonacci
// backoff relative to its decayed (recent) failure memory, not its historical
// peak. Decay keeps a small memory for repeat offenders while avoiding
// "punished for ancient history" cooldowns.
// =============================================================================

func newDecayFixture(t *testing.T, failCount int64) (*isolationDB, *TokenRouter) {
	t.Helper()
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	ch := isolationChannel(501, 1, 5001)
	ch.FailCount = failCount
	acc := isolationAccount(5001, 50)
	db.seedChannel(ch, acc, route)
	return db, newIsolationRouter(db)
}

// cooldownDurationMs extracts the cooldown duration (cooldownUntil - beforeMs)
// written by the most recent UpdateChannelCooldownFields call.
func cooldownDurationMs(t *testing.T, db *isolationDB, beforeMs int64) int64 {
	t.Helper()
	updates := db.lastCooldownUpdates
	if updates == nil {
		t.Fatal("expected cooldown update, got none")
	}
	raw, ok := updates["cooldownUntil"]
	if !ok {
		t.Fatal("cooldown update missing cooldownUntil")
	}
	cooldownUntil, ok := raw.(*string)
	if !ok || cooldownUntil == nil {
		t.Fatalf("cooldownUntil has unexpected type %T", raw)
	}
	untilMs := ParseISOTimeMs(cooldownUntil)
	if untilMs == nil {
		t.Fatalf("cannot parse cooldownUntil %q", *cooldownUntil)
	}
	return *untilMs - beforeMs
}

// TestRecordSuccess_DecaysFailCount proves one success writes failCount/2 and
// repeated successes keep halving (geometric decay toward zero), while the
// four reset fields still clear.
func TestRecordSuccess_DecaysFailCount(t *testing.T) {
	db, tr := newDecayFixture(t, 10)
	ctx := context.Background()

	if err := tr.RecordSuccess(ctx, 501, 100, 0.001, nil, nil); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	if db.successCalls != 1 {
		t.Fatalf("expected 1 success write, got %d", db.successCalls)
	}
	if v, ok := db.lastSuccessUpdates["failCount"]; !ok || v != int64(5) {
		t.Fatalf("success write failCount = %v (present=%v), want 5", v, ok)
	}
	if v, ok := db.lastSuccessUpdates["consecutiveFailCount"]; !ok || v != int64(0) {
		t.Fatalf("success write consecutiveFailCount = %v (present=%v), want 0", v, ok)
	}
	if raw, exists := db.lastSuccessUpdates["cooldownUntil"]; !exists || raw != nil {
		t.Fatalf("success write cooldownUntil = %v (present=%v), want nil", raw, exists)
	}
	if got := db.getChannel(501); got == nil || got.FailCount != 5 {
		t.Fatalf("channel failCount after success = %v, want 5", got)
	}

	// Geometric decay: 10 → 5 → 2 → 1 → 0.
	for _, want := range []int64{2, 1, 0} {
		if err := tr.RecordSuccess(ctx, 501, 100, 0.001, nil, nil); err != nil {
			t.Fatalf("RecordSuccess (want %d): %v", want, err)
		}
		if got := db.getChannel(501); got == nil || got.FailCount != want {
			t.Fatalf("channel failCount = %v, want %d", got, want)
		}
	}
}

// TestRecordSuccess_DecayShortensNextFailureCooldown proves the end-to-end
// behavior: N accumulated failures → one success → next failure cools far less
// than without the success. failCount=10 + failure = 11 → 15·fib(11)=1335s;
// with an intervening success failCount decays to 5, next failure = 6 →
// 15·fib(6)=120s.
func TestRecordSuccess_DecayShortensNextFailureCooldown(t *testing.T) {
	failCtx := func() SiteRuntimeFailureContext {
		status := 500
		errText := "upstream error"
		model := "gpt-test"
		return SiteRuntimeFailureContext{Status: &status, ErrorText: &errText, ModelName: &model}
	}

	// Control: no success between failures — cooldown based on failCount 11.
	controlDB, controlRouter := newDecayFixture(t, 10)
	beforeMs := time.Now().UnixMilli()
	if err := controlRouter.RecordFailure(context.Background(), 501, failCtx(), nil); err != nil {
		t.Fatalf("control RecordFailure: %v", err)
	}
	controlMs := cooldownDurationMs(t, controlDB, beforeMs)
	if controlMs < 1_330_000 || controlMs > 1_340_000 {
		t.Fatalf("control cooldown = %dms, want ≈ 15·fib(11)s = 1335000ms", controlMs)
	}

	// Decayed: one success before the failure — cooldown based on failCount 6.
	decayedDB, decayedRouter := newDecayFixture(t, 10)
	if err := decayedRouter.RecordSuccess(context.Background(), 501, 100, 0.001, nil, nil); err != nil {
		t.Fatalf("decayed RecordSuccess: %v", err)
	}
	beforeMs = time.Now().UnixMilli()
	if err := decayedRouter.RecordFailure(context.Background(), 501, failCtx(), nil); err != nil {
		t.Fatalf("decayed RecordFailure: %v", err)
	}
	decayedMs := cooldownDurationMs(t, decayedDB, beforeMs)
	if decayedMs < 115_000 || decayedMs > 125_000 {
		t.Fatalf("decayed cooldown = %dms, want ≈ 15·fib(6)s = 120000ms", decayedMs)
	}

	if decayedMs >= controlMs {
		t.Fatalf("decayed cooldown %dms not shorter than control %dms", decayedMs, controlMs)
	}
	if got := decayedDB.getChannel(501); got == nil || got.FailCount != 6 {
		t.Fatalf("channel failCount after decay+failure = %v, want 6", got)
	}
}

// TestRecordSuccess_DecaysMemberFailCount proves the OAuth route-unit member
// path also halves member failCount on success instead of leaving it stale.
func TestRecordSuccess_DecaysMemberFailCount(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	unitID := int64(7)
	ch := isolationChannel(301, 1, 3001)
	ch.OAuthRouteUnitID = &unitID
	acc := isolationAccount(3001, 30)
	db.seedChannel(ch, acc, route)

	unit := store.OAuthRouteUnit{
		ID: unitID, SiteID: 30, Provider: "codex", Name: "unit", Strategy: "round_robin", Enabled: true,
	}
	db.seedMember(store.OAuthRouteUnitMember{ID: 1, UnitID: unitID, AccountID: 3001, FailCount: 10}, acc, unit)

	tr := newIsolationRouter(db)
	if err := tr.RecordSuccess(context.Background(), 301, 100, 0.001, nil, nil); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	if db.memberCalls != 1 {
		t.Fatalf("expected 1 member success write, got %d", db.memberCalls)
	}
	if db.lastMemberID != 1 {
		t.Fatalf("expected member 1 updated, got %d", db.lastMemberID)
	}
	if v, ok := db.lastMemberUpdates["failCount"]; !ok || v != int64(5) {
		t.Fatalf("member success write failCount = %v (present=%v), want 5", v, ok)
	}
	// Outer OAuth channel never accumulated failCount; success keeps it clean.
	if got := db.getChannel(301); got == nil || got.FailCount != 0 {
		t.Fatalf("outer oauth channel failCount mutated: %v, want 0", got)
	}
}
