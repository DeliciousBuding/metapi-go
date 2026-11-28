package backup

import (
	"strconv"
	"strings"
)

// channelSkipReasons are the source features that change which requests reach
// the provider. Reproducing a channel without them would make its traffic
// behave differently than it did in AxonHub, so the channel is left behind
// under a named reason instead.
func channelSkipReasons(channel AxonHubSourceChannel) []string {
	var reasons []string
	for _, field := range channel.UnsupportedFields {
		reasons = append(reasons, "channel_field_unsupported:"+field)
	}
	for _, field := range channel.Settings.UnsupportedFields {
		reasons = append(reasons, "channel_settings_unsupported:"+field)
	}
	for _, field := range channel.Policies.UnsupportedFields {
		reasons = append(reasons, "channel_policy_unsupported:"+field)
	}
	switch channel.Policies.Stream {
	case "", "unlimited", "require", "forbid":
	default:
		reasons = append(reasons, "channel_stream_policy_unsupported")
	}
	if channel.Settings.HasUnsupportedRateLimit {
		reasons = append(reasons, "channel_rate_limit_unsupported")
	}
	return reasons
}

// channelResiduals are source features this importer deliberately does not
// carry because Metapi owns an equivalent workflow. They are reported so the
// operator knows the channel is not a byte-for-byte copy, but they do not
// change which requests reach the provider, so the channel still imports.
func channelResiduals(channel AxonHubSourceChannel) []string {
	prefix := "channel-" + strconv.Itoa(channel.ID) + ":"
	var notes []string
	add := func(reason string) { notes = append(notes, prefix+reason) }
	if channel.Type == "antigravity" {
		add("source_endpoint_fallback_not_imported")
	}
	if channel.Policies.AutoDisableRuleCount > 0 {
		add("source_auto_disable_rules_replaced_by_grant_cooldown")
	}
	if len(channel.Settings.RetryableStatusCodes) > 0 || channel.Settings.HasRetryableErrorPatterns {
		add("source_retry_policy_replaced_by_gateway_retry_policy")
	}
	if channel.Settings.HasProviderQuota {
		add("provider_quota_polling_not_imported")
	}
	// The management key only feeds provider quota polling, which is already a
	// residual; it is never attached to inference requests, so leaving the
	// channel behind would lose its traffic for no routing benefit.
	if channel.Credentials.HasManagementKey {
		add("management_api_key_not_imported")
	}
	if channel.Settings.PassThroughUserAgent != nil {
		add("pass_through_user_agent_not_configurable")
	}
	if channel.Settings.PassThroughBody != nil {
		add("pass_through_body_not_configurable")
	}
	if channel.AutoSyncSupportedModels || strings.TrimSpace(channel.AutoSyncModelPattern) != "" {
		add("model_auto_sync_replaced_by_model_refresh")
	}
	if len(channel.ManualModels) > 0 {
		add("manual_models_not_imported_until_synced")
	}
	if strings.TrimSpace(channel.DefaultTestModel) != "" && !axonHubUsesCodexImage(channel) {
		add("default_test_model_not_imported")
	}
	return notes
}
