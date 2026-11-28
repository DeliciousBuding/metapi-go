package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/scheduler"
)

func (h *backupHandler) getWebdavConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := loadWebdavBackupConfig(h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read WebDAV config")
		return
	}

	writeWebdavConfigResponse(w, http.StatusOK, true, cfg, loadWebdavBackupState(h.db))
}

// PUT /api/settings/backup/webdav
func (h *backupHandler) saveWebdavConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled         *bool   `json:"enabled"`
		FileUrl         *string `json:"fileUrl"`
		Username        *string `json:"username"`
		Password        *string `json:"password"`
		ClearPassword   *bool   `json:"clearPassword"`
		ExportType      *string `json:"exportType"`
		AutoSyncEnabled *bool   `json:"autoSyncEnabled"`
		AutoSyncCron    *string `json:"autoSyncCron"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cfg, err := loadWebdavBackupConfig(h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read WebDAV config")
		return
	}

	if body.Enabled != nil {
		cfg.Enabled = *body.Enabled
	}
	if body.FileUrl != nil {
		cfg.FileURL = strings.TrimSpace(*body.FileUrl)
	}
	if body.Username != nil {
		cfg.Username = *body.Username
	}
	if body.ClearPassword != nil && *body.ClearPassword {
		cfg.Password = ""
	} else if body.Password != nil {
		cfg.Password = *body.Password
	}
	if body.ExportType != nil {
		cfg.ExportType = strings.TrimSpace(strings.ToLower(*body.ExportType))
	}
	if body.AutoSyncEnabled != nil {
		cfg.AutoSyncEnabled = *body.AutoSyncEnabled
	}
	if body.AutoSyncCron != nil {
		cfg.AutoSyncCron = strings.TrimSpace(*body.AutoSyncCron)
	}
	normalizeWebdavBackupConfig(&cfg)

	if cfg.AutoSyncEnabled && !scheduler.ValidateCronExpr(cfg.AutoSyncCron) {
		writeError(w, http.StatusBadRequest, "invalid auto-sync cron expression")
		return
	}
	if err := validateWebdavBackupConfig(cfg, false); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := saveWebdavBackupConfig(h.db, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save WebDAV config")
		return
	}
	if err := app.ReloadWebdavBackup(); err != nil {
		slog.Warn("settings: webdav backup reload after save failed", "error", err)
	}

	writeWebdavConfigResponse(w, http.StatusOK, true, cfg, loadWebdavBackupState(h.db))
}

func defaultWebdavBackupConfig() webdavBackupConfig {
	return webdavBackupConfig{
		ExportType:   "all",
		AutoSyncCron: backupWebdavDefaultAutoSyncCron,
	}
}

func normalizeWebdavBackupConfig(cfg *webdavBackupConfig) {
	cfg.FileURL = strings.TrimSpace(cfg.FileURL)
	cfg.ExportType = strings.TrimSpace(strings.ToLower(cfg.ExportType))
	if cfg.ExportType == "" {
		cfg.ExportType = "all"
	}
	cfg.AutoSyncCron = strings.TrimSpace(cfg.AutoSyncCron)
	if cfg.AutoSyncCron == "" {
		cfg.AutoSyncCron = backupWebdavDefaultAutoSyncCron
	}
	if !cfg.Enabled {
		cfg.AutoSyncEnabled = false
	}
}

func validateWebdavBackupConfig(cfg webdavBackupConfig, requireEnabled bool) error {
	if cfg.ExportType != "all" && cfg.ExportType != "accounts" && cfg.ExportType != "preferences" {
		return errInvalidBackupExportType
	}
	if requireEnabled && !cfg.Enabled {
		return fmt.Errorf("WebDAV not enabled")
	}
	if (cfg.Enabled || requireEnabled) && !isValidWebdavFileURL(cfg.FileURL) {
		return fmt.Errorf("invalid WebDAV file URL")
	}
	if cfg.AutoSyncEnabled && !cfg.Enabled {
		return fmt.Errorf("auto sync requires WebDAV to be enabled first")
	}
	return nil
}

func writeWebdavConfigResponse(w http.ResponseWriter, status int, success bool, cfg webdavBackupConfig, state map[string]any) {
	writeJSON(w, status, webdavConfigResponsePayload(success, cfg, state))
}

func webdavConfigResponsePayload(success bool, cfg webdavBackupConfig, state map[string]any) map[string]any {
	config := maskedWebdavBackupConfig(cfg)
	payload := map[string]any{
		"success": success,
		"config":  config,
		"state":   state,
	}
	for k, v := range config {
		payload[k] = v
	}
	return payload
}

func maskedWebdavBackupConfig(cfg webdavBackupConfig) map[string]any {
	return map[string]any{
		"enabled":         cfg.Enabled,
		"fileUrl":         cfg.FileURL,
		"username":        cfg.Username,
		"password":        "",
		"hasPassword":     cfg.Password != "",
		"passwordMasked":  maskValue(cfg.Password),
		"exportType":      cfg.ExportType,
		"autoSyncEnabled": cfg.AutoSyncEnabled,
		"autoSyncCron":    cfg.AutoSyncCron,
	}
}

func writeWebdavFailureResponse(w http.ResponseWriter, status int, cfg webdavBackupConfig, state map[string]any, message string) {
	payload := webdavConfigResponsePayload(false, cfg, state)
	payload["message"] = sanitizeWebdavError(message, cfg)
	writeJSON(w, status, payload)
}

func sanitizeWebdavError(message string, cfg webdavBackupConfig) string {
	result := message
	if cfg.Password != "" {
		result = strings.ReplaceAll(result, cfg.Password, "****")
	}
	if cfg.Username != "" {
		result = strings.ReplaceAll(result, cfg.Username, "****")
	}
	return result
}
