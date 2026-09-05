package platform

import "strings"

// Check-in answers are classified here, next to the rest of the upstream message
// semantics (error_classification.go), because four callers ask the same
// question about the same sentence: the New API adapter's cookie retry ladder,
// the check-in runner (status, runtime health, event level), the structured
// failure classifier (checkin_logs.failure_reason) and the runtime-health
// reader. Three of the four used to keep their own list and the lists
// disagreed:
//
//   - 11 of the 16 "already checked in" wordings the runner accepted were
//     written to checkin_logs as code=unknown_error;
//   - an upstream whose check-in is switched off (New API answers
//     签到功能未启用) matched none of the three lists, so a healthy account was
//     reported unhealthy with an error-level event (#1267);
//   - six registered platforms (openai, claude, gemini, gemini-cli,
//     antigravity, sensetime) answer with StandardAdapter's own default wording,
//     which no list contained either — the same unknown_error, on every check-in
//     cycle, for a third of the platform registry.
//
// The wordings below are enumerated from what supported upstreams and our own
// adapters actually return (New API's model/checkin.go and its three locale
// catalogs; StandardAdapter/BaseAdapter defaults; the TS heritage list). Nothing
// here is a guessed phrasing: a speculative string would soften a real failure
// into "unsupported" or "already done", which is the one thing a classifier must
// not do.
const (
	// CheckinUnsupportedMessage is what an adapter returns when the platform has
	// no check-in at all. Adapters and the vocabulary below reference the same
	// constant so a message this codebase authored cannot fall out of the
	// classifier that reads it — that drift is what left six platforms reporting
	// unknown_error.
	DefaultCheckinUnsupportedMessage = "checkin endpoint not supported"
)

// checkinEndpointAbsentWordings mean "there is no check-in route to call".
var checkinEndpointAbsentWordings = []string{
	"invalid url (post /api/user/checkin)",
	"checkin endpoint not found",
	"签到端点不存在",
}

// checkinNotOfferedWordings mean "the route may exist, but this site does not
// offer check-in to this account".
var checkinNotOfferedWordings = []string{
	DefaultCheckinUnsupportedMessage,
	"check-in is not supported",
	"checkin is not supported",
	"does not support checkin",
	"not support checkin",
	"check-in feature is not enabled",
	// Heritage wording, recognized for health entries written before the source
	// field existed; no current emitter produces it.
	"unsupported checkin endpoint",
	"站点不支持签到",
	"签到功能未启用",
	"簽到功能未啟用",
}

// alreadyCheckedInWordings mean "today's check-in already happened".
var alreadyCheckedInWordings = []string{
	"already checked in",
	"already signed",
	"already sign in",
	"already claim",
	"claimed today",
	"今日已签到",
	"今天已签到",
	"今天已经签到",
	"今日已经签到",
	"已经签到",
	"已签到",
	"重复签到",
	"已经领取",
	"已领取",
	"领取过",
	"签到达",
}

// IsCheckinEndpointAbsent reports whether the answer says the check-in route
// itself is missing. This is the narrow question the New API cookie retry ladder
// asks: an absent route can be a disguised session failure, so it is worth one
// more probe, whereas "the site does not offer check-in" is already a complete
// answer and probing further only costs an upstream call.
func IsCheckinEndpointAbsent(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	// A bare 404 is not enough; it has to be a 404 for the check-in route.
	if strings.Contains(lower, "http 404") && strings.Contains(lower, "/api/user/checkin") {
		return true
	}
	return containsAnyWording(lower, checkinEndpointAbsentWordings)
}

// IsUnsupportedCheckinMessage reports whether a check-in answer means "this site
// does not offer check-in" rather than "check-in failed".
func IsUnsupportedCheckinMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	return IsCheckinEndpointAbsent(lower) || containsAnyWording(lower, checkinNotOfferedWordings)
}

// IsAlreadyCheckedInMessage reports whether the answer means today's check-in
// already happened.
func IsAlreadyCheckedInMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	return containsAnyWording(lower, alreadyCheckedInWordings)
}

// IsManualVerificationRequiredMessage reports whether the upstream needs a human
// to solve a Turnstile challenge once before automated check-in can proceed.
func IsManualVerificationRequiredMessage(message string) bool {
	text := strings.TrimSpace(message)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "turnstile token 为空") {
		return true
	}
	if !strings.Contains(lower, "turnstile") {
		return false
	}
	return strings.Contains(lower, "token") ||
		strings.Contains(text, "校验") ||
		strings.Contains(text, "验证") ||
		strings.Contains(lower, "manual")
}

func containsAnyWording(lower string, wordings []string) bool {
	for _, w := range wordings {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}
