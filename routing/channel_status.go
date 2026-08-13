package routing

import "time"

// Channel runtime status vocabulary for read-only operators (#622/#624).
const (
	ChannelStatusEnabled          = "enabled"
	ChannelStatusCooldown         = "cooldown"
	ChannelStatusBreakerOpen      = "breaker_open"
	ChannelStatusManuallyDisabled = "manually_disabled"
)

// ChannelRuntimeStatus classifies a channel's current routing state by overlaying
// the in-memory site/model runtime breaker and the persisted cooldown window on
// top of the channel's enabled flag. It is intentionally read-only: it never
// mutates routing state, so callers can render the status without hard-disabling
// (soft isolation only).
func ChannelRuntimeStatus(siteID int64, modelName string, enabled bool, cooldownUntil *string) string {
	if !enabled {
		return ChannelStatusManuallyDisabled
	}
	nowISO := time.Now().UTC().Format(time.RFC3339)
	details := GetSiteRuntimeHealthDetails(siteID, modelName)
	if details.GlobalBreakerOpen || details.ModelBreakerOpen {
		return ChannelStatusBreakerOpen
	}
	if IsCooldownActive(cooldownUntil, nowISO) {
		return ChannelStatusCooldown
	}
	return ChannelStatusEnabled
}
