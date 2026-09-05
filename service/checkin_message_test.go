package service

import "testing"

// The vocabulary moved here from three private copies (the check-in runner, the
// structured failure classifier and IsUnsupportedCheckinRuntimeHealth), so this
// table is the one place that decides what "this site has no check-in" means.
// The localized cases are the observed failure: a real New API upstream answers
// 签到功能未启用 from its DoCheckin handler, and before this change none of the
// three lists recognized it — the account was written as checkin failed /
// unknown_error / runtimeHealth=unhealthy with an error-level event even though
// its relay worked.
func TestIsUnsupportedCheckinMessage(t *testing.T) {
	cases := []struct {
		message string
		want    bool
	}{
		// Endpoint absent.
		{"invalid url (POST /api/user/checkin)", true},
		{"HTTP 404 /api/user/checkin not found", true},
		{"checkin endpoint not found", true},
		{"签到端点不存在", true},
		// Feature present but not offered / not enabled.
		{"check-in is not supported", true},
		{"checkin is not supported", true},
		{"this site does not support checkin", true},
		{"does not support checkin feature", true},
		{"Check-in is not supported by Sub2API", true},
		{"站点不支持签到", true},
		// New API: the hardcoded answer plus its three locale catalogs.
		{"签到功能未启用", true},
		{"簽到功能未啟用", true},
		{"Check-in feature is not enabled", true},
		// Real failures must stay failures.
		{"", false},
		{"checkin success", false},
		{"normal error message", false},
		{"签到失败，请稍后重试", false},
		{"签到失败：更新额度出错", false},
		{"Check-in failed, please try again later", false},
		// A different non-failure family, owned by its own classifier.
		{"今日已签到", false},
		{"Already checked in today", false},
		// Credential failures must never be softened into "unsupported".
		{"token expired", false},
		{"401 Unauthorized", false},
	}
	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			if got := IsUnsupportedCheckinMessage(tc.message); got != tc.want {
				t.Fatalf("IsUnsupportedCheckinMessage(%q) = %v, want %v", tc.message, got, tc.want)
			}
		})
	}
}

// The health reader keeps its own state/source rule and only borrows the
// vocabulary, so an entry written before the source was recorded is still
// recognized by its reason.
func TestIsUnsupportedCheckinRuntimeHealthReasonFallbackSharesVocabulary(t *testing.T) {
	localized := &RuntimeHealthEntry{
		State:  HealthDegraded,
		Reason: "签到功能未启用",
		Source: HealthSourceBalance,
	}
	if !IsUnsupportedCheckinRuntimeHealth(localized) {
		t.Fatal("a degraded entry whose reason is the upstream's localized 'check-in not enabled' must be recognized as unsupported")
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
}
