package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func loadWebdavBackupConfig(db *store.DB) (webdavBackupConfig, error) {
	cfg := defaultWebdavBackupConfig()

	var raw string
	err := db.Get(&raw, "SELECT value FROM settings WHERE key = ?", backupWebdavConfigSettingKey)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if strings.TrimSpace(raw) == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return defaultWebdavBackupConfig(), nil
	}
	normalizeWebdavBackupConfig(&cfg)
	return cfg, nil
}

func saveWebdavBackupConfig(db *store.DB, cfg webdavBackupConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return saveBackupSettingString(db, backupWebdavConfigSettingKey, string(data))
}

func saveBackupSettingString(db *store.DB, key, value string) error {
	const query = `INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`
	_, err := db.Exec(query, key, value)
	return err
}

func loadWebdavBackupState(db *store.DB) map[string]any {
	state := defaultWebdavBackupState()

	var raw string
	err := db.Get(&raw, "SELECT value FROM settings WHERE key = ?", backupWebdavStateSettingKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return state
	}
	var stored map[string]any
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return state
	}
	if v, ok := stored["lastSyncAt"]; ok {
		state["lastSyncAt"] = v
	}
	if v, ok := stored["lastAttemptAt"]; ok {
		state["lastAttemptAt"] = v
	}
	if v, ok := stored["lastError"]; ok {
		state["lastError"] = v
	}
	return state
}

func defaultWebdavBackupState() map[string]any {
	return map[string]any{
		"lastSyncAt":    nil,
		"lastAttemptAt": nil,
		"lastError":     nil,
	}
}

func updateWebdavBackupState(db *store.DB, syncErr error) map[string]any {
	previous := loadWebdavBackupState(db)
	now := time.Now().UTC().Format(time.RFC3339)
	state := map[string]any{
		"lastSyncAt":    previous["lastSyncAt"],
		"lastAttemptAt": now,
		"lastError":     nil,
	}
	if syncErr != nil {
		state["lastError"] = syncErr.Error()
	} else {
		state["lastSyncAt"] = now
	}
	data, err := json.Marshal(state)
	if err != nil {
		slog.Warn("settings: failed to marshal webdav backup state", "error", err)
	} else if saveErr := saveBackupSettingString(db, backupWebdavStateSettingKey, string(data)); saveErr != nil {
		slog.Warn("settings: failed to persist webdav backup state", "error", saveErr)
	}
	return state
}
