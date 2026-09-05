package service

import "testing"

// The health reader keeps its own state/source rule and only borrows the
// vocabulary from platform, so an entry written before the source field existed
// is still recognized by its reason — and cannot drift from what the check-in
// runner and the failure classifier decide.
func TestIsUnsupportedCheckinRuntimeHealthReasonFallbackSharesVocabulary(t *testing.T) {
	localized := &RuntimeHealthEntry{
		State:  HealthDegraded,
		Reason: "签到功能未启用",
		Source: HealthSourceBalance,
	}
	if !IsUnsupportedCheckinRuntimeHealth(localized) {
		t.Fatal("a degraded entry whose reason is the upstream's localized 'check-in not enabled' must be recognized as unsupported")
	}

	adapterDefault := &RuntimeHealthEntry{
		State:  HealthDegraded,
		Reason: "checkin endpoint not supported for openai",
		Source: HealthSourceBalance,
	}
	if !IsUnsupportedCheckinRuntimeHealth(adapterDefault) {
		t.Fatal("a degraded entry quoting an adapter's own unsupported answer must be recognized as unsupported")
	}

	realFailure := &RuntimeHealthEntry{
		State:  HealthDegraded,
		Reason: "签到失败，请稍后重试",
		Source: HealthSourceBalance,
	}
	if IsUnsupportedCheckinRuntimeHealth(realFailure) {
		t.Fatal("a genuine check-in failure was classified as unsupported; the state would be preserved instead of re-evaluated")
	}

	wrongState := &RuntimeHealthEntry{
		State:  HealthUnhealthy,
		Reason: "签到功能未启用",
		Source: HealthSourceBalance,
	}
	if IsUnsupportedCheckinRuntimeHealth(wrongState) {
		t.Fatal("only a degraded entry may be preserved as unsupported check-in")
	}

	bySource := &RuntimeHealthEntry{
		State:  HealthDegraded,
		Reason: "site does not support check-in endpoint",
		Source: HealthSourceCheckin,
	}
	if !IsUnsupportedCheckinRuntimeHealth(bySource) {
		t.Fatal("a degraded entry written by the check-in runner is unsupported by source, whatever the reason text")
	}
}
