package checkin

import (
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/platform"
)

// ---- isAlreadyCheckedInMessage Tests (12 patterns) ----

func TestIsAlreadyCheckedInMessage_Positive(t *testing.T) {
	positiveCases := []string{
		// English: "already checked in", "already signed", "already sign in"
		"already checked in today",
		"You have already checked in",
		"already signed in for today",
		"already sign in detected",
		// Chinese: "今日已签到", "今天已签到", "今天已经签到", "今日已经签到", "已经签到", "已签到", "重复签到", "签到达"
		"今日已签到",
		"今天已签到",
		"今天已经签到",
		"今日已经签到",
		"已经签到",
		"已签到",
		"重复签到",
		"签到达",
		// Claimed: "already claimed", "claimed today", "已领取", "领取过"
		"already claimed today",
		"reward already claimed",
		"claimed today's reward",
		"今日已领取",
		"已经领取奖励",
		"领取过今日奖励",
		// Case insensitive
		"Already Checked In",
		"ALREADY SIGNED",
		"ALREADY CLAIMED",
	}
	for _, msg := range positiveCases {
		t.Run(msg, func(t *testing.T) {
			if !isAlreadyCheckedInMessage(msg) {
				t.Errorf("expected true for: %q", msg)
			}
		})
	}
}

func TestIsAlreadyCheckedInMessage_Negative(t *testing.T) {
	negativeCases := []string{
		"",
		" ",
		"checkin success",
		"checkin failed",
		"ok",
		"something else entirely",
	}
	for _, msg := range negativeCases {
		t.Run(msg, func(t *testing.T) {
			if isAlreadyCheckedInMessage(msg) {
				t.Errorf("expected false for: %q", msg)
			}
		})
	}
}

// ---- isUnsupportedCheckinMessage Tests (7 patterns) ----

func TestIsUnsupportedCheckinMessage_Positive(t *testing.T) {
	positiveCases := []string{
		"invalid url (POST /api/user/checkin)",
		"HTTP 404 /api/user/checkin not found",
		"checkin endpoint not found",
		"check-in is not supported",
		"checkin is not supported",
		"this site does not support checkin",
		"does not support checkin feature",
	}
	for _, msg := range positiveCases {
		t.Run(msg, func(t *testing.T) {
			if !isUnsupportedCheckinMessage(msg) {
				t.Errorf("expected true for: %q", msg)
			}
		})
	}
}

func TestIsUnsupportedCheckinMessage_Negative(t *testing.T) {
	negativeCases := []string{
		"",
		"checkin success",
		"normal error message",
	}
	for _, msg := range negativeCases {
		t.Run(msg, func(t *testing.T) {
			if isUnsupportedCheckinMessage(msg) {
				t.Errorf("expected false for: %q", msg)
			}
		})
	}
}

// ---- isManualVerificationRequiredMessage Tests ----

func TestIsManualVerificationRequiredMessage_Positive(t *testing.T) {
	positiveCases := []string{
		"Turnstile token 为空",
		"turnstile 校验失败",
		"Turnstile 验证码错误",
		"turnstile token is required",
	}
	for _, msg := range positiveCases {
		t.Run(msg, func(t *testing.T) {
			if !isManualVerificationRequiredMessage(msg) {
				t.Errorf("expected true for: %q", msg)
			}
		})
	}
}

func TestIsManualVerificationRequiredMessage_Negative(t *testing.T) {
	negativeCases := []string{
		"",
		"normal error",
		"turnstile without additional keyword", // no token/校验/验证
	}
	for _, msg := range negativeCases {
		t.Run(msg, func(t *testing.T) {
			if isManualVerificationRequiredMessage(msg) {
				t.Errorf("expected false for: %q", msg)
			}
		})
	}
	// "turnstile" alone should be false since it needs token/校验/验证
	if isManualVerificationRequiredMessage("turnstile") {
		t.Error("expected false for 'turnstile' alone (needs token/校验/验证)")
	}
}

// ---- shouldAttemptAutoRelogin Tests ----

func TestShouldAttemptAutoRelogin_Checkin(t *testing.T) {
	positiveCases := []string{
		"jwt expired",
		"token expired",
		"invalid access token",
		"new-api-user header required",
		"access token is missing",
	}
	for _, msg := range positiveCases {
		t.Run(msg, func(t *testing.T) {
			if !shouldAttemptAutoRelogin(msg) {
				t.Errorf("expected true for: %q", msg)
			}
		})
	}

	negativeCases := []string{
		"",
		"some random error",
		"connection timeout",
	}
	for _, msg := range negativeCases {
		t.Run(msg, func(t *testing.T) {
			if shouldAttemptAutoRelogin(msg) {
				t.Errorf("expected false for: %q", msg)
			}
		})
	}
}

// ---- IsSiteDisabled Tests ----

func TestIsSiteDisabled(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"disabled", true},
		{" disabled ", true},
		{"active", false},
		{"", false},   // empty → "active"
		{"  ", false}, // whitespace → "active"
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := IsSiteDisabled(tt.status)
			if got != tt.want {
				t.Errorf("IsSiteDisabled(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

// ---- orUsername Tests ----

func TestOrUsername(t *testing.T) {
	name := "testuser"
	empty := ""
	whitespace := "   "

	t.Run("valid username", func(t *testing.T) {
		got := orUsername(&name, 123)
		if got != "testuser" {
			t.Errorf("expected 'testuser', got %q", got)
		}
	})

	t.Run("nil username", func(t *testing.T) {
		got := orUsername(nil, 456)
		if got != "ID:456" {
			t.Errorf("expected 'ID:456', got %q", got)
		}
	})

	t.Run("empty username", func(t *testing.T) {
		got := orUsername(&empty, 789)
		if got != "ID:789" {
			t.Errorf("expected 'ID:789', got %q", got)
		}
	})

	t.Run("whitespace username", func(t *testing.T) {
		got := orUsername(&whitespace, 999)
		if got != "ID:999" {
			t.Errorf("expected 'ID:999', got %q", got)
		}
	})
}

// ---- CheckinOptions validation ----

func TestCheckinOptions_Default(t *testing.T) {
	var opts *CheckinOptions
	// nil options: SkipEvent=false, ScheduleMode=""
	if opts != nil {
		t.Error("nil options should be nil")
	}
}

func TestCheckinOptions_WithScheduleMode(t *testing.T) {
	opts := &CheckinOptions{
		SkipEvent:    true,
		ScheduleMode: "interval",
	}
	if !opts.SkipEvent {
		t.Error("SkipEvent should be true")
	}
	if opts.ScheduleMode != "interval" {
		t.Errorf("ScheduleMode should be 'interval', got %q", opts.ScheduleMode)
	}
}

// ---- CheckinResult construction ----

func TestCheckinResult_Success(t *testing.T) {
	r := CheckinResult{
		Success: true,
		Status:  CheckinSuccess,
		Skipped: false,
		Reason:  "",
		Message: "checkin success",
		Reward:  "100",
	}
	if !r.Success {
		t.Error("success result should have Success=true")
	}
	if r.Status != CheckinSuccess {
		t.Errorf("status should be success, got %s", r.Status)
	}
}

func TestCheckinResult_Skipped(t *testing.T) {
	r := CheckinResult{
		Success: true,
		Status:  CheckinSkipped,
		Skipped: true,
		Reason:  "site_disabled",
		Message: "site disabled",
	}
	if !r.Skipped {
		t.Error("skipped result should have Skipped=true")
	}
	if r.Status != CheckinSkipped {
		t.Errorf("status should be skipped, got %s", r.Status)
	}
}

func TestCheckinResult_Failed(t *testing.T) {
	r := CheckinResult{
		Success: false,
		Status:  CheckinFailed,
		Skipped: false,
		Message: "connection timeout",
	}
	if r.Success {
		t.Error("failed result should have Success=false")
	}
	if r.Status != CheckinFailed {
		t.Errorf("status should be failed, got %s", r.Status)
	}
}

// ---- CheckinAllResult construction ----

func TestCheckinAllResult(t *testing.T) {
	r := CheckinAllResult{
		AccountID: 1,
		Username:  "testuser",
		Site:      "example.com",
		Result:    CheckinResult{Success: true, Status: CheckinSuccess, Reward: "50"},
	}
	if r.AccountID != 1 {
		t.Errorf("AccountID should be 1, got %d", r.AccountID)
	}
	if r.Username != "testuser" {
		t.Errorf("Username should be 'testuser', got %q", r.Username)
	}
	if r.Site != "example.com" {
		t.Errorf("Site should be 'example.com', got %q", r.Site)
	}
	if !r.Result.Success {
		t.Error("result should be successful")
	}
}

// ---- CheckinExecutionStatus constants ----

func TestCheckinExecutionStatus_Constants(t *testing.T) {
	if CheckinSuccess != "success" {
		t.Errorf("CheckinSuccess should be 'success', got %q", CheckinSuccess)
	}
	if CheckinFailed != "failed" {
		t.Errorf("CheckinFailed should be 'failed', got %q", CheckinFailed)
	}
	if CheckinSkipped != "skipped" {
		t.Errorf("CheckinSkipped should be 'skipped', got %q", CheckinSkipped)
	}
}

// ---- shouldRetryTransient Tests (issue #668: transient-retry classification decision) ----
//
// These tests pin the retry-decision predicate used by CheckinAccount (step
// 6.5). A failure is retried once ONLY when it classifies as ClassTransient
// (rate-limit / timeout / 5xx / network). Auth/billing/model/validation/state
// failures must NOT retry — they require operator intervention.

func TestShouldRetryTransient_TransientMessagesRetry(t *testing.T) {
	// Realistic adapter-produced failure messages that the platform
	// classifier maps to ClassTransient (httpStatus=0, message-driven).
	transientMessages := []string{
		"HTTP 429: rate limit exceeded",
		"HTTP 429: too many requests",
		"HTTP 503: service unavailable",
		"HTTP 502: bad gateway",
		"HTTP 504: gateway timeout",
		"request: connection refused",
		"request: connection reset by peer",
		"rate limit exceeded",
		"request timed out",
		"too many requests",
	}
	for _, msg := range transientMessages {
		t.Run(msg, func(t *testing.T) {
			// Sanity-check the underlying classifier so the retry decision
			// and the platform classification agree.
			if platform.ClassifyUpstreamError(0, msg) != platform.ClassTransient {
				t.Fatalf("classifier returned %s for %q, want ClassTransient",
					platform.ClassifyUpstreamError(0, msg), msg)
			}
			result := &platform.CheckinResult{Success: false, Message: msg}
			if !shouldRetryTransient(result) {
				t.Errorf("shouldRetryTransient(%q) = false, want true", msg)
			}
		})
	}
}

func TestShouldRetryTransient_NonTransientMessagesNoRetry(t *testing.T) {
	// Failures that must NOT trigger the transient retry path.
	nonTransientCases := []struct {
		msg   string
		label string
	}{
		{"jwt expired", "expired"},
		{"invalid access token", "expired"},
		{"access token is invalid", "expired"},
		{"HTTP 401: unauthorized", "auth"},
		{"insufficient quota", "billing"},
		{"no payment method", "billing"},
		{"model not supported", "model"},
		{"invalid_argument", "validation"},
		{"already checked in", "state"},
		{"turnstile token 为空", "verification"},
		{"some unknown opaque error", "unknown"},
	}
	for _, tt := range nonTransientCases {
		t.Run(tt.label, func(t *testing.T) {
			if platform.ClassifyUpstreamError(0, tt.msg) == platform.ClassTransient {
				t.Fatalf("classifier returned ClassTransient for %q (%s); pick a non-transient fixture",
					tt.msg, tt.label)
			}
			result := &platform.CheckinResult{Success: false, Message: tt.msg}
			if shouldRetryTransient(result) {
				t.Errorf("shouldRetryTransient(%q) = true, want false (%s)", tt.msg, tt.label)
			}
		})
	}
}

func TestShouldRetryTransient_SuccessAndNilNoRetry(t *testing.T) {
	// A successful checkin must never enter the retry path.
	success := &platform.CheckinResult{Success: true, Message: "checkin success", Reward: "50"}
	if shouldRetryTransient(success) {
		t.Error("shouldRetryTransient(success) = true, want false")
	}

	// Defensive: a nil result must not panic or retry.
	if shouldRetryTransient(nil) {
		t.Error("shouldRetryTransient(nil) = true, want false")
	}

	// An empty-message failure is NOT transient (ClassUnknown), so no retry.
	empty := &platform.CheckinResult{Success: false, Message: ""}
	if shouldRetryTransient(empty) {
		t.Error("shouldRetryTransient(empty failure) = true, want false")
	}
}

// ---- Pacing & backoff helper Tests (issue #668: same-site pacing) ----
//
// These pin the existence and magnitude of the delays used between same-site
// checkins (CheckinAll) and before the transient retry (CheckinAccount).

func TestSameSitePacingDelay(t *testing.T) {
	for i := 0; i < 200; i++ {
		d := sameSitePacingDelay()
		// Base 1s + 0-249ms jitter → [1000ms, 1249ms]. Use a safe envelope.
		if d < 950*time.Millisecond || d > 1300*time.Millisecond {
			t.Errorf("sameSitePacingDelay() = %v, want ~1s (950ms-1300ms) on iteration %d", d, i)
		}
	}
}

func TestSameSitePacingDelay_AlwaysPositive(t *testing.T) {
	// Must never be zero or negative (would mean no pacing).
	for i := 0; i < 200; i++ {
		if sameSitePacingDelay() <= 0 {
			t.Fatal("sameSitePacingDelay() returned non-positive duration")
		}
	}
}

func TestTransientRetryBackoff(t *testing.T) {
	for i := 0; i < 200; i++ {
		d := transientRetryBackoff()
		// Base 2s + 0-999ms jitter → [2000ms, 2999ms].
		if d < 2*time.Second || d > 3*time.Second {
			t.Errorf("transientRetryBackoff() = %v, want 2s-3s on iteration %d", d, i)
		}
	}
}
