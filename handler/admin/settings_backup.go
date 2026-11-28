package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterBackupRoutes registers all /api/settings/backup routes.
func RegisterBackupRoutes(r chi.Router, db *store.DB) {
	handler := &backupHandler{db: db}

	r.Get("/api/settings/backup/export", handler.exportBackup)
	r.Post("/api/settings/backup/import", handler.importBackup)
	// Import plan preview before commit.
	r.Post("/api/settings/backup/import/preview", handler.previewBackupImport)
	r.Get("/api/settings/backup/webdav", handler.getWebdavConfig)
	r.Put("/api/settings/backup/webdav", handler.saveWebdavConfig)
	r.Post("/api/settings/backup/webdav/export", handler.exportToWebdav)
	r.Post("/api/settings/backup/webdav/import", handler.importFromWebdav)
}

type backupHandler struct {
	db *store.DB
}

const (
	backupWebdavConfigSettingKey    = "backup_webdav_config_v1"
	backupWebdavStateSettingKey     = "backup_webdav_state_v1"
	backupWebdavDefaultAutoSyncCron = "0 */6 * * *"
	backupWebdavFetchTimeout        = 15 * time.Second
)

var backupWebdavImportMaxBytes int64 = 20 << 20

var allowPrivateWebdavTargets bool

type webdavBackupConfig struct {
	Enabled         bool   `json:"enabled"`
	FileURL         string `json:"fileUrl"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	ExportType      string `json:"exportType"`
	AutoSyncEnabled bool   `json:"autoSyncEnabled"`
	AutoSyncCron    string `json:"autoSyncCron"`
}

// GET /api/settings/backup/export?type=
func (h *backupHandler) exportBackup(w http.ResponseWriter, r *http.Request) {
	exportType := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("type")))
	if exportType == "" {
		exportType = "all"
	}
	backup, err := buildBackupPayload(h.db, exportType)
	if err != nil {
		status := backupExportErrorStatus(err)
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, backup)
}

var errInvalidBackupExportType = backupsvc.ErrInvalidExportType

func backupExportErrorStatus(err error) int {
	if errors.Is(err, errInvalidBackupExportType) {
		return http.StatusBadRequest
	}
	var limitErr backupsvc.ExportLimitError
	if errors.As(err, &limitErr) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusInternalServerError
}

func buildBackupPayload(db *store.DB, exportType string) (map[string]any, error) {
	return backupsvc.BuildPayload(db.DB, exportType)
}
