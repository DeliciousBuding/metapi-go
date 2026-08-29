package store

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestApplyRuntimeSettingsAppliesCheckinSchedule(t *testing.T) {
	rt := &config.RuntimeSettings{
		CheckinCron:          config.DefaultCheckinCron,
		CheckinScheduleMode:  "cron",
		CheckinIntervalHours: config.DefaultCheckinIntervalHours,
	}

	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"checkin_cron":           `"15 9 * * *"`,
		"checkin_schedule_mode":  `"interval"`,
		"checkin_interval_hours": `6`,
	})

	if rt.CheckinCron != "15 9 * * *" {
		t.Fatalf("CheckinCron = %q, want updated cron", rt.CheckinCron)
	}
	if rt.CheckinScheduleMode != "interval" {
		t.Fatalf("CheckinScheduleMode = %q, want interval", rt.CheckinScheduleMode)
	}
	if rt.CheckinIntervalHours != 6 {
		t.Fatalf("CheckinIntervalHours = %d, want 6", rt.CheckinIntervalHours)
	}
}

func TestApplyRuntimeSettingsIgnoresInvalidCheckinInterval(t *testing.T) {
	rt := &config.RuntimeSettings{
		CheckinCron:          config.DefaultCheckinCron,
		CheckinScheduleMode:  "cron",
		CheckinIntervalHours: config.DefaultCheckinIntervalHours,
	}

	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"checkin_interval_hours": `48`,
	})

	if rt.CheckinIntervalHours != config.DefaultCheckinIntervalHours {
		t.Fatalf("CheckinIntervalHours = %d, want fallback %d", rt.CheckinIntervalHours, config.DefaultCheckinIntervalHours)
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsJSONArray(t *testing.T) {
	rt := &config.RuntimeSettings{GlobalAllowedModels: []string{"stale"}}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"global_allowed_models": `["gpt-4o"," claude-3.7-sonnet ","gpt-4o"]`,
	})
	want := []string{"gpt-4o", "claude-3.7-sonnet"}
	if len(rt.GlobalAllowedModels) != len(want) {
		t.Fatalf("GlobalAllowedModels = %#v, want %#v", rt.GlobalAllowedModels, want)
	}
	for i := range want {
		if rt.GlobalAllowedModels[i] != want[i] {
			t.Fatalf("GlobalAllowedModels = %#v, want %#v", rt.GlobalAllowedModels, want)
		}
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsDoubleEncodedExact(t *testing.T) {
	rt := &config.RuntimeSettings{GlobalAllowedModels: []string{"stale"}}
	// Value as stored after JSON.stringify(JSON.stringify([...]))
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"global_allowed_models": `"[\"model-alpha\",\" model-beta \",\"model-gamma\"]"`,
	})
	want := []string{"model-alpha", "model-beta", "model-gamma"}
	if len(rt.GlobalAllowedModels) != len(want) {
		t.Fatalf("GlobalAllowedModels = %#v, want %#v", rt.GlobalAllowedModels, want)
	}
	for i := range want {
		if rt.GlobalAllowedModels[i] != want[i] {
			t.Fatalf("GlobalAllowedModels = %#v, want %#v", rt.GlobalAllowedModels, want)
		}
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsExplicitEmpty(t *testing.T) {
	rt := &config.RuntimeSettings{GlobalAllowedModels: []string{"stale"}}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"global_allowed_models": `[]`,
	})
	if rt.GlobalAllowedModels == nil || len(rt.GlobalAllowedModels) != 0 {
		t.Fatalf("GlobalAllowedModels = %#v, want empty non-nil slice", rt.GlobalAllowedModels)
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsInvalidDoesNotWipe(t *testing.T) {
	rt := &config.RuntimeSettings{GlobalAllowedModels: []string{"keep-me", "also"}}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"global_allowed_models": `{"oops":true}`,
	})
	if len(rt.GlobalAllowedModels) != 2 || rt.GlobalAllowedModels[0] != "keep-me" {
		t.Fatalf("invalid value wiped allowlist: %#v", rt.GlobalAllowedModels)
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsCommaSeparatedLegacy(t *testing.T) {
	rt := &config.RuntimeSettings{GlobalAllowedModels: []string{}}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"global_allowed_models": `gpt-4o, claude-3, gemini-pro`,
	})
	want := []string{"gpt-4o", "claude-3", "gemini-pro"}
	if len(rt.GlobalAllowedModels) != len(want) {
		t.Fatalf("GlobalAllowedModels = %#v, want %#v", rt.GlobalAllowedModels, want)
	}
	for i := range want {
		if rt.GlobalAllowedModels[i] != want[i] {
			t.Fatalf("GlobalAllowedModels = %#v, want %#v", rt.GlobalAllowedModels, want)
		}
	}
}

func TestParseStringListSettingEmptyRawRejected(t *testing.T) {
	if list, ok := parseStringListSetting(""); ok || list != nil {
		t.Fatalf("empty raw should be rejected, got %#v ok=%v", list, ok)
	}
}

func TestApplyRuntimeSettingsAppliesBranding(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"system_name":    `"My Gateway"`,
		"logo":           `"https://example.com/logo.png"`,
		"footer":         `"Powered by Metapi"`,
		"about":          `"About copy"`,
		"server_address": `"https://gw.example.com"`,
	})
	if rt.SystemName != "My Gateway" || rt.Logo != "https://example.com/logo.png" || rt.Footer != "Powered by Metapi" {
		t.Fatalf("branding = %+v", rt)
	}
	if rt.About != "About copy" || rt.ServerAddress != "https://gw.example.com" {
		t.Fatalf("branding = %+v", rt)
	}
}

func TestApplyRuntimeSettingsIgnoresEmptyBranding(t *testing.T) {
	rt := &config.RuntimeSettings{SystemName: "keep"}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"system_name": `""`,
	})
	if rt.SystemName != "keep" {
		t.Fatalf("SystemName = %q, want preserved", rt.SystemName)
	}
}

func TestApplyRuntimeSettingsDecodesJSONEncodedNotificationStrings(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"webhook_url":        `"https://hooks.example.test/path"`,
		"telegram_bot_token": `"bot-token"`,
		"smtp_pass":          `"smtp-secret"`,
		"ntfy_topic":         `"alerts"`,
	})
	if rt.WebhookUrl != "https://hooks.example.test/path" || rt.TelegramBotToken != "bot-token" {
		t.Fatalf("decoded notification strings = %#v", rt)
	}
	if rt.SmtpPass != "smtp-secret" || rt.NtfyTopic != "alerts" {
		t.Fatalf("decoded notification secrets = %#v", rt)
	}
}

func TestApplyRuntimeSettingsDecodesNotifyTaskTogglesObject(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"notify_task_toggles": `{"token_expired":true,"low_balance":false}`,
	})
	if rt.NotifyTaskToggles == nil || !rt.NotifyTaskToggles["token_expired"] || rt.NotifyTaskToggles["low_balance"] {
		t.Fatalf("NotifyTaskToggles = %#v", rt.NotifyTaskToggles)
	}
}

// ---- Hydration tests for settings that were written but never read back ----
//
// upsertSettingDB (handler/admin/settings.go) JSON-marshals every value
// before persisting: bools become "true"/"false", strings become
// "\"value\"", ints become "60". ApplyRuntimeSettings must decode those
// exact formats. These tests simulate a restart where the settings table
// already contains persisted values and LoadRuntimeSettings feeds them
// through ApplyRuntimeSettings.

func TestApplyRuntimeSettings_SmtpSecure_ReadBack(t *testing.T) {
	rt := &config.RuntimeSettings{SmtpSecure: false}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"smtp_secure": `true`,
	})
	if !rt.SmtpSecure {
		t.Fatalf("SmtpSecure = %v, want true", rt.SmtpSecure)
	}
}

func TestApplyRuntimeSettings_SmtpSecure_False_ReadBack(t *testing.T) {
	rt := &config.RuntimeSettings{SmtpSecure: true}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"smtp_secure": `false`,
	})
	if rt.SmtpSecure {
		t.Fatalf("SmtpSecure = %v, want false", rt.SmtpSecure)
	}
}

func TestApplyRuntimeSettings_NotifyCooldownSec_ReadBack(t *testing.T) {
	rt := &config.RuntimeSettings{NotifyCooldownSec: 0}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"notify_cooldown_sec": `120`,
	})
	if rt.NotifyCooldownSec != 120 {
		t.Fatalf("NotifyCooldownSec = %d, want 120", rt.NotifyCooldownSec)
	}
}

func TestApplyRuntimeSettings_TelegramApiBaseUrl_ReadBack(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"telegram_api_base_url": `"https://api.telegram.local"`,
	})
	if rt.TelegramApiBaseUrl != "https://api.telegram.local" {
		t.Fatalf("TelegramApiBaseUrl = %q, want %q", rt.TelegramApiBaseUrl, "https://api.telegram.local")
	}
}

func TestApplyRuntimeSettings_TelegramUseSystemProxy_ReadBack(t *testing.T) {
	rt := &config.RuntimeSettings{TelegramUseSystemProxy: false}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"telegram_use_system_proxy": `true`,
	})
	if !rt.TelegramUseSystemProxy {
		t.Fatalf("TelegramUseSystemProxy = %v, want true", rt.TelegramUseSystemProxy)
	}
}

func TestApplyRuntimeSettings_TelegramMessageThreadId_ReadBack(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"telegram_message_thread_id": `"12345"`,
	})
	if rt.TelegramMessageThreadId != "12345" {
		t.Fatalf("TelegramMessageThreadId = %q, want %q", rt.TelegramMessageThreadId, "12345")
	}
}

func TestApplyRuntimeSettings_SystemProxyUrl_ReadBack(t *testing.T) {
	rt := &config.RuntimeSettings{}
	ApplyRuntimeSettings(&config.Config{}, rt, map[string]string{
		"system_proxy_url": `"http://proxy.local:8080"`,
	})
	if rt.SystemProxyUrl != "http://proxy.local:8080" {
		t.Fatalf("SystemProxyUrl = %q, want %q", rt.SystemProxyUrl, "http://proxy.local:8080")
	}
}
