package routing

import (
	"strings"
	"testing"
)

// =============================================================================
// P0-3 — structured cooldown reason classification
// =============================================================================

func TestClassifyCooldownReason_StatusClasses(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		text     string
		wantCode string
	}{
		{"usage limit 429", 429, "usage_limit_reached: quota exhausted", CooldownReasonCodeUsageLimit},
		{"usage limit wording", 429, "You have reached your subscription limit", CooldownReasonCodeUsageLimit},
		{"plain rate limit 429", 429, "slow down", CooldownReasonCodeRateLimited},
		{"auth 401", 401, "invalid api key", CooldownReasonCodeAuthError},
		{"auth 403", 403, "forbidden", CooldownReasonCodeAuthError},
		{"upstream 500", 500, "internal server error", CooldownReasonCodeUpstreamError},
		{"upstream 503", 503, "service unavailable", CooldownReasonCodeUpstreamError},
		{"client 404", 404, "no such endpoint", CooldownReasonCodeClientError},
		{"client 400", 400, "invalid request body", CooldownReasonCodeClientError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.status
			text := tt.text
			code, summary := ClassifyCooldownReason(SiteRuntimeFailureContext{
				Status:    &status,
				ErrorText: &text,
			}, CooldownTriggerTraffic)
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
			if summary != tt.text {
				t.Errorf("summary = %q, want passthrough %q", summary, tt.text)
			}
		})
	}
}

func TestClassifyCooldownReason_TextPatterns(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		source   CooldownTriggerSource
		wantCode string
	}{
		{"timeout text", "context deadline exceeded (Client.Timeout)", CooldownTriggerTraffic, CooldownReasonCodeTimeout},
		{"timed out text", "request timed out while waiting upstream", CooldownTriggerTraffic, CooldownReasonCodeTimeout},
		{"network reset", "connection reset by peer", CooldownTriggerTraffic, CooldownReasonCodeNetworkError},
		{"network refused", "dial tcp: connection refused", CooldownTriggerTraffic, CooldownReasonCodeNetworkError},
		{"opaque traffic failure", "something nobody anticipated", CooldownTriggerTraffic, CooldownReasonCodeUnknown},
		{"opaque probe failure", "", CooldownTriggerProbe, CooldownReasonCodeProbeFailure},
		{"probe with text still network", "connection reset by peer", CooldownTriggerProbe, CooldownReasonCodeNetworkError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := tt.text
			code, _ := ClassifyCooldownReason(SiteRuntimeFailureContext{ErrorText: &text}, tt.source)
			if code != tt.wantCode {
				t.Errorf("code = %q, want %q", code, tt.wantCode)
			}
		})
	}
}

func TestClassifyCooldownReason_NilContext(t *testing.T) {
	code, summary := ClassifyCooldownReason(SiteRuntimeFailureContext{}, CooldownTriggerTraffic)
	if code != CooldownReasonCodeUnknown {
		t.Errorf("code = %q, want %q", code, CooldownReasonCodeUnknown)
	}
	if summary != "" {
		t.Errorf("summary = %q, want empty", summary)
	}
}

func TestSanitizeCooldownReasonSummary_TruncatesAtRuneLimit(t *testing.T) {
	long := strings.Repeat("上", CooldownReasonSummaryMaxRunes+50)
	out := SanitizeCooldownReasonSummary(long)
	if got := len([]rune(out)); got != CooldownReasonSummaryMaxRunes {
		t.Fatalf("truncated length = %d runes, want %d", got, CooldownReasonSummaryMaxRunes)
	}
}

func TestSanitizeCooldownReasonSummary_FlattensControlChars(t *testing.T) {
	out := SanitizeCooldownReasonSummary("line1\nline2\r\ttab\x00nul")
	if strings.ContainsAny(out, "\n\r\t\x00") {
		t.Fatalf("control characters survived sanitization: %q", out)
	}
	// Consecutive control chars each become one space (no collapsing).
	if out != "line1 line2  tab nul" {
		t.Fatalf("sanitized = %q, want %q", out, "line1 line2  tab nul")
	}
}

func TestSanitizeCooldownReasonSummary_Blank(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t"} {
		if out := SanitizeCooldownReasonSummary(in); out != "" {
			t.Fatalf("Sanitize(%q) = %q, want empty", in, out)
		}
	}
}

func TestCooldownReasonSummaryArg_NullWhenEmpty(t *testing.T) {
	if got := CooldownReasonSummaryArg(""); got != nil {
		t.Fatalf("empty summary arg = %v, want nil", got)
	}
	if got := CooldownReasonSummaryArg("boom"); got != "boom" {
		t.Fatalf("summary arg = %v, want %q", got, "boom")
	}
}
