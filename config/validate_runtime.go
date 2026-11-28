package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/robfig/cron/v3"
)

// Validate checks the runtime-mutable settings and returns all validation
// errors. Boot-time call path mirrors Config.Validate: warnings for
// non-fatal issues, exit on critical ones before binding the port.
func (r *RuntimeSettings) Validate() []error {
	var errs []error

	// --- Critical: CheckinScheduleMode must be "cron" or "interval" ---
	mode := strings.TrimSpace(strings.ToLower(r.CheckinScheduleMode))
	if mode != "cron" && mode != "interval" && mode != "window" {
		errs = append(errs, &configError{
			field:    "checkin_schedule_mode",
			value:    r.CheckinScheduleMode,
			msg:      "must be 'cron', 'interval', or 'window'",
			critical: true,
		})
	}

	// --- Critical: DBType must be "sqlite" or "postgres" ---
	dbType := strings.TrimSpace(strings.ToLower(r.DbType))
	if dbType != "sqlite" && dbType != "postgres" {
		errs = append(errs, &configError{
			field:    "db_type",
			value:    r.DbType,
			msg:      "must be 'sqlite' or 'postgres'",
			critical: true,
		})
	}
	// --- Warning: Cron expressions must be parseable ---
	if !ValidateCronExpr(r.CheckinCron) {
		errs = append(errs, &configError{
			field:    "checkin_cron",
			value:    r.CheckinCron,
			msg:      "invalid cron expression",
			critical: false,
		})
	}
	if !ValidateCronExpr(r.BalanceRefreshCron) {
		errs = append(errs, &configError{
			field:    "balance_refresh_cron",
			value:    r.BalanceRefreshCron,
			msg:      "invalid cron expression",
			critical: false,
		})
	}
	if !ValidateCronExpr(r.ModelSyncCron) {
		errs = append(errs, &configError{
			field:    "model_sync_cron",
			value:    r.ModelSyncCron,
			msg:      "invalid cron expression",
			critical: false,
		})
	}
	if !ValidateCronExpr(r.LogCleanupCron) {
		errs = append(errs, &configError{
			field:    "log_cleanup_cron",
			value:    r.LogCleanupCron,
			msg:      "invalid cron expression",
			critical: false,
		})
	}

	// --- Warning: NotifyCooldownSec >= 0 ---
	if r.NotifyCooldownSec < 0 {
		errs = append(errs, &configError{
			field:    "notify_cooldown_sec",
			value:    fmt.Sprintf("%d", r.NotifyCooldownSec),
			msg:      "must be >= 0",
			critical: false,
		})
	}

	// --- Warning: ProxyFirstByteTimeoutSec >= 0 ---
	if r.ProxyFirstByteTimeoutSec < 0 {
		errs = append(errs, &configError{
			field:    "proxy_first_byte_timeout_sec",
			value:    fmt.Sprintf("%d", r.ProxyFirstByteTimeoutSec),
			msg:      "must be >= 0",
			critical: false,
		})
	}

	// --- Warning: TokenRouterFailureCooldownMaxSec >= 0 ---
	if r.TokenRouterFailureCooldownMaxSec < 0 {
		errs = append(errs, &configError{
			field:    "token_router_failure_cooldown_max_sec",
			value:    fmt.Sprintf("%d", r.TokenRouterFailureCooldownMaxSec),
			msg:      "must be >= 0",
			critical: false,
		})
	}

	// --- Warning: CheckinIntervalHours in [1, 24] ---
	if r.CheckinIntervalHours < 1 || r.CheckinIntervalHours > 24 {
		errs = append(errs, &configError{
			field:    "checkin_interval_hours",
			value:    fmt.Sprintf("%d", r.CheckinIntervalHours),
			msg:      "must be in [1, 24]",
			critical: false,
		})
	}

	// --- Warning: RoutingWeights all >= 0 ---
	rw := r.RoutingWeights
	if rw.BaseWeightFactor < 0 {
		errs = append(errs, &configError{
			field:    "base_weight_factor",
			value:    fmt.Sprintf("%f", rw.BaseWeightFactor),
			msg:      "must be >= 0",
			critical: false,
		})
	}
	if rw.ValueScoreFactor < 0 {
		errs = append(errs, &configError{
			field:    "value_score_factor",
			value:    fmt.Sprintf("%f", rw.ValueScoreFactor),
			msg:      "must be >= 0",
			critical: false,
		})
	}
	if rw.CostWeight < 0 {
		errs = append(errs, &configError{
			field:    "cost_weight",
			value:    fmt.Sprintf("%f", rw.CostWeight),
			msg:      "must be >= 0",
			critical: false,
		})
	}
	if rw.BalanceWeight < 0 {
		errs = append(errs, &configError{
			field:    "balance_weight",
			value:    fmt.Sprintf("%f", rw.BalanceWeight),
			msg:      "must be >= 0",
			critical: false,
		})
	}
	if rw.UsageWeight < 0 {
		errs = append(errs, &configError{
			field:    "usage_weight",
			value:    fmt.Sprintf("%f", rw.UsageWeight),
			msg:      "must be >= 0",
			critical: false,
		})
	}

	// --- Critical: Default AUTH_TOKEN ---
	if r.AuthToken == DefaultAuthToken {
		errs = append(errs, &configError{
			field:    "auth_token",
			value:    "(default)",
			msg:      "UNSAFE: using default admin token — set AUTH_TOKEN",
			critical: true,
		})
	}

	if r.ProxySessionChannelConcurrencyLimit < 1 {
		errs = append(errs, &configError{
			field:    "proxy_session_channel_concurrency_limit",
			value:    fmt.Sprintf("%d", r.ProxySessionChannelConcurrencyLimit),
			msg:      "should be >= 1 — 0 disables per-channel concurrency control",
			critical: false,
		})
	}
	if r.ProxySessionChannelQueueWaitMs < 0 {
		errs = append(errs, &configError{
			field:    "proxy_session_channel_queue_wait_ms",
			value:    fmt.Sprintf("%d", r.ProxySessionChannelQueueWaitMs),
			msg:      "should be >= 0 — negative values are treated as 0 (no queue wait)",
			critical: false,
		})
	}

	// --- Warning: notify webhook URLs must be well-formed http(s) ---
	webhookUrls := []struct{ field, val string }{
		{"webhook_url", r.WebhookUrl},
		{"bark_url", r.BarkUrl},
		{"feishu_webhook", r.FeishuWebhook},
		{"dingtalk_webhook", r.DingtalkWebhook},
		{"wecom_webhook", r.WecomWebhook},
		{"ntfy_url", r.NtfyUrl},
		{"telegram_api_base_url", r.TelegramApiBaseUrl},
	}
	for _, u := range webhookUrls {
		if err := validateUrl(u.field, u.val, true); err != nil {
			errs = append(errs, err)
		}
	}
	// --- Warning: system proxy URL must parse (scheme left open) ---
	if err := validateUrl("system_proxy_url", r.SystemProxyUrl, false); err != nil {
		errs = append(errs, err)
	}

	// --- Warning: checkin window bounds must be HH:mm ---
	if err := validateHhMm("checkin_window_start", r.CheckinWindowStart); err != nil {
		errs = append(errs, err)
	}
	if err := validateHhMm("checkin_window_end", r.CheckinWindowEnd); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// validateHhMm validates a 24h HH:mm window bound (2 digits : 2 digits with
// hours 00-23 and minutes 00-59). Empty values are treated as unset.
func validateHhMm(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) != 5 || value[2] != ':' {
		return &configError{
			field:    field,
			value:    value,
			msg:      "must be HH:mm (2 digits : 2 digits)",
			critical: false,
		}
	}
	hh, errH := strconv.Atoi(value[:2])
	mm, errM := strconv.Atoi(value[3:])
	if errH != nil || errM != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return &configError{
			field:    field,
			value:    value,
			msg:      "must be HH:mm with hours 00-23 and minutes 00-59",
			critical: false,
		}
	}
	return nil
}

// ValidateCronExpr checks if a cron expression is parseable. Auto-normalizes
// 5-field expressions (minute hour dom month dow) to 6-field (second...)
// for compatibility with cron.WithSeconds(), matching the scheduler behavior.
// This is the canonical cron validation used by both config.Validate and
// scheduler.ValidateCronExpr.
func ValidateCronExpr(expr string) bool {
	if strings.TrimSpace(expr) == "" {
		return false
	}
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		expr = "0 " + expr
	}
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err == nil
}
