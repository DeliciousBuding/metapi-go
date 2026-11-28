package admin

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// sensitiveRuntimeFieldPresent reports whether a runtime-settings patch
// touches an egress destination or the admin network ACL. Changing any of
// these can redirect credentials or open the management API, so the operator
// must re-present the master token. The UI submits only changed fields, so an
// ordinary settings save is unaffected.
func sensitiveRuntimeFieldPresent(body map[string]any) bool {
	for _, key := range []string{
		"systemProxyUrl",
		"telegramApiBaseUrl",
		"telegramUseSystemProxy",
		"telegramBotToken",
		"telegramChatId",
		"adminIpAllowlist",
		"smtpHost",
		"smtpPort",
		"smtpUser",
		"smtpPass",
		"smtpFrom",
		"smtpTo",
		"webhookUrl",
		"feishuWebhook",
		"dingtalkWebhook",
		"wecomWebhook",
		"ntfyUrl",
		"barkUrl",
		"serverChanKey",
	} {
		if _, ok := body[key]; ok {
			return true
		}
	}
	return false
}

// PUT /api/settings/runtime
func (h *settingsHandler) updateRuntime(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Egress endpoints and the admin network ACL are credential-bearing
	// controls: a session alone must not be able to redirect upstream traffic,
	// exfiltrate the Telegram bot token, or widen the management allowlist.
	if sensitiveRuntimeFieldPresent(body) && !auth.ReauthConfirmed(r) {
		auth.WriteReauthRequired(w)
		return
	}

	for _, apply := range []func(map[string]any) *settingsApplyError{
		h.applyProxyAccessSettings,
		h.applyCheckinSettings,
		h.applyBalanceScheduleSettings,
		h.applyModelSyncScheduleSettings,
		h.applyLogCleanupSettings,
		h.applyFeatureToggleSettings,
		h.applyProxySessionSettings,
		h.applyRoutingSettings,
		h.applyNotifySettings,
		h.applyFilterSettings,
		h.applySiteBrandingSettings,
	} {
		if err := apply(body); err != nil {
			// Single funnel: 400-class apply failures
			// are user-side validation errors, so they carry the machine-
			// readable invalidSettingsValue code; 5xx internals keep the raw
			// message with no code (backend text stays the display fallback).
			code := ""
			if err.status == http.StatusBadRequest {
				code = ErrorCodeInvalidSettingsValue
			}
			writeErrorCode(w, err.status, code, err.msg)
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	logSettingsEvent(h.db, "status", "Runtime settings updated", "Runtime settings updated", "info", now)

	// One snapshot for the whole response body.
	rt := config.Runtime()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":             true,
		"message":             "Runtime settings updated",
		"globalAllowedModels": stringSliceOrEmpty(rt.GlobalAllowedModels),
		"globalBlockedBrands": stringSliceOrEmpty(rt.GlobalBlockedBrands),
	})
}

func applyBoolSettingDB(db *store.DB, body map[string]any, key string, dbKey string, set func(*config.RuntimeSettings, bool)) error {
	if v, ok := body[key]; ok {
		val, err := toBoolStrict(v)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { set(r, val) })
		if err := upsertSettingDB(db, dbKey, val); err != nil {
			return err
		}
	}
	return nil
}

type settingExecer interface {
	Rebind(string) string
	Exec(string, ...any) (sql.Result, error)
}

func upsertSettingDB(db settingExecer, key string, value any) error {
	// Normalize nil string slices to empty arrays so we never persist JSON null
	// for list settings (null historically rehydrated as a wiped allowlist).
	if value == nil {
		value = []string{}
	}
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("settings: marshal %q: %w", key, err)
	}
	query := db.Rebind(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if _, err := db.Exec(query, key, string(jsonValue)); err != nil {
		return fmt.Errorf("settings: upsert %q: %w", key, err)
	}
	return nil
}

func logSettingsEvent(db *store.DB, eventType, title, message, level, createdAt string) {
	// read is a BOOLEAN column on PostgreSQL: bind FALSE, not the integer
	// literal 0 (PG rejects integer literals for booleans).
	const query = `INSERT INTO events (type, title, message, level, related_type, created_at, "read")
		VALUES (?, ?, ?, ?, 'settings', ?, FALSE)`
	if _, err := db.Exec(query, eventType, title, message, level, createdAt); err != nil {
		slog.Warn("settings: failed to log settings event", "type", eventType, "error", err)
	}
}
