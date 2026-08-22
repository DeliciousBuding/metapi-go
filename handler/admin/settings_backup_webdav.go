package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/internal/ssrf"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/jmoiron/sqlx"
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

// POST /api/settings/backup/webdav/export
func (h *backupHandler) exportToWebdav(w http.ResponseWriter, r *http.Request) {
	cfg, err := loadWebdavBackupConfig(h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read WebDAV config")
		return
	}
	if err := validateWebdavBackupConfig(cfg, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Type string `json:"type"`
	}
	if err := decodeOptionalJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	exportType := cfg.ExportType
	if strings.TrimSpace(body.Type) != "" {
		exportType = body.Type
	}

	backup, err := buildBackupPayload(h.db, exportType)
	if err != nil {
		status := backupExportErrorStatus(err)
		writeError(w, status, err.Error())
		return
	}
	data, err := json.Marshal(backup)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to serialize backup data")
		return
	}

	client := newWebdavHTTPClient()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPut, cfg.FileURL, bytes.NewReader(data))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid WebDAV file URL")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.Username != "" || cfg.Password != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		state := updateWebdavBackupState(h.db, err)
		writeWebdavFailureResponse(w, http.StatusBadGateway, cfg, state, "WebDAV export request failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		errMsg := sanitizeWebdavError(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))), cfg)
		state := updateWebdavBackupState(h.db, errors.New(errMsg))
		writeWebdavFailureResponse(w, http.StatusBadGateway, cfg, state, errMsg)
		return
	}

	state := updateWebdavBackupState(h.db, nil)
	respPayload := webdavConfigResponsePayload(true, cfg, state)
	respPayload["message"] = "WebDAV export succeeded"
	respPayload["fileUrl"] = cfg.FileURL
	writeJSON(w, http.StatusOK, respPayload)
}

func loadWebdavBackupConfig(db *sqlx.DB) (webdavBackupConfig, error) {
	cfg := defaultWebdavBackupConfig()

	var raw string
	err := db.Get(&raw, db.Rebind("SELECT value FROM settings WHERE key = ?"), backupWebdavConfigSettingKey)
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

func saveWebdavBackupConfig(db *sqlx.DB, cfg webdavBackupConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return saveBackupSettingString(db, backupWebdavConfigSettingKey, string(data))
}

func saveBackupSettingString(db *sqlx.DB, key, value string) error {
	query := db.Rebind(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	_, err := db.Exec(query, key, value)
	return err
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

func isValidWebdavFileURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	if parsed.Host == "" || parsed.User != nil {
		return false
	}
	if port := parsed.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	// Explicit SSRF guard: reject literal IPs in private/loopback/link-local/
	// unspecified ranges at the URL-validation layer. This complements the
	// dial-time DNS resolution check in ssrf.RejectUnsafeWebdavDialHost and the
	// hostname-level guard in ssrf.IsAllowedWebdavTargetHost. The
	// allowPrivateWebdavTargets flag (test-only) bypasses this guard so
	// httptest servers on 127.0.0.1 work.
	host := parsed.Hostname()
	if !allowPrivateWebdavTargets && ssrf.IsPrivateOrLoopbackLiteral(host) {
		return false
	}
	return ssrf.IsAllowedWebdavTargetHost(host, allowPrivateWebdavTargets)
}

func decodeOptionalJSONRequest(r *http.Request, dst any) error {
	raw, err := readAdminJSONBody(r.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
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

func loadWebdavBackupState(db *sqlx.DB) map[string]any {
	state := defaultWebdavBackupState()

	var raw string
	err := db.Get(&raw, db.Rebind("SELECT value FROM settings WHERE key = ?"), backupWebdavStateSettingKey)
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

func updateWebdavBackupState(db *sqlx.DB, syncErr error) map[string]any {
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

// POST /api/settings/backup/webdav/import
func (h *backupHandler) importFromWebdav(w http.ResponseWriter, r *http.Request) {
	cfg, err := loadWebdavBackupConfig(h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read WebDAV config")
		return
	}
	if err := validateWebdavBackupConfig(cfg, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	client := newWebdavHTTPClient()
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, cfg.FileURL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid WebDAV file URL")
		return
	}
	if cfg.Username != "" || cfg.Password != "" {
		req.SetBasicAuth(cfg.Username, cfg.Password)
	}

	resp, err := client.Do(req)
	if err != nil {
		state := updateWebdavBackupState(h.db, errors.New(sanitizeWebdavError(err.Error(), cfg)))
		writeWebdavFailureResponse(w, http.StatusBadGateway, cfg, state, "WebDAV import request failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		errMsg := sanitizeWebdavError(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))), cfg)
		state := updateWebdavBackupState(h.db, errors.New(errMsg))
		writeWebdavFailureResponse(w, http.StatusBadGateway, cfg, state, errMsg)
		return
	}

	body, err := readLimitedWebdavBody(resp.Body, backupWebdavImportMaxBytes)
	if err != nil {
		errMsg := sanitizeWebdavError(err.Error(), cfg)
		state := updateWebdavBackupState(h.db, errors.New(errMsg))
		status := http.StatusBadRequest
		var tooLarge webdavImportTooLargeError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeWebdavFailureResponse(w, status, cfg, state, errMsg)
		return
	}

	var backup struct {
		Tables map[string]json.RawMessage `json:"tables"`
	}
	if err := decodeBackupPayload(body, &backup); err != nil || backup.Tables == nil {
		errMsg := "invalid WebDAV backup data: expected a JSON object with a tables field"
		if err != nil {
			errMsg = fmt.Sprintf("%s：%v", errMsg, err)
		}
		state := updateWebdavBackupState(h.db, errors.New(errMsg))
		writeWebdavFailureResponse(w, http.StatusBadRequest, cfg, state, errMsg)
		return
	}

	imported, err := importBackupTables(h.db, backup.Tables)
	if err != nil {
		status := backupImportErrorStatus(err)
		state := updateWebdavBackupState(h.db, errors.New(sanitizeWebdavError(err.Error(), cfg)))
		writeWebdavFailureResponse(w, status, cfg, state, err.Error())
		return
	}

	state := updateWebdavBackupState(h.db, nil)
	payload := webdavConfigResponsePayload(true, cfg, state)
	payload["message"] = "WebDAV import completed"
	payload["imported"] = imported
	payload["appliedSettings"] = []any{}
	writeJSON(w, http.StatusOK, payload)
}

func newWebdavHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   backupWebdavFetchTimeout,
		Transport: newWebdavHTTPTransport(),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after %d redirects", len(via))
			}
			if !isValidWebdavFileURL(req.URL.String()) {
				return fmt.Errorf("refusing WebDAV redirect to unsafe target")
			}
			if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing WebDAV redirect from https to %s", req.URL.Scheme)
			}
			return nil
		},
	}
}

func newWebdavHTTPTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if !allowPrivateWebdavTargets {
				host, _, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				if err := ssrf.RejectUnsafeWebdavDialHost(ctx, host, allowPrivateWebdavTargets); err != nil {
					return nil, err
				}
			}
			return dialer.DialContext(ctx, network, address)
		},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: backupWebdavFetchTimeout,
		IdleConnTimeout:       30 * time.Second,
	}
}

func readLimitedWebdavBody(body io.Reader, maxBytes int64) ([]byte, error) {
	limited := &io.LimitedReader{R: body, N: maxBytes + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read WebDAV backup: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, webdavImportTooLargeError{maxBytes: maxBytes}
	}
	return data, nil
}

type webdavImportTooLargeError struct {
	maxBytes int64
}

func (e webdavImportTooLargeError) Error() string {
	return fmt.Sprintf("backup file exceeds the max size of %d bytes", e.maxBytes)
}

func decodeBackupPayload(raw []byte, dst any) error {
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra struct{}
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("backup payload must contain a single JSON value")
		}
		return err
	}
	return nil
}
