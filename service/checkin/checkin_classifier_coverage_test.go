package checkin

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/platform"
)

// The runner and the structured failure classifier used to keep separate lists
// for the same question, and they disagreed: 11 of the 16 "already checked in"
// wordings the runner accepted were persisted as code=unknown_error, so
// checkin_logs said "unknown" about a state the product had already named in the
// same breath (status=skipped, health=healthy). Both now read one vocabulary;
// this test is the drift-proof — whatever the vocabulary accepts, the classifier
// must name with the matching code.
func TestClassifyFailureReasonNamesEveryWordingTheVocabularyAccepts(t *testing.T) {
	cases := []struct {
		message string
		want    FailureReasonCode
	}{
		// Unsupported / no check-in here.
		{"invalid url (POST /api/user/checkin)", CodeCheckinNotSupported},
		{"HTTP 404: /api/user/checkin not found", CodeCheckinNotSupported},
		{"checkin endpoint not found", CodeCheckinNotSupported},
		{"签到端点不存在", CodeCheckinNotSupported},
		{"check-in is not supported", CodeCheckinNotSupported},
		{"Check-in is not supported by Sub2API", CodeCheckinNotSupported},
		{"does not support checkin", CodeCheckinNotSupported},
		{"not support checkin", CodeCheckinNotSupported},
		{"站点不支持签到", CodeCheckinNotSupported},
		{"unsupported checkin endpoint", CodeCheckinNotSupported},
		// New API with the feature switched off (the #1267 observed failure).
		{"签到功能未启用", CodeCheckinNotSupported},
		{"簽到功能未啟用", CodeCheckinNotSupported},
		{"Check-in feature is not enabled", CodeCheckinNotSupported},
		// Our own adapters' defaults (the six-platform hole).
		{platform.DefaultCheckinUnsupportedMessage, CodeCheckinNotSupported},
		{"checkin endpoint not supported for openai", CodeCheckinNotSupported},
		{"codex oauth connections do not support checkin", CodeCheckinNotSupported},
		{"grok oauth connections do not support checkin", CodeCheckinNotSupported},
		{"CLIProxyAPI does not support checkin", CodeCheckinNotSupported},
		// Already checked in today.
		{"already checked in today", CodeAlreadyCheckedIn},
		{"already signed in for today", CodeAlreadyCheckedIn},
		{"already sign in detected", CodeAlreadyCheckedIn},
		{"already claimed today", CodeAlreadyCheckedIn},
		{"claimed today's reward", CodeAlreadyCheckedIn},
		{"今日已签到", CodeAlreadyCheckedIn},
		{"今天已签到", CodeAlreadyCheckedIn},
		{"今日已经签到", CodeAlreadyCheckedIn},
		{"已签到", CodeAlreadyCheckedIn},
		{"重复签到", CodeAlreadyCheckedIn},
		{"已经领取", CodeAlreadyCheckedIn},
		{"已领取", CodeAlreadyCheckedIn},
		{"领取过", CodeAlreadyCheckedIn},
		{"签到达", CodeAlreadyCheckedIn},
		// Manual verification.
		{"Turnstile token 为空", CodeManualTurnstileRequired},
		{"Turnstile 校验失败，请刷新重试！", CodeManualTurnstileRequired},
		{"turnstile token is required", CodeManualTurnstileRequired},
		// The one wording the two old copies disagreed about: the classifier
		// accepted it, the runner did not, so the account went unhealthy while
		// the log said "manual verification required".
		{"manual verification needed: turnstile", CodeManualTurnstileRequired},
	}
	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			got := ClassifyFailureReason(ClassifyFailureInput{Message: tc.message, Status: "failed"})
			if got.Code != tc.want {
				t.Fatalf("ClassifyFailureReason(%q).Code = %q, want %q", tc.message, got.Code, tc.want)
			}
			if got.Code == CodeUnknownError {
				t.Fatalf("ClassifyFailureReason(%q) fell through to unknown_error", tc.message)
			}
		})
	}
}

// Real failures must keep failing: softening one of these into "unsupported" or
// "already done" would hide a broken account, which is the failure mode this
// family has produced twice.
func TestClassifyFailureReasonKeepsRealFailures(t *testing.T) {
	cases := []struct {
		message string
		notWant FailureReasonCode
	}{
		{"签到失败，请稍后重试", CodeCheckinNotSupported},
		{"签到失败：更新额度出错", CodeCheckinNotSupported},
		{"Check-in failed, please try again later", CodeCheckinNotSupported},
		{"签到失败，请稍后重试", CodeAlreadyCheckedIn},
		{"token expired", CodeCheckinNotSupported},
		{"401 Unauthorized", CodeCheckinNotSupported},
		{"turnstile", CodeManualTurnstileRequired},
	}
	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			got := ClassifyFailureReason(ClassifyFailureInput{Message: tc.message, Status: "failed"})
			if got.Code == tc.notWant {
				t.Fatalf("ClassifyFailureReason(%q).Code = %q; a real failure must not be softened", tc.message, got.Code)
			}
		})
	}
	if got := ClassifyFailureReason(ClassifyFailureInput{Message: "token expired", Status: "failed"}); got.Code != CodeTokenExpired {
		t.Fatalf("token expired classified as %q, want token_expired", got.Code)
	}
}
