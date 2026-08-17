package store

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestApplyRuntimeSettingsAppliesCheckinSchedule(t *testing.T) {
	cfg := &config.Config{
		CheckinCron:          config.DefaultCheckinCron,
		CheckinScheduleMode:  "cron",
		CheckinIntervalHours: config.DefaultCheckinIntervalHours,
	}

	ApplyRuntimeSettings(cfg, map[string]string{
		"checkin_cron":           `"15 9 * * *"`,
		"checkin_schedule_mode":  `"interval"`,
		"checkin_interval_hours": `6`,
	})

	if cfg.CheckinCron != "15 9 * * *" {
		t.Fatalf("CheckinCron = %q, want updated cron", cfg.CheckinCron)
	}
	if cfg.CheckinScheduleMode != "interval" {
		t.Fatalf("CheckinScheduleMode = %q, want interval", cfg.CheckinScheduleMode)
	}
	if cfg.CheckinIntervalHours != 6 {
		t.Fatalf("CheckinIntervalHours = %d, want 6", cfg.CheckinIntervalHours)
	}
}

func TestApplyRuntimeSettingsIgnoresInvalidCheckinInterval(t *testing.T) {
	cfg := &config.Config{
		CheckinCron:          config.DefaultCheckinCron,
		CheckinScheduleMode:  "cron",
		CheckinIntervalHours: config.DefaultCheckinIntervalHours,
	}

	ApplyRuntimeSettings(cfg, map[string]string{
		"checkin_interval_hours": `48`,
	})

	if cfg.CheckinIntervalHours != config.DefaultCheckinIntervalHours {
		t.Fatalf("CheckinIntervalHours = %d, want fallback %d", cfg.CheckinIntervalHours, config.DefaultCheckinIntervalHours)
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsJSONArray(t *testing.T) {
	cfg := &config.Config{GlobalAllowedModels: []string{"stale"}}
	ApplyRuntimeSettings(cfg, map[string]string{
		"global_allowed_models": `["gpt-4o"," claude-3.7-sonnet ","gpt-4o"]`,
	})
	want := []string{"gpt-4o", "claude-3.7-sonnet"}
	if len(cfg.GlobalAllowedModels) != len(want) {
		t.Fatalf("GlobalAllowedModels = %#v, want %#v", cfg.GlobalAllowedModels, want)
	}
	for i := range want {
		if cfg.GlobalAllowedModels[i] != want[i] {
			t.Fatalf("GlobalAllowedModels = %#v, want %#v", cfg.GlobalAllowedModels, want)
		}
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsDoubleEncodedExact(t *testing.T) {
	cfg := &config.Config{GlobalAllowedModels: []string{"stale"}}
	// Value as stored after JSON.stringify(JSON.stringify([...]))
	ApplyRuntimeSettings(cfg, map[string]string{
		"global_allowed_models": `"[\"model-alpha\",\" model-beta \",\"model-gamma\"]"`,
	})
	want := []string{"model-alpha", "model-beta", "model-gamma"}
	if len(cfg.GlobalAllowedModels) != len(want) {
		t.Fatalf("GlobalAllowedModels = %#v, want %#v", cfg.GlobalAllowedModels, want)
	}
	for i := range want {
		if cfg.GlobalAllowedModels[i] != want[i] {
			t.Fatalf("GlobalAllowedModels = %#v, want %#v", cfg.GlobalAllowedModels, want)
		}
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsExplicitEmpty(t *testing.T) {
	cfg := &config.Config{GlobalAllowedModels: []string{"stale"}}
	ApplyRuntimeSettings(cfg, map[string]string{
		"global_allowed_models": `[]`,
	})
	if cfg.GlobalAllowedModels == nil || len(cfg.GlobalAllowedModels) != 0 {
		t.Fatalf("GlobalAllowedModels = %#v, want empty non-nil slice", cfg.GlobalAllowedModels)
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsInvalidDoesNotWipe(t *testing.T) {
	cfg := &config.Config{GlobalAllowedModels: []string{"keep-me", "also"}}
	ApplyRuntimeSettings(cfg, map[string]string{
		"global_allowed_models": `{"oops":true}`,
	})
	if len(cfg.GlobalAllowedModels) != 2 || cfg.GlobalAllowedModels[0] != "keep-me" {
		t.Fatalf("invalid value wiped allowlist: %#v", cfg.GlobalAllowedModels)
	}
}

func TestApplyRuntimeSettingsGlobalAllowedModelsCommaSeparatedLegacy(t *testing.T) {
	cfg := &config.Config{GlobalAllowedModels: []string{}}
	ApplyRuntimeSettings(cfg, map[string]string{
		"global_allowed_models": `gpt-4o, claude-3, gemini-pro`,
	})
	want := []string{"gpt-4o", "claude-3", "gemini-pro"}
	if len(cfg.GlobalAllowedModels) != len(want) {
		t.Fatalf("GlobalAllowedModels = %#v, want %#v", cfg.GlobalAllowedModels, want)
	}
	for i := range want {
		if cfg.GlobalAllowedModels[i] != want[i] {
			t.Fatalf("GlobalAllowedModels = %#v, want %#v", cfg.GlobalAllowedModels, want)
		}
	}
}

func TestParseStringListSettingEmptyRawRejected(t *testing.T) {
	if list, ok := parseStringListSetting(""); ok || list != nil {
		t.Fatalf("empty raw should be rejected, got %#v ok=%v", list, ok)
	}
}

func TestApplyRuntimeSettingsAppliesBranding(t *testing.T) {
	cfg := &config.Config{}
	ApplyRuntimeSettings(cfg, map[string]string{
		"system_name":       `"My Gateway"`,
		"logo":              `"https://example.com/logo.png"`,
		"footer":            `"Powered by Metapi"`,
		"about":             `"About copy"`,
		"home_page_content": `"Welcome"`,
		"server_address":    `"https://gw.example.com"`,
	})
	if cfg.SystemName != "My Gateway" || cfg.Logo != "https://example.com/logo.png" || cfg.Footer != "Powered by Metapi" {
		t.Fatalf("branding = %+v", cfg)
	}
	if cfg.About != "About copy" || cfg.HomePageContent != "Welcome" || cfg.ServerAddress != "https://gw.example.com" {
		t.Fatalf("branding = %+v", cfg)
	}
}

func TestApplyRuntimeSettingsIgnoresEmptyBranding(t *testing.T) {
	cfg := &config.Config{SystemName: "keep"}
	ApplyRuntimeSettings(cfg, map[string]string{
		"system_name": `""`,
	})
	if cfg.SystemName != "keep" {
		t.Fatalf("SystemName = %q, want preserved", cfg.SystemName)
	}
}

func TestApplyRuntimeSettingsDecodesJSONEncodedNotificationStrings(t *testing.T) {
	cfg := &config.Config{}
	ApplyRuntimeSettings(cfg, map[string]string{
		"webhook_url":        `"https://hooks.example.test/path"`,
		"telegram_bot_token": `"bot-token"`,
		"smtp_pass":          `"smtp-secret"`,
		"ntfy_topic":         `"alerts"`,
	})
	if cfg.WebhookUrl != "https://hooks.example.test/path" || cfg.TelegramBotToken != "bot-token" {
		t.Fatalf("decoded notification strings = %#v", cfg)
	}
	if cfg.SmtpPass != "smtp-secret" || cfg.NtfyTopic != "alerts" {
		t.Fatalf("decoded notification secrets = %#v", cfg)
	}
}

func TestApplyRuntimeSettingsDecodesNotifyTaskTogglesObject(t *testing.T) {
	cfg := &config.Config{}
	ApplyRuntimeSettings(cfg, map[string]string{
		"notify_task_toggles": `{"token_expired":true,"low_balance":false}`,
	})
	if cfg.NotifyTaskToggles == nil || !cfg.NotifyTaskToggles["token_expired"] || cfg.NotifyTaskToggles["low_balance"] {
		t.Fatalf("NotifyTaskToggles = %#v", cfg.NotifyTaskToggles)
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
	cfg := &config.Config{SmtpSecure: false}
	ApplyRuntimeSettings(cfg, map[string]string{
		"smtp_secure": `true`,
	})
	if !cfg.SmtpSecure {
		t.Fatalf("SmtpSecure = %v, want true", cfg.SmtpSecure)
	}
}

func TestApplyRuntimeSettings_SmtpSecure_False_ReadBack(t *testing.T) {
	cfg := &config.Config{SmtpSecure: true}
	ApplyRuntimeSettings(cfg, map[string]string{
		"smtp_secure": `false`,
	})
	if cfg.SmtpSecure {
		t.Fatalf("SmtpSecure = %v, want false", cfg.SmtpSecure)
	}
}

func TestApplyRuntimeSettings_NotifyCooldownSec_ReadBack(t *testing.T) {
	cfg := &config.Config{NotifyCooldownSec: 0}
	ApplyRuntimeSettings(cfg, map[string]string{
		"notify_cooldown_sec": `120`,
	})
	if cfg.NotifyCooldownSec != 120 {
		t.Fatalf("NotifyCooldownSec = %d, want 120", cfg.NotifyCooldownSec)
	}
}

func TestApplyRuntimeSettings_TelegramApiBaseUrl_ReadBack(t *testing.T) {
	cfg := &config.Config{}
	ApplyRuntimeSettings(cfg, map[string]string{
		"telegram_api_base_url": `"https://api.telegram.local"`,
	})
	if cfg.TelegramApiBaseUrl != "https://api.telegram.local" {
		t.Fatalf("TelegramApiBaseUrl = %q, want %q", cfg.TelegramApiBaseUrl, "https://api.telegram.local")
	}
}

func TestApplyRuntimeSettings_TelegramUseSystemProxy_ReadBack(t *testing.T) {
	cfg := &config.Config{TelegramUseSystemProxy: false}
	ApplyRuntimeSettings(cfg, map[string]string{
		"telegram_use_system_proxy": `true`,
	})
	if !cfg.TelegramUseSystemProxy {
		t.Fatalf("TelegramUseSystemProxy = %v, want true", cfg.TelegramUseSystemProxy)
	}
}

func TestApplyRuntimeSettings_TelegramMessageThreadId_ReadBack(t *testing.T) {
	cfg := &config.Config{}
	ApplyRuntimeSettings(cfg, map[string]string{
		"telegram_message_thread_id": `"12345"`,
	})
	if cfg.TelegramMessageThreadId != "12345" {
		t.Fatalf("TelegramMessageThreadId = %q, want %q", cfg.TelegramMessageThreadId, "12345")
	}
}

func TestApplyRuntimeSettings_SystemProxyUrl_ReadBack(t *testing.T) {
	cfg := &config.Config{}
	ApplyRuntimeSettings(cfg, map[string]string{
		"system_proxy_url": `"http://proxy.local:8080"`,
	})
	if cfg.SystemProxyUrl != "http://proxy.local:8080" {
		t.Fatalf("SystemProxyUrl = %q, want %q", cfg.SystemProxyUrl, "http://proxy.local:8080")
	}
}
