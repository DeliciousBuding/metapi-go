package admin

import (
	"encoding/json"
	"net/http"

	"github.com/deliciousbuding/metapi-go/config"
)

// applyNotifySettings applies the notification channel fields.
func (h *settingsHandler) applyNotifySettings(body map[string]any) *settingsApplyError {
	// Notify: Webhook
	if err := applyBoolSettingDB(h.db, body, "webhookEnabled", "webhook_enabled", func(r *config.RuntimeSettings, v bool) { r.WebhookEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "webhookUrl", "webhook_url", func(r *config.RuntimeSettings, v string) { r.WebhookUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: Bark
	if err := applyBoolSettingDB(h.db, body, "barkEnabled", "bark_enabled", func(r *config.RuntimeSettings, v bool) { r.BarkEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "barkUrl", "bark_url", func(r *config.RuntimeSettings, v string) { r.BarkUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: ServerChan
	if err := applyBoolSettingDB(h.db, body, "serverChanEnabled", "serverchan_enabled", func(r *config.RuntimeSettings, v bool) { r.ServerChanEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "serverChanKey", "serverchan_key", func(r *config.RuntimeSettings, v string) { r.ServerChanKey = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: Telegram
	if err := applyBoolSettingDB(h.db, body, "telegramEnabled", "telegram_enabled", func(r *config.RuntimeSettings, v bool) { r.TelegramEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "telegramApiBaseUrl", "telegram_api_base_url", func(r *config.RuntimeSettings, v string) { r.TelegramApiBaseUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "telegramBotToken", "telegram_bot_token", func(r *config.RuntimeSettings, v string) { r.TelegramBotToken = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "telegramChatId", "telegram_chat_id", func(r *config.RuntimeSettings, v string) { r.TelegramChatId = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "telegramUseSystemProxy", "telegram_use_system_proxy", func(r *config.RuntimeSettings, v bool) { r.TelegramUseSystemProxy = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "telegramMessageThreadId", "telegram_message_thread_id", func(r *config.RuntimeSettings, v string) { r.TelegramMessageThreadId = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: SMTP
	if err := applyBoolSettingDB(h.db, body, "smtpEnabled", "smtp_enabled", func(r *config.RuntimeSettings, v bool) { r.SmtpEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpHost", "smtp_host", func(r *config.RuntimeSettings, v string) { r.SmtpHost = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if v, ok := body["smtpPort"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "smtpPort must be a number")
		}
		port := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.SmtpPort = port })
		upsertSettingDB(h.db, "smtp_port", port)
	}
	if err := applyBoolSettingDB(h.db, body, "smtpSecure", "smtp_secure", func(r *config.RuntimeSettings, v bool) { r.SmtpSecure = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpUser", "smtp_user", func(r *config.RuntimeSettings, v string) { r.SmtpUser = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpPass", "smtp_pass", func(r *config.RuntimeSettings, v string) { r.SmtpPass = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpFrom", "smtp_from", func(r *config.RuntimeSettings, v string) { r.SmtpFrom = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "smtpTo", "smtp_to", func(r *config.RuntimeSettings, v string) { r.SmtpTo = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Notify: Feishu / DingTalk / WeCom / Ntfy
	if err := applyBoolSettingDB(h.db, body, "feishuEnabled", "feishu_enabled", func(r *config.RuntimeSettings, v bool) { r.FeishuEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "feishuWebhook", "feishu_webhook", func(r *config.RuntimeSettings, v string) { r.FeishuWebhook = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "feishuSecret", "feishu_secret", func(r *config.RuntimeSettings, v string) { r.FeishuSecret = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "dingtalkEnabled", "dingtalk_enabled", func(r *config.RuntimeSettings, v bool) { r.DingtalkEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "dingtalkWebhook", "dingtalk_webhook", func(r *config.RuntimeSettings, v string) { r.DingtalkWebhook = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "dingtalkSecret", "dingtalk_secret", func(r *config.RuntimeSettings, v string) { r.DingtalkSecret = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "wecomEnabled", "wecom_enabled", func(r *config.RuntimeSettings, v bool) { r.WecomEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "wecomWebhook", "wecom_webhook", func(r *config.RuntimeSettings, v string) { r.WecomWebhook = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyBoolSettingDB(h.db, body, "ntfyEnabled", "ntfy_enabled", func(r *config.RuntimeSettings, v bool) { r.NtfyEnabled = v }); err != nil {
		return failSettings(http.StatusBadRequest, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "ntfyUrl", "ntfy_url", func(r *config.RuntimeSettings, v string) { r.NtfyUrl = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "ntfyTopic", "ntfy_topic", func(r *config.RuntimeSettings, v string) { r.NtfyTopic = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}
	if err := applyStringSettingDB(h.db, body, "ntfyToken", "ntfy_token", func(r *config.RuntimeSettings, v string) { r.NtfyToken = v }); err != nil {
		return failSettings(http.StatusInternalServerError, err.Error())
	}

	// Per-alert-type mute toggles (JSON object). The map is replaced
	// wholesale on the draft — published snapshots share map references, so
	// in-place mutation is forbidden by the RuntimeSettings contract.
	if v, ok := body["notifyTaskToggles"]; ok {
		if raw, mErr := json.Marshal(v); mErr == nil {
			upsertSettingDB(h.db, "notify_task_toggles", v)
			// Re-hydrate from the marshaled value so runtime matches persisted.
			fresh := map[string]bool{}
			if uErr := json.Unmarshal(raw, &fresh); uErr == nil {
				config.UpdateRuntime(func(r *config.RuntimeSettings) { r.NotifyTaskToggles = fresh })
			}
		}
	}

	// Notify cooldown
	if v, ok := body["notifyCooldownSec"]; ok {
		n, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "notifyCooldownSec must be a number")
		}
		if n < 0 {
			return failSettings(http.StatusBadRequest, "alert cooldown must be a number >= 0 (seconds)")
		}
		cooldownSec := int(n)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.NotifyCooldownSec = cooldownSec })
		upsertSettingDB(h.db, "notify_cooldown_sec", cooldownSec)
	}

	return nil
}
