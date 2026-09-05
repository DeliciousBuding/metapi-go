package service

import "strings"

// IsUnsupportedCheckinMessage reports whether an upstream check-in answer means
// "this site does not offer check-in" rather than "check-in failed".
//
// One vocabulary with three readers, which used to be three separate lists:
// the check-in runner (status, runtime health, event level), the structured
// failure classifier (checkin_logs.failure_reason) and
// IsUnsupportedCheckinRuntimeHealth (whether a balance refresh may overwrite the
// state). Each list was English-only, so a real New API upstream — whose
// DoCheckin handler answers the hardcoded 签到功能未启用, with 簽到功能未啟用 and
// "Check-in feature is not enabled" in its locale catalogs — matched none of
// them. An account whose relay worked perfectly was therefore recorded as
// checkin failed / unknown_error / runtimeHealth=unhealthy plus an error-level
// event, and stayed that way until the next hourly balance refresh happened to
// overwrite the health entry. The already-checked-in classifier next door has
// been locale-aware all along; this one now is too.
//
// The list is enumerated from what supported upstreams actually return, not
// guessed: adding a speculative phrasing here would silently swallow real
// failures, which is the one thing this classifier must not do.
func IsUnsupportedCheckinMessage(message string) bool {
	text := strings.TrimSpace(message)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)

	// Endpoint absent (one-api family and anything else with no check-in route).
	if strings.Contains(lower, "invalid url (post /api/user/checkin)") {
		return true
	}
	if strings.Contains(lower, "http 404") && strings.Contains(lower, "/api/user/checkin") {
		return true
	}
	if strings.Contains(lower, "checkin endpoint not found") {
		return true
	}
	if strings.Contains(lower, "签到端点不存在") {
		return true
	}

	// Feature present but not offered / not enabled.
	for _, phrase := range [...]string{
		"check-in is not supported",
		"checkin is not supported",
		"does not support checkin",
		"not support checkin",
		"check-in feature is not enabled",
		"站点不支持签到",
		"签到功能未启用",
		"簽到功能未啟用",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
