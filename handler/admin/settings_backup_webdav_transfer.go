package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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

	// A WebDAV restore reads a backup file, so it gets the same provenance
	// rule as a pasted payload: tables this build no longer has are skipped
	// and reported rather than failing the whole restore.
	result, err := importBackupTables(h.db, backup.Tables, backupPayloadHasProvenance(body))
	if err != nil {
		status := backupImportErrorStatus(err)
		state := updateWebdavBackupState(h.db, errors.New(sanitizeWebdavError(err.Error(), cfg)))
		writeWebdavFailureResponse(w, status, cfg, state, err.Error())
		return
	}

	state := updateWebdavBackupState(h.db, nil)
	payload := webdavConfigResponsePayload(true, cfg, state)
	payload["message"] = "WebDAV import completed"
	payload["imported"] = result.imported
	payload["appliedSettings"] = []any{}
	if result.skippedSettings > 0 {
		payload["skippedSettings"] = result.skippedSettings
	}
	if len(result.newDownstreamApiKeys) > 0 {
		payload["newDownstreamApiKeys"] = result.newDownstreamApiKeys
	}
	logImportedDownstreamKeys(h.db, result.newDownstreamApiKeys)
	writeJSON(w, http.StatusOK, payload)
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
