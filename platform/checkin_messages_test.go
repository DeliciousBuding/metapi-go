package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// One owner for the check-in wordings. The tables below came from three private
// copies (the check-in runner, the structured failure classifier and the
// runtime-health reader) plus the New API adapter's own; every case they pinned
// is pinned here, and the localized ones are the observed failures:
//   - a real New API upstream with check-in switched off answers 签到功能未启用,
//     which none of the copies recognized (#1267);
//   - StandardAdapter's default answer was in no copy at all, so six registered
//     platforms reported unknown_error on every check-in cycle.
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
		// Our own adapters' defaults (the six-platform hole).
		{DefaultCheckinUnsupportedMessage, true},
		{"checkin endpoint not supported for openai", true},
		{"codex oauth connections do not support checkin", true},
		{"CLIProxyAPI does not support checkin", true},
		// Heritage wording kept for health entries written before the source field.
		{"unsupported checkin endpoint", true},
		// Real failures must stay failures.
		{"", false},
		{"checkin success", false},
		{"normal error message", false},
		{"签到失败，请稍后重试", false},
		{"签到失败：更新额度出错", false},
		{"Check-in failed, please try again later", false},
		{"今日已签到", false},
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

// The narrow predicate is what the New API cookie retry ladder asks: an absent
// route may be a disguised session failure and is worth one more probe, while
// "this site does not offer check-in" is already a complete answer.
func TestIsCheckinEndpointAbsent(t *testing.T) {
	absent := []string{
		"invalid url (POST /api/user/checkin)",
		"HTTP 404: /api/user/checkin not found",
		"checkin endpoint not found",
		"签到端点不存在",
	}
	for _, msg := range absent {
		if !IsCheckinEndpointAbsent(msg) {
			t.Errorf("IsCheckinEndpointAbsent(%q) = false, want true", msg)
		}
		if !IsUnsupportedCheckinMessage(msg) {
			t.Errorf("IsUnsupportedCheckinMessage(%q) = false, want true (endpoint-absent is a subset)", msg)
		}
	}
	notAbsent := []string{
		"签到功能未启用",
		"Check-in is not supported by Sub2API",
		DefaultCheckinUnsupportedMessage,
		"HTTP 404: /api/user/models not found", // a 404 for another route proves nothing
		"",
		"normal error",
	}
	for _, msg := range notAbsent {
		if IsCheckinEndpointAbsent(msg) {
			t.Errorf("IsCheckinEndpointAbsent(%q) = true, want false", msg)
		}
	}
}

func TestIsAlreadyCheckedInMessage(t *testing.T) {
	positive := []string{
		// English
		"already checked in today",
		"You have already checked in",
		"already signed in for today",
		"already sign in detected",
		"already claimed today",
		"reward already claimed",
		"claimed today's reward",
		// Chinese (New API answers 今日已签到)
		"今日已签到",
		"今天已签到",
		"今天已经签到",
		"今日已经签到",
		"已经签到",
		"已签到",
		"重复签到",
		"签到达",
		"今日已领取",
		"已经领取奖励",
		"领取过今日奖励",
		// Case insensitive
		"Already Checked In",
		"ALREADY SIGNED",
		"ALREADY CLAIMED",
	}
	for _, msg := range positive {
		t.Run(msg, func(t *testing.T) {
			if !IsAlreadyCheckedInMessage(msg) {
				t.Errorf("expected true for: %q", msg)
			}
		})
	}
	negative := []string{"", " ", "checkin success", "checkin failed", "ok", "something else entirely", "签到功能未启用"}
	for _, msg := range negative {
		t.Run(msg, func(t *testing.T) {
			if IsAlreadyCheckedInMessage(msg) {
				t.Errorf("expected false for: %q", msg)
			}
		})
	}
}

func TestIsManualVerificationRequiredMessage(t *testing.T) {
	positive := []string{
		"Turnstile token 为空",
		"turnstile 校验失败",
		"Turnstile 验证码错误",
		"turnstile token is required",
		"manual verification needed: turnstile",
	}
	for _, msg := range positive {
		t.Run(msg, func(t *testing.T) {
			if !IsManualVerificationRequiredMessage(msg) {
				t.Errorf("expected true for: %q", msg)
			}
		})
	}
	negative := []string{"", "normal error", "turnstile without additional keyword", "turnstile"}
	for _, msg := range negative {
		t.Run(msg, func(t *testing.T) {
			if IsManualVerificationRequiredMessage(msg) {
				t.Errorf("expected false for: %q", msg)
			}
		})
	}
}

// Every registered platform must answer a check-in attempt with a sentence this
// package recognizes. This is the gate that was missing while six platforms
// answered with StandardAdapter's default wording and were classified as
// unknown_error downstream.
func TestEveryAdapterCheckinAnswerIsRecognized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"success":false,"message":"invalid url (POST /api/user/checkin)"}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	adapters := ListAdapters()
	if len(adapters) < 10 {
		t.Fatalf("ListAdapters() = %d adapters, want the whole registry (a short list would make this gate vacuous)", len(adapters))
	}
	for _, a := range adapters {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		res, err := a.Checkin(ctx, srv.URL, "probe-token", nil, nil)
		cancel()
		msg := ""
		if res != nil {
			msg = res.Message
		}
		if err != nil {
			if msg == "" {
				msg = err.Error()
			} else {
				msg += " | " + err.Error()
			}
		}
		if msg == "" {
			t.Errorf("%s: empty check-in answer; an empty message classifies as unknown downstream", a.PlatformName())
			continue
		}
		recognized := IsUnsupportedCheckinMessage(msg) ||
			IsAlreadyCheckedInMessage(msg) ||
			IsManualVerificationRequiredMessage(msg) ||
			IsCloudflareChallengeMessage(msg) ||
			IsTokenExpiredError(0, msg) ||
			ClassifyUpstreamError(0, msg) != ClassUnknown
		if !recognized {
			t.Errorf("%s: check-in answer %q is recognized by no classifier; it would be persisted as unknown_error", a.PlatformName(), msg)
		}
	}
}
